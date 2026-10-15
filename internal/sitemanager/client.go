// Package sitemanager is a minimal client for the UniFi Site Manager API
// (api.ui.com): the hosts, sites, and devices an API key can see, and the
// cloud connector URL for reaching a console's Network application.
package sitemanager

import (
	"context"
	"net/url"

	"github.com/ginkio-nl/unifictl/internal/httpx"
)

// DefaultURL is the Site Manager API base URL.
const DefaultURL = "https://api.ui.com"

// Client talks to the Site Manager API.
type Client struct {
	base string
	req  *httpx.Requester
}

// New creates a Client.
func New(base string, req *httpx.Requester) *Client {
	return &Client{base: base, req: req}
}

// Host is a UniFi console or server (e.g. a UDM Pro, or a UniFi OS Server).
type Host struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	Owner         bool   `json:"owner"`
	IsBlocked     bool   `json:"isBlocked"`
	ReportedState *struct {
		Name     string `json:"name"`
		Hostname string `json:"hostname"`
		Version  string `json:"version"`
	} `json:"reportedState"`
}

// Name returns the host's display name.
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

// Site is one network managed by a host. A host has one site (a customer's
// own console) or many (a hosted server).
type Site struct {
	SiteID string `json:"siteId"`
	HostID string `json:"hostId"`
	Meta   struct {
		Desc     string `json:"desc"` // the site's name
		Name     string `json:"name"` // internal reference, e.g. "default"
		Timezone string `json:"timezone"`
	} `json:"meta"`
	Statistics struct {
		Counts  Counts `json:"counts"`
		ISPInfo *struct {
			Name         string `json:"name"`
			Organization string `json:"organization"`
		} `json:"ispInfo"`
	} `json:"statistics"`
}

// Counts are a site's device and client totals.
type Counts struct {
	TotalDevice   int `json:"totalDevice"`
	OfflineDevice int `json:"offlineDevice"`
	WifiClient    int `json:"wifiClient"`
	WiredClient   int `json:"wiredClient"`
	GuestClient   int `json:"guestClient"`
}

// HostDevices is the device inventory of one host. Site Manager does not
// say which of the host's sites a device belongs to.
type HostDevices struct {
	HostID   string   `json:"hostId"`
	HostName string   `json:"hostName"`
	Devices  []Device `json:"devices"`
}

// Device is a UniFi device.
type Device struct {
	ID             string `json:"id"`
	MAC            string `json:"mac"`
	Name           string `json:"name"`
	Model          string `json:"model"`
	IP             string `json:"ip"`
	Status         string `json:"status"`         // "online", "offline"
	Version        string `json:"version"`        // firmware
	FirmwareStatus string `json:"firmwareStatus"` // "upToDate", "updateAvailable"
	IsConsole      bool   `json:"isConsole"`
}

// NetworkBase returns the root of a console's Network application, reached
// through the cloud connector. ui.com only allows this when the API key's
// account owns the console and the key has the Network application scope
// for it.
func (c *Client) NetworkBase(hostID string) string {
	return c.base + "/v1/connector/consoles/" + url.PathEscape(hostID) + "/proxy/network"
}

// Requester returns the HTTP requester, to share its key and rate limit.
func (c *Client) Requester() *httpx.Requester { return c.req }

type listPage[T any] struct {
	Data      []T    `json:"data"`
	NextToken string `json:"nextToken"`
}

// Hosts lists all hosts visible to the API key.
func (c *Client) Hosts(ctx context.Context) ([]Host, error) {
	return listAll[Host](ctx, c, "/v1/hosts")
}

// Sites lists all sites visible to the API key.
func (c *Client) Sites(ctx context.Context) ([]Site, error) {
	return listAll[Site](ctx, c, "/v1/sites")
}

// Devices lists the device inventory of every host visible to the API key.
func (c *Client) Devices(ctx context.Context) ([]HostDevices, error) {
	return listAll[HostDevices](ctx, c, "/v1/devices")
}

func listAll[T any](ctx context.Context, c *Client, path string) ([]T, error) {
	var all []T
	next := ""
	for {
		q := url.Values{"pageSize": {"500"}}
		if next != "" {
			q.Set("nextToken", next)
		}
		var p listPage[T]
		if err := c.req.GetJSON(ctx, c.base+path+"?"+q.Encode(), &p); err != nil {
			return nil, err
		}
		all = append(all, p.Data...)
		if p.NextToken == "" || p.NextToken == next {
			return all, nil
		}
		next = p.NextToken
	}
}
