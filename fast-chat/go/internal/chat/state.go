package chat

import (
	"log"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
)

const (
	MaxMessages = 1000 // Circular buffer size
)

// ChatState holds the global chat state
type ChatState struct {
	messages     []*Message
	connections  map[uuid.UUID]*Connection
	nicknamePool *NicknamePool
	logger       *MessageLogger
	nextID       atomic.Int64
	messagesMu   sync.RWMutex // Protects messages slice
	connsMu      sync.RWMutex // Protects connections map
}

// NewChatState creates a new chat state
func NewChatState(logFile string) (*ChatState, error) {
	// Create logger
	logger, err := NewMessageLogger(logFile)
	if err != nil {
		return nil, err
	}

	state := &ChatState{
		messages:     make([]*Message, 0, MaxMessages),
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

	// Populate circular buffer with last MaxMessages
	if len(loadedMessages) > 0 {
		start := 0
		if len(loadedMessages) > MaxMessages {
			start = len(loadedMessages) - MaxMessages
			log.Printf("Keeping last %d messages in memory (discarding %d older messages)", MaxMessages, totalLoaded-MaxMessages)
		}
		state.messages = loadedMessages[start:]

		// Update nextID to continue from last message
		if len(state.messages) > 0 {
			lastID := state.messages[len(state.messages)-1].ID
			state.nextID.Store(lastID)
			log.Printf("Resuming message IDs from %d", lastID)
		}
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

// AddMessage adds a message to the circular buffer
func (s *ChatState) AddMessage(nickname, text string) *Message {
	id := s.nextID.Add(1)
	msg := NewMessage(id, nickname, text)

	s.messagesMu.Lock()
	defer s.messagesMu.Unlock()

	// Add message to circular buffer
	if len(s.messages) >= MaxMessages {
		// Remove oldest message
		s.messages = s.messages[1:]
	}
	s.messages = append(s.messages, msg)

	// Log message asynchronously
	if s.logger != nil {
		s.logger.Log(msg)
	}

	return msg
}

// GetLastN returns the last N messages (for initial page load)
func (s *ChatState) GetLastN(n int) []*Message {
	s.messagesMu.RLock()
	defer s.messagesMu.RUnlock()

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
	s.messagesMu.RLock()
	defer s.messagesMu.RUnlock()

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

// Broadcast sends a message to all connections
func (s *ChatState) Broadcast(msg string) {
	// Minimize lock hold time by copying connection pointers
	s.connsMu.RLock()
	conns := make([]*Connection, 0, len(s.connections))
	for _, conn := range s.connections {
		conns = append(conns, conn)
	}
	s.connsMu.RUnlock()

	// Send to all connections without holding the lock
	for _, conn := range conns {
		conn.Send(msg)
	}
}

// BroadcastExcept sends a message to all connections except one
func (s *ChatState) BroadcastExcept(msg string, exceptID uuid.UUID) {
	// Minimize lock hold time by copying connection pointers
	s.connsMu.RLock()
	conns := make([]*Connection, 0, len(s.connections))
	for id, conn := range s.connections {
		if id != exceptID {
			conns = append(conns, conn)
		}
	}
	s.connsMu.RUnlock()

	// Send to all connections without holding the lock
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
