package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"filippo.io/age"
)

// encryptStream returns a reader yielding src encrypted to recipient.
func encryptStream(src io.Reader, recipient string) (io.Reader, error) {
	rcpt, err := age.ParseX25519Recipient(recipient)
	if err != nil {
		return nil, fmt.Errorf("parsing age recipient: %w", err)
	}
	pr, pw := io.Pipe()
	go func() {
		w, err := age.Encrypt(pw, rcpt)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		if _, err := io.Copy(w, src); err != nil {
			pw.CloseWithError(err)
			return
		}
		pw.CloseWithError(w.Close())
	}()
	return pr, nil
}

// decryptStream wraps src with age decryption using the identity file at path.
func decryptStream(src io.Reader, identityPath string) (io.Reader, error) {
	f, err := os.Open(identityPath)
	if err != nil {
		return nil, fmt.Errorf("opening identity: %w", err)
	}
	defer f.Close()
	ids, err := age.ParseIdentities(f)
	if err != nil {
		return nil, fmt.Errorf("parsing identity: %w", err)
	}
	r, err := age.Decrypt(src, ids...)
	if err != nil {
		return nil, fmt.Errorf("decrypting: %w", err)
	}
	return r, nil
}

func isEncryptedKey(key string) bool { return strings.HasSuffix(key, ".age") }
