package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractArchive(t *testing.T) {
	// Create a source directory with files
	srcDir := t.TempDir()
	dataDir := filepath.Join(srcDir, "mydata")
	os.MkdirAll(filepath.Join(dataDir, "subdir"), 0755)
	os.WriteFile(filepath.Join(dataDir, "file1.txt"), []byte("content1"), 0644)
	os.WriteFile(filepath.Join(dataDir, "subdir", "file2.txt"), []byte("content2"), 0644)

	// Create archive
	var buf bytes.Buffer
	if err := CreateArchive(&buf, dataDir); err != nil {
		t.Fatalf("CreateArchive: %v", err)
	}

	// Extract to a new temp dir
	destDir := t.TempDir()
	if err := ExtractArchive(&buf, destDir, ""); err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}

	// Verify files
	got1, err := os.ReadFile(filepath.Join(destDir, "mydata", "file1.txt"))
	if err != nil {
		t.Fatalf("reading file1.txt: %v", err)
	}
	if string(got1) != "content1" {
		t.Errorf("file1.txt = %q, want %q", got1, "content1")
	}

	got2, err := os.ReadFile(filepath.Join(destDir, "mydata", "subdir", "file2.txt"))
	if err != nil {
		t.Fatalf("reading file2.txt: %v", err)
	}
	if string(got2) != "content2" {
		t.Errorf("file2.txt = %q, want %q", got2, "content2")
	}
}

func TestExtractArchiveSingleFile(t *testing.T) {
	// Create a source directory with files
	srcDir := t.TempDir()
	dataDir := filepath.Join(srcDir, "mydata")
	os.MkdirAll(filepath.Join(dataDir, "subdir"), 0755)
	os.WriteFile(filepath.Join(dataDir, "file1.txt"), []byte("content1"), 0644)
	os.WriteFile(filepath.Join(dataDir, "subdir", "file2.txt"), []byte("content2"), 0644)

	// Create archive
	var buf bytes.Buffer
	if err := CreateArchive(&buf, dataDir); err != nil {
		t.Fatalf("CreateArchive: %v", err)
	}

	// Extract only file2.txt
	destDir := t.TempDir()
	if err := ExtractArchive(&buf, destDir, "mydata/subdir/file2.txt"); err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}

	// file2.txt should exist
	got, err := os.ReadFile(filepath.Join(destDir, "mydata", "subdir", "file2.txt"))
	if err != nil {
		t.Fatalf("reading file2.txt: %v", err)
	}
	if string(got) != "content2" {
		t.Errorf("file2.txt = %q, want %q", got, "content2")
	}

	// file1.txt should NOT exist
	if _, err := os.Stat(filepath.Join(destDir, "mydata", "file1.txt")); !os.IsNotExist(err) {
		t.Error("file1.txt should not have been extracted")
	}
}

func TestExtractArchiveNonexistentFile(t *testing.T) {
	// Create a source directory with a file
	srcDir := t.TempDir()
	dataDir := filepath.Join(srcDir, "mydata")
	os.MkdirAll(dataDir, 0755)
	os.WriteFile(filepath.Join(dataDir, "file1.txt"), []byte("content1"), 0644)

	// Create archive
	var buf bytes.Buffer
	if err := CreateArchive(&buf, dataDir); err != nil {
		t.Fatalf("CreateArchive: %v", err)
	}

	// Try to extract a file that doesn't exist in the archive
	destDir := t.TempDir()
	err := ExtractArchive(&buf, destDir, "mydata/nonexistent.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("not found in archive")) {
		t.Errorf("expected 'not found in archive' error, got: %v", err)
	}
}
