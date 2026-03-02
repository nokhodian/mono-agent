package util

import (
	"math/rand"
	"time"
)

// SleepConfig holds all configurable sleep durations used throughout the agent
// to simulate human-like timing behaviour.
type SleepConfig struct {
	// TypingMin is the minimum pause between simulated keystrokes.
	TypingMin time.Duration
	// TypingMax is the maximum pause between simulated keystrokes.
	TypingMax time.Duration

	// ActionMin is the minimum pause before performing a UI action (click, etc.).
	ActionMin time.Duration
	// ActionMax is the maximum pause before performing a UI action.
	ActionMax time.Duration

	// PageLoad is the duration to wait for a page to finish loading.
	PageLoad time.Duration

	// ScrollMin is the minimum pause between scroll steps.
	ScrollMin time.Duration
	// ScrollMax is the maximum pause between scroll steps.
	ScrollMax time.Duration

	// LongWait is used when a longer pause is required (e.g. between major steps).
	LongWait time.Duration

	// RetryWait is the delay before retrying a failed operation.
	RetryWait time.Duration

	// WarningWait is the delay used when a warning condition is detected.
	WarningWait time.Duration

	// ConcurrencyWait is the delay between spawning concurrent operations.
	ConcurrencyWait time.Duration
}

// DefaultSleepConfig provides sensible defaults that mimic natural human
// interaction timing.
var DefaultSleepConfig = SleepConfig{
	TypingMin:       50 * time.Millisecond,
	TypingMax:       150 * time.Millisecond,
	ActionMin:       500 * time.Millisecond,
	ActionMax:       1500 * time.Millisecond,
	PageLoad:        3 * time.Second,
	ScrollMin:       300 * time.Millisecond,
	ScrollMax:       800 * time.Millisecond,
	LongWait:        5 * time.Second,
	RetryWait:       10 * time.Second,
	WarningWait:     30 * time.Second,
	ConcurrencyWait: 2 * time.Second,
}

// SleepRandom sleeps for a random duration between min and max (inclusive).
// If min >= max the function sleeps for exactly min.
func SleepRandom(min, max time.Duration) {
	if min >= max {
		time.Sleep(min)
		return
	}
	delta := max - min
	jitter := time.Duration(rand.Int63n(int64(delta)))
	time.Sleep(min + jitter)
}
