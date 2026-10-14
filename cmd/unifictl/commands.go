package main

import (
	"context"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/ginkio-nl/unifictl/internal/fleet"
	"github.com/ginkio-nl/unifictl/internal/output"
	"github.com/ginkio-nl/unifictl/internal/unifi"
)

// ConsolesCmd lists consoles.
type ConsolesCmd struct {
	Check bool `help:"Contact each console's Network application and report its version."`
}

type consoleRow struct {
	fleet.Console
	Network string `json:"network,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (c *ConsolesCmd) Run(ctx context.Context, g *Globals) error {
	consoles, err := g.consoles(ctx)
	if err != nil {
		return err
	}

	rows := make([]consoleRow, len(consoles))
	for i, con := range consoles {
		rows[i].Console = con
	}
	if c.Check {
		infos, errs := fleet.Each(ctx, consoles, g.Concurrency, func(ctx context.Context, con fleet.Console) (unifi.Info, error) {
			return con.Client.Info(ctx)
		})
		for i := range rows {
			if errs[i] != nil {
				rows[i].Error = errs[i].Error()
				g.failures++
			} else {
				rows[i].Network = infos[i].ApplicationVersion
			}
		}
	}

	if g.Output == "json" {
		return output.JSON(os.Stdout, rows)
	}
	headers := []string{"SOURCE", "CONSOLE", "ID", "VERSION"}
	if c.Check {
		headers = append(headers, "NETWORK", "STATUS")
	}
	table := make([][]string, len(rows))
	for i, r := range rows {
		table[i] = []string{r.Source, r.Name, r.ID, dash(r.Version)}
		if c.Check {
			status := "ok"
			if r.Error != "" {
				status = r.Error
			}
			table[i] = append(table[i], dash(r.Network), status)
		}
	}
	return output.Table(os.Stdout, headers, table)
}

// SitesCmd lists sites.
type SitesCmd struct{}

type siteRow struct {
	Source  string     `json:"source"`
	Console string     `json:"console"`
	Site    unifi.Site `json:"site"`
}

func (c *SitesCmd) Run(ctx context.Context, g *Globals) error {
	sites, err := g.sites(ctx)
	if err != nil {
		return err
	}

	if g.Output == "json" {
		rows := make([]siteRow, len(sites))
		for i, s := range sites {
			rows[i] = siteRow{Source: s.Console.Source, Console: s.Console.Name, Site: s.Site}
		}
		return output.JSON(os.Stdout, rows)
	}
	table := make([][]string, len(sites))
	for i, s := range sites {
		table[i] = []string{s.Console.Source, s.Console.Name, s.Site.Name, s.Site.InternalReference, s.Site.ID}
	}
	return output.Table(os.Stdout, []string{"SOURCE", "CONSOLE", "SITE", "REFERENCE", "ID"}, table)
}

// DevicesCmd lists devices.
type DevicesCmd struct {
	State []string `help:"Only show devices in these states (e.g. ONLINE, OFFLINE)."`
}

func (c *DevicesCmd) Run(ctx context.Context, g *Globals) error {
	sites, err := g.sites(ctx)
	if err != nil {
		return err
	}
	rows, errs := fleet.PerSite(ctx, sites, g.Concurrency, func(ctx context.Context, s fleet.SiteRef) ([]unifi.Device, error) {
		return s.Console.Client.Devices(ctx, s.Site.ID)
	})
	g.warn(errs)
	rows = filterRows(rows, c.State, func(d unifi.Device) string { return d.State })

	if g.Output == "json" {
		return output.JSON(os.Stdout, rows)
	}
	table := make([][]string, len(rows))
	for i, r := range rows {
		d := r.Item
		table[i] = []string{r.Source, r.Console, r.Site, d.Name, d.Model, dash(d.IPAddress), d.MACAddress, d.State}
	}
	return output.Table(os.Stdout, []string{"SOURCE", "CONSOLE", "SITE", "NAME", "MODEL", "IP", "MAC", "STATE"}, table)
}

// ClientsCmd lists connected clients.
type ClientsCmd struct {
	Type []string `help:"Only show clients of these types (e.g. WIRED, WIRELESS, VPN)."`
}

func (c *ClientsCmd) Run(ctx context.Context, g *Globals) error {
	sites, err := g.sites(ctx)
	if err != nil {
		return err
	}
	rows, errs := fleet.PerSite(ctx, sites, g.Concurrency, func(ctx context.Context, s fleet.SiteRef) ([]unifi.NetClient, error) {
		return s.Console.Client.Clients(ctx, s.Site.ID)
	})
	g.warn(errs)
	rows = filterRows(rows, c.Type, func(nc unifi.NetClient) string { return nc.Type })

	if g.Output == "json" {
		return output.JSON(os.Stdout, rows)
	}
	table := make([][]string, len(rows))
	for i, r := range rows {
		nc := r.Item
		connected := "-"
		if !nc.ConnectedAt.IsZero() {
			connected = time.Since(nc.ConnectedAt).Round(time.Minute).String()
		}
		table[i] = []string{r.Source, r.Console, r.Site, dash(nc.Name), nc.Type, dash(nc.IPAddress), dash(nc.MACAddress), connected}
	}
	return output.Table(os.Stdout, []string{"SOURCE", "CONSOLE", "SITE", "NAME", "TYPE", "IP", "MAC", "CONNECTED"}, table)
}

// filterRows keeps rows whose field (compared case-insensitively) is in want.
func filterRows[T any](rows []fleet.Row[T], want []string, field func(T) string) []fleet.Row[T] {
	if len(want) == 0 {
		return rows
	}
	return slices.DeleteFunc(rows, func(r fleet.Row[T]) bool {
		return !slices.ContainsFunc(want, func(w string) bool { return strings.EqualFold(w, field(r.Item)) })
	})
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
