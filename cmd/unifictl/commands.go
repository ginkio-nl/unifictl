package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/ginkio-nl/unifictl/internal/output"
	"github.com/ginkio-nl/unifictl/internal/report"
)

// SitesCmd lists sites.
type SitesCmd struct {
	Name    []string `short:"n" help:"Only show sites whose name matches these patterns (e.g. 'keesde*')."`
	Offline bool     `help:"Only show sites with offline devices."`
}

func (c *SitesCmd) Run(ctx context.Context, g *Globals) error {
	sm, err := g.client()
	if err != nil {
		return err
	}
	hosts, err := sm.Hosts(ctx)
	if err != nil {
		return fmt.Errorf("list hosts: %w", err)
	}
	sites, err := sm.Sites(ctx)
	if err != nil {
		return fmt.Errorf("list sites: %w", err)
	}

	var rows []report.Site
	for _, s := range report.Sites(hosts, sites) {
		if !g.ignored(s.Name) && report.Match(c.Name, s.Name, s.SiteName) && (!c.Offline || s.Offline > 0) {
			rows = append(rows, s)
		}
	}

	if g.Output == "json" {
		return output.JSON(os.Stdout, rows)
	}
	table := make([][]string, len(rows))
	for i, s := range rows {
		table[i] = []string{s.Name, strconv.Itoa(s.Devices), strconv.Itoa(s.Offline), strconv.Itoa(s.Clients), dash(s.ISP), s.Host}
	}
	return output.Table(os.Stdout, []string{"SITE", "DEVICES", "OFFLINE", "CLIENTS", "ISP", "HOST"}, table)
}

// DevicesCmd lists devices.
type DevicesCmd struct {
	Site    []string `short:"s" help:"Only show devices of sites matching these patterns. Only works for hosts with a single site."`
	Host    []string `help:"Only show devices of hosts matching these patterns."`
	Offline bool     `help:"Only show offline devices."`
	Updates bool     `help:"Only show devices with a firmware update available."`
}

func (c *DevicesCmd) Run(ctx context.Context, g *Globals) error {
	sm, err := g.client()
	if err != nil {
		return err
	}
	hosts, err := sm.Hosts(ctx)
	if err != nil {
		return fmt.Errorf("list hosts: %w", err)
	}
	sites, err := sm.Sites(ctx)
	if err != nil {
		return fmt.Errorf("list sites: %w", err)
	}
	inventory, err := sm.Devices(ctx)
	if err != nil {
		return fmt.Errorf("list devices: %w", err)
	}

	var rows []report.Device
	for _, d := range report.Devices(hosts, sites, inventory) {
		if g.ignored(d.Site) || !report.Match(c.Site, d.Site) || !report.Match(c.Host, d.Host) {
			continue
		}
		if (c.Offline && d.Status != "offline") || (c.Updates && !d.UpdateAvailable) {
			continue
		}
		rows = append(rows, d)
	}

	if g.Output == "json" {
		return output.JSON(os.Stdout, rows)
	}
	table := make([][]string, len(rows))
	for i, d := range rows {
		update := ""
		if d.UpdateAvailable {
			update = "yes"
		}
		table[i] = []string{dash(d.Site), d.Name, d.Model, dash(d.IP), d.MAC, d.Status, dash(d.Firmware), update, d.Host}
	}
	return output.Table(os.Stdout, []string{"SITE", "NAME", "MODEL", "IP", "MAC", "STATUS", "FIRMWARE", "UPDATE", "HOST"}, table)
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
