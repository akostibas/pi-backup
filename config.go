package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Hostname    string   `yaml:"hostname"`
	Bucket      string   `yaml:"bucket"`
	Region      string   `yaml:"region"`
	Directories []string `yaml:"directories"`
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

	return &cfg, nil
}
