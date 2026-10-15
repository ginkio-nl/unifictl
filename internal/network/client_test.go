package network

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ginkio-nl/unifictl/internal/httpx"
)

func TestVPNSubnets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/s/abc123/rest/networkconf" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"meta": {"rc": "ok"}, "data": [
			{"name": "Staff", "purpose": "corporate", "ip_subnet": "10.1.10.1/24"},
			{"name": "WG", "purpose": "remote-user-vpn", "vpn_type": "wireguard-server", "ip_subnet": "10.1.81.1/24",
			 "x_wireguard_private_key": "SECRET-WG"},
			{"name": "To-B", "purpose": "site-vpn", "remote_vpn_subnets": ["172.16.84.0/24", "172.16.85.0/24"],
			 "x_ipsec_pre_shared_key": "SECRET-PSK"}
		]}`)
	}))
	defer srv.Close()

	c := New(srv.URL, httpx.New(httpx.Options{APIKey: "k", Timeout: 5 * time.Second}))
	got, err := c.VPNSubnets(context.Background(), "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Servers) != 1 || got.Servers["WG"] != "10.1.81.1/24" {
		t.Errorf("servers = %v", got.Servers)
	}
	if r := got.Tunnels["To-B"]; len(got.Tunnels) != 1 || len(r) != 2 || r[0] != "172.16.84.0/24" {
		t.Errorf("tunnels = %v", got.Tunnels)
	}
	if s := fmt.Sprintf("%+v", got); strings.Contains(s, "SECRET") {
		t.Errorf("secrets leaked into result: %s", s)
	}
	if got := maskedPrefix(got.Servers["WG"]); got != "10.1.81.0/24" {
		t.Errorf("maskedPrefix = %s", got)
	}
}
