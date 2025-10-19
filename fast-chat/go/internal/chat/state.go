package chat

import (
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

const (
	MaxMessages = 1000 // Circular buffer size
)

// ChatState holds the global chat state
type ChatState struct {
	messages     *RingBuffer // Lock-free ring buffer for messages
	connections  map[uuid.UUID]*Connection
	nicknamePool *NicknamePool
	logger       *MessageLogger
	nextID       atomic.Int64
	connsMu      sync.RWMutex // Protects connections map
}

// NewChatState creates a new chat state
func NewChatState(logFile string, flushInterval time.Duration) (*ChatState, error) {
	logger, err := NewMessageLogger(logFile, flushInterval)
	if err != nil {
		return nil, err
	}

	state := &ChatState{
		messages:     NewRingBuffer(MaxMessages),
		connections:  make(map[uuid.UUID]*Connection),
		nicknamePool: NewNicknamePool(),
		logger:       logger,
	}

	// Load messages from log file
	loadedMessages, err := LoadMessages(logFile)
	if err != nil {
		logger.Close()
		return nil, err
	}

	// Log the number of messages loaded
	totalLoaded := len(loadedMessages)
	if totalLoaded == 0 {
		log.Println("No previous messages found, starting fresh")
	} else {
		log.Printf("Loaded %d messages from log file", totalLoaded)
	}

	// Populate ring buffer with last MaxMessages
	if len(loadedMessages) > 0 {
		start := 0
		if len(loadedMessages) > MaxMessages {
			start = len(loadedMessages) - MaxMessages
			log.Printf("Keeping last %d messages in memory (discarding %d older messages)", MaxMessages, totalLoaded-MaxMessages)
		}

		// Push messages into ring buffer
		for _, msg := range loadedMessages[start:] {
			state.messages.Push(msg)
		}

		// Update nextID to continue from last message
		lastMsg := loadedMessages[len(loadedMessages)-1]
		state.nextID.Store(lastMsg.ID)
		log.Printf("Resuming message IDs from %d", lastMsg.ID)
	}

	return state, nil
}

// Close gracefully shuts down the chat state
func (s *ChatState) Close() error {
	if s.logger != nil {
		return s.logger.Close()
	}
	return nil
}

// AddMessage adds a message to the ring buffer (lock-free!)
func (s *ChatState) AddMessage(nickname, text string) *Message {
	id := s.nextID.Add(1)
	msg := NewMessage(id, nickname, text)

	// Add message to lock-free ring buffer (no lock needed!)
	s.messages.Push(msg)

	// Log message asynchronously
	if s.logger != nil {
		s.logger.Log(msg)
	}

	return msg
}

// GetLastN returns the last N messages (for initial page load)
// Lock-free read from ring buffer
func (s *ChatState) GetLastN(n int) []*Message {
	return s.messages.GetLast(n)
}

// GetHistory returns messages for history requests
// skip = how many to skip from the end, take = how many to return
// Lock-free read from ring buffer
func (s *ChatState) GetHistory(skip, take int) []*Message {
	return s.messages.GetHistory(skip, take)
}

// AddConnection adds a new connection
func (s *ChatState) AddConnection(conn *Connection) {
	s.connsMu.Lock()
	defer s.connsMu.Unlock()
	s.connections[conn.ID] = conn
}

// RemoveConnection removes a connection and releases its nickname
// Returns the nickname if it was removed, empty string if already removed
func (s *ChatState) RemoveConnection(id uuid.UUID) string {
	s.connsMu.Lock()
	defer s.connsMu.Unlock()

	conn, exists := s.connections[id]
	if !exists {
		return "" // Already removed
	}

	// Check if already closed (race condition protection)
	if conn.IsClosed() {
		delete(s.connections, id)
		return "" // Already handled
	}

	// Get nickname before closing
	nickname := conn.GetNickname()

	// Close and cleanup
	conn.Close()
	delete(s.connections, id)

	// Release nickname back to pool
	if nickname != "" {
		s.nicknamePool.Release(nickname)
	}

	return nickname
}

// GetConnection gets a connection by ID
func (s *ChatState) GetConnection(id uuid.UUID) (*Connection, bool) {
	s.connsMu.RLock()
	defer s.connsMu.RUnlock()
	conn, exists := s.connections[id]
	return conn, exists
}

// ConnectionCount returns the number of active connections
func (s *ChatState) ConnectionCount() int {
	s.connsMu.RLock()
	defer s.connsMu.RUnlock()
	return len(s.connections)
}

// Broadcast sends a message to all active connections
func (s *ChatState) Broadcast(msg string) {
	// Minimize lock hold time by copying connection pointers
	s.connsMu.RLock()
	conns := make([]*Connection, 0, len(s.connections))
	for _, conn := range s.connections {
		// Only include connections that aren't closed
		if !conn.IsClosed() {
			conns = append(conns, conn)
		}
	}
	s.connsMu.RUnlock()

	// Send to all active connections without holding the lock
	for _, conn := range conns {
		conn.Send(msg)
	}
}

// BroadcastExcept sends a message to all active connections except one
func (s *ChatState) BroadcastExcept(msg string, exceptID uuid.UUID) {
	// Minimize lock hold time by copying connection pointers
	s.connsMu.RLock()
	conns := make([]*Connection, 0, len(s.connections))
	for id, conn := range s.connections {
		// Only include connections that aren't closed and aren't the exception
		if id != exceptID && !conn.IsClosed() {
			conns = append(conns, conn)
		}
	}
	s.connsMu.RUnlock()

	// Send to all active connections without holding the lock
	for _, conn := range conns {
		conn.Send(msg)
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
