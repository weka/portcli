package tui

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// countingScreen records how often the screen is initialised and finalised.
type countingScreen struct {
	tcell.SimulationScreen
	inits atomic.Int32
	finis atomic.Int32
}

func (c *countingScreen) Init() error {
	c.inits.Add(1)
	return c.SimulationScreen.Init()
}

func (c *countingScreen) Fini() {
	c.finis.Add(1)
	c.SimulationScreen.Fini()
}

// The screen must be initialised exactly once and finalised exactly once.
//
// This is not a style preference. tview's SetScreen calls Init and its Stop
// calls Fini; doing either a second time leaves a real terminal rendering
// almost nothing and delivering no key events at all, Ctrl-C included, which
// looks precisely like a hung process. A simulation screen tolerates the
// double call, so no rendering assertion can catch this — only counting can.
func TestScreenIsInitialisedAndFinalisedExactlyOnce(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // keep the real state file out of it

	c, cfg := fakePort(t)
	screen := &countingScreen{SimulationScreen: tcell.NewSimulationScreen("UTF-8")}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- run(ctx, c, Options{
			Version: "test", Config: cfg, View: "blueprints", Refresh: time.Hour,
		}, screen)
	}()

	// Wait for the first paint, which proves Init happened.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if screen.inits.Load() > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := screen.inits.Load(); got != 1 {
		t.Fatalf("screen initialised %d times, want exactly 1 — a second Init kills input on a real terminal", got)
	}

	screen.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("q did not quit — the key never reached the app")
	}

	if got := screen.inits.Load(); got != 1 {
		t.Errorf("screen initialised %d times, want exactly 1", got)
	}
	if got := screen.finis.Load(); got != 1 {
		t.Errorf("screen finalised %d times, want exactly 1; a stray Fini leaves the terminal unrestored", got)
	}
}

// Run's terminal probe must reject a TERM it cannot drive, with a sentence
// rather than a blank screen.
func TestRunRejectsAnUnusableTerminal(t *testing.T) {
	t.Setenv("TERM", "definitely-not-a-real-terminal")

	c, cfg := fakePort(t)
	err := Run(context.Background(), c, Options{Version: "test", Config: cfg})
	if err == nil {
		t.Fatal("expected an error for an unknown TERM")
	}
	if got := err.Error(); !strings.Contains(got, "definitely-not-a-real-terminal") {
		t.Errorf("error should name the TERM that failed, got %q", got)
	}
	t.Logf("reported: %v", err)
}

// A view must not be added to the page set twice, which would leave a stale
// primitive behind when the same view is revisited.
func TestPushIsIdempotentForTheSameView(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	c, cfg := fakePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a := newApp(ctx, c, Options{Version: "test", Config: cfg, View: "blueprints", Refresh: time.Hour})
	before := pageCount(a.body)

	// Re-open the view already on the stack.
	if err := a.open("blueprints"); err != nil {
		t.Fatalf("open: %v", err)
	}
	if after := pageCount(a.body); after != before {
		t.Errorf("page count went %d -> %d; the same view should reuse its page", before, after)
	}
}

func pageCount(p *tview.Pages) int { return p.GetPageCount() }
