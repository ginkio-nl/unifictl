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

// Console is a console to read from.
type Console struct {
	Name   string
	Client *Client
	// DisplayName maps a site to the name to show (see report.DisplayName).
	DisplayName func(site Site, consoleSites int) string
}

// SiteRef is a site on a console, with the name to show for it.
type SiteRef struct {
	Console Console
	Site    Site
	Name    string
}

// Error is a failure on one console or site.
type Error struct {
	Where string
	Err   error
}

func (e Error) Error() string { return fmt.Sprintf("%s: %v", e.Where, e.Err) }
func (e Error) Unwrap() error { return e.Err }

// Sites lists the sites on the consoles for which keep returns true (given
// the site's display name), with at most limit requests in flight.
func Sites(ctx context.Context, consoles []Console, limit int, keep func(site string) bool) ([]SiteRef, []error) {
	lists, errs := each(ctx, consoles, limit, func(ctx context.Context, c Console) ([]Site, error) {
		return c.Client.Sites(ctx)
	})
	var sites []SiteRef
	var failures []error
	for i, c := range consoles {
		if errs[i] != nil {
			failures = append(failures, Error{Where: c.Name, Err: errs[i]})
			continue
		}
		for _, s := range lists[i] {
			if name := c.DisplayName(s, len(lists[i])); keep(name) {
				sites = append(sites, SiteRef{Console: c, Site: s, Name: name})
			}
		}
	}
	return sites, failures
}

// PerSite runs fn for every site, with at most limit sites in flight, and
// concatenates the results. Failures are returned per site; the results of
// the other sites are still returned.
func PerSite[T any](ctx context.Context, sites []SiteRef, limit int, fn func(context.Context, SiteRef) ([]T, error)) ([]T, []error) {
	lists, errs := each(ctx, sites, limit, fn)
	var out []T
	var failures []error
	for i, s := range sites {
		if errs[i] != nil {
			failures = append(failures, Error{Where: s.Console.Name + "/" + s.Name, Err: errs[i]})
			continue
		}
		out = append(out, lists[i]...)
	}
	return out, failures
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
	// VPN marks a VPN server's client subnet (see WithVPNs); VLAN is unused.
	VPN bool `json:"vpn,omitempty"`
}

// Networks reads every network of the sites, sorted by site and VLAN.
func Networks(ctx context.Context, sites []SiteRef, limit int) ([]Row, []error) {
	rows, failures := PerSite(ctx, sites, limit, func(ctx context.Context, s SiteRef) ([]Row, error) {
		list, err := s.Console.Client.Networks(ctx, s.Site.ID)
		if err != nil {
			return nil, err
		}
		// The list has no IPv4 details; fetch them per network.
		details, errs := each(ctx, list, limit, func(ctx context.Context, n Network) (Network, error) {
			return s.Console.Client.Network(ctx, s.Site.ID, n.ID)
		})
		rows := make([]Row, len(details))
		for i, n := range details {
			if errs[i] != nil {
				return nil, fmt.Errorf("network %s: %w", list[i].Name, errs[i])
			}
			rows[i] = networkRow(s, n)
		}
		return rows, nil
	})
	slices.SortStableFunc(rows, func(a, b Row) int {
		return cmp.Or(
			cmp.Compare(strings.ToLower(a.Site), strings.ToLower(b.Site)),
			cmp.Compare(a.VLAN, b.VLAN),
			cmp.Compare(strings.ToLower(a.Network), strings.ToLower(b.Network)),
		)
	})
	return rows, failures
}

