package network

import (
	"cmp"
	"context"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"sync"
)

// Console is a console to read networks from.
type Console struct {
	Name   string
	Client *Client
	// DisplayName maps a site to the name to show (see report.DisplayName).
	DisplayName func(site Site, consoleSites int) string
}

// Row is one network of one site.
type Row struct {
	Site    string `json:"site"`
	Console string `json:"console"`
	Network string `json:"network"`
	VLAN    int    `json:"vlan"`
	Subnet  string `json:"subnet,omitempty"`  // e.g. 10.0.1.0/24
	Gateway string `json:"gateway,omitempty"` // the gateway's address in the subnet
	DHCP    string `json:"dhcp,omitempty"`    // range, or mode if not serving
	Enabled bool   `json:"enabled"`
}

// Error is a failure on one console or site.
type Error struct {
	Where string
	Err   error
}

func (e Error) Error() string { return fmt.Sprintf("%s: %v", e.Where, e.Err) }
func (e Error) Unwrap() error { return e.Err }

// Collect reads all networks of the sites on the consoles for which keep
// returns true (given the site's display name), with at most limit requests
// in flight. Failures are returned per console or site; the rows of
// everything else are still returned, sorted by site and VLAN.
func Collect(ctx context.Context, consoles []Console, limit int, keep func(site string) bool) ([]Row, []error) {
	type siteRef struct {
		console Console
		site    Site
		name    string
	}

	siteLists, errs := each(ctx, consoles, limit, func(ctx context.Context, c Console) ([]Site, error) {
		return c.Client.Sites(ctx)
	})
	var sites []siteRef
	var failures []error
	for i, c := range consoles {
		if errs[i] != nil {
			failures = append(failures, Error{Where: c.Name, Err: errs[i]})
			continue
		}
		for _, s := range siteLists[i] {
			name := c.DisplayName(s, len(siteLists[i]))
			if keep(name) {
				sites = append(sites, siteRef{console: c, site: s, name: name})
			}
		}
	}

	netLists, errs := each(ctx, sites, limit, func(ctx context.Context, s siteRef) ([]Network, error) {
		list, err := s.console.Client.Networks(ctx, s.site.ID)
		if err != nil {
			return nil, err
		}
		// The list has no IPv4 details; fetch them per network.
		details, errs := each(ctx, list, limit, func(ctx context.Context, n Network) (Network, error) {
			return s.console.Client.Network(ctx, s.site.ID, n.ID)
		})
		for i, err := range errs {
			if err != nil {
				return nil, fmt.Errorf("network %s: %w", list[i].Name, err)
			}
		}
		return details, nil
	})

	var rows []Row
	for i, s := range sites {
		if errs[i] != nil {
			failures = append(failures, Error{Where: s.console.Name + "/" + s.name, Err: errs[i]})
			continue
		}
		for _, n := range netLists[i] {
			rows = append(rows, row(s.name, s.console.Name, n))
		}
	}
	slices.SortStableFunc(rows, func(a, b Row) int {
		return cmp.Or(
			cmp.Compare(strings.ToLower(a.Site), strings.ToLower(b.Site)),
			cmp.Compare(a.VLAN, b.VLAN),
			cmp.Compare(strings.ToLower(a.Network), strings.ToLower(b.Network)),
		)
	})
	return rows, failures
}

func row(site, console string, n Network) Row {
	r := Row{Site: site, Console: console, Network: n.Name, VLAN: n.VLANID, Enabled: n.Enabled}
	if v4 := n.IPv4; v4 != nil {
		if addr, err := netip.ParseAddr(v4.HostIPAddress); err == nil {
			r.Gateway = addr.String()
			if p, err := addr.Prefix(v4.PrefixLength); err == nil {
				r.Subnet = p.String()
			}
		}
		if d := v4.DHCP; d != nil {
			if rg := d.IPAddressRange; rg != nil && rg.Start != "" {
				r.DHCP = rg.Start + " - " + rg.Stop
			} else {
				r.DHCP = strings.ToLower(d.Mode)
			}
		}
	}
	return r
}

// each runs fn for every input with at most limit concurrent calls,
// preserving input order in the results.
func each[In, Out any](ctx context.Context, inputs []In, limit int, fn func(context.Context, In) (Out, error)) ([]Out, []error) {
	outs := make([]Out, len(inputs))
	errs := make([]error, len(inputs))
	sem := make(chan struct{}, max(limit, 1))
	var wg sync.WaitGroup
	for i, in := range inputs {
		wg.Go(func() {
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				errs[i] = ctx.Err()
				return
			}
			outs[i], errs[i] = fn(ctx, in)
		})
	}
	wg.Wait()
	return outs, errs
}
