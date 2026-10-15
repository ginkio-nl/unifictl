package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDotenvPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("UNIFICTL_TEST_VAR=ok\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, mode := range []os.FileMode{0o644, 0o640, 0o604, 0o660} {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		err := loadDotenv(path)
		if err == nil || !strings.Contains(err.Error(), "chmod 600") {
			t.Errorf("mode %04o: err = %v, want refusal", mode, err)
		}
	}

	for _, mode := range []os.FileMode{0o600, 0o400} {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		os.Unsetenv("UNIFICTL_TEST_VAR")
		if err := loadDotenv(path); err != nil {
			t.Errorf("mode %04o: %v", mode, err)
		}
		if got := os.Getenv("UNIFICTL_TEST_VAR"); got != "ok" {
			t.Errorf("mode %04o: var = %q", mode, got)
		}
	}
	os.Unsetenv("UNIFICTL_TEST_VAR")

	if err := loadDotenv(filepath.Join(dir, "missing")); err != nil {
		t.Errorf("missing file: %v", err)
	}
}
