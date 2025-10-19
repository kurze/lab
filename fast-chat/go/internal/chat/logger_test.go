package chat

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestMessageLoggerWriteAndLoad(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "logger-test-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	logger, err := NewMessageLogger(tmpFile.Name(), time.Second)
	if err != nil {
		t.Fatal(err)
	}

	msg1 := NewMessage(1, "alice", "hello")
	msg2 := NewMessage(2, "bob", "world")

	logger.Log(msg1)
	logger.Log(msg2)

	time.Sleep(50 * time.Millisecond)

	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadMessages(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(loaded))
	}

	if loaded[0].ID != 1 || loaded[0].Nickname != "alice" {
		t.Errorf("First message incorrect: %v", loaded[0])
	}
	if loaded[1].ID != 2 || loaded[1].Nickname != "bob" {
		t.Errorf("Second message incorrect: %v", loaded[1])
	}
}

func TestMessageLoggerEmptyFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "logger-test-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	loaded, err := LoadMessages(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded) != 0 {
		t.Errorf("Expected 0 messages from empty file, got %d", len(loaded))
	}
}

func TestMessageLoggerNonexistentFile(t *testing.T) {
	loaded, err := LoadMessages("/nonexistent/file.jsonl")
	if err != nil {
		t.Fatal(err)
	}

	if loaded != nil {
		t.Error("Expected nil for nonexistent file")
	}
}

func TestMessageLoggerMalformedData(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "logger-test-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString(`{"id":1,"nickname":"alice","text":"hello","timestamp":"2024-01-01T00:00:00Z"}` + "\n")
	tmpFile.WriteString(`{malformed json}` + "\n")
	tmpFile.WriteString(`{"id":2,"nickname":"bob","text":"world","timestamp":"2024-01-01T00:00:01Z"}` + "\n")
	tmpFile.Close()

	loaded, err := LoadMessages(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded) != 2 {
		t.Errorf("Expected 2 valid messages (malformed skipped), got %d", len(loaded))
	}
}

func TestMessageLoggerConcurrentWrites(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "logger-test-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	logger, err := NewMessageLogger(tmpFile.Name(), time.Second)
	if err != nil {
		t.Fatal(err)
	}

	const numMessages = 100
	done := make(chan bool, numMessages)

	for i := 0; i < numMessages; i++ {
		go func(id int) {
			msg := NewMessage(int64(id), "user", "message")
			logger.Log(msg)
			done <- true
		}(i)
	}

	for i := 0; i < numMessages; i++ {
		<-done
	}

	time.Sleep(100 * time.Millisecond)

	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadMessages(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded) != numMessages {
		t.Errorf("Expected %d messages, got %d", numMessages, len(loaded))
	}
}

func TestMessageLoggerBufferFull(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "logger-test-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	logger, err := NewMessageLogger(tmpFile.Name(), time.Second)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 200; i++ {
		msg := NewMessage(int64(i), "user", "message")
		logger.Log(msg)
	}

	time.Sleep(200 * time.Millisecond)

	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadMessages(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded) < 100 {
		t.Errorf("Expected at least 100 messages (buffer size), got %d", len(loaded))
	}
}

func TestMessageLoggerCloseFlushes(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "logger-test-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	logger, err := NewMessageLogger(tmpFile.Name(), time.Second)
	if err != nil {
		t.Fatal(err)
	}

	msg := NewMessage(1, "user", "message")
	logger.Log(msg)

	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadMessages(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded) != 1 {
		t.Errorf("Close should flush buffered messages, expected 1, got %d", len(loaded))
	}
}

func TestLoadMessagesValidJSON(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "logger-test-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	msg := NewMessage(1, "alice", "hello")
	data, _ := json.Marshal(msg)
	tmpFile.Write(data)
	tmpFile.WriteString("\n")
	tmpFile.Close()

	loaded, err := LoadMessages(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(loaded))
	}

	if loaded[0].Nickname != "alice" {
		t.Errorf("Expected nickname 'alice', got '%s'", loaded[0].Nickname)
	}
}
