package report

import (
	"encoding/json"
	"testing"

	"github.com/ginkio-nl/unifictl/internal/sitemanager"
)

// Shaped like a real account: a hosted server with several sites (one of
// them its own empty "Default"), and a customer's console with one site
// called "Default".
const (
	hostsJSON = `[
		{"id": "srv", "reportedState": {"name": "hosted.example.com"}},
		{"id": "udm", "reportedState": {"name": "Acme B.V."}},
		{"id": "gone", "isBlocked": true, "reportedState": {"name": "Blocked"}}
	]`
	sitesJSON = `[
		{"siteId": "s1", "hostId": "srv", "meta": {"desc": "Zeta Corp", "name": "abc"},
		 "statistics": {"counts": {"totalDevice": 5, "offlineDevice": 1, "wifiClient": 3, "wiredClient": 2, "guestClient": 1}, "ispInfo": {"name": "KPN"}}},
		{"siteId": "s0", "hostId": "srv", "meta": {"desc": "Default", "name": "default"}, "statistics": {"counts": {}, "ispInfo": null}},
		{"siteId": "s1", "hostId": "srv", "meta": {"desc": "Zeta Corp (duplicate)"}},
		{"siteId": "a1", "hostId": "udm", "meta": {"desc": "Default", "name": "default"},
		 "statistics": {"counts": {"totalDevice": 2, "wiredClient": 4}, "ispInfo": {"name": "Ziggo"}}},
		{"siteId": "x1", "hostId": "gone", "meta": {"desc": "Hidden"}}
	]`
	devicesJSON = `[
		{"hostId": "srv", "devices": [
			{"name": "ap-2", "model": "U7 Pro", "mac": "m2", "status": "offline", "version": "8.0", "firmwareStatus": "upToDate"},
			{"name": "ap-1", "model": "U6 LR", "mac": "m1", "status": "online", "version": "7.9", "firmwareStatus": "updateAvailable"}
		]},
		{"hostId": "udm", "devices": [
			{"name": "udm", "model": "UDM Pro", "mac": "m3", "ip": "10.0.0.1", "status": "online", "firmwareStatus": "upToDate", "isConsole": true}
		]},
		{"hostId": "gone", "devices": [{"name": "hidden"}]}
	]`
)

func load(t *testing.T) ([]sitemanager.Host, []sitemanager.Site, []sitemanager.HostDevices) {
	t.Helper()
	var h []sitemanager.Host
	var s []sitemanager.Site
	var d []sitemanager.HostDevices
	for src, dst := range map[string]any{hostsJSON: &h, sitesJSON: &s, devicesJSON: &d} {
		if err := json.Unmarshal([]byte(src), dst); err != nil {
			t.Fatal(err)
		}
	}
	return h, s, d
}

func TestSites(t *testing.T) {
	hosts, sites, _ := load(t)
	got := Sites(hosts, sites)

	want := []Site{
		// Single-site console: shown by the console's name, like ui.com.
		{Name: "Acme B.V.", SiteName: "Default", Host: "Acme B.V.", ISP: "Ziggo", Devices: 2, Clients: 4, SiteID: "a1", HostID: "udm"},
		// The hosted server's own Default site keeps its name.
		{Name: "Default", SiteName: "Default", Host: "hosted.example.com", SiteID: "s0", HostID: "srv"},
		// Duplicate s1 listed once; blocked host left out.
		{Name: "Zeta Corp", SiteName: "Zeta Corp", Host: "hosted.example.com", ISP: "KPN", Devices: 5, Offline: 1, Clients: 6, SiteID: "s1", HostID: "srv"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d sites, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("site %d:\n got %+v\nwant %+v", i, got[i], want[i])
		}
	}
}

func TestDevices(t *testing.T) {
	hosts, sites, inv := load(t)
	got := Devices(hosts, sites, inv)

	type row struct{ site, name, status string }
	want := []row{
		// Multi-site host: site unknown (sorted first, empty site).
		{"", "ap-1", "online"},
		{"", "ap-2", "offline"},
		// Single-site host: attributed to its site.
		{"Acme B.V.", "udm", "online"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d devices, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if g := (row{got[i].Site, got[i].Name, got[i].Status}); g != w {
			t.Errorf("device %d = %+v, want %+v", i, g, w)
		}
	}
	if !got[0].UpdateAvailable || got[1].UpdateAvailable {
		t.Errorf("update flags: ap-1=%v ap-2=%v", got[0].UpdateAvailable, got[1].UpdateAvailable)
	}
}

func TestMatch(t *testing.T) {
	cases := []struct {
		patterns []string
		values   []string
		want     bool
	}{
		{nil, []string{"x"}, true},
		{[]string{"keesde*"}, []string{"KeesdeBoekhouder010"}, true},
		{[]string{"acme"}, []string{"Acme B.V."}, false},
		{[]string{"x"}, []string{""}, false},
	}
	for _, c := range cases {
		if got := Match(c.patterns, c.values...); got != c.want {
			t.Errorf("Match(%v, %v) = %v, want %v", c.patterns, c.values, got, c.want)
		}
	}
}
