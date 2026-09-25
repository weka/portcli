package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/config"
)

// fakeEntities serves a blueprint with predictable entity identifiers.
func fakeEntities(t *testing.T, n int) (*client.Client, *config.Config) {
	t.Helper()
	var rows []string
	for i := range n {
		rows = append(rows, fmt.Sprintf(`{"identifier":"e%02d","title":"entity %d","properties":{"status":"OK"}}`, i, i))
	}
	body := `{"ok":true,"entities":[` + strings.Join(rows, ",") + `]}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/auth/access_token":
			_, _ = w.Write([]byte(`{"accessToken":"t"}`))
		case strings.HasSuffix(r.URL.Path, "/entities"):
			_, _ = w.Write([]byte(body))
		case strings.HasPrefix(r.URL.Path, "/v1/blueprints/"):
			_, _ = w.Write([]byte(`{"ok":true,"blueprint":{"identifier":"bp","schema":{"properties":{"status":{"type":"string"}}}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"ok":false}`))
		}
	}))
	t.Cleanup(ts.Close)
	cfg := &config.Config{BaseURL: ts.URL, ClientID: "id", ClientSecret: "s"}
	return client.New(cfg), cfg
}

// A confirmation that names a different entity than the highlighted row would
// be the worst possible bug in a delete path, so the row the cursor is on and
// the row an operation receives must be provably the same one.
func TestConfirmationNamesTheHighlightedRow(t *testing.T) {
	c, cfg := fakeEntities(t, 30)

	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(120, 24)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	a := newApp(ctx, c, Options{
		Version: "test", Config: cfg, View: "entities bp", Refresh: time.Hour,
	})
	a.app.SetScreen(sim)
	go a.app.Run()
	defer a.app.Stop()

	snap := func() string {
		ch := make(chan string, 1)
		a.app.QueueUpdate(func() { ch <- screenText(sim) })
		select {
		case s := <-ch:
			return s
		case <-time.After(5 * time.Second):
			return ""
		}
	}
	waitFor(t, snap, "e00")

	// The row an operation would receive, read on the UI goroutine.
	selected := func() string {
		ch := make(chan string, 1)
		a.app.QueueUpdate(func() {
			tv := a.top().(*tableView)
			rows := tv.selection()
			if len(rows) == 0 {
				ch <- ""
				return
			}
			ch <- rows[0].ID
		})
		return <-ch
	}

	if got := selected(); got != "e00" {
		t.Fatalf("initial selection = %q, want the first row", got)
	}

	// Move down three times; j is left to tview.Table, so this also checks
	// that our key handling has not swallowed it.
	for range 3 {
		sim.InjectKey(tcell.KeyRune, 'j', tcell.ModNone)
		time.Sleep(50 * time.Millisecond)
	}
	want := "e03"
	if got := selected(); got != want {
		t.Fatalf("after three j presses selection = %q, want %q", got, want)
	}

	// ctrl-d must confirm, and the confirmation must name that exact row.
	sim.InjectKey(tcell.KeyCtrlD, 0, tcell.ModNone)
	waitFor(t, snap, "cannot be undone")
	screen := snap()
	if !strings.Contains(screen, "Delete "+want+"?") {
		t.Errorf("confirmation does not name the highlighted row %q:\n%s", want, screen)
	}

	// Escape must cancel without deleting.
	sim.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitFor(t, snap, "cancelled")
}

// Marking rows makes an operation apply to the marks rather than the cursor,
// and the confirmation has to say so.
func TestMarkedRowsDriveTheOperation(t *testing.T) {
	c, cfg := fakeEntities(t, 10)

	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(120, 24)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	a := newApp(ctx, c, Options{Version: "test", Config: cfg, View: "entities bp", Refresh: time.Hour})
	a.app.SetScreen(sim)
	go a.app.Run()
	defer a.app.Stop()

	snap := func() string {
		ch := make(chan string, 1)
		a.app.QueueUpdate(func() { ch <- screenText(sim) })
		select {
		case s := <-ch:
			return s
		case <-time.After(5 * time.Second):
			return ""
		}
	}
	waitFor(t, snap, "e00")

	// Mark the first row, move down, mark the second.
	sim.InjectKey(tcell.KeyRune, ' ', tcell.ModNone)
	time.Sleep(50 * time.Millisecond)
	sim.InjectKey(tcell.KeyRune, 'j', tcell.ModNone)
	time.Sleep(50 * time.Millisecond)
	sim.InjectKey(tcell.KeyRune, ' ', tcell.ModNone)
	waitFor(t, snap, "2 marked")

	sim.InjectKey(tcell.KeyCtrlD, 0, tcell.ModNone)
	waitFor(t, snap, "cannot be undone")
	screen := snap()
	for _, want := range []string{"Delete 2 entities?", "e00", "e01"} {
		if !strings.Contains(screen, want) {
			t.Errorf("confirmation missing %q:\n%s", want, screen)
		}
	}

	sim.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitFor(t, snap, "cancelled")
}
