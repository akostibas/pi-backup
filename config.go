package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Directory struct {
	Path        string   `yaml:"path"`
	SqliteFiles []string `yaml:"sqlite_files,omitempty"`
	Excludes    []string `yaml:"excludes,omitempty"`
}

type Config struct {
	Hostname    string      `yaml:"hostname"`
	Bucket      string      `yaml:"bucket"`
	Region      string      `yaml:"region"`
	Directories []Directory `yaml:"directories"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	if cfg.Hostname == "" {
		return nil, fmt.Errorf("config: hostname is required")
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("config: bucket is required")
	}
	if cfg.Region == "" {
		return nil, fmt.Errorf("config: region is required")
	}
	if len(cfg.Directories) == 0 {
		return nil, fmt.Errorf("config: at least one directory is required")
	}
	for i, d := range cfg.Directories {
		if d.Path == "" {
			return nil, fmt.Errorf("config: directories[%d].path is required", i)
		}
		for _, rel := range d.SqliteFiles {
			if err := validateRelative(rel); err != nil {
				return nil, fmt.Errorf("config: directories[%d].sqlite_files: %w", i, err)
			}
		}
		for _, rel := range d.Excludes {
			if err := validateRelative(rel); err != nil {
				return nil, fmt.Errorf("config: directories[%d].excludes: %w", i, err)
			}
		}
	}

	return &cfg, nil
}

// validateRelative ensures p is a relative path that doesn't escape via "..".
func validateRelative(p string) error {
	if p == "" {
		return fmt.Errorf("path is empty")
	}
	if filepath.IsAbs(p) {
		return fmt.Errorf("%q must be relative to its directory", p)
	}
	clean := filepath.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%q escapes the directory", p)
	}
	return nil
}
