// Command unifictl gives an overview of all UniFi sites on a ui.com account.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/joho/godotenv"

	"github.com/ginkio-nl/unifictl/internal/config"
	"github.com/ginkio-nl/unifictl/internal/httpx"
	"github.com/ginkio-nl/unifictl/internal/output"
	"github.com/ginkio-nl/unifictl/internal/report"
	"github.com/ginkio-nl/unifictl/internal/sitemanager"
)

// keyEnv names the environment variable holding the ui.com API key.
const keyEnv = "UNIFI_API_KEY"

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
	Config   string        `short:"c" type:"path" default:"~/.config/unifictl/config.yaml" env:"UNIFICTL_CONFIG" help:"Settings file (optional)."`
	NoIgnore bool          `help:"Show sites on the config file's ignore list too."`
	Output   string        `short:"o" enum:"table,json,xlsx" default:"table" help:"Output format (${enum}); xlsx needs -f or a redirect."`
	File     string        `short:"f" type:"path" help:"Write the output to this file instead of stdout."`
	Timeout  time.Duration `default:"30s" help:"Per-request timeout."`

	ignore []string
}

// CLI is the root command.
type CLI struct {
	Globals

	Sites    SitesCmd         `cmd:"" help:"List all sites with device and client counts."`
	Devices  DevicesCmd       `cmd:"" help:"List all devices."`
	Networks NetworksCmd      `cmd:"" help:"List all networks (name, VLAN, subnet) per site."`
	VPN      VPNCmd           `cmd:"" name:"vpn" help:"List VPN servers and site-to-site tunnels per site."`
	Version  kong.VersionFlag `help:"Print version and exit."`
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

// client loads the config file and builds a Site Manager client from the API
// key in the environment.
func (g *Globals) client() (*sitemanager.Client, error) {
	cfg, err := config.Load(g.Config)
	if err != nil {
		return nil, err
	}
	if !g.NoIgnore {
		g.ignore = cfg.Ignore
	}

	key := os.Getenv(keyEnv)
	if key == "" {
		return nil, fmt.Errorf("%s is not set: add it to ~/.config/unifictl/.env (create a key at unifi.ui.com → Settings → API Keys; for networks it must come from the consoles' owner and include UniFi Applications → Network)", keyEnv)
	}
	req := httpx.New(httpx.Options{APIKey: key, Timeout: g.Timeout, Limiter: httpx.NewLimiter(2)})
	return sitemanager.New(sitemanager.DefaultURL, req), nil
}

// checkOutput rejects output settings that can't work, before any slow
// requests are made.
func (g *Globals) checkOutput() error {
	if g.Output != "xlsx" {
		return nil
	}
	if g.File == "" {
		if fi, err := os.Stdout.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
			return errors.New("xlsx is a binary format: add -f FILE.xlsx or redirect the output to a file")
		}
	}
	return nil
}

// write sends a command's output to stdout or to --file.
func (g *Globals) write(fn func(w io.Writer) error) error {
	if g.File == "" {
		return fn(os.Stdout)
	}
	f, err := os.Create(g.File)
	if err != nil {
		return err
	}
	if err := fn(f); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", g.File)
	return nil
}

// json writes v as JSON to stdout or --file.
func (g *Globals) json(v any) error {
	return g.write(func(w io.Writer) error { return output.JSON(w, v) })
}

// table writes a text table to stdout or --file.
func (g *Globals) table(headers []string, rows [][]string) error {
	return g.write(func(w io.Writer) error { return output.Table(w, headers, rows) })
}

// xlsx writes a worksheet to --file or stdout.
func (g *Globals) xlsx(sheet output.Sheet) error {
	return g.write(func(w io.Writer) error { return output.XLSX(w, sheet) })
}

// ignored reports whether a site is on the config file's ignore list.
func (g *Globals) ignored(site string) bool {
	return site != "" && len(g.ignore) > 0 && report.Match(g.ignore, site)
}

func dotenvPaths() []string {
	paths := []string{".env"}
	if dir, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(dir, ".config", "unifictl", ".env"))
	}
	return paths
}

// loadDotenv sets variables from a .env file if it exists. Variables already
// in the environment take precedence. The file holds API keys that can change
// settings, so it is refused unless only its owner can read it (0600 or 0400).
func loadDotenv(path string) error {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return fmt.Errorf("%s is readable by others (mode %04o); it holds API keys, so fix it with: chmod 600 %s", path, perm, path)
	}
	if err := godotenv.Load(path); err != nil {
		return fmt.Errorf("load %s: %w", path, err)
	}
	return nil
}
