package harvest

import (
	"sync"
)

// Buffer manages thread-safe chunking and buffering of records
// for sequential processing or streaming iteration pipelines.
type Buffer[T any] struct {
	mu    sync.RWMutex
	yield func([]T, error) bool
	items []T
	size  int
}

// NewBuffer instantiates a new thread-safe chunked accumulator.
func NewBuffer[T any](size int, yield func([]T, error) bool) *Buffer[T] {
	return &Buffer[T]{
		items: make([]T, 0, size),
		yield: yield,
		size:  size,
	}
}

func (b *Buffer[T]) AppendAndFlush(items ...T) bool {

	b.Append(items...)

	if len(b.items) < b.size {
		return true
	}

	return b.Flush()
}

// Append safely pushes items to the internal queue using write-lock protection boundaries.
func (b *Buffer[T]) Append(items ...T) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.items) >= b.size {
		// Invoke the iterator yield execution frame directly on the calling thread context
		if !b.yield(b.items, nil) {
			return false
		}
		// Reset the internal slice keeping the pre-allocated underlying capacity intact
		b.items = make([]T, 0, b.size)
	}

	return true
}

// Flush drains any leftover items that did not reach the threshold size
// at the tail-end of a data stream execution.
func (b *Buffer[T]) Flush() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.items) > 0 {
		if !b.yield(b.items, nil) {
			return false
		}
		b.items = make([]T, 0, b.size)
	}

	return true
}
