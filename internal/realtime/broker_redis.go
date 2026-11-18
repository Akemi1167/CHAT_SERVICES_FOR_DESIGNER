package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"
)

// RedisBroker dung Pub/Sub de fanout event giua cac pod.
// Luu y: Pub/Sub la at-most-once. Neu can "khong mat tin nhan khi mat ket noi",
// client nen goi lai GET /conversations/:id/messages de dong bo (cursor-based).
type RedisBroker struct {
	rdb     *redis.Client
	channel string
	buffer  int
}

func NewRedisBroker(rdb *redis.Client, channel string, buffer int) *RedisBroker {
	if buffer <= 0 {
		buffer = 1024
	}
	return &RedisBroker{rdb: rdb, channel: channel, buffer: buffer}
}

func (b *RedisBroker) Publish(ctx context.Context, ev Event) error {
	raw, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("realtime: marshal event: %w", err)
	}
	if err := b.rdb.Publish(ctx, b.channel, raw).Err(); err != nil {
		return fmt.Errorf("realtime: redis publish: %w", err)
	}
	return nil
}

func (b *RedisBroker) Subscribe(ctx context.Context) (<-chan Event, error) {
	sub := b.rdb.Subscribe(ctx, b.channel)
	if _, err := sub.Receive(ctx); err != nil {
		return nil, fmt.Errorf("realtime: redis subscribe: %w", err)
	}

	out := make(chan Event, b.buffer)
	go func() {
		defer close(out)
		defer func() { _ = sub.Close() }()
		for msg := range sub.Channel() {
			var ev Event
			if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
				slog.Error("realtime: unmarshal event", "err", err)
				continue
			}
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (b *RedisBroker) Close() error { return nil }
