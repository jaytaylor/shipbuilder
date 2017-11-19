package main

import (
	"sync"
)

// GlobalPortTracker enables rolling port allocations for guarding against
// port contention and conflicts.
type GlobalPortTracker struct {
	Min int
	Max int
	val int
	mu  sync.Mutex
}

// Next retrieves the next port value to begin the sequence check with.
func (tracker *GlobalPortTracker) Next() int {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	if tracker.val == 0 || tracker.val > tracker.Max {
		tracker.val = tracker.Min
	}

	tracker.val++

	return tracker.val
}

// Using sets the currently in-use value.
func (tracker *GlobalPortTracker) Using(val int) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	tracker.val = val
}
