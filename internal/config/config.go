// Package config loads the unifictl source configuration.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Source types.
const (
	TypeSiteManager = "sitemanager"
	TypeLocal       = "local"
)

// Defaults applied during validation.
const (
	DefaultSiteManagerURL = "https://api.ui.com"
	DefaultLocalAPIPath   = "/proxy/network/integration"

	// Site Manager is rate limited per API key; local consoles are not.
	DefaultSiteManagerConcurrency = 4
	DefaultLocalConcurrency       = 8
)

// Config is the top-level configuration file.
type Config struct {
	Sources []Source `yaml:"sources"`
}

// Source is somewhere consoles come from: either a Site Manager account
// (one key, many consoles reached via the cloud connector) or a single local
// console.
type Source struct {
	Name      string `yaml:"name"`
	Type      string `yaml:"type"`
	APIKey    string `yaml:"api_key"`
	APIKeyEnv string `yaml:"api_key_env"`

	// URL is the console URL for local sources, or the Site Manager base URL
	// (default https://api.ui.com) for sitemanager sources.
	URL string `yaml:"url"`

	// MaxConcurrency caps in-flight requests for this source.
	MaxConcurrency int `yaml:"max_concurrency"`

	// Local only.
	APIPath  string `yaml:"api_path"`
	Insecure bool   `yaml:"insecure"`

	// Site Manager only: glob patterns on console name or ID. Empty means all.
	Consoles []string `yaml:"consoles"`
}

// Key returns the source's API key, preferring the environment variable
// named by api_key_env over an inline api_key.
func (s Source) Key() (string, error) {
	if s.APIKeyEnv != "" {
		if v := os.Getenv(s.APIKeyEnv); v != "" {
			return v, nil
		}
		if s.APIKey == "" {
			return "", fmt.Errorf("environment variable %s is not set (export it or add it to .env)", s.APIKeyEnv)
		}
	}
	if s.APIKey == "" {
		return "", errors.New("no api_key or api_key_env configured")
	}
	return s.APIKey, nil
}

// Load reads and validates the configuration file at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config file %s not found (see config.example.yaml)", path)
		}
		return nil, err
	}
	return Parse(data)
}

// Parse decodes and validates configuration data.
func Parse(data []byte) (*Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if len(c.Sources) == 0 {
		return errors.New("no sources configured")
	}
	seen := make(map[string]bool)
	for i := range c.Sources {
		s := &c.Sources[i]
		if s.Name == "" {
			return fmt.Errorf("source #%d: name is required", i+1)
		}
		if seen[s.Name] {
			return fmt.Errorf("source %q: duplicate name", s.Name)
		}
		seen[s.Name] = true

		switch s.Type {
		case TypeSiteManager:
			if s.URL == "" {
				s.URL = DefaultSiteManagerURL
			}
			if s.APIPath != "" || s.Insecure {
				return fmt.Errorf("source %q: api_path and insecure only apply to local sources", s.Name)
			}
			if s.MaxConcurrency == 0 {
				s.MaxConcurrency = DefaultSiteManagerConcurrency
			}
		case TypeLocal:
			if s.URL == "" {
				return fmt.Errorf("source %q: url is required", s.Name)
			}
			if len(s.Consoles) > 0 {
				return fmt.Errorf("source %q: consoles only applies to sitemanager sources", s.Name)
			}
			if s.APIPath == "" {
				s.APIPath = DefaultLocalAPIPath
			}
			if s.MaxConcurrency == 0 {
				s.MaxConcurrency = DefaultLocalConcurrency
			}
		case "":
			return fmt.Errorf("source %q: type is required (%s or %s)", s.Name, TypeSiteManager, TypeLocal)
		default:
			return fmt.Errorf("source %q: unknown type %q (want %s or %s)", s.Name, s.Type, TypeSiteManager, TypeLocal)
		}
		if s.MaxConcurrency < 1 {
			return fmt.Errorf("source %q: max_concurrency must be at least 1", s.Name)
		}
		s.URL = strings.TrimRight(s.URL, "/")
	}
	return nil
}

// Select returns the sources whose names are in names, or all of them if
// names is empty. Unknown names are an error.
func (c *Config) Select(names []string) ([]Source, error) {
	if len(names) == 0 {
		return c.Sources, nil
	}
	byName := make(map[string]Source, len(c.Sources))
	for _, s := range c.Sources {
		byName[s.Name] = s
	}
	out := make([]Source, 0, len(names))
	for _, n := range names {
		s, ok := byName[n]
		if !ok {
			return nil, fmt.Errorf("unknown source %q", n)
		}
		out = append(out, s)
	}
	return out, nil
}
