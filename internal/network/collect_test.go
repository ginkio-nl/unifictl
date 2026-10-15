package network

import (
	"slices"
	"testing"
)

func TestOverlaps(t *testing.T) {
	rows := []Row{
		{Site: "A", Network: "Default", Subnet: "192.168.1.0/24"},
		{Site: "B", Network: "Default", Subnet: "192.168.1.0/24"},
		{Site: "C", Network: "Big", Subnet: "192.168.0.0/16"}, // contains A and B
		{Site: "A", Network: "Staff", Subnet: "10.1.10.0/24"},
		{Site: "A", Network: "Mgmt", Subnet: "10.1.10.0/25"}, // same site: not reported
		{Site: "D", Network: "Unique", Subnet: "10.4.0.0/24"},
		{Site: "E", Network: "Third-party", Subnet: ""},
	}
	got := Overlaps(rows)
	want := [][]string{
		{"B", "C"},
		{"A", "C"},
		{"A", "B"},
		nil,
		nil,
		nil,
		nil,
	}
	for i := range want {
		if !slices.Equal(got[i], want[i]) {
			t.Errorf("%s/%s: got %v, want %v", rows[i].Site, rows[i].Network, got[i], want[i])
		}
	}
}

func TestWithVPNs(t *testing.T) {
	rows := []Row{
		{Site: "B", Network: "Default", VLAN: 1, Subnet: "192.168.2.0/24"},
		{Site: "A", Network: "Staff", VLAN: 10, Subnet: "10.1.10.0/24"},
	}
	vpns := []VPNRow{
		{Site: "A", Kind: "server", Name: "WG", Subnet: "192.168.2.0/24", Enabled: true},
		{Site: "A", Kind: "site-to-site", Name: "To-B", RemoteSubnets: []string{"192.168.2.0/24"}},
		{Site: "C", Kind: "server", Name: "NoSubnet"},
	}
	got := WithVPNs(rows, vpns)

	var names []string
	for _, r := range got {
		names = append(names, r.Site+"/"+r.Network)
	}
	if want := []string{"A/Staff", "A/WG", "B/Default"}; !slices.Equal(names, want) {
		t.Fatalf("rows = %v, want %v", names, want)
	}
	if !got[1].VPN || got[0].VPN {
		t.Errorf("VPN flags: %+v", got)
	}
	// The VPN server clashes with B's network; the tunnel doesn't count.
	ov := Overlaps(got)
	if !slices.Equal(ov[1], []string{"B"}) || !slices.Equal(ov[2], []string{"A"}) || ov[0] != nil {
		t.Errorf("overlaps = %v", ov)
	}
}
