// Package unifi is a minimal client for the UniFi Network Integration API (v1).
// It works against a console directly or through the Site Manager cloud
// connector; only the base URL differs.
package unifi

import (
	"context"
	"net/url"
	"strconv"
	"time"

	"github.com/ginkio/unifictl/internal/httpx"
)

// pageSize is the maximum page size accepted by the Integration API.
const pageSize = 200

// Client talks to one UniFi Network application.
type Client struct {
	base string // e.g. https://192.168.1.1/proxy/network/integration
	req  *httpx.Requester
}

// New creates a Client. base is the Integration API root, without /v1.
func New(base string, req *httpx.Requester) *Client {
	return &Client{base: base, req: req}
}

// Info is the application information returned by /v1/info.
type Info struct {
	ApplicationVersion string `json:"applicationVersion"`
}

// Site is a UniFi site.
type Site struct {
	ID                string `json:"id"`
	InternalReference string `json:"internalReference"`
	Name              string `json:"name"`
}

// Device is an adopted UniFi device.
type Device struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Model      string   `json:"model"`
	MACAddress string   `json:"macAddress"`
	IPAddress  string   `json:"ipAddress"`
	State      string   `json:"state"`
	Features   []string `json:"features"`
}

// NetClient is a connected network client (named to avoid clashing with Client).
type NetClient struct {
	Type           string    `json:"type"`
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	ConnectedAt    time.Time `json:"connectedAt"`
	IPAddress      string    `json:"ipAddress"`
	MACAddress     string    `json:"macAddress"`
	UplinkDeviceID string    `json:"uplinkDeviceId"`
}

type page[T any] struct {
	Offset     int `json:"offset"`
	Limit      int `json:"limit"`
	Count      int `json:"count"`
	TotalCount int `json:"totalCount"`
	Data       []T `json:"data"`
}

// Info returns the Network application version.
func (c *Client) Info(ctx context.Context) (Info, error) {
	var info Info
	err := c.req.GetJSON(ctx, c.base+"/v1/info", &info)
	return info, err
}

// Sites lists all sites on the console.
func (c *Client) Sites(ctx context.Context) ([]Site, error) {
	return getAll[Site](ctx, c, "/v1/sites")
}

// Devices lists adopted devices in a site.
func (c *Client) Devices(ctx context.Context, siteID string) ([]Device, error) {
	return getAll[Device](ctx, c, "/v1/sites/"+url.PathEscape(siteID)+"/devices")
}

// Clients lists connected clients in a site.
func (c *Client) Clients(ctx context.Context, siteID string) ([]NetClient, error) {
	return getAll[NetClient](ctx, c, "/v1/sites/"+url.PathEscape(siteID)+"/clients")
}

func getAll[T any](ctx context.Context, c *Client, path string) ([]T, error) {
	var all []T
	for offset := 0; ; {
		q := url.Values{
			"offset": {strconv.Itoa(offset)},
			"limit":  {strconv.Itoa(pageSize)},
		}
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
