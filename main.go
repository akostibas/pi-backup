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
	configPath := flag.String("config", "/opt/pi-backup/config.yaml", "path to config file")
	dryRun := flag.Bool("dry-run", false, "log planned uploads without uploading")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	log.SetFlags(0) // systemd/journald adds its own timestamps

	cfg, err := LoadConfig(*configPath)
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
