package chat

import (
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// TransportType indicates the connection transport mechanism
type TransportType int

const (
	TransportWebSocket TransportType = iota
	TransportWebTransport
)

// Connection represents a client connection
type Connection struct {
	ID        uuid.UUID
	nickname  atomic.Value // stores string
	Transport TransportType
	lastSeen  atomic.Int64 // Unix nano timestamp
	SendChan  chan string  // Channel for sending messages to this connection
	closed    atomic.Bool  // Track if connection is already closed
}

// NewConnection creates a new connection
func NewConnection(transport TransportType) *Connection {
	conn := &Connection{
		ID:        uuid.New(),
		Transport: transport,
		SendChan:  make(chan string, 256), // Buffered channel
	}
	conn.lastSeen.Store(time.Now().UnixNano())
	conn.nickname.Store("") // Initialize with empty string
	return conn
}

// SetNickname sets the connection's nickname (lock-free)
func (c *Connection) SetNickname(nickname string) {
	c.nickname.Store(nickname)
}

// GetNickname gets the connection's nickname (lock-free)
func (c *Connection) GetNickname() string {
	if val := c.nickname.Load(); val != nil {
		return val.(string)
	}
	return ""
}

// UpdateLastSeen updates the last seen timestamp (lock-free)
func (c *Connection) UpdateLastSeen() {
	c.lastSeen.Store(time.Now().UnixNano())
}

// GetLastSeen gets the last seen timestamp (lock-free)
func (c *Connection) GetLastSeen() time.Time {
	return time.Unix(0, c.lastSeen.Load())
}

// Send sends a message to this connection (non-blocking)
// Returns false if the connection is closed or buffer is full
func (c *Connection) Send(msg string) bool {
	// Don't try to send to closed connections
	if c.IsClosed() {
		return false
	}

	select {
	case c.SendChan <- msg:
		return true
	default:
		// Drop message if buffer is full (slow client)
		return false
	}
}

// Close closes the connection's send channel (idempotent, safe to call multiple times)
func (c *Connection) Close() {
	if c.closed.CompareAndSwap(false, true) {
		close(c.SendChan)
	}
}

// IsClosed returns whether the connection has been closed
func (c *Connection) IsClosed() bool {
	return c.closed.Load()
}
