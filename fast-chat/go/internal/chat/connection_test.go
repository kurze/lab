package chat

import (
	"sync"
	"testing"
	"time"
)

func TestNewConnection(t *testing.T) {
	conn := NewConnection(TransportWebSocket)

	if conn.ID.String() == "" {
		t.Error("Connection ID should not be empty")
	}

	if conn.Transport != TransportWebSocket {
		t.Errorf("Expected WebSocket transport, got %v", conn.Transport)
	}

	if conn.SendChan == nil {
		t.Error("SendChan should be initialized")
	}

	if conn.IsClosed() {
		t.Error("New connection should not be closed")
	}
}

func TestConnectionSetGetNickname(t *testing.T) {
	conn := NewConnection(TransportWebSocket)

	if conn.GetNickname() != "" {
		t.Error("Initial nickname should be empty")
	}

	conn.SetNickname("alice")

	if conn.GetNickname() != "alice" {
		t.Errorf("Expected nickname 'alice', got '%s'", conn.GetNickname())
	}
}

func TestConnectionLastSeen(t *testing.T) {
	conn := NewConnection(TransportWebSocket)

	before := time.Now()
	time.Sleep(10 * time.Millisecond)
	conn.UpdateLastSeen()
	after := time.Now()

	lastSeen := conn.GetLastSeen()

	if lastSeen.Before(before) || lastSeen.After(after) {
		t.Error("LastSeen should be between before and after UpdateLastSeen call")
	}
}

func TestConnectionSend(t *testing.T) {
	conn := NewConnection(TransportWebSocket)

	go func() {
		for range conn.SendChan {
		}
	}()

	if !conn.Send("test message") {
		t.Error("Send should succeed on open connection")
	}

	conn.Close()

	if conn.Send("another message") {
		t.Error("Send should fail on closed connection")
	}
}

func TestConnectionSendBufferFull(t *testing.T) {
	conn := NewConnection(TransportWebSocket)

	for i := 0; i < 256; i++ {
		if !conn.Send("message") {
			t.Fatalf("Send failed at message %d (buffer should be 256)", i)
		}
	}

	if conn.Send("overflow") {
		t.Error("Send should fail when buffer is full")
	}
}

func TestConnectionClose(t *testing.T) {
	conn := NewConnection(TransportWebSocket)

	if conn.IsClosed() {
		t.Error("Connection should not be closed initially")
	}

	conn.Close()

	if !conn.IsClosed() {
		t.Error("Connection should be closed after Close()")
	}

	select {
	case _, ok := <-conn.SendChan:
		if ok {
			t.Error("SendChan should be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("SendChan should be immediately closed")
	}
}

func TestConnectionCloseIdempotent(t *testing.T) {
	conn := NewConnection(TransportWebSocket)

	conn.Close()
	conn.Close()
	conn.Close()

	if !conn.IsClosed() {
		t.Error("Connection should remain closed")
	}
}

func TestConnectionConcurrentSend(t *testing.T) {
	conn := NewConnection(TransportWebSocket)

	received := make([]string, 0, 100)
	mu := sync.Mutex{}

	go func() {
		for msg := range conn.SendChan {
			mu.Lock()
			received = append(received, msg)
			mu.Unlock()
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			conn.Send("message")
		}(i)
	}

	wg.Wait()
	conn.Close()

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	count := len(received)
	mu.Unlock()

	if count != 100 {
		t.Errorf("Expected 100 messages, got %d", count)
	}
}

func TestConnectionTransportType(t *testing.T) {
	wsConn := NewConnection(TransportWebSocket)
	if wsConn.Transport != TransportWebSocket {
		t.Error("WebSocket transport not set correctly")
	}

	wtConn := NewConnection(TransportWebTransport)
	if wtConn.Transport != TransportWebTransport {
		t.Error("WebTransport transport not set correctly")
	}
}

func TestConnectionConcurrentNicknameAccess(t *testing.T) {
	conn := NewConnection(TransportWebSocket)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)

		go func(id int) {
			defer wg.Done()
			conn.SetNickname("user")
		}(i)

		go func() {
			defer wg.Done()
			_ = conn.GetNickname()
		}()
	}

	wg.Wait()
}

func TestConnectionConcurrentLastSeenAccess(t *testing.T) {
	conn := NewConnection(TransportWebSocket)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)

		go func() {
			defer wg.Done()
			conn.UpdateLastSeen()
		}()

		go func() {
			defer wg.Done()
			_ = conn.GetLastSeen()
		}()
	}

	wg.Wait()
}
