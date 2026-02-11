package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
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

	checksumsPath := filepath.Join(filepath.Dir(configPath), "checksums.json")
	checksums, err := LoadChecksums(checksumsPath)
	if err != nil {
		log.Fatalf("error loading checksums: %v", err)
	}

	for _, dir := range cfg.Directories {
		key := S3Key(cfg.Hostname, dir, now)
		slug := PathSlug(dir)

		archivePath, hash, err := createArchiveWithHash(dir)
		if err != nil {
			log.Printf("error creating archive for %s: %v", dir, err)
			failed = append(failed, dir)
			continue
		}

		if checksums[slug] == hash {
			if *dryRun {
				log.Printf("[dry-run] would skip %s (unchanged)", dir)
			} else {
				log.Printf("skipping %s (unchanged)", dir)
			}
			os.Remove(archivePath)
			continue
		}

		if *dryRun {
			log.Printf("[dry-run] would upload %s -> s3://%s/%s", dir, cfg.Bucket, key)
			os.Remove(archivePath)
			continue
		}

		log.Printf("backing up %s -> s3://%s/%s", dir, cfg.Bucket, key)

		if err := uploadArchive(ctx, cfg, key, archivePath); err != nil {
			log.Printf("error backing up %s: %v", dir, err)
			failed = append(failed, dir)
			os.Remove(archivePath)
			continue
		}
		os.Remove(archivePath)

		checksums[slug] = hash
		if err := SaveChecksums(checksumsPath, checksums); err != nil {
			log.Printf("warning: failed to save checksums: %v", err)
		}

		log.Printf("completed %s", dir)
	}

	if len(failed) > 0 {
		log.Fatalf("failed to back up %d directories: %v", len(failed), failed)
	}
}

// createArchiveWithHash creates a temp archive for dir and returns
// the temp file path and its SHA-256 hex digest.
func createArchiveWithHash(dir string) (path string, hash string, err error) {
	info, err := os.Stat(dir)
	if err != nil {
		return "", "", fmt.Errorf("accessing directory: %w", err)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("%s is not a directory", dir)
	}

	tmpFile, err := os.CreateTemp("", "pi-backup-*.tar.gz")
	if err != nil {
		return "", "", fmt.Errorf("creating temp file: %w", err)
	}
	defer tmpFile.Close()

	if err := CreateArchive(tmpFile, dir); err != nil {
		os.Remove(tmpFile.Name())
		return "", "", fmt.Errorf("creating archive: %w", err)
	}

	if _, err := tmpFile.Seek(0, 0); err != nil {
		os.Remove(tmpFile.Name())
		return "", "", fmt.Errorf("seeking temp file: %w", err)
	}

	h := sha256.New()
	if _, err := io.Copy(h, tmpFile); err != nil {
		os.Remove(tmpFile.Name())
		return "", "", fmt.Errorf("computing hash: %w", err)
	}

	return tmpFile.Name(), fmt.Sprintf("%x", h.Sum(nil)), nil
}

// uploadArchive uploads a temp archive file to S3.
func uploadArchive(ctx context.Context, cfg *Config, key, archivePath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("opening archive: %w", err)
	}
	defer f.Close()

	return UploadToS3(ctx, cfg.Region, cfg.Bucket, key, f)
}
