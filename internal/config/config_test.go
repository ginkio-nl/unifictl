package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad(t *testing.T) {
	cfg, err := Load(write(t, "# comment\nignore:\n  - Default\n  - 'keesde*'\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cfg.Ignore, []string{"Default", "keesde*"}) {
		t.Errorf("ignore = %v", cfg.Ignore)
	}
}

func TestLoadMissingOrEmpty(t *testing.T) {
	for name, path := range map[string]string{
		"missing": filepath.Join(t.TempDir(), "nope.yaml"),
		"empty":   write(t, ""),
	} {
		cfg, err := Load(path)
		if err != nil || len(cfg.Ignore) != 0 {
			t.Errorf("%s: cfg = %+v, err = %v", name, cfg, err)
		}
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	// The pre-0.2 format must not be silently ignored.
	_, err := Load(write(t, "sources:\n  - name: msp\n    type: sitemanager\n"))
	if err == nil || !strings.Contains(err.Error(), "sources") {
		t.Fatalf("err = %v", err)
	}
}
