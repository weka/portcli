package tui

import (
	"context"
	"time"

	"github.com/rivo/tview"
)

// requestTimeout bounds one background fetch. The client's own 30s transport
// timeout is the backstop for a wedged connection; this is tighter so a slow
// endpoint cannot hold a refresh slot for that long.
const requestTimeout = 20 * time.Second

// refresher owns a view's background polling loop.
//
// Three rules keep it safe, and breaking any of them produces a hang with no
// stack trace:
//
//  1. Only apply touches tview state. fetch may only call the client.
//  2. Never call QueueUpdateDraw from the UI goroutine — it blocks until the
//     main loop drains it, and on the UI goroutine that loop is the caller.
//     Key handlers already run there and may mutate widgets directly.
//  3. stop never waits for the goroutine to exit. It cannot: stop runs on the
//     UI goroutine, and the goroutine may be parked in QueueUpdateDraw
//     waiting for that same loop. Cancellation is instead re-checked inside
//     the queued closure, where it runs on the UI goroutine.
type refresher struct {
	app    *tview.Application
	cancel context.CancelFunc
	// gen is only ever touched on the UI goroutine. A closure queued just
	// before a view switch lands after it and no-ops, so a slow fetch for the
	// view being left cannot repaint the view being entered.
	gen uint64
}

// start replaces any running loop. fetch runs off the UI goroutine; apply runs
// on it. An immediate first fetch happens before the first tick, so a view
// paints as soon as it can rather than after one interval.
func (r *refresher) start(
	parent context.Context,
	interval time.Duration,
	fetch func(context.Context) (any, error),
	apply func(data any, err error),
) {
	r.stop()
	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel
	r.gen++
	gen := r.gen

	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			reqCtx, cancelReq := context.WithTimeout(ctx, requestTimeout)
			data, err := fetch(reqCtx)
			cancelReq()

			// Cheap pre-check: if we already lost the race, do not even queue.
			if ctx.Err() != nil {
				return
			}

			r.app.QueueUpdateDraw(func() {
				// Authoritative check, on the UI goroutine. See rule 3.
				if ctx.Err() != nil || gen != r.gen {
					return
				}
				apply(data, err)
			})

			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

// stop cancels the loop and returns immediately. It must never block.
func (r *refresher) stop() {
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
}
