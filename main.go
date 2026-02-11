package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"
)

var version = "dev"

func main() {
	log.SetFlags(0) // systemd/journald adds its own timestamps

	// Check for --version before anything else
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-version") {
		fmt.Println(version)
		os.Exit(0)
	}

	// Parse --config from the beginning of args (before subcommand)
	configPath := "/opt/pi-backup/config.yaml"
	restArgs := os.Args[1:]
	for i := 0; i < len(restArgs); i++ {
		if restArgs[i] == "--config" || restArgs[i] == "-config" {
			if i+1 < len(restArgs) {
				configPath = restArgs[i+1]
				restArgs = append(restArgs[:i], restArgs[i+2:]...)
				break
			}
		}
	}

	// Route to restore subcommand
	if len(restArgs) > 0 && restArgs[0] == "restore" {
		cfg, err := LoadConfig(configPath)
		if err != nil {
			log.Fatalf("error: %v", err)
		}
		if os.Getenv("AWS_ACCESS_KEY_ID") == "" || os.Getenv("AWS_SECRET_ACCESS_KEY") == "" {
			log.Fatal("error: AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY must be set")
		}
		runRestore(cfg, restArgs[1:])
		return
	}

	// Default: backup mode (use flag package for remaining flags)
	fs := flag.NewFlagSet("backup", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "log planned uploads without uploading")
	fs.Parse(restArgs)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	// Verify AWS credentials are present before starting
	if os.Getenv("AWS_ACCESS_KEY_ID") == "" || os.Getenv("AWS_SECRET_ACCESS_KEY") == "" {
		log.Fatal("error: AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY must be set")
	}

	now := time.Now()
	ctx := context.Background()
	var failed []string

	for _, dir := range cfg.Directories {
		key := S3Key(cfg.Hostname, dir, now)

		if *dryRun {
			log.Printf("[dry-run] would upload %s -> s3://%s/%s", dir, cfg.Bucket, key)
			continue
		}

		log.Printf("backing up %s -> s3://%s/%s", dir, cfg.Bucket, key)

		if err := backupDirectory(ctx, cfg, dir, key); err != nil {
			log.Printf("error backing up %s: %v", dir, err)
			failed = append(failed, dir)
			continue
		}

		log.Printf("completed %s", dir)
	}

	if len(failed) > 0 {
		log.Fatalf("failed to back up %d directories: %v", len(failed), failed)
	}
}

func backupDirectory(ctx context.Context, cfg *Config, dir, key string) error {
	// Check that the source directory exists
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("accessing directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}

	// Create archive to a temp file (needed for S3 multipart upload which requires seekable reader)
	tmpFile, err := os.CreateTemp("", "pi-backup-*.tar.gz")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if err := CreateArchive(tmpFile, dir); err != nil {
		return fmt.Errorf("creating archive: %w", err)
	}

	// Seek back to beginning for upload
	if _, err := tmpFile.Seek(0, 0); err != nil {
		return fmt.Errorf("seeking temp file: %w", err)
	}

	return UploadToS3(ctx, cfg.Region, cfg.Bucket, key, tmpFile)
}
