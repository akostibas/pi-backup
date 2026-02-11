package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yaml := `hostname: cherry
bucket: pi-backup-123456
region: us-east-1
directories:
  - /opt/homeassistant/config
  - /opt/pihole/etc-pihole
`
	os.WriteFile(path, []byte(yaml), 0644)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Hostname != "cherry" {
		t.Errorf("Hostname = %q, want %q", cfg.Hostname, "cherry")
	}
	if cfg.Bucket != "pi-backup-123456" {
		t.Errorf("Bucket = %q, want %q", cfg.Bucket, "pi-backup-123456")
	}
	if cfg.Region != "us-east-1" {
		t.Errorf("Region = %q, want %q", cfg.Region, "us-east-1")
	}
	if len(cfg.Directories) != 2 {
		t.Errorf("len(Directories) = %d, want 2", len(cfg.Directories))
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadConfigMissingFields(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{"missing hostname", "bucket: b\nregion: r\ndirectories:\n  - /d\n"},
		{"missing bucket", "hostname: h\nregion: r\ndirectories:\n  - /d\n"},
		{"missing region", "hostname: h\nbucket: b\ndirectories:\n  - /d\n"},
		{"missing directories", "hostname: h\nbucket: b\nregion: r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			os.WriteFile(path, []byte(tt.yaml), 0644)

			_, err := LoadConfig(path)
			if err == nil {
				t.Fatal("expected error for missing field")
			}
		})
	}
}
