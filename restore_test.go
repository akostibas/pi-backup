package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"syscall"
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
	if err := CreateArchive(&buf, dataDir, nil, nil); err != nil {
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
	if err := CreateArchive(&buf, dataDir, nil, nil); err != nil {
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
	if err := CreateArchive(&buf, dataDir, nil, nil); err != nil {
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

// TestExtractArchiveAppliesUidGid verifies that ExtractArchive honours the
// uid/gid in tar headers via Lchown. Regression test for Bug 2.
//
// We write a tar by hand with header.Uid/Gid set to the current process's
// own uid/gid (chowning to yourself is always permitted, even unprivileged),
// extract it, and confirm the on-disk uid/gid matches.
func TestExtractArchiveAppliesUidGid(t *testing.T) {
	myUid := os.Getuid()
	myGid := os.Getgid()

	// Build an archive in memory with explicit header uid/gid.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	entries := []struct {
		name     string
		typeflag byte
		mode     int64
		body     string
	}{
		{"mydata", tar.TypeDir, 0755, ""},
		{"mydata/file.txt", tar.TypeReg, 0644, "hello"},
	}
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     e.mode,
			Typeflag: e.typeflag,
			Uid:      myUid,
			Gid:      myGid,
		}
		if e.typeflag == tar.TypeReg {
			hdr.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if e.typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	destDir := t.TempDir()
	if err := ExtractArchive(&buf, destDir, ""); err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}

	for _, e := range entries {
		path := filepath.Join(destDir, e.name)
		st, err := os.Lstat(path)
		if err != nil {
			t.Fatalf("Lstat %s: %v", path, err)
		}
		sys, ok := st.Sys().(*syscall.Stat_t)
		if !ok {
			t.Skip("Stat_t unavailable on this platform")
		}
		if int(sys.Uid) != myUid || int(sys.Gid) != myGid {
			t.Errorf("%s uid/gid = %d/%d, want %d/%d",
				path, sys.Uid, sys.Gid, myUid, myGid)
		}
	}
}

// TestExtractArchiveChownFailureNonFatal verifies that an unprivileged
// process trying to chown to a uid it doesn't own does not fail the whole
// restore — it just logs a warning. We synthesise a tar entry with uid 0
// (root) and extract as the current (non-root) test user.
func TestExtractArchiveChownFailureNonFatal(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root user to exercise chown failure path")
	}

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	hdr := &tar.Header{
		Name:     "mydata/file.txt",
		Mode:     0644,
		Typeflag: tar.TypeReg,
		Size:     int64(len("hi")),
		Uid:      0, // root — non-root test user can't chown to this
		Gid:      0,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	destDir := t.TempDir()
	if err := ExtractArchive(&buf, destDir, ""); err != nil {
		t.Fatalf("ExtractArchive should not fail on chown error: %v", err)
	}

	// File should still be present with content intact.
	got, err := os.ReadFile(filepath.Join(destDir, "mydata", "file.txt"))
	if err != nil {
		t.Fatalf("reading extracted file: %v", err)
	}
	if string(got) != "hi" {
		t.Errorf("file content = %q, want %q", got, "hi")
	}
}
