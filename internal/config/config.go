// Package config loads unifictl's optional settings file.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the settings file. API keys don't belong here; they live in .env.
type Config struct {
	// Ignore lists sites to hide everywhere, as case-insensitive glob
	// patterns matched against the site name unifictl shows.
	Ignore []string `yaml:"ignore"`
}

// Load reads the config file at path. A missing file is an empty config.
func Load(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return cfg, fmt.Errorf("%s: %w (see config.example.yaml for the format)", path, err)
	}
	return cfg, nil
}
