// Package bulk runs one operation across many entity identifiers with bounded
// concurrency, reporting each result as it lands.
package bulk

import (
	"context"
	"sync"
	"sync/atomic"
)

// DefaultConcurrency bounds how many entity mutations are in flight at once.
// Port rate-limits, and a whole blueprint can be thousands of entities, so the
// work is deliberately throttled rather than fanned out completely.
const DefaultConcurrency = 5

// Apply runs fn over every id, at most concurrency at a time, and returns how
// many failed.
//
// onResult is called once per id with that id's outcome, from the worker
// goroutine but serialised — so an implementation may print or mutate shared
// state without locking. Results arrive in completion order, not input order.
func Apply(ctx context.Context, ids []string, concurrency int, fn func(context.Context, string) error, onResult func(id string, err error)) int {
	if concurrency < 1 {
		concurrency = 1
	}

	var (
		failed atomic.Int32
		mu     sync.Mutex
		sem    = make(chan struct{}, concurrency)
		wg     sync.WaitGroup
	)

	for _, id := range ids {
		// Stop handing out new work once cancelled. Requests already in flight
		// are carrying the same ctx and will fail on their own.
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			err := fn(ctx, id)
			if err != nil {
				failed.Add(1)
			}
			if onResult != nil {
				mu.Lock()
				onResult(id, err)
				mu.Unlock()
			}
		}(id)
	}

	wg.Wait()
	return int(failed.Load())
}
