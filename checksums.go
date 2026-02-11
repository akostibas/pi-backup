package main

import (
	"encoding/json"
	"os"
)

// LoadChecksums reads a slug->hash map from a JSON file.
// Returns an empty map if the file does not exist.
func LoadChecksums(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}

	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// SaveChecksums writes a slug->hash map to a JSON file.
func SaveChecksums(path string, m map[string]string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
