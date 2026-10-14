package fleet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ginkio/unifictl/internal/config"
	"github.com/ginkio/unifictl/internal/unifi"
)

const testKey = "secret"

// fakeAPI serves a Site Manager account with two consoles (one reachable via
// the connector, one offline) plus a local console under /local.
func fakeAPI(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-API-KEY") != testKey {
				w.WriteHeader(http.StatusUnauthorized)
				writeJSON(w, map[string]string{"code": "unauthorized", "message": "bad key"})
				return
			}
			h(w, r)
		}
	}

	// Site Manager host listing, paginated across two pages.
	mux.HandleFunc("GET /v1/hosts", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("nextToken") == "" {
			writeJSON(w, map[string]any{
				"data":      []any{map[string]any{"id": "host-a", "reportedState": map[string]any{"name": "Customer A", "version": "4.3.6"}}},
				"nextToken": "p2",
			})
			return
		}
		writeJSON(w, map[string]any{"data": []any{
			map[string]any{"id": "host-b", "reportedState": map[string]any{"name": "Customer B"}},
			map[string]any{"id": "host-blocked", "isBlocked": true},
		}})
	}))

	// Customer A via connector: two sites; site-2 has 250 devices (pagination).
	mux.HandleFunc("GET /v1/connector/consoles/host-a/proxy/network/integration/v1/sites", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"totalCount": 2, "data": []any{
			map[string]any{"id": "site-1", "internalReference": "default", "name": "Main Office"},
			map[string]any{"id": "site-2", "internalReference": "wh", "name": "Warehouse"},
		}})
	}))
	mux.HandleFunc("GET /v1/connector/consoles/host-a/proxy/network/integration/v1/sites/{site}/devices", auth(func(w http.ResponseWriter, r *http.Request) {
		total := 1
		if r.PathValue("site") == "site-2" {
			total = 250
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		var data []any
		for i := offset; i < min(offset+limit, total); i++ {
			data = append(data, map[string]any{"id": strconv.Itoa(i), "name": "dev", "state": "ONLINE"})
		}
		writeJSON(w, map[string]any{"offset": offset, "count": len(data), "totalCount": total, "data": data})
	}))

	// Customer B is offline: connector returns 502.
	mux.HandleFunc("/v1/connector/consoles/host-b/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		writeJSON(w, map[string]any{"code": "console_offline", "message": "console is offline"})
	})

	// Local console.
	mux.HandleFunc("GET /local/proxy/network/integration/v1/sites", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"totalCount": 1, "data": []any{map[string]any{"id": "lab", "internalReference": "default", "name": "Lab"}}})
	}))

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testSources(t *testing.T, base string) []config.Source {
	t.Helper()
	cfg, err := config.Parse([]byte(`
sources:
  - name: msp
    type: sitemanager
    url: ` + base + `
    api_key: ` + testKey + `
  - name: lab
    type: local
    url: ` + base + `/local
    api_key: ` + testKey + `
`))
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Sources
}

func TestEndToEnd(t *testing.T) {
	srv := fakeAPI(t)
	ctx := context.Background()

	consoles, errs := Resolve(ctx, testSources(t, srv.URL), Filter{}, 5*time.Second)
	if len(errs) != 0 {
		t.Fatalf("resolve errors: %v", errs)
	}
	var names []string
	for _, c := range consoles {
		names = append(names, c.Source+"/"+c.Name)
	}
	if got, want := strings.Join(names, ","), "msp/Customer A,msp/Customer B,lab/lab"; got != want {
		t.Fatalf("consoles = %s, want %s", got, want)
	}
	if consoles[0].Version != "4.3.6" {
		t.Errorf("version = %q", consoles[0].Version)
	}

	sites, errs := Sites(ctx, consoles, Filter{}, 4)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "msp/Customer B") || !strings.Contains(errs[0].Error(), "console is offline") {
		t.Fatalf("expected one offline error for Customer B, got %v", errs)
	}
	if len(sites) != 3 {
		t.Fatalf("got %d sites, want 3", len(sites))
	}

	rows, errs := PerSite(ctx, sites[:2], 4, func(ctx context.Context, s SiteRef) ([]unifi.Device, error) {
		return s.Console.Client.Devices(ctx, s.Site.ID)
	})
	if len(errs) != 0 {
		t.Fatalf("devices errors: %v", errs)
	}
	if len(rows) != 251 {
		t.Fatalf("got %d devices, want 251", len(rows))
	}
	if r := rows[len(rows)-1]; r.Source != "msp" || r.Console != "Customer A" || r.Site != "Warehouse" {
		t.Errorf("last row tagged %+v", r)
	}
}

func TestFilters(t *testing.T) {
	srv := fakeAPI(t)
	ctx := context.Background()
	f := Filter{Consoles: []string{"customer a"}, Sites: []string{"wh"}}

	consoles, errs := Resolve(ctx, testSources(t, srv.URL), f, 5*time.Second)
	if len(errs) != 0 || len(consoles) != 1 || consoles[0].Name != "Customer A" {
		t.Fatalf("consoles = %+v, errs = %v", consoles, errs)
	}
	sites, errs := Sites(ctx, consoles, f, 4)
	if len(errs) != 0 || len(sites) != 1 || sites[0].Site.Name != "Warehouse" {
		t.Fatalf("sites = %+v, errs = %v", sites, errs)
	}
}

func TestBadKey(t *testing.T) {
	srv := fakeAPI(t)
	srcs := testSources(t, srv.URL)[:1]
	srcs[0].APIKey = "wrong"

	_, errs := Resolve(context.Background(), srcs, Filter{}, 5*time.Second)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "401") {
		t.Fatalf("errs = %v", errs)
	}
}

func TestMatch(t *testing.T) {
	cases := []struct {
		patterns []string
		values   []string
		want     bool
	}{
		{nil, []string{"x"}, true},
		{[]string{"acme*"}, []string{"ACME Corp"}, true},
		{[]string{"acme*"}, []string{"Other", "acme-id"}, true},
		{[]string{"acme"}, []string{"ACME Corp"}, false},
		{[]string{"x"}, []string{""}, false},
	}
	for _, c := range cases {
		if got := Match(c.patterns, c.values...); got != c.want {
			t.Errorf("Match(%v, %v) = %v, want %v", c.patterns, c.values, got, c.want)
		}
	}
}
