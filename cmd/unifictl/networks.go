package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/ginkio-nl/unifictl/internal/network"
	"github.com/ginkio-nl/unifictl/internal/output"
	"github.com/ginkio-nl/unifictl/internal/report"
)

// NetworksCmd lists networks per site.
type NetworksCmd struct {
	Name []string `short:"n" help:"Only show sites whose name matches these patterns (e.g. 'keesde*')."`
}

func (c *NetworksCmd) Run(ctx context.Context, g *Globals) error {
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

	// Only consoles that run a Network application have sites.
	hasSites := make(map[string]bool)
	for _, s := range sites {
		hasSites[s.HostID] = true
	}

	var consoles []network.Console
	for _, h := range hosts {
		if h.IsBlocked || !hasSites[h.ID] {
			continue
		}
		name := h.Name()
		if !h.Owner {
			// ui.com only connects an API key to consoles its account owns.
			warn("%s: skipped, networks need an API key from the console's owner", name)
			continue
		}
		consoles = append(consoles, network.Console{
			Name:   name,
			Client: network.New(sm.NetworkBase(h.ID), sm.Requester()),
			DisplayName: func(s network.Site, consoleSites int) string {
				return report.DisplayName(s.Name, s.InternalReference, name, consoleSites)
			},
		})
	}

	all, errs := network.Collect(ctx, consoles, 8)
	for _, err := range errs {
		warn("%v", err)
	}

	var rows []network.Row
	for _, r := range all {
		if report.Match(c.Name, r.Site) {
			rows = append(rows, r)
		}
	}

	if g.Output == "json" {
		return output.JSON(os.Stdout, rows)
	}
	table := make([][]string, len(rows))
	for i, r := range rows {
		name := r.Network
		if !r.Enabled {
			name += " (disabled)"
		}
		table[i] = []string{r.Site, name, strconv.Itoa(r.VLAN), dash(r.Subnet), dash(r.Gateway), dash(r.DHCP)}
	}
	return output.Table(os.Stdout, []string{"SITE", "NETWORK", "VLAN", "SUBNET", "GATEWAY", "DHCP"}, table)
}

// warn prints a warning on stderr.
func warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "warning: "+format+"\n", args...)
}
