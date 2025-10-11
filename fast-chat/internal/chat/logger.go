package chat

import (
	"encoding/json"
	"os"
	"sync"
)

// MessageLogger handles async logging of messages to a file
type MessageLogger struct {
	file      *os.File
	logChan   chan *Message
	closeChan chan struct{}
	wg        sync.WaitGroup
	mu        sync.Mutex
}

// NewMessageLogger creates a new async message logger
func NewMessageLogger(filename string) (*MessageLogger, error) {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	logger := &MessageLogger{
		file:      file,
		logChan:   make(chan *Message, 100), // Buffer 100 messages
		closeChan: make(chan struct{}),
	}

	// Start background writer
	logger.wg.Add(1)
	go logger.writeLoop()

	return logger, nil
}

// Log queues a message for async writing
func (l *MessageLogger) Log(msg *Message) {
	select {
	case l.logChan <- msg:
		// Message queued successfully
	default:
		// Channel full, log would block - skip this message
		// In production, you might want to handle this differently
	}
}

// writeLoop processes messages from the queue and writes to file
func (l *MessageLogger) writeLoop() {
	defer l.wg.Done()
	encoder := json.NewEncoder(l.file)

	for {
		select {
		case msg := <-l.logChan:
			// Write message as JSON line
			if err := encoder.Encode(msg); err != nil {
				// Log error but continue (don't crash the app)
				// In production, you might want proper error handling
			}

		case <-l.closeChan:
			// Drain remaining messages before closing
			for {
				select {
				case msg := <-l.logChan:
					encoder.Encode(msg)
				default:
					return
				}
			}
		}
	}
}

// Close gracefully shuts down the logger
func (l *MessageLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	close(l.closeChan)
	l.wg.Wait()
	return l.file.Close()
}

// LoadMessages loads messages from a log file
func LoadMessages(filename string) ([]*Message, error) {
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // File doesn't exist yet, return empty
		}
		return nil, err
	}
	defer file.Close()

	var messages []*Message
	decoder := json.NewDecoder(file)

	for decoder.More() {
		var msg Message
		if err := decoder.Decode(&msg); err != nil {
			// Skip malformed lines
			continue
		}
		messages = append(messages, &msg)
	}

	return messages, nil
}
