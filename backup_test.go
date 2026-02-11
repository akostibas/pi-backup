package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"sort"
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
	if err := CreateArchive(&buf, subdir); err != nil {
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
	if err := CreateArchive(&buf1, subdir); err != nil {
		t.Fatalf("first CreateArchive: %v", err)
	}

	// Small delay so atime/ctime would differ if not zeroed
	time.Sleep(10 * time.Millisecond)

	if err := CreateArchive(&buf2, subdir); err != nil {
		t.Fatalf("second CreateArchive: %v", err)
	}

	if !bytes.Equal(buf1.Bytes(), buf2.Bytes()) {
		t.Error("CreateArchive produced different output for identical files")
	}
}
