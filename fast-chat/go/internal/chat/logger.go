package chat

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MessageLogger handles async logging of messages to a file
type MessageLogger struct {
	file          *os.File
	writer        *bufio.Writer
	logChan       chan *Message
	closeChan     chan struct{}
	wg            sync.WaitGroup
	mu            sync.Mutex
	flushInterval time.Duration
}

func NewMessageLogger(filename string, flushInterval time.Duration) (*MessageLogger, error) {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	logger := &MessageLogger{
		file:          file,
		writer:        bufio.NewWriter(file),
		logChan:       make(chan *Message, 100),
		closeChan:     make(chan struct{}),
		flushInterval: flushInterval,
	}

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
	encoder := json.NewEncoder(l.writer)
	ticker := time.NewTicker(l.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case msg := <-l.logChan:
			if err := encoder.Encode(msg); err != nil {
				log.Printf("Failed to write message to log file: %v", err)
			}

		case <-ticker.C:
			if err := l.writer.Flush(); err != nil {
				log.Printf("Failed to flush log buffer: %v", err)
			}

		case <-l.closeChan:
			for {
				select {
				case msg := <-l.logChan:
					if err := encoder.Encode(msg); err != nil {
						log.Printf("Failed to write message during shutdown: %v", err)
					}
				default:
					l.writer.Flush()
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
	l.writer.Flush()
	return l.file.Close()
}

// LoadMessages loads messages from a log file
func LoadMessages(filename string) ([]*Message, error) {
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	var messages []*Message
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var msg Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		messages = append(messages, &msg)
	}

	return messages, nil
}

func ArchiveLogFile(filename string) error {
	info, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if info.Size() == 0 {
		return nil
	}

	timestamp := time.Now().Format("20060102-150405")
	dir := filepath.Dir(filename)
	base := filepath.Base(filename)
	archiveName := filepath.Join(dir, fmt.Sprintf("%s.%s.gz", base, timestamp))

	sourceFile, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	archiveFile, err := os.Create(archiveName)
	if err != nil {
		return err
	}
	defer archiveFile.Close()

	gzipWriter := gzip.NewWriter(archiveFile)
	defer gzipWriter.Close()

	if _, err := io.Copy(gzipWriter, sourceFile); err != nil {
		os.Remove(archiveName)
		return err
	}

	if err := gzipWriter.Close(); err != nil {
		os.Remove(archiveName)
		return err
	}

	if err := archiveFile.Close(); err != nil {
		os.Remove(archiveName)
		return err
	}

	if err := os.Truncate(filename, 0); err != nil {
		return err
	}

	log.Printf("Archived %d bytes to %s", info.Size(), archiveName)
	return nil
}
