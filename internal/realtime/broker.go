package realtime

import "context"

// Broker tach lop van chuyen event giua cac instance.
// - memory: 1 instance (local dev)
// - redis : nhieu instance (prod) — chi can doi config REDIS_ENABLED=true
type Broker interface {
	Publish(ctx context.Context, ev Event) error
	// Subscribe tra channel event (gom ca event do chinh instance nay publish).
	Subscribe(ctx context.Context) (<-chan Event, error)
	Close() error
}
