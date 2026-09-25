package tui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/config"
)

// fakePort serves just enough of the API for the shell to come up.
func fakePort(t *testing.T) (*client.Client, *config.Config) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/auth/access_token":
			_, _ = w.Write([]byte(`{"accessToken":"t"}`))
		case r.URL.Path == "/v1/blueprints":
			_, _ = w.Write([]byte(`{"ok":true,"blueprints":[
				{"identifier":"deployment","title":"Deployment","description":"a deploy","updatedAt":"2026-09-25T04:44:40.647Z"},
				{"identifier":"service","title":"Service","description":"a service","updatedAt":"2026-09-24T01:02:03.000Z"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"ok":false}`))
		}
	}))
	t.Cleanup(ts.Close)
	cfg := &config.Config{BaseURL: ts.URL, ClientID: "abcd1234efgh", ClientSecret: "shh"}
	return client.New(cfg), cfg
}

// screenText flattens the simulation screen into lines of plain text.
func screenText(sim tcell.SimulationScreen) string {
	cells, w, h := sim.GetContents()
	var b strings.Builder
	for y := range h {
		for x := range w {
			runes := cells[y*w+x].Runes
			if len(runes) == 0 || runes[0] == 0 {
				b.WriteRune(' ')
				continue
			}
			b.WriteRune(runes[0])
		}
		b.WriteRune('\n')
	}
	return b.String()
}

// One end-to-end pass over the real widget tree. Unit tests cannot catch a nil
// primitive, a page never added, or a key that reaches the wrong handler —
// this can. Deliberately the only test of its kind: assertions about drawn
// frames are brittle, so it checks that the app runs and shows its table, not
// how the pixels landed.
func TestShellStartsRendersAndQuits(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // never write the real ~/.portcli/tui.json
	c, cfg := fakePort(t)

	sim := tcell.NewSimulationScreen("UTF-8")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	a := newApp(ctx, c, Options{
		Version: "test",
		Config:  cfg,
		View:    "blueprints", // do not depend on the developer's saved state
		Refresh: time.Hour,    // one fetch; no background churn mid-assertion
	})
	a.app.SetScreen(sim)
	// After SetScreen, not before: SetScreen calls Init, which resets the size.
	sim.SetSize(150, 40)

	done := make(chan error, 1)
	go func() { done <- a.app.Run() }()

	// snapshot reads the screen from the UI goroutine. tcell's simulation
	// screen does not lock GetContents against its own drawing, so reading it
	// directly from the test goroutine races with every frame.
	snapshot := func() string {
		ch := make(chan string, 1)
		a.app.QueueUpdate(func() { ch <- screenText(sim) })
		select {
		case s := <-ch:
			return s
		case <-time.After(5 * time.Second):
			return ""
		}
	}

	waitFor(t, snapshot, "deployment")

	got := snapshot()
	for _, want := range []string{
		"IDENTIFIER", // the table header rendered
		"deployment", // rows arrived from the fake API
		"service",
		"Base URL",   // the context panel rendered
		"blueprints", // breadcrumb
		"abcd…efgh",  // client id is masked
	} {
		if !strings.Contains(got, want) {
			t.Errorf("screen is missing %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "shh") {
		t.Error("the client secret must never be rendered")
	}

	// "?" opens help, Escape closes it, "q" quits: the three paths that prove
	// key dispatch reaches overlays and back.
	// "Navigate" rather than a later section: the help table is taller than
	// the viewport, so anything further down is scrolled off and absent from
	// the rendered frame.
	sim.InjectKey(tcell.KeyRune, '?', tcell.ModNone)
	waitFor(t, snapshot, "Navigate")

	sim.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitFor(t, snapshot, "IDENTIFIER")

	sim.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("q did not quit the app")
	}
}

// waitFor polls a screen snapshot until it contains want.
func waitFor(t *testing.T, snapshot func() string, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		last = snapshot()
		if strings.Contains(last, want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q on screen:\n%s", want, last)
}
