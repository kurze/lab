package chat

import (
	"sync"
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
	Nickname  string
	Transport TransportType
	LastSeen  time.Time
	SendChan  chan string // Channel for sending messages to this connection
	mu        sync.RWMutex
}

// NewConnection creates a new connection
func NewConnection(transport TransportType) *Connection {
	return &Connection{
		ID:        uuid.New(),
		Transport: transport,
		LastSeen:  time.Now(),
		SendChan:  make(chan string, 256), // Buffered channel
	}
}

// SetNickname sets the connection's nickname
func (c *Connection) SetNickname(nickname string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Nickname = nickname
}

// GetNickname gets the connection's nickname
func (c *Connection) GetNickname() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Nickname
}

// UpdateLastSeen updates the last seen timestamp
func (c *Connection) UpdateLastSeen() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LastSeen = time.Now()
}

// Send sends a message to this connection (non-blocking)
func (c *Connection) Send(msg string) {
	select {
	case c.SendChan <- msg:
	default:
		// Drop message if buffer is full (slow client)
	}
}

// Close closes the connection's send channel
func (c *Connection) Close() {
	close(c.SendChan)
}
