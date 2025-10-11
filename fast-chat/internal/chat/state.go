package chat

import (
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
)

const (
	MaxMessages = 100 // Circular buffer size
)

// ChatState holds the global chat state
type ChatState struct {
	messages     []*Message
	connections  map[uuid.UUID]*Connection
	nicknamePool *NicknamePool
	nextID       atomic.Int64
	mu           sync.RWMutex
}

// NewChatState creates a new chat state
func NewChatState() *ChatState {
	return &ChatState{
		messages:     make([]*Message, 0, MaxMessages),
		connections:  make(map[uuid.UUID]*Connection),
		nicknamePool: NewNicknamePool(),
	}
}

// AddMessage adds a message to the circular buffer
func (s *ChatState) AddMessage(nickname, text string) *Message {
	id := s.nextID.Add(1)
	msg := NewMessage(id, nickname, text)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Add message to circular buffer
	if len(s.messages) >= MaxMessages {
		// Remove oldest message
		s.messages = s.messages[1:]
	}
	s.messages = append(s.messages, msg)

	return msg
}

// GetLastN returns the last N messages (for initial page load)
func (s *ChatState) GetLastN(n int) []*Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.messages) == 0 {
		return nil
	}

	if n > len(s.messages) {
		n = len(s.messages)
	}

	// Get last n messages
	start := len(s.messages) - n
	result := make([]*Message, n)
	copy(result, s.messages[start:])
	return result
}

// GetHistory returns messages for history requests
// skip = how many to skip from the end, take = how many to return
func (s *ChatState) GetHistory(skip, take int) []*Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.messages) == 0 {
		return nil
	}

	// Calculate range
	end := len(s.messages) - skip
	if end <= 0 {
		return nil
	}

	start := end - take
	if start < 0 {
		start = 0
	}

	result := make([]*Message, end-start)
	copy(result, s.messages[start:end])
	return result
}

// AddConnection adds a new connection
func (s *ChatState) AddConnection(conn *Connection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connections[conn.ID] = conn
}

// RemoveConnection removes a connection and releases its nickname
func (s *ChatState) RemoveConnection(id uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if conn, exists := s.connections[id]; exists {
		// Release nickname back to pool
		nickname := conn.GetNickname()
		if nickname != "" {
			s.nicknamePool.Release(nickname)
		}
		conn.Close()
		delete(s.connections, id)
	}
}

// GetConnection gets a connection by ID
func (s *ChatState) GetConnection(id uuid.UUID) (*Connection, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	conn, exists := s.connections[id]
	return conn, exists
}

// ConnectionCount returns the number of active connections
func (s *ChatState) ConnectionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.connections)
}

// Broadcast sends a message to all connections
func (s *ChatState) Broadcast(msg string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, conn := range s.connections {
		conn.Send(msg)
	}
}

// BroadcastExcept sends a message to all connections except one
func (s *ChatState) BroadcastExcept(msg string, exceptID uuid.UUID) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for id, conn := range s.connections {
		if id != exceptID {
			conn.Send(msg)
		}
	}
}

// AllocateNickname allocates a nickname from the pool
func (s *ChatState) AllocateNickname() string {
	return s.nicknamePool.Allocate()
}

// NicknamePoolStats returns stats about the nickname pool
func (s *ChatState) NicknamePoolStats() (available, used int) {
	return s.nicknamePool.Available(), s.nicknamePool.Used()
}
