package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// ListBackups lists S3 objects under {hostname}/{slug}/ prefix.
// If dir is empty, lists all backups for the hostname.
func ListBackups(ctx context.Context, cfg *Config, dir string) ([]string, error) {
	prefix := cfg.Hostname + "/"
	if dir != "" {
		slug := PathSlug(dir)
		prefix = fmt.Sprintf("%s/%s/", cfg.Hostname, slug)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Region))
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg)
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(cfg.Bucket),
		Prefix: aws.String(prefix),
	}

	var keys []string
	paginator := s3.NewListObjectsV2Paginator(client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing objects: %w", err)
		}
		for _, obj := range page.Contents {
			keys = append(keys, aws.ToString(obj.Key))
		}
	}

	sort.Strings(keys)
	return keys, nil
}

// FindLatestBackup returns the most recent backup key for a directory.
func FindLatestBackup(ctx context.Context, cfg *Config, dir string) (string, error) {
	keys, err := ListBackups(ctx, cfg, dir)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", fmt.Errorf("no backups found for %s", dir)
	}
	// Timestamps sort lexically, so last key is most recent
	return keys[len(keys)-1], nil
}

// DownloadFromS3 downloads an object from S3 and writes it to w.
func DownloadFromS3(ctx context.Context, region, bucket, key string, w io.Writer) error {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg)
	result, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("downloading s3://%s/%s: %w", bucket, key, err)
	}
	defer result.Body.Close()

	if _, err := io.Copy(w, result.Body); err != nil {
		return fmt.Errorf("writing download: %w", err)
	}

	return nil
}

// ExtractArchive extracts a tar.gz archive from r into destDir.
// If fileFilter is non-empty, only extract entries matching that path.
func ExtractArchive(r io.Reader, destDir, fileFilter string) error {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("opening gzip: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	found := false

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		if fileFilter != "" && hdr.Name != fileFilter {
			continue
		}
		found = true

		target := filepath.Join(destDir, hdr.Name)

		// Prevent path traversal
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) &&
			filepath.Clean(target) != filepath.Clean(destDir) {
			return fmt.Errorf("invalid path in archive: %s", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return fmt.Errorf("creating directory %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("creating parent directory: %w", err)
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return fmt.Errorf("creating file %s: %w", target, err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return fmt.Errorf("writing file %s: %w", target, err)
			}
			f.Close()
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("creating parent directory: %w", err)
			}
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return fmt.Errorf("creating symlink %s: %w", target, err)
			}
		}

		if fileFilter != "" {
			break // Found and extracted the requested file
		}
	}

	if fileFilter != "" && !found {
		return fmt.Errorf("file %q not found in archive", fileFilter)
	}

	return nil
}

// RestoreBackup downloads a backup from S3 and extracts it.
func RestoreBackup(ctx context.Context, cfg *Config, key, destDir, fileFilter string) error {
	tmpFile, err := os.CreateTemp("", "pi-restore-*.tar.gz")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	log.Printf("downloading s3://%s/%s", cfg.Bucket, key)
	if err := DownloadFromS3(ctx, cfg.Region, cfg.Bucket, key, tmpFile); err != nil {
		return err
	}

	if _, err := tmpFile.Seek(0, 0); err != nil {
		return fmt.Errorf("seeking temp file: %w", err)
	}

	log.Printf("extracting to %s", destDir)
	return ExtractArchive(tmpFile, destDir, fileFilter)
}

// runRestore handles the "restore" subcommand.
func runRestore(cfg *Config, args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: pi-backup restore list [<directory>]\n")
		fmt.Fprintf(os.Stderr, "       pi-backup restore <directory> [--snapshot <TS>] [--file <path>] [--dest <dir>]\n")
		os.Exit(1)
	}

	ctx := context.Background()

	// Handle "restore list"
	if args[0] == "list" {
		dir := ""
		if len(args) > 1 {
			dir = args[1]
		}
		keys, err := ListBackups(ctx, cfg, dir)
		if err != nil {
			log.Fatalf("error: %v", err)
		}
		if len(keys) == 0 {
			if dir != "" {
				fmt.Printf("No backups found for %s\n", dir)
			} else {
				fmt.Println("No backups found")
			}
			return
		}
		for _, key := range keys {
			fmt.Println(key)
		}
		return
	}

	// Handle "restore <directory>"
	dir := args[0]

	fs := flag.NewFlagSet("restore", flag.ExitOnError)
	snapshot := fs.String("snapshot", "", "restore a specific snapshot (timestamp like 2026-02-11T03-00-00Z)")
	fileFilter := fs.String("file", "", "extract only this file from the archive")
	dest := fs.String("dest", "", "extract to alternate location (default: parent of directory)")
	fs.Parse(args[1:])

	// Determine the S3 key
	var key string
	if *snapshot != "" {
		slug := PathSlug(dir)
		key = fmt.Sprintf("%s/%s/%s.tar.gz", cfg.Hostname, slug, *snapshot)
	} else {
		var err error
		key, err = FindLatestBackup(ctx, cfg, dir)
		if err != nil {
			log.Fatalf("error: %v", err)
		}
	}

	// Determine destination directory
	destDir := filepath.Dir(dir)
	if *dest != "" {
		destDir = *dest
	}

	if err := RestoreBackup(ctx, cfg, key, destDir, *fileFilter); err != nil {
		log.Fatalf("error: %v", err)
	}

	log.Printf("restore complete")
}
