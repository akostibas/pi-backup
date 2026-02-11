package main

import (
	"path/filepath"
	"testing"
)

func TestLoadChecksumsMissingFile(t *testing.T) {
	m, err := LoadChecksums("/nonexistent/checksums.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestSaveAndLoadChecksums(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "checksums.json")

	original := map[string]string{
		"opt-pi-backup-test-data":   "abc123",
		"opt-homeassistant-config": "def456",
	}

	if err := SaveChecksums(path, original); err != nil {
		t.Fatalf("SaveChecksums: %v", err)
	}

	loaded, err := LoadChecksums(path)
	if err != nil {
		t.Fatalf("LoadChecksums: %v", err)
	}

	if len(loaded) != len(original) {
		t.Fatalf("got %d entries, want %d", len(loaded), len(original))
	}
	for k, v := range original {
		if loaded[k] != v {
			t.Errorf("key %q: got %q, want %q", k, loaded[k], v)
		}
	}
}
