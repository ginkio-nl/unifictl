// Package sitemanager is a minimal client for the UniFi Site Manager API
// (api.ui.com), used to discover consoles and reach them via the cloud
// connector.
package sitemanager

import (
	"context"
	"net/url"

	"github.com/ginkio/unifictl/internal/httpx"
)

// Client talks to the Site Manager API.
type Client struct {
	base string // e.g. https://api.ui.com
	req  *httpx.Requester
}

// New creates a Client.
func New(base string, req *httpx.Requester) *Client {
	return &Client{base: base, req: req}
}

// Host is a console registered to the account.
type Host struct {
	ID            string         `json:"id"`
	HardwareID    string         `json:"hardwareId"`
	Type          string         `json:"type"`
	IPAddress     string         `json:"ipAddress"`
	IsBlocked     bool           `json:"isBlocked"`
	ReportedState *ReportedState `json:"reportedState"`
}

// ReportedState is the subset of a host's self-reported state we use.
type ReportedState struct {
	Name     string `json:"name"`
	Hostname string `json:"hostname"`
	Version  string `json:"version"`
	State    string `json:"state"`
}

// Name returns the best available display name for the host.
func (h Host) Name() string {
	if rs := h.ReportedState; rs != nil {
		if rs.Name != "" {
			return rs.Name
		}
		if rs.Hostname != "" {
			return rs.Hostname
		}
	}
	return h.ID
}

type hostsPage struct {
	Data      []Host `json:"data"`
	NextToken string `json:"nextToken"`
}

// Hosts lists all consoles visible to the API key.
func (c *Client) Hosts(ctx context.Context) ([]Host, error) {
	var all []Host
	next := ""
	for {
		q := url.Values{"pageSize": {"100"}}
		if next != "" {
			q.Set("nextToken", next)
		}
		var p hostsPage
		if err := c.req.GetJSON(ctx, c.base+"/v1/hosts?"+q.Encode(), &p); err != nil {
			return nil, err
		}
		all = append(all, p.Data...)
		if p.NextToken == "" || p.NextToken == next {
			return all, nil
		}
		next = p.NextToken
	}
}

// NetworkBase returns the Network Integration API root for a console,
// reached through the cloud connector.
func (c *Client) NetworkBase(hostID string) string {
	return c.base + "/v1/connector/consoles/" + url.PathEscape(hostID) + "/proxy/network/integration"
}
