package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// PathSlug converts a directory path to a slug for S3 keys.
// e.g. "/opt/homeassistant/config" -> "opt-homeassistant-config"
func PathSlug(dir string) string {
	cleaned := filepath.Clean(dir)
	cleaned = strings.TrimPrefix(cleaned, "/")
	return strings.ReplaceAll(cleaned, "/", "-")
}

// S3Key builds the full S3 object key.
func S3Key(hostname, dir string, t time.Time) string {
	slug := PathSlug(dir)
	ts := t.UTC().Format("2006-01-02T15-04-05Z")
	return fmt.Sprintf("%s/%s/%s.tar.gz", hostname, slug, ts)
}

// CreateArchive creates a tar.gz archive of dir and writes it to w.
// Paths inside the archive are relative to dir's parent.
func CreateArchive(w io.Writer, dir string) error {
	gw := gzip.NewWriter(w)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Build relative path starting from the base directory name
		rel, err := filepath.Rel(filepath.Dir(dir), path)
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = rel

		// Handle symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			header.Linkname = link
		}

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// Only write content for regular files
		if !info.Mode().IsRegular() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(tw, f)
		return err
	})
}

// UploadToS3 uploads data from r to the given S3 bucket and key.
func UploadToS3(ctx context.Context, region, bucket, key string, r io.Reader) error {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}

	client := s3.NewFromConfig(cfg)
	tm := transfermanager.New(client)

	_, err = tm.UploadObject(ctx, &transfermanager.UploadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   r,
	})
	if err != nil {
		return fmt.Errorf("uploading to s3://%s/%s: %w", bucket, key, err)
	}

	return nil
}
