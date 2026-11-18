package realtime

import (
	"context"
	"log/slog"
	"sync"
)

type MemoryBroker struct {
	mu     sync.RWMutex
	subs   []chan Event
	closed bool
	buf    int
}

func NewMemoryBroker(buffer int) *MemoryBroker {
	if buffer <= 0 {
		buffer = 1024
	}
	return &MemoryBroker{buf: buffer}
}

func (b *MemoryBroker) Publish(_ context.Context, ev Event) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return nil
	}
	for _, ch := range b.subs {
		select {
		case ch <- ev:
		default:
			slog.Warn("realtime: memory broker buffer full, event bi bo", "type", ev.Type)
		}
	}
	return nil
}

func (b *MemoryBroker) Subscribe(_ context.Context) (<-chan Event, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan Event, b.buf)
	b.subs = append(b.subs, ch)
	return ch, nil
}

func (b *MemoryBroker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	for _, ch := range b.subs {
		close(ch)
	}
	b.subs = nil
	return nil
}
