package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// SnapshotResult captures the override and exclude paths that result from
// snapshotting one or more SQLite databases inside a directory. All paths are
// absolute. Cleanup removes the on-disk snapshot files.
type SnapshotResult struct {
	Overrides map[string]string // live abs path -> snapshot abs path
	Excludes  map[string]bool   // abs paths to skip during archive
	Cleanup   func()
}

// PrepareSnapshots takes online .backup snapshots of every SQLite file in d
// and returns the overrides and implicit excludes (the -wal/-shm/-journal
// siblings) needed to splice them into an archive of d.Path. The caller must
// invoke result.Cleanup when done.
//
// If d declares no sqlite files, this returns an empty result with a no-op
// cleanup. User-defined excludes from d are also folded into the result.
func PrepareSnapshots(d Directory) (*SnapshotResult, error) {
	res := &SnapshotResult{
		Overrides: map[string]string{},
		Excludes:  map[string]bool{},
		Cleanup:   func() {},
	}

	// User-declared excludes always apply.
	for _, rel := range d.Excludes {
		res.Excludes[filepath.Join(d.Path, rel)] = true
	}

	if len(d.SqliteFiles) == 0 {
		return res, nil
	}

	tmpDir, err := os.MkdirTemp("", "pi-backup-snapshot-*")
	if err != nil {
		return nil, fmt.Errorf("creating snapshot tmpdir: %w", err)
	}
	res.Cleanup = func() { os.RemoveAll(tmpDir) }

	for _, rel := range d.SqliteFiles {
		live := filepath.Join(d.Path, rel)
		// Snapshot filename mirrors the relative path so collisions are
		// impossible across multiple DBs in the same dir.
		snap := filepath.Join(tmpDir, filepath.Base(rel))
		if _, exists := res.Overrides[live]; exists {
			res.Cleanup()
			return nil, fmt.Errorf("duplicate sqlite_file entry: %s", rel)
		}
		if err := snapshotSqlite(live, snap); err != nil {
			res.Cleanup()
			return nil, fmt.Errorf("snapshotting %s: %w", live, err)
		}
		res.Overrides[live] = snap
		// Implicit excludes for the WAL/SHM/journal siblings.
		for _, suffix := range []string{"-wal", "-shm", "-journal"} {
			res.Excludes[live+suffix] = true
		}
	}
	return res, nil
}

// snapshotSqlite invokes `sqlite3 src ".backup dest"`. The sqlite3 CLI uses
// SQLite's online backup API: it copies pages while holding a shared lock
// and retries pages that change mid-copy. The resulting file is a single
// self-contained .db with no WAL/SHM siblings.
func snapshotSqlite(src, dest string) error {
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("source missing: %w", err)
	}
	// .backup with quoted dest path. sqlite3 understands the path verbatim
	// when single-quoted; escape any single quotes in dest to be safe.
	cmd := exec.Command("sqlite3", src, fmt.Sprintf(".backup '%s'", dest))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sqlite3 .backup: %w: %s", err, string(out))
	}
	return nil
}
