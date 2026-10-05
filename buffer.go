package harvest

import (
	"log/slog"
	"sync"
	"time"
)

// Buffer manages thread-safe chunking and buffering of records
// for sequential processing or streaming iteration pipelines.
type Buffer[T any] struct {
	mu     sync.RWMutex
	items  []T
	size   int64
	seen   int64
	ticker *time.Ticker
}

// NewBuffer instantiates a new thread-safe chunked accumulator.
func NewBuffer[T any](size int64) *Buffer[T] {
	return &Buffer[T]{
		items: make([]T, 0, size),
		size:  size,
	}
}

// Append safely pushes items to the internal queue using write-lock protection boundaries.
func (b *Buffer[T]) Append(items ...T) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items = append(b.items, items...)
	b.seen += int64(len(items))
}

// Seen returns the total count of items that have passed through the buffer.
func (b *Buffer[T]) Seen() int64 {
	return b.seen
}

// Size returns the current number of accumulated items residing in the buffer.
func (b *Buffer[T]) Size() int64 {
	return int64(len(b.items))
}

// CollectAndReset returns a copy of accumulated elements and resets the
// internal slice if the threshold is met, entirely isolated from locks.
// If force is true, it flushes the buffer regardless of the threshold size.
func (b *Buffer[T]) CollectAndReset(force bool) []T {
	b.mu.Lock()
	defer b.mu.Unlock()

	item_sz := int64(len(b.items))

	if item_sz == 0 || (!force && item_sz < b.size) {
		return nil
	}

	readyItems := b.items
	b.items = make([]T, 0, b.size)
	return readyItems
}

// StartStatsTicker initializes a background logging loop that emits
// buffer metrics every 30 seconds using the default duration.
func (b *Buffer[T]) StartStatsTicker() {
	b.StartStatsTickerWithDuration(30 * time.Second)
}

// StartStatsTickerWithDuration initializes a background logging loop that emits
// buffer metrics at the specified time duration intervals.
func (b *Buffer[T]) StartStatsTickerWithDuration(d time.Duration) {

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.ticker != nil {
		return
	}

	b.ticker = time.NewTicker(d)

	go func() {
		for {
			select {
			case <-b.ticker.C:
				slog.Debug("Buffer", "size", len(b.items), "seen", b.seen)
			}
		}
	}()
}

// Close gracefully shuts down the background stats ticker if it is running.
func (b *Buffer[T]) Close() error {

	if b.ticker != nil {
		b.ticker.Stop()
	}

	return nil
}
