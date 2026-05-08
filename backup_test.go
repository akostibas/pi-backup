package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"testing"
	"time"
)

func TestPathSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/opt/homeassistant/config", "opt-homeassistant-config"},
		{"/opt/pihole/etc-pihole", "opt-pihole-etc-pihole"},
		{"/opt/jellyfin/config", "opt-jellyfin-config"},
		{"/opt/reolink-alerter/ftp-data", "opt-reolink-alerter-ftp-data"},
		{"/single", "single"},
		{"relative/path", "relative-path"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := PathSlug(tt.input)
			if got != tt.want {
				t.Errorf("PathSlug(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestS3Key(t *testing.T) {
	ts := time.Date(2026, 2, 11, 3, 0, 0, 0, time.UTC)
	got := S3Key("cherry", "/opt/homeassistant/config", ts)
	want := "cherry/opt-homeassistant-config/2026-02-11T03-00-00Z.tar.gz"
	if got != want {
		t.Errorf("S3Key() = %q, want %q", got, want)
	}
}

func TestCreateArchive(t *testing.T) {
	// Create a temp directory with some files
	dir := t.TempDir()
	subdir := filepath.Join(dir, "config")
	os.MkdirAll(filepath.Join(subdir, "nested"), 0755)
	os.WriteFile(filepath.Join(subdir, "file1.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(subdir, "nested", "file2.txt"), []byte("world"), 0644)

	var buf bytes.Buffer
	if err := CreateArchive(&buf, subdir, nil, nil); err != nil {
		t.Fatalf("CreateArchive: %v", err)
	}

	// Read back the archive and verify contents
	gr, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	var names []string
	contents := map[string]string{}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar.Next: %v", err)
		}
		names = append(names, hdr.Name)
		if hdr.Typeflag == tar.TypeReg {
			data, _ := io.ReadAll(tr)
			contents[hdr.Name] = string(data)
		}
	}

	sort.Strings(names)
	wantNames := []string{"config", "config/file1.txt", "config/nested", "config/nested/file2.txt"}
	sort.Strings(wantNames)

	if len(names) != len(wantNames) {
		t.Fatalf("got %d entries %v, want %d entries %v", len(names), names, len(wantNames), wantNames)
	}
	for i := range names {
		if names[i] != wantNames[i] {
			t.Errorf("entry %d: got %q, want %q", i, names[i], wantNames[i])
		}
	}

	if contents["config/file1.txt"] != "hello" {
		t.Errorf("file1.txt content = %q, want %q", contents["config/file1.txt"], "hello")
	}
	if contents["config/nested/file2.txt"] != "world" {
		t.Errorf("file2.txt content = %q, want %q", contents["config/nested/file2.txt"], "world")
	}
}

func TestCreateArchiveDeterministic(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "data")
	os.MkdirAll(filepath.Join(subdir, "sub"), 0755)
	os.WriteFile(filepath.Join(subdir, "a.txt"), []byte("aaa"), 0644)
	os.WriteFile(filepath.Join(subdir, "sub", "b.txt"), []byte("bbb"), 0644)

	var buf1, buf2 bytes.Buffer
	if err := CreateArchive(&buf1, subdir, nil, nil); err != nil {
		t.Fatalf("first CreateArchive: %v", err)
	}

	// Small delay so atime/ctime would differ if not zeroed
	time.Sleep(10 * time.Millisecond)

	if err := CreateArchive(&buf2, subdir, nil, nil); err != nil {
		t.Fatalf("second CreateArchive: %v", err)
	}

	if !bytes.Equal(buf1.Bytes(), buf2.Bytes()) {
		t.Error("CreateArchive produced different output for identical files")
	}
}

// TestCreateArchivePreservesUidGid verifies that tar headers carry the
// on-disk uid/gid for plain (non-overridden) files.
func TestCreateArchivePreservesUidGid(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "data")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(subdir, "owned.txt")
	if err := os.WriteFile(filePath, []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	st, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("Stat_t unavailable on this platform")
	}
	wantUid, wantGid := int(sys.Uid), int(sys.Gid)

	var buf bytes.Buffer
	if err := CreateArchive(&buf, subdir, nil, nil); err != nil {
		t.Fatalf("CreateArchive: %v", err)
	}

	gr, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatal(err)
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
			t.Fatal(err)
		}
		if hdr.Name == "data/owned.txt" {
			found = true
			if hdr.Uid != wantUid || hdr.Gid != wantGid {
				t.Errorf("header uid/gid = %d/%d, want %d/%d",
					hdr.Uid, hdr.Gid, wantUid, wantGid)
			}
		}
	}
	if !found {
		t.Fatal("data/owned.txt not found in archive")
	}
}

// TestCreateArchiveOverridePreservesOriginalUidGid verifies that when an
// override (e.g. SQLite snapshot) is supplied, the tar header still carries
// the live file's uid/gid rather than the override tempfile's owner. This
// is a regression test for Bug 1.
func TestCreateArchiveOverridePreservesOriginalUidGid(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "data")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}

	// Live file: this is what the walk visits.
	livePath := filepath.Join(subdir, "live.db")
	if err := os.WriteFile(livePath, []byte("LIVE"), 0644); err != nil {
		t.Fatal(err)
	}

	// Override file in a different temp dir to ensure it's a distinct inode;
	// tweak its mtime so we can confirm the live mtime won (existing
	// behaviour) while we're at it.
	overrideDir := t.TempDir()
	overridePath := filepath.Join(overrideDir, "snapshot.db")
	if err := os.WriteFile(overridePath, []byte("SNAPSHOTTED"), 0644); err != nil {
		t.Fatal(err)
	}
	differentTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(overridePath, differentTime, differentTime); err != nil {
		t.Fatal(err)
	}

	liveSt, err := os.Stat(livePath)
	if err != nil {
		t.Fatal(err)
	}
	liveSys, ok := liveSt.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("Stat_t unavailable on this platform")
	}
	wantUid, wantGid := int(liveSys.Uid), int(liveSys.Gid)

	overrides := map[string]string{livePath: overridePath}

	var buf bytes.Buffer
	if err := CreateArchive(&buf, subdir, overrides, nil); err != nil {
		t.Fatalf("CreateArchive: %v", err)
	}

	gr, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatal(err)
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
			t.Fatal(err)
		}
		if hdr.Name == "data/live.db" {
			found = true
			if hdr.Uid != wantUid || hdr.Gid != wantGid {
				t.Errorf("override entry header uid/gid = %d/%d, want %d/%d (from live file)",
					hdr.Uid, hdr.Gid, wantUid, wantGid)
			}
			// Sanity: content should be the override's bytes, not live.
			data, _ := io.ReadAll(tr)
			if string(data) != "SNAPSHOTTED" {
				t.Errorf("override content = %q, want %q", data, "SNAPSHOTTED")
			}
		}
	}
	if !found {
		t.Fatal("data/live.db not found in archive")
	}
}
