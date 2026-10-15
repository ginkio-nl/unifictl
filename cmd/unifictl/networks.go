package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ginkio-nl/unifictl/internal/network"
	"github.com/ginkio-nl/unifictl/internal/output"
	"github.com/ginkio-nl/unifictl/internal/report"
)

// NetworksCmd lists networks per site.
type NetworksCmd struct {
	Name     []string `short:"n" help:"Only show sites whose name matches these patterns (e.g. 'keesde*')."`
	Verbose  bool     `short:"v" help:"Also show each network's gateway and DHCP range."`
	Overlaps bool     `help:"Also list VPN server subnets, and mark subnets that overlap one at another site (highlighted in xlsx)."`
}

// networkOut is a network row as written to JSON.
type networkOut struct {
	network.Row
	OverlapsWith []string `json:"overlapsWith,omitempty"`
}

func (c *NetworksCmd) Run(ctx context.Context, g *Globals) error {
	if err := g.checkOutput(); err != nil {
		return err
	}
	sites, err := g.networkSites(ctx, c.Name)
	if err != nil {
		return err
	}
	rows, errs := network.Networks(ctx, sites, 8)
	warnAll(errs)

	// Overlaps are computed over the shown sites only, including their VPN
	// servers' client subnets, which need to be routable too.
	overlaps := make([][]string, len(rows))
	if c.Overlaps {
		vpns, errs := network.VPNs(ctx, sites, 8)
		warnAll(errs)
		rows = network.WithVPNs(rows, vpns)
		overlaps = network.Overlaps(rows)
	}

	switch g.Output {
	case "json":
		out := make([]networkOut, len(rows))
		for i, r := range rows {
			out[i] = networkOut{Row: r, OverlapsWith: overlaps[i]}
		}
		return g.json(out)

	case "xlsx":
		sheet := output.Sheet{Name: "Networks", Headers: []string{"Site", "Network", "VLAN", "Subnet"}}
		if c.Verbose {
			sheet.Headers = append(sheet.Headers, "Gateway", "DHCP")
		}
		sheet.Headers = append(sheet.Headers, "Enabled")
		if c.Overlaps {
			sheet.Headers = append(sheet.Headers, "Overlaps with")
			sheet.Highlight = make([]bool, len(rows))
		}
		for i, r := range rows {
			row := []any{r.Site, r.Network, vlanCell(r), r.Subnet}
			if c.Verbose {
				row = append(row, r.Gateway, r.DHCP)
			}
			row = append(row, yesNo(r.Enabled))
			if c.Overlaps {
				row = append(row, strings.Join(overlaps[i], ", "))
				sheet.Highlight[i] = len(overlaps[i]) > 0
			}
			sheet.Rows = append(sheet.Rows, row)
		}
		return g.xlsx(sheet)
	}

	headers := []string{"SITE", "NETWORK", "VLAN", "SUBNET"}
	if c.Verbose {
		headers = append(headers, "GATEWAY", "DHCP")
	}
	if c.Overlaps {
		headers = append(headers, "OVERLAPS WITH")
	}
	table := make([][]string, len(rows))
	for i, r := range rows {
		name := r.Network
		if !r.Enabled {
			name += " (disabled)"
		}
		table[i] = []string{r.Site, name, fmt.Sprint(vlanCell(r)), dash(r.Subnet)}
		if c.Verbose {
			table[i] = append(table[i], dash(r.Gateway), dash(r.DHCP))
		}
		if c.Overlaps {
			table[i] = append(table[i], dash(strings.Join(overlaps[i], ", ")))
		}
	}
	return g.table(headers, table)
}

// VPNCmd lists VPN servers and site-to-site tunnels per site.
type VPNCmd struct {
	Name []string `short:"n" help:"Only show sites whose name matches these patterns (e.g. 'keesde*')."`
}

func (c *VPNCmd) Run(ctx context.Context, g *Globals) error {
	if err := g.checkOutput(); err != nil {
		return err
	}
	sites, err := g.networkSites(ctx, c.Name)
	if err != nil {
		return err
	}
	rows, errs := network.VPNs(ctx, sites, 8)
	warnAll(errs)

	switch g.Output {
	case "json":
		return g.json(rows)
	case "xlsx":
		sheet := output.Sheet{Name: "VPN", Headers: []string{"Site", "Kind", "Name", "Type", "Subnet", "Enabled"}}
		for _, r := range rows {
			sheet.Rows = append(sheet.Rows, []any{r.Site, r.Kind, r.Name, r.Type, vpnSubnet(r), yesNo(r.Enabled)})
		}
		return g.xlsx(sheet)
	}
	table := make([][]string, len(rows))
	for i, r := range rows {
		table[i] = []string{r.Site, r.Kind, r.Name, r.Type, dash(vpnSubnet(r)), yesNo(r.Enabled)}
	}
	return g.table([]string{"SITE", "KIND", "NAME", "TYPE", "SUBNET", "ENABLED"}, table)
}

// networkSites returns the sites whose Network application unifictl can
// read, after the ignore list and the name patterns. Consoles it can't read
// are reported as warnings.
func (g *Globals) networkSites(ctx context.Context, names []string) ([]network.SiteRef, error) {
	sm, err := g.client()
	if err != nil {
		return nil, err
	}
	hosts, err := sm.Hosts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list hosts: %w", err)
	}
	sites, err := sm.Sites(ctx)
	if err != nil {
		return nil, fmt.Errorf("list sites: %w", err)
	}

	keep := func(site string) bool {
		return !g.ignored(site) && report.Match(names, site)
	}

	// Only query consoles that have at least one site we're going to show
	// (consoles without a Network application have no sites at all).
	wanted := make(map[string]bool)
	for _, s := range report.Sites(hosts, sites) {
		if keep(s.Name) {
			wanted[s.HostID] = true
		}
	}

	var consoles []network.Console
	for _, h := range hosts {
		if h.IsBlocked || !wanted[h.ID] {
			continue
		}
		name := h.Name()
		if !h.Owner {
			// ui.com only connects an API key to consoles its account owns.
			warn("%s: skipped, this needs an API key from the console's owner", name)
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

	refs, errs := network.Sites(ctx, consoles, 8, keep)
	warnAll(errs)
	return refs, nil
}

// warn prints a warning on stderr.
func warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "warning: "+format+"\n", args...)
}

// vlanCell is a network's VLAN ID, or "VPN" for a VPN server's subnet.
func vlanCell(r network.Row) any {
	if r.VPN {
		return "VPN"
	}
	return r.VLAN
}

// vpnSubnet is a VPN server's client subnet, or a tunnel's remote subnets.
func vpnSubnet(r network.VPNRow) string {
	if len(r.RemoteSubnets) > 0 {
		return "remote: " + strings.Join(r.RemoteSubnets, ", ")
	}
	return r.Subnet
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func warnAll(errs []error) {
	for _, err := range errs {
		warn("%v", err)
	}
}
