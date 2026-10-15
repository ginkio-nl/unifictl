// Package network reads networks from UniFi Network applications through
// the Network Integration API (read-only).
package network

import (
	"context"
	"net/url"
	"strconv"

	"github.com/ginkio-nl/unifictl/internal/httpx"
)

const pageSize = 200

// Client talks to one console's Network Integration API.
type Client struct {
	base string // e.g. .../proxy/network/integration
	req  *httpx.Requester
}

// New creates a Client. base is the Integration API root, without /v1.
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
	err := c.req.GetJSON(ctx, c.base+"/v1/sites/"+url.PathEscape(siteID)+"/networks/"+url.PathEscape(networkID), &n)
	return n, err
}

func getAll[T any](ctx context.Context, c *Client, path string) ([]T, error) {
	var all []T
	for offset := 0; ; {
		q := url.Values{"offset": {strconv.Itoa(offset)}, "limit": {strconv.Itoa(pageSize)}}
		var p page[T]
		if err := c.req.GetJSON(ctx, c.base+path+"?"+q.Encode(), &p); err != nil {
			return nil, err
		}
		all = append(all, p.Data...)
		offset += len(p.Data)
		if len(p.Data) == 0 || offset >= p.TotalCount {
			return all, nil
		}
	}
}
