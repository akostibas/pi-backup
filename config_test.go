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
  - path: /opt/homeassistant/config
    sqlite_files:
      - home-assistant_v2.db
  - path: /opt/pihole/etc-pihole
    sqlite_files:
      - pihole-FTL.db
      - gravity.db
    excludes:
      - gravity_old.db
  - path: /opt/reolink-alerter/ftp-data
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
	if len(cfg.Directories) != 3 {
		t.Fatalf("len(Directories) = %d, want 3", len(cfg.Directories))
	}
	if cfg.Directories[0].Path != "/opt/homeassistant/config" {
		t.Errorf("Directories[0].Path = %q", cfg.Directories[0].Path)
	}
	if got, want := cfg.Directories[0].SqliteFiles, []string{"home-assistant_v2.db"}; !equalSlice(got, want) {
		t.Errorf("Directories[0].SqliteFiles = %v, want %v", got, want)
	}
	if got, want := cfg.Directories[1].Excludes, []string{"gravity_old.db"}; !equalSlice(got, want) {
		t.Errorf("Directories[1].Excludes = %v, want %v", got, want)
	}
	if len(cfg.Directories[2].SqliteFiles) != 0 {
		t.Errorf("Directories[2].SqliteFiles = %v, want empty", cfg.Directories[2].SqliteFiles)
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
		{"missing hostname", "bucket: b\nregion: r\ndirectories:\n  - path: /d\n"},
		{"missing bucket", "hostname: h\nregion: r\ndirectories:\n  - path: /d\n"},
		{"missing region", "hostname: h\nbucket: b\ndirectories:\n  - path: /d\n"},
		{"missing directories", "hostname: h\nbucket: b\nregion: r\n"},
		{"missing directory path", "hostname: h\nbucket: b\nregion: r\ndirectories:\n  - sqlite_files: [x.db]\n"},
		{"absolute sqlite path", "hostname: h\nbucket: b\nregion: r\ndirectories:\n  - path: /d\n    sqlite_files: [/x.db]\n"},
		{"escaping exclude", "hostname: h\nbucket: b\nregion: r\ndirectories:\n  - path: /d\n    excludes: [../x]\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			os.WriteFile(path, []byte(tt.yaml), 0644)

			_, err := LoadConfig(path)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
