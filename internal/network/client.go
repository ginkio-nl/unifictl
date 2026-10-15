// Package network reads networks and VPNs from UniFi Network applications
// (read-only). It uses the documented Network Integration API, plus the
// application's older, undocumented API for VPN subnets, which the
// Integration API doesn't expose.
package network

import (
	"context"
	"net/url"
	"strconv"

	"github.com/ginkio-nl/unifictl/internal/httpx"
)

const pageSize = 200

// Client talks to one console's Network application.
type Client struct {
	base string // the application root, e.g. .../proxy/network
	req  *httpx.Requester
}

// New creates a Client. base is the Network application root; the
// Integration API lives under base + "/integration".
func New(base string, req *httpx.Requester) *Client {
	return &Client{base: base, req: req}
}

// Site is a site as the Network application reports it.
type Site struct {
	ID                string `json:"id"`
	InternalReference string `json:"internalReference"`
	Name              string `json:"name"`
}

// Network is a network (VLAN). The list endpoint leaves IPv4 empty; the
// details endpoint fills it in.
type Network struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Management string `json:"management"` // "GATEWAY", "SWITCH", "UNMANAGED"
	Enabled    bool   `json:"enabled"`
	VLANID     int    `json:"vlanId"`
	Default    bool   `json:"default"`
	IPv4       *IPv4  `json:"ipv4Configuration,omitempty"`
}

// IPv4 is a network's IPv4 configuration.
type IPv4 struct {
	HostIPAddress string `json:"hostIpAddress"`
	PrefixLength  int    `json:"prefixLength"`
	DHCP          *DHCP  `json:"dhcpConfiguration,omitempty"`
}

// DHCP is a network's DHCP configuration.
type DHCP struct {
	Mode           string `json:"mode"` // e.g. "SERVER", "RELAY"
	IPAddressRange *struct {
		Start string `json:"start"`
		Stop  string `json:"stop"`
	} `json:"ipAddressRange,omitempty"`
}

type page[T any] struct {
	TotalCount int `json:"totalCount"`
	Data       []T `json:"data"`
}

// Sites lists the console's sites.
func (c *Client) Sites(ctx context.Context) ([]Site, error) {
	return getAll[Site](ctx, c, "/v1/sites")
}

// Networks lists a site's networks (without IPv4 details).
func (c *Client) Networks(ctx context.Context, siteID string) ([]Network, error) {
	return getAll[Network](ctx, c, "/v1/sites/"+url.PathEscape(siteID)+"/networks")
}

// Network returns one network with its details.
func (c *Client) Network(ctx context.Context, siteID, networkID string) (Network, error) {
	var n Network
	err := c.req.GetJSON(ctx, c.base+"/integration/v1/sites/"+url.PathEscape(siteID)+"/networks/"+url.PathEscape(networkID), &n)
	return n, err
}

// VPN is a VPN server or site-to-site tunnel. The API only reports these
// summary fields; there is no details endpoint.
type VPN struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"` // e.g. WIREGUARD, L2TP, OPENVPN, IPSEC
	Enabled bool   `json:"enabled"`
}

// VPNServers lists a site's VPN servers.
func (c *Client) VPNServers(ctx context.Context, siteID string) ([]VPN, error) {
	return getAll[VPN](ctx, c, "/v1/sites/"+url.PathEscape(siteID)+"/vpn/servers")
}

// SiteToSiteTunnels lists a site's site-to-site VPN tunnels.
func (c *Client) SiteToSiteTunnels(ctx context.Context, siteID string) ([]VPN, error) {
	return getAll[VPN](ctx, c, "/v1/sites/"+url.PathEscape(siteID)+"/vpn/site-to-site-tunnels")
}

// VPNSubnets holds the subnets of a site's VPNs, by VPN name. They come
// from the Network application's undocumented API (/api/s/<site>/rest/
// networkconf), as the Integration API doesn't report them.
type VPNSubnets struct {
	Servers map[string]string   // VPN server name -> client subnet
	Tunnels map[string][]string // site-to-site tunnel name -> remote subnets
}

// legacyNetwork holds only the fields unifictl reads. The endpoint also
// returns VPN private keys and pre-shared keys; those are never decoded.
type legacyNetwork struct {
	Name             string   `json:"name"`
	Purpose          string   `json:"purpose"`
	IPSubnet         string   `json:"ip_subnet"`
	RemoteVPNSubnets []string `json:"remote_vpn_subnets"`
}

// VPNSubnets reads the subnets of a site's VPNs. siteRef is the site's
// internal reference (Site.InternalReference), not its ID.
func (c *Client) VPNSubnets(ctx context.Context, siteRef string) (VPNSubnets, error) {
	var resp struct {
		Data []legacyNetwork `json:"data"`
	}
	out := VPNSubnets{Servers: map[string]string{}, Tunnels: map[string][]string{}}
	if err := c.req.GetJSON(ctx, c.base+"/api/s/"+url.PathEscape(siteRef)+"/rest/networkconf", &resp); err != nil {
		return out, err
	}
	for _, n := range resp.Data {
		switch n.Purpose {
		case "remote-user-vpn":
			out.Servers[n.Name] = n.IPSubnet
		case "site-vpn":
			out.Tunnels[n.Name] = n.RemoteVPNSubnets
		}
	}
	return out, nil
}

func getAll[T any](ctx context.Context, c *Client, path string) ([]T, error) {
	var all []T
	for offset := 0; ; {
		q := url.Values{"offset": {strconv.Itoa(offset)}, "limit": {strconv.Itoa(pageSize)}}
		var p page[T]
		if err := c.req.GetJSON(ctx, c.base+"/integration"+path+"?"+q.Encode(), &p); err != nil {
			return nil, err
		}
		all = append(all, p.Data...)
		offset += len(p.Data)
		if len(p.Data) == 0 || offset >= p.TotalCount {
			return all, nil
		}
	}
}
