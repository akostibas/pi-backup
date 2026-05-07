package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func requireSqlite3(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not on PATH")
	}
}

// makeSqliteDB writes a small SQLite database at path with one inserted row.
func makeSqliteDB(t *testing.T, path string) {
	t.Helper()
	cmd := exec.Command("sqlite3", path,
		"PRAGMA journal_mode=WAL;",
		"CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT);",
		"INSERT INTO t (v) VALUES ('hello');",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("seeding sqlite db: %v\n%s", err, out)
	}
}

func TestSnapshotSqlite(t *testing.T) {
	requireSqlite3(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "data.db")
	dest := filepath.Join(dir, "snap.db")
	makeSqliteDB(t, src)

	if err := snapshotSqlite(src, dest); err != nil {
		t.Fatalf("snapshotSqlite: %v", err)
	}

	// Snapshot should be readable as a SQLite DB and contain the row.
	out, err := exec.Command("sqlite3", dest, "SELECT v FROM t;").CombinedOutput()
	if err != nil {
		t.Fatalf("reading snapshot: %v\n%s", err, out)
	}
	if got := string(bytes.TrimSpace(out)); got != "hello" {
		t.Errorf("snapshot content = %q, want %q", got, "hello")
	}
}

func TestPrepareSnapshots(t *testing.T) {
	requireSqlite3(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	makeSqliteDB(t, dbPath)
	// Create dummy WAL/SHM siblings to confirm they get marked excluded.
	os.WriteFile(dbPath+"-wal", []byte("wal"), 0644)
	os.WriteFile(dbPath+"-shm", []byte("shm"), 0644)
	excludeMe := filepath.Join(dir, "skip.bin")
	os.WriteFile(excludeMe, []byte("nope"), 0644)

	d := Directory{
		Path:        dir,
		SqliteFiles: []string{"data.db"},
		Excludes:    []string{"skip.bin"},
	}
	res, err := PrepareSnapshots(d)
	if err != nil {
		t.Fatalf("PrepareSnapshots: %v", err)
	}
	defer res.Cleanup()

	if got, ok := res.Overrides[dbPath]; !ok {
		t.Errorf("missing override for %s", dbPath)
	} else if _, err := os.Stat(got); err != nil {
		t.Errorf("snapshot file missing: %v", err)
	}
	for _, suf := range []string{"-wal", "-shm", "-journal"} {
		if !res.Excludes[dbPath+suf] {
			t.Errorf("expected exclude for %s%s", dbPath, suf)
		}
	}
	if !res.Excludes[excludeMe] {
		t.Errorf("expected user exclude for %s", excludeMe)
	}
}

func TestCreateArchiveWithOverrideAndExclude(t *testing.T) {
	requireSqlite3(t)
	parent := t.TempDir()
	dir := filepath.Join(parent, "data")
	os.MkdirAll(dir, 0755)
	dbPath := filepath.Join(dir, "real.db")
	makeSqliteDB(t, dbPath)
	os.WriteFile(dbPath+"-wal", []byte("wal-bytes"), 0644)
	os.WriteFile(dbPath+"-shm", []byte("shm-bytes"), 0644)
	dropPath := filepath.Join(dir, "drop.txt")
	os.WriteFile(dropPath, []byte("should not appear"), 0644)
	keepPath := filepath.Join(dir, "keep.txt")
	os.WriteFile(keepPath, []byte("kept"), 0644)

	d := Directory{
		Path:        dir,
		SqliteFiles: []string{"real.db"},
		Excludes:    []string{"drop.txt"},
	}
	snap, err := PrepareSnapshots(d)
	if err != nil {
		t.Fatalf("PrepareSnapshots: %v", err)
	}
	defer snap.Cleanup()

	var buf bytes.Buffer
	if err := CreateArchive(&buf, dir, snap.Overrides, snap.Excludes); err != nil {
		t.Fatalf("CreateArchive: %v", err)
	}

	gr, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	tr := tar.NewReader(gr)
	got := map[string]int64{} // name -> size
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar: %v", err)
		}
		got[hdr.Name] = hdr.Size
		io.Copy(io.Discard, tr)
	}

	mustContain := []string{"data", "data/real.db", "data/keep.txt"}
	mustNotContain := []string{"data/drop.txt", "data/real.db-wal", "data/real.db-shm"}
	for _, n := range mustContain {
		if _, ok := got[n]; !ok {
			t.Errorf("archive missing %s; have %v", n, got)
		}
	}
	for _, n := range mustNotContain {
		if _, ok := got[n]; ok {
			t.Errorf("archive should not contain %s", n)
		}
	}
	// real.db's tar size should equal the snapshot's on-disk size.
	si, err := os.Stat(snap.Overrides[dbPath])
	if err != nil {
		t.Fatalf("stat snapshot: %v", err)
	}
	if got["data/real.db"] != si.Size() {
		t.Errorf("archived real.db size = %d, want %d (snapshot size)", got["data/real.db"], si.Size())
	}
}
