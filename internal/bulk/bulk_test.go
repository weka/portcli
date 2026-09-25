package bulk

import (
	"fmt"
	"sort"
	"sync/atomic"
	"testing"
)

func TestApplyRunsEveryIDAndCountsFailures(t *testing.T) {
	ids := []string{"a", "b", "c", "d", "e"}

	var seen []string
	failed := Apply(ids, 2,
		func(id string) error {
			if id == "b" || id == "d" {
				return fmt.Errorf("boom %s", id)
			}
			return nil
		},
		func(id string, err error) { seen = append(seen, id) },
	)

	if failed != 2 {
		t.Errorf("failed = %d, want 2", failed)
	}
	// onResult is serialised, so appending without a lock is safe — that
	// guarantee is the reason callers may print from it.
	sort.Strings(seen)
	if got := fmt.Sprint(seen); got != "[a b c d e]" {
		t.Errorf("onResult saw %v, want every id exactly once", seen)
	}
}

func TestApplyRespectsConcurrencyLimit(t *testing.T) {
	const limit = 3
	var inFlight, peak atomic.Int32

	ids := make([]string, 50)
	for i := range ids {
		ids[i] = fmt.Sprintf("id-%d", i)
	}

	Apply(ids, limit, func(string) error {
		n := inFlight.Add(1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		inFlight.Add(-1)
		return nil
	}, nil)

	if p := peak.Load(); p > limit {
		t.Errorf("peak concurrency = %d, want <= %d", p, limit)
	}
}

func TestApplyEdgeCases(t *testing.T) {
	t.Run("no ids does nothing", func(t *testing.T) {
		called := false
		if failed := Apply(nil, 5, func(string) error { called = true; return nil }, nil); failed != 0 {
			t.Errorf("failed = %d, want 0", failed)
		}
		if called {
			t.Error("fn ran with no ids")
		}
	})

	t.Run("nil onResult is allowed", func(t *testing.T) {
		var ran atomic.Int32
		Apply([]string{"a", "b"}, 2, func(string) error { ran.Add(1); return nil }, nil)
		if ran.Load() != 2 {
			t.Errorf("fn ran %d times, want 2", ran.Load())
		}
	})

	// A non-positive limit would otherwise make the semaphore channel
	// unbuffered and deadlock every worker.
	t.Run("zero concurrency is clamped, not deadlocked", func(t *testing.T) {
		var ran atomic.Int32
		Apply([]string{"a", "b", "c"}, 0, func(string) error { ran.Add(1); return nil }, nil)
		if ran.Load() != 3 {
			t.Errorf("fn ran %d times, want 3", ran.Load())
		}
	})

	t.Run("all failing", func(t *testing.T) {
		failed := Apply([]string{"a", "b"}, 2, func(string) error { return fmt.Errorf("no") }, nil)
		if failed != 2 {
			t.Errorf("failed = %d, want 2", failed)
		}
	})
}
