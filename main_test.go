package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDryRun(t *testing.T) {
	// Build the binary
	dir := t.TempDir()
	bin := filepath.Join(dir, "pi-backup")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = filepath.Dir(os.Args[0])
	// Use the module directory
	if wd, err := os.Getwd(); err == nil {
		build.Dir = wd
	}
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	// Write a test config
	configPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(configPath, []byte(`hostname: test
bucket: test-bucket
region: us-east-1
directories:
  - /tmp
`), 0644)

	// Run with --dry-run (should succeed without AWS credentials)
	cmd := exec.Command(bin, "--config", configPath, "--dry-run")
	cmd.Env = append(os.Environ(), "AWS_ACCESS_KEY_ID=fake", "AWS_SECRET_ACCESS_KEY=fake")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dry-run failed: %v\n%s", err, out)
	}

	if !contains(string(out), "[dry-run]") {
		t.Errorf("expected [dry-run] in output, got: %s", out)
	}
}

func TestMissingAWSCredentials(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "pi-backup")
	build := exec.Command("go", "build", "-o", bin, ".")
	if wd, err := os.Getwd(); err == nil {
		build.Dir = wd
	}
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	configPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(configPath, []byte(`hostname: test
bucket: test-bucket
region: us-east-1
directories:
  - /tmp
`), 0644)

	// Run without AWS credentials — should fail
	cmd := exec.Command(bin, "--config", configPath)
	// Explicitly clear AWS env vars
	cmd.Env = []string{"HOME=" + os.Getenv("HOME"), "PATH=" + os.Getenv("PATH")}
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error when AWS credentials are missing")
	}

	if !contains(string(out), "AWS_ACCESS_KEY_ID") {
		t.Errorf("expected AWS credential error, got: %s", out)
	}
}

func TestVersionFlag(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "pi-backup")
	build := exec.Command("go", "build", "-ldflags=-X main.version=v1.2.3", "-o", bin, ".")
	if wd, err := os.Getwd(); err == nil {
		build.Dir = wd
	}
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	cmd := exec.Command(bin, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("--version failed: %v\n%s", err, out)
	}

	if !contains(string(out), "v1.2.3") {
		t.Errorf("expected version v1.2.3, got: %s", out)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
