// Command unifictl queries UniFi consoles and sites across multiple sources.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/joho/godotenv"

	"github.com/ginkio/unifictl/internal/config"
	"github.com/ginkio/unifictl/internal/fleet"
)

var version = "dev"

// Globals are flags shared by every command.
type Globals struct {
	Config      string        `short:"c" type:"path" default:"~/.config/unifictl/config.yaml" env:"UNIFICTL_CONFIG" help:"Path to the config file."`
	Source      []string      `short:"S" help:"Only use these sources (by name)."`
	Console     []string      `short:"C" help:"Only query consoles matching these globs (name or ID)."`
	Site        []string      `short:"s" help:"Only query sites matching these globs (name, reference, or ID)."`
	Output      string        `short:"o" enum:"table,json" default:"table" help:"Output format (${enum})."`
	Timeout     time.Duration `default:"30s" help:"Per-request timeout."`
	Concurrency int           `default:"16" help:"Maximum consoles/sites queried in parallel (per-source request limits still apply)."`

	failures int
}

// CLI is the root command.
type CLI struct {
	Globals

	Consoles ConsolesCmd      `cmd:"" help:"List consoles from all sources."`
	Sites    SitesCmd         `cmd:"" help:"List sites across all consoles."`
	Devices  DevicesCmd       `cmd:"" help:"List devices across all sites."`
	Clients  ClientsCmd       `cmd:"" help:"List connected clients across all sites."`
	Version  kong.VersionFlag `help:"Print version and exit."`
}

func main() {
	// Loaded before parsing so .env can also set UNIFICTL_CONFIG.
	if err := loadDotenv(".env"); err != nil {
		fmt.Fprintf(os.Stderr, "unifictl: error: %v\n", err)
		os.Exit(1)
	}

	var cli CLI
	kctx := kong.Parse(&cli,
		kong.Name("unifictl"),
		kong.Description("Query UniFi consoles and sites across Site Manager accounts and local consoles."),
		kong.UsageOnError(),
		kong.Vars{"version": version},
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	kctx.BindTo(ctx, (*context.Context)(nil))

	err := kctx.Run(&cli.Globals)
	if err == nil && cli.failures > 0 {
		err = fmt.Errorf("%d target(s) failed; results are incomplete", cli.failures)
	}
	kctx.FatalIfErrorf(err)
}

func (g *Globals) filter() fleet.Filter {
	return fleet.Filter{Consoles: g.Console, Sites: g.Site}
}

// warn reports partial failures on stderr and remembers them for the exit code.
func (g *Globals) warn(errs []error) {
	for _, err := range errs {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
	g.failures += len(errs)
}

// consoles loads the config and resolves the selected consoles.
func (g *Globals) consoles(ctx context.Context) ([]fleet.Console, error) {
	if err := loadDotenv(filepath.Join(filepath.Dir(g.Config), ".env")); err != nil {
		return nil, err
	}
	cfg, err := config.Load(g.Config)
	if err != nil {
		return nil, err
	}
	srcs, err := cfg.Select(g.Source)
	if err != nil {
		return nil, err
	}
	consoles, errs := fleet.Resolve(ctx, srcs, g.filter(), g.Timeout)
	g.warn(errs)
	return consoles, nil
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

// sites resolves consoles and lists the selected sites on each.
func (g *Globals) sites(ctx context.Context) ([]fleet.SiteRef, error) {
	consoles, err := g.consoles(ctx)
	if err != nil {
		return nil, err
	}
	sites, errs := fleet.Sites(ctx, consoles, g.filter(), g.Concurrency)
	g.warn(errs)
	return sites, nil
}
