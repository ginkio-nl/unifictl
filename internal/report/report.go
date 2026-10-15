// Package report turns raw Site Manager data into the rows unifictl shows.
package report

import (
	"cmp"
	"path"
	"slices"
	"strings"

	"github.com/ginkio-nl/unifictl/internal/sitemanager"
)

// Site is one row of `unifictl sites`.
type Site struct {
	Name     string `json:"name"`     // as shown on unifi.ui.com (see siteName)
	SiteName string `json:"siteName"` // the site's own name in UniFi
	Host     string `json:"host"`
	ISP      string `json:"isp,omitempty"`
	Devices  int    `json:"devices"`
	Offline  int    `json:"offline"`
	Clients  int    `json:"clients"`
	SiteID   string `json:"siteId"`
	HostID   string `json:"hostId"`
}

// Device is one row of `unifictl devices`.
type Device struct {
	Site            string `json:"site,omitempty"` // empty if the host has several sites
	Host            string `json:"host"`
	Name            string `json:"name"`
	Model           string `json:"model"`
	IP              string `json:"ip,omitempty"`
	MAC             string `json:"mac"`
	Status          string `json:"status"`
	Firmware        string `json:"firmware,omitempty"`
	UpdateAvailable bool   `json:"updateAvailable"`
}

// DisplayName is the name unifi.ui.com shows for a site.
//
// A console with a single site (a customer's own UDM, Cloud Gateway, …)
// usually keeps UniFi's auto-created site name "Default"; ui.com then shows
// the console's name instead. A server with many sites shows each site's own
// name — including its own, usually empty, "Default" site.
//
// This mirrors observed ui.com behaviour, not a documented rule. If names
// ever look wrong, this is the place to look; JSON output keeps the raw
// site name in "siteName".
func DisplayName(siteName, reference, hostName string, hostSites int) string {
	if hostSites == 1 && strings.EqualFold(siteName, "default") {
		return hostName
	}
	if siteName != "" {
		return siteName
	}
	return reference
}

func siteName(s sitemanager.Site, h hostInfo) string {
	return DisplayName(s.Meta.Desc, s.Meta.Name, h.name, h.sites)
}

type hostInfo struct {
	name  string
	sites int
}

// index maps host IDs to their name and number of sites, and returns the
// sites with duplicates removed. The same site can be listed once per host
// that knows it (e.g. an old controller still registered after a
// migration); the first listing wins.
func index(hosts []sitemanager.Host, sites []sitemanager.Site) (map[string]hostInfo, []sitemanager.Site) {
	info := make(map[string]hostInfo, len(hosts))
	for _, h := range hosts {
		if !h.IsBlocked {
			info[h.ID] = hostInfo{name: h.Name()}
		}
	}
	seen := make(map[string]bool)
	var unique []sitemanager.Site
	for _, s := range sites {
		h, ok := info[s.HostID]
		if !ok || seen[s.SiteID] {
			continue
		}
		seen[s.SiteID] = true
		h.sites++
		info[s.HostID] = h
		unique = append(unique, s)
	}
	return info, unique
}

// Sites builds the site rows, sorted by name.
func Sites(hosts []sitemanager.Host, sites []sitemanager.Site) []Site {
	info, sites := index(hosts, sites)
	rows := make([]Site, 0, len(sites))
	for _, s := range sites {
		h := info[s.HostID]
		c := s.Statistics.Counts
		row := Site{
			Name:     siteName(s, h),
			SiteName: s.Meta.Desc,
			Host:     h.name,
			Devices:  c.TotalDevice,
			Offline:  c.OfflineDevice,
			Clients:  c.WifiClient + c.WiredClient + c.GuestClient,
			SiteID:   s.SiteID,
			HostID:   s.HostID,
		}
		if isp := s.Statistics.ISPInfo; isp != nil {
			row.ISP = isp.Name
		}
		rows = append(rows, row)
	}
	slices.SortStableFunc(rows, func(a, b Site) int {
		return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return rows
}

// Devices builds the device rows, sorted by site, host, and name.
//
// Site Manager doesn't say which site a device belongs to. For a host with
// a single site that's obvious; for a host with several, Site is left empty.
func Devices(hosts []sitemanager.Host, sites []sitemanager.Site, inventory []sitemanager.HostDevices) []Device {
	info, sites := index(hosts, sites)
	only := make(map[string]string) // host ID -> name of its only site
	for _, s := range sites {
		if h := info[s.HostID]; h.sites == 1 {
			only[s.HostID] = siteName(s, h)
		}
	}

	var rows []Device
	for _, hd := range inventory {
		h, ok := info[hd.HostID]
		if !ok {
			continue
		}
		for _, d := range hd.Devices {
			rows = append(rows, Device{
				Site:            only[hd.HostID],
				Host:            h.name,
				Name:            d.Name,
				Model:           d.Model,
				IP:              d.IP,
				MAC:             d.MAC,
				Status:          d.Status,
				Firmware:        d.Version,
				UpdateAvailable: d.FirmwareStatus == "updateAvailable",
			})
		}
	}
	slices.SortStableFunc(rows, func(a, b Device) int {
		return cmp.Or(
			cmp.Compare(strings.ToLower(a.Site), strings.ToLower(b.Site)),
			cmp.Compare(strings.ToLower(a.Host), strings.ToLower(b.Host)),
			cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
		)
	})
	return rows
}

// Match reports whether any value matches any glob pattern
// (case-insensitive). No patterns matches everything.
func Match(patterns []string, values ...string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		p = strings.ToLower(p)
		for _, v := range values {
			if v == "" {
				continue
			}
			if ok, _ := path.Match(p, strings.ToLower(v)); ok {
				return true
			}
		}
	}
	return false
}
