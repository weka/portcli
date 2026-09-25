package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/weka/portcli/internal/config"
)

// authServer counts authentications and serves a token that changes each time,
// so a test can tell a replay apart from a fresh request.
type authServer struct {
	auths    atomic.Int32
	requests atomic.Int32
	handler  func(w http.ResponseWriter, r *http.Request, token string)
}

func (a *authServer) start(t *testing.T) *Client {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/auth/access_token" {
			n := a.auths.Add(1)
			fmt.Fprintf(w, `{"accessToken":"token-%d"}`, n)
			return
		}
		a.requests.Add(1)
		a.handler(w, r, r.Header.Get("Authorization"))
	}))
	t.Cleanup(ts.Close)
	return New(&config.Config{BaseURL: ts.URL, ClientID: "id", ClientSecret: "secret"})
}

// A TUI fires many requests at once on every view switch. The token must be
// fetched once for the burst, not once per request, and the write must not
// race.
func TestConcurrentRequestsAuthenticateOnce(t *testing.T) {
	srv := &authServer{handler: func(w http.ResponseWriter, _ *http.Request, _ string) {
		_, _ = w.Write([]byte(`{"ok":true,"blueprints":[]}`))
	}}
	c := srv.start(t)

	const n = 20
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = c.ListBlueprints(context.Background())
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
	}
	if got := srv.auths.Load(); got != 1 {
		t.Errorf("authenticated %d times, want 1 — the lock must span the auth round-trip", got)
	}
	if got := srv.requests.Load(); got != n {
		t.Errorf("served %d requests, want %d", got, n)
	}
}

// Port tokens expire with no refresh endpoint and no warning. A one-shot CLI
// run never notices; a long session does, and without a retry every later
// request fails forever.
func TestExpiredTokenIsReplacedAndRequestReplayed(t *testing.T) {
	srv := &authServer{handler: func(w http.ResponseWriter, _ *http.Request, token string) {
		if token == "Bearer token-1" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"expired"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"blueprints":[{"identifier":"bp"}]}`))
	}}
	c := srv.start(t)

	bps, err := c.ListBlueprints(context.Background())
	if err != nil {
		t.Fatalf("a single 401 should be retried, got %v", err)
	}
	if len(bps) != 1 || bps[0].Identifier != "bp" {
		t.Errorf("got %v, want the replayed result", bps)
	}
	if got := srv.auths.Load(); got != 2 {
		t.Errorf("authenticated %d times, want 2 (initial + refresh)", got)
	}
	if got := srv.requests.Load(); got != 2 {
		t.Errorf("made %d API requests, want 2 (original + replay)", got)
	}
}

// Retrying forever would turn wrong credentials into a hang, and would hide
// the real error behind a second identical one.
func TestPersistent401IsReportedNotRetriedForever(t *testing.T) {
	srv := &authServer{handler: func(w http.ResponseWriter, _ *http.Request, _ string) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"nope"}`))
	}}
	c := srv.start(t)

	_, err := c.ListBlueprints(context.Background())
	if err == nil {
		t.Fatal("expected an error for persistently rejected credentials")
	}
	if !IsUnauthorized(err) {
		t.Errorf("error should still read as a 401: %v", err)
	}
	if got := srv.requests.Load(); got != 2 {
		t.Errorf("made %d attempts, want exactly 2", got)
	}
}

// The replayed request must carry the body again. Reusing the original
// io.Reader sends an empty payload the second time, which Port accepts — so a
// PATCH would silently write nothing.
func TestReplayResendsTheRequestBody(t *testing.T) {
	var bodies []string
	var mu sync.Mutex
	srv := &authServer{handler: func(w http.ResponseWriter, r *http.Request, token string) {
		data, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(data))
		mu.Unlock()
		if token == "Bearer token-1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}}
	c := srv.start(t)

	if err := c.UpdateEntityProperties(context.Background(), "bp", "e1", map[string]any{"status": "OK"}); err != nil {
		t.Fatalf("UpdateEntityProperties: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("saw %d requests, want 2", len(bodies))
	}
	if bodies[0] != bodies[1] {
		t.Errorf("replay body differs:\n first %q\nsecond %q", bodies[0], bodies[1])
	}
	if bodies[1] == "" {
		t.Error("replay sent an empty body — the reader was not rebuilt")
	}
}

func TestContextCancellationAbortsInFlightRequest(t *testing.T) {
	release := make(chan struct{})
	srv := &authServer{handler: func(w http.ResponseWriter, _ *http.Request, _ string) {
		<-release
		_, _ = w.Write([]byte(`{"ok":true,"blueprints":[]}`))
	}}
	c := srv.start(t)
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := c.ListBlueprints(ctx)
	if err == nil {
		t.Fatal("expected cancellation to surface as an error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	// The 30s transport timeout must not be what ends this.
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("took %s; cancellation should be immediate", elapsed)
	}
}

// PollEntity used to time.Sleep between attempts, so a cancellation only took
// effect one interval later.
func TestPollEntityStopsOnCancellation(t *testing.T) {
	srv := &authServer{handler: func(w http.ResponseWriter, _ *http.Request, _ string) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}}
	c := srv.start(t)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := c.PollEntity(ctx, "bp", "e1", time.Hour, 10*time.Second,
		func(_ *Entity, fetchErr error) (bool, error) { return fetchErr == nil, nil })

	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	// With time.Sleep this would have taken the full 10s interval.
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("took %s; the wait must be cancellable, not a bare sleep", elapsed)
	}
}

func TestSearchFilterTranslatesTopLevelFields(t *testing.T) {
	// Port addresses an entity's top-level fields with a "$" prefix and treats
	// an unprefixed name as an ordinary property, which quietly matches
	// nothing — so this translation is the difference between a filter working
	// and silently returning zero rows.
	for _, tc := range []struct {
		field, want string
	}{
		{"identifier", "$identifier"},
		{"title", "$title"},
		{"createdAt", "$createdAt"},
		{"updatedBy", "$updatedBy"},
		{"$identifier", "$identifier"}, // already prefixed, left alone
		{"status", "status"},           // an ordinary property
		{"owner", "owner"},
	} {
		t.Run(tc.field, func(t *testing.T) {
			if got := searchProperty(tc.field); got != tc.want {
				t.Errorf("searchProperty(%q) = %q, want %q", tc.field, got, tc.want)
			}
		})
	}
}
