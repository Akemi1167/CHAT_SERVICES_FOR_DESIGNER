package realtime

import (
	"sync"

	"github.com/google/uuid"
)

// Client = 1 ket noi WebSocket cua 1 user (1 user co the mo nhieu tab/thiet bi).
// Client khong biet gi ve fiber/websocket: no chi la 1 hop thu (channel).
type Client struct {
	ID     string
	UserID uuid.UUID

	send      chan []byte
	closed    chan struct{}
	closeOnce sync.Once
}

func NewClient(userID uuid.UUID, buffer int) *Client {
	if buffer <= 0 {
		buffer = 256
	}
	return &Client{
		ID:     uuid.NewString(),
		UserID: userID,
		send:   make(chan []byte, buffer),
		closed: make(chan struct{}),
	}
}

// Outbound: transport layer doc channel nay roi ghi xuong socket (1 writer duy nhat).
func (c *Client) Outbound() <-chan []byte { return c.send }

func (c *Client) Done() <-chan struct{} { return c.closed }

// push khong block: neu client cham (buffer day) thi dong ket noi, FE se reconnect + resync.
func (c *Client) push(b []byte) bool {
	select {
	case <-c.closed:
		return false
	case c.send <- b:
		return true
	default:
		return false
	}
}

func (c *Client) Close() {
	c.closeOnce.Do(func() {
		close(c.closed)
		close(c.send)
	})
}
