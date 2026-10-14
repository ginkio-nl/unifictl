// Package fleet resolves configured sources into consoles and runs queries
// concurrently across consoles and sites.
package fleet

import (
	"context"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/ginkio-nl/unifictl/internal/config"
	"github.com/ginkio-nl/unifictl/internal/httpx"
	"github.com/ginkio-nl/unifictl/internal/sitemanager"
	"github.com/ginkio-nl/unifictl/internal/unifi"
)

// Console is a reachable UniFi Network application.
type Console struct {
	Source  string        `json:"source"`
	Name    string        `json:"name"`
	ID      string        `json:"id"`
	Version string        `json:"version,omitempty"` // as reported by Site Manager
	Client  *unifi.Client `json:"-"`
}

// SiteRef is a site together with the console it lives on.
type SiteRef struct {
	Console Console
	Site    unifi.Site
}

// Row is a single result tagged with where it came from.
type Row[T any] struct {
	Source  string `json:"source"`
	Console string `json:"console"`
	Site    string `json:"site"`
	Item    T      `json:"item"`
}

// Error records a failure against a source, console, or site.
type Error struct {
	Where string
	Err   error
}

func (e Error) Error() string { return fmt.Sprintf("%s: %v", e.Where, e.Err) }
func (e Error) Unwrap() error { return e.Err }

// Filter narrows which consoles and sites are queried. Patterns are
// case-insensitive globs; empty means everything.
type Filter struct {
	Consoles []string
	Sites    []string
}

// Resolve turns sources into consoles. Site Manager sources are expanded by
// listing the account's hosts.
func Resolve(ctx context.Context, srcs []config.Source, f Filter, timeout time.Duration) ([]Console, []error) {
	lists, errs := Each(ctx, srcs, len(srcs), func(ctx context.Context, s config.Source) ([]Console, error) {
		return resolveSource(ctx, s, f, timeout)
	})

	var out []Console
	var failures []error
	for i, s := range srcs {
		if errs[i] != nil {
			failures = append(failures, Error{Where: s.Name, Err: errs[i]})
			continue
		}
		out = append(out, lists[i]...)
	}
	return out, failures
}

func resolveSource(ctx context.Context, s config.Source, f Filter, timeout time.Duration) ([]Console, error) {
	key, err := s.Key()
	if err != nil {
		return nil, err
	}
	req := httpx.New(httpx.Options{
		APIKey:   key,
		Insecure: s.Insecure,
		Timeout:  timeout,
		Limiter:  httpx.NewLimiter(s.MaxConcurrency),
	})

	switch s.Type {
	case config.TypeLocal:
		if !Match(f.Consoles, s.Name, s.URL) {
			return nil, nil
		}
		return []Console{{
			Source: s.Name,
			Name:   s.Name,
			ID:     s.URL,
			Client: unifi.New(s.URL+s.APIPath, req),
		}}, nil

	case config.TypeSiteManager:
		sm := sitemanager.New(s.URL, req)
		hosts, err := sm.Hosts(ctx)
		if err != nil {
			return nil, fmt.Errorf("list hosts: %w", err)
		}
		var out []Console
		for _, h := range hosts {
			name := h.Name()
			if h.IsBlocked || !Match(s.Consoles, name, h.ID) || !Match(f.Consoles, name, h.ID) {
				continue
			}
			c := Console{
				Source: s.Name,
				Name:   name,
				ID:     h.ID,
				Client: unifi.New(sm.NetworkBase(h.ID), req),
			}
			if h.ReportedState != nil {
				c.Version = h.ReportedState.Version
			}
			out = append(out, c)
		}
		return out, nil
	}
	return nil, fmt.Errorf("unknown source type %q", s.Type)
}

// Sites lists sites on every console, keeping those that match f.Sites.
func Sites(ctx context.Context, consoles []Console, f Filter, limit int) ([]SiteRef, []error) {
	lists, errs := Each(ctx, consoles, limit, func(ctx context.Context, c Console) ([]unifi.Site, error) {
		return c.Client.Sites(ctx)
	})

	var refs []SiteRef
	var failures []error
	for i, c := range consoles {
		if errs[i] != nil {
			failures = append(failures, Error{Where: c.Source + "/" + c.Name, Err: errs[i]})
			continue
		}
		for _, s := range lists[i] {
			if Match(f.Sites, s.Name, s.InternalReference, s.ID) {
				refs = append(refs, SiteRef{Console: c, Site: s})
			}
		}
	}
	return refs, failures
}

// PerSite runs fn against every site and flattens the results into rows.
func PerSite[T any](ctx context.Context, sites []SiteRef, limit int, fn func(context.Context, SiteRef) ([]T, error)) ([]Row[T], []error) {
	lists, errs := Each(ctx, sites, limit, fn)

	var rows []Row[T]
	var failures []error
	for i, s := range sites {
		label := SiteLabel(s.Site)
		if errs[i] != nil {
			failures = append(failures, Error{Where: s.Console.Source + "/" + s.Console.Name + "/" + label, Err: errs[i]})
			continue
		}
		for _, item := range lists[i] {
			rows = append(rows, Row[T]{Source: s.Console.Source, Console: s.Console.Name, Site: label, Item: item})
		}
	}
	return rows, failures
}

// Each runs fn for every input with at most limit concurrent calls,
// preserving input order in the results.
func Each[In, Out any](ctx context.Context, inputs []In, limit int, fn func(context.Context, In) (Out, error)) ([]Out, []error) {
	limit = max(limit, 1)
	outs := make([]Out, len(inputs))
	errs := make([]error, len(inputs))
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i, in := range inputs {
		wg.Go(func() {
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				errs[i] = ctx.Err()
				return
			}
			outs[i], errs[i] = fn(ctx, in)
		})
	}
	wg.Wait()
	return outs, errs
}

// SiteLabel returns a human-friendly site identifier.
func SiteLabel(s unifi.Site) string {
	for _, v := range []string{s.Name, s.InternalReference, s.ID} {
		if v != "" {
			return v
		}
	}
	return "?"
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
