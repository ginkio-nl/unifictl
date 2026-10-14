// Command unifictl gives an overview of all UniFi sites on a ui.com account.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/joho/godotenv"

	"github.com/ginkio-nl/unifictl/internal/httpx"
	"github.com/ginkio-nl/unifictl/internal/sitemanager"
)

// keyEnv names the environment variable holding the ui.com API key.
const keyEnv = "UNIFI_SITEMANAGER_KEY"

// version is set via -ldflags by the Makefile; see buildVersion.
var version string

// buildVersion returns the ldflags version, falling back to the module
// version Go embeds for `go install ...@vX.Y.Z` builds.
func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

// Globals are flags shared by every command.
type Globals struct {
	Output  string        `short:"o" enum:"table,json" default:"table" help:"Output format (${enum})."`
	Timeout time.Duration `default:"30s" help:"Per-request timeout."`
}

// CLI is the root command.
type CLI struct {
	Globals

	Sites   SitesCmd         `cmd:"" help:"List all sites with device and client counts."`
	Devices DevicesCmd       `cmd:"" help:"List all devices."`
	Version kong.VersionFlag `help:"Print version and exit."`
}

func main() {
	// Real environment variables win over .env files, and ./.env wins over
	// the one in the config directory.
	for _, p := range dotenvPaths() {
		if err := loadDotenv(p); err != nil {
			fmt.Fprintf(os.Stderr, "unifictl: error: %v\n", err)
			os.Exit(1)
		}
	}

	var cli CLI
	kctx := kong.Parse(&cli,
		kong.Name("unifictl"),
		kong.Description("Overview of all UniFi sites and devices on a ui.com account."),
		kong.UsageOnError(),
		kong.Vars{"version": buildVersion()},
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	kctx.BindTo(ctx, (*context.Context)(nil))

	kctx.FatalIfErrorf(kctx.Run(&cli.Globals))
}

// client builds a Site Manager client from the API key in the environment.
func (g *Globals) client() (*sitemanager.Client, error) {
	key := os.Getenv(keyEnv)
	if key == "" {
		return nil, fmt.Errorf("%s is not set: add it to ~/.config/unifictl/.env (create a key at unifi.ui.com → Settings → API Keys)", keyEnv)
	}
	req := httpx.New(httpx.Options{APIKey: key, Timeout: g.Timeout, Limiter: httpx.NewLimiter(4)})
	return sitemanager.New(sitemanager.DefaultURL, req), nil
}

func dotenvPaths() []string {
	paths := []string{".env"}
	if dir, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(dir, ".config", "unifictl", ".env"))
	}
	return paths
}

// loadDotenv sets variables from a .env file if it exists. Variables already
// in the environment take precedence.
func loadDotenv(path string) error {
	err := godotenv.Load(path)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("load %s: %w", path, err)
}
