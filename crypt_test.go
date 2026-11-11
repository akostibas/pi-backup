package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
)

func TestEncryptRoundTrip(t *testing.T) {
	src := t.TempDir()
	data := filepath.Join(src, "data")
	os.MkdirAll(data, 0o755)
	os.WriteFile(filepath.Join(data, "secret.txt"), []byte("hunter2"), 0o600)

	var archive bytes.Buffer
	if err := CreateArchive(&archive, data, nil, nil); err != nil {
		t.Fatal(err)
	}

	id, _ := age.GenerateX25519Identity()
	enc, err := encryptStream(bytes.NewReader(archive.Bytes()), id.Recipient().String())
	if err != nil {
		t.Fatal(err)
	}
	var ciphertext bytes.Buffer
	if _, err := ciphertext.ReadFrom(enc); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext.Bytes(), []byte("hunter2")) || bytes.Equal(ciphertext.Bytes(), archive.Bytes()) {
		t.Fatal("ciphertext leaks plaintext")
	}

	idFile := filepath.Join(t.TempDir(), "key.txt")
	os.WriteFile(idFile, []byte(id.String()+"\n"), 0o600)
	dec, err := decryptStream(&ciphertext, idFile)
	if err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if err := ExtractArchive(dec, dest, ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dest, "data", "secret.txt")); string(got) != "hunter2" {
		t.Fatalf("restored %q", got)
	}

	other, _ := age.GenerateX25519Identity()
	os.WriteFile(idFile, []byte(other.String()+"\n"), 0o600)
	enc2, _ := encryptStream(bytes.NewReader(archive.Bytes()), id.Recipient().String())
	if _, err := decryptStream(enc2, idFile); err == nil {
		t.Fatal("wrong key decrypted")
	}
}

func TestConfigRejectsBadRecipient(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.yaml")
	os.WriteFile(p, []byte("hostname: h\nbucket: b\nregion: r\nage_recipient: nope\ndirectories:\n  - path: /x\n"), 0o600)
	if _, err := LoadConfig(p); err == nil {
		t.Fatal("bad age_recipient accepted")
	}
}
