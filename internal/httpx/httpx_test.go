package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetriesOn429(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	var out struct{ OK bool }
	r := New(Options{APIKey: "k", Timeout: time.Second, Limiter: NewLimiter(1)})
	if err := r.GetJSON(context.Background(), srv.URL, &out); err != nil {
		t.Fatal(err)
	}
	if !out.OK || calls.Load() != 3 {
		t.Fatalf("ok=%v calls=%d", out.OK, calls.Load())
	}
}

func TestGivesUpAfterMaxRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"code":"rate_limit","message":"slow down"}`))
	}))
	defer srv.Close()

	err := New(Options{APIKey: "k", Timeout: time.Second}).GetJSON(context.Background(), srv.URL, &struct{}{})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 429 || apiErr.Message != "slow down" {
		t.Fatalf("err = %v", err)
	}
}
