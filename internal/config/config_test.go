package config

import (
	"strings"
	"testing"
)

func TestParseDefaults(t *testing.T) {
	cfg, err := Parse([]byte(`
sources:
  - name: msp
    type: sitemanager
    api_key: k
  - name: lab
    type: local
    url: https://10.0.0.1/
    api_key: k
`))
	if err != nil {
		t.Fatal(err)
	}
	msp, lab := cfg.Sources[0], cfg.Sources[1]
	if msp.URL != DefaultSiteManagerURL || msp.MaxConcurrency != DefaultSiteManagerConcurrency {
		t.Errorf("msp defaults: %+v", msp)
	}
	if lab.URL != "https://10.0.0.1" || lab.APIPath != DefaultLocalAPIPath || lab.MaxConcurrency != DefaultLocalConcurrency {
		t.Errorf("lab defaults: %+v", lab)
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"no sources":     `sources: []`,
		"missing type":   "sources:\n  - name: a\n",
		"unknown type":   "sources:\n  - name: a\n    type: cloud\n",
		"duplicate":      "sources:\n  - {name: a, type: sitemanager}\n  - {name: a, type: sitemanager}\n",
		"local no url":   "sources:\n  - {name: a, type: local}\n",
		"local consoles": "sources:\n  - {name: a, type: local, url: x, consoles: [y]}\n",
		"sm insecure":    "sources:\n  - {name: a, type: sitemanager, insecure: true}\n",
		"unknown field":  "sources:\n  - {name: a, type: sitemanager, apikey: x}\n",
	}
	for name, in := range cases {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestKeyFromEnv(t *testing.T) {
	t.Setenv("UNIFICTL_TEST_KEY", "from-env")
	s := Source{Name: "a", APIKey: "inline", APIKeyEnv: "UNIFICTL_TEST_KEY"}
	if k, _ := s.Key(); k != "from-env" {
		t.Errorf("key = %q", k)
	}
	s.APIKeyEnv, s.APIKey = "UNIFICTL_TEST_UNSET", ""
	if _, err := s.Key(); err == nil || !strings.Contains(err.Error(), "UNIFICTL_TEST_UNSET") {
		t.Errorf("err = %v", err)
	}
}