func networkRow(s SiteRef, n Network) Row {
	r := Row{Site: s.Name, Console: s.Console.Name, Network: n.Name, VLAN: n.VLANID, Enabled: n.Enabled}
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

// WithVPNs adds the VPN servers' client subnets to the network rows, after
// each site's networks. Site-to-site tunnels are left out: their remote
// subnets point at another site's networks on purpose.
func WithVPNs(rows []Row, vpns []VPNRow) []Row {
	out := slices.Clone(rows)
	for _, v := range vpns {
		if v.Kind == "server" && v.Subnet != "" {
			out = append(out, Row{Site: v.Site, Console: v.Console, Network: v.Name, Subnet: v.Subnet, Enabled: v.Enabled, VPN: true})
		}
	}
	slices.SortStableFunc(out, func(a, b Row) int {
		return cmp.Or(
			cmp.Compare(strings.ToLower(a.Site), strings.ToLower(b.Site)),
			boolCompare(a.VPN, b.VPN),
			cmp.Compare(a.VLAN, b.VLAN),
			cmp.Compare(strings.ToLower(a.Network), strings.ToLower(b.Network)),
		)
	})
	return out
}

func boolCompare(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// Overlaps returns, for each row, the other sites that have a network whose
// subnet overlaps the row's subnet. Overlaps within one site are not
// reported, nor are rows without a subnet.
func Overlaps(rows []Row) [][]string {
	prefixes := make([]netip.Prefix, len(rows))
	for i, r := range rows {
		if p, err := netip.ParsePrefix(r.Subnet); err == nil {
			prefixes[i] = p
		}
	}
	out := make([][]string, len(rows))
	for i := range rows {
		if !prefixes[i].IsValid() {
			continue
		}
		for j := range rows {
			if rows[j].Site == rows[i].Site || !prefixes[j].IsValid() || !prefixes[i].Overlaps(prefixes[j]) {
				continue
			}
			if !slices.Contains(out[i], rows[j].Site) {
				out[i] = append(out[i], rows[j].Site)
			}
		}
		slices.SortFunc(out[i], func(a, b string) int { return cmp.Compare(strings.ToLower(a), strings.ToLower(b)) })
	}
	return out
}

// VPNRow is one VPN server or site-to-site tunnel of one site.
type VPNRow struct {
	Site    string `json:"site"`
	Console string `json:"console"`
	Kind    string `json:"kind"` // "server" or "site-to-site"
	Name    string `json:"name"`
	Type    string `json:"type"` // e.g. WIREGUARD, L2TP, OPENVPN, IPSEC
	Enabled bool   `json:"enabled"`
	// Subnet is a VPN server's client subnet, e.g. 10.0.80.0/24.
	Subnet string `json:"subnet,omitempty"`
	// RemoteSubnets are the subnets a site-to-site tunnel routes to.
	RemoteSubnets []string `json:"remoteSubnets,omitempty"`
}

// VPNs reads the VPN servers and site-to-site tunnels of the sites, sorted
// by site, kind, and name. Subnets come from the Network application's
// undocumented API; if that fails for a site, its VPNs are still returned,
// without subnets, and the failure is reported.
func VPNs(ctx context.Context, sites []SiteRef, limit int) ([]VPNRow, []error) {
	type result struct {
		rows    []VPNRow
		partial error // subnets unavailable; rows are still valid
	}
	results, errs := each(ctx, sites, limit, func(ctx context.Context, s SiteRef) (result, error) {
		servers, err := s.Console.Client.VPNServers(ctx, s.Site.ID)
		if err != nil {
			return result{}, fmt.Errorf("vpn servers: %w", err)
		}
		tunnels, err := s.Console.Client.SiteToSiteTunnels(ctx, s.Site.ID)
		if err != nil {
			return result{}, fmt.Errorf("site-to-site tunnels: %w", err)
		}
		var res result
		var subnets VPNSubnets
		if len(servers)+len(tunnels) > 0 {
			subnets, err = s.Console.Client.VPNSubnets(ctx, s.Site.InternalReference)
			if err != nil {
				res.partial = fmt.Errorf("vpn subnets unavailable: %w", err)
			}
		}
		for _, v := range servers {
			res.rows = append(res.rows, VPNRow{
				Site: s.Name, Console: s.Console.Name, Kind: "server", Name: v.Name, Type: v.Type, Enabled: v.Enabled,
				Subnet: maskedPrefix(subnets.Servers[v.Name]),
			})
		}
		for _, v := range tunnels {
			res.rows = append(res.rows, VPNRow{
				Site: s.Name, Console: s.Console.Name, Kind: "site-to-site", Name: v.Name, Type: v.Type, Enabled: v.Enabled,
				RemoteSubnets: subnets.Tunnels[v.Name],
			})
		}
		return res, nil
	})

	var rows []VPNRow
	var failures []error
	for i, s := range sites {
		where := s.Console.Name + "/" + s.Name
		if errs[i] != nil {
			failures = append(failures, Error{Where: where, Err: errs[i]})
			continue
		}
		if results[i].partial != nil {
			failures = append(failures, Error{Where: where, Err: results[i].partial})
		}
		rows = append(rows, results[i].rows...)
	}
	slices.SortStableFunc(rows, func(a, b VPNRow) int {
		return cmp.Or(
			cmp.Compare(strings.ToLower(a.Site), strings.ToLower(b.Site)),
			cmp.Compare(a.Kind, b.Kind),
			cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
		)
	})
	return rows, failures
}

// maskedPrefix turns a gateway address with prefix length ("10.0.80.1/24")
// into its network ("10.0.80.0/24"). Anything unparsable is returned as is.
func maskedPrefix(s string) string {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return s
	}
	return p.Masked().String()
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
