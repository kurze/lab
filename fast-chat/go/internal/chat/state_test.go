package chat

import (
	"os"
	"sync"
	"testing"
	"time"
)

func TestChatStateAddRemoveConnection(t *testing.T) {
	state, cleanup := createTestState(t)
	defer cleanup()

	conn := NewConnection(TransportWebSocket)
	conn.SetNickname("testuser")

	state.AddConnection(conn)

	if state.ConnectionCount() != 1 {
		t.Errorf("Expected 1 connection, got %d", state.ConnectionCount())
	}

	retrieved, exists := state.GetConnection(conn.ID)
	if !exists {
		t.Error("Connection should exist")
	}
	if retrieved.GetNickname() != "testuser" {
		t.Errorf("Expected nickname 'testuser', got '%s'", retrieved.GetNickname())
	}

	nickname := state.RemoveConnection(conn.ID)
	if nickname != "testuser" {
		t.Errorf("Expected removed nickname 'testuser', got '%s'", nickname)
	}

	if state.ConnectionCount() != 0 {
		t.Errorf("Expected 0 connections, got %d", state.ConnectionCount())
	}
}

func TestChatStateConcurrentConnections(t *testing.T) {
	state, cleanup := createTestState(t)
	defer cleanup()

	const numGoroutines = 50
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			conn := NewConnection(TransportWebSocket)
			state.AddConnection(conn)
			time.Sleep(1 * time.Millisecond)
			state.RemoveConnection(conn.ID)
		}(i)
	}

	wg.Wait()

	if state.ConnectionCount() != 0 {
		t.Errorf("Expected 0 connections after cleanup, got %d", state.ConnectionCount())
	}
}

func TestChatStateBroadcast(t *testing.T) {
	state, cleanup := createTestState(t)
	defer cleanup()

	conns := make([]*Connection, 3)
	for i := 0; i < 3; i++ {
		conn := NewConnection(TransportWebSocket)
		state.AddConnection(conn)
		conns[i] = conn
	}

	state.Broadcast("test message")

	for i, conn := range conns {
		select {
		case msg := <-conn.SendChan:
			if msg != "test message" {
				t.Errorf("Connection %d: expected 'test message', got '%s'", i, msg)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("Connection %d: timeout waiting for broadcast", i)
		}
	}
}

func TestChatStateBroadcastExcept(t *testing.T) {
	state, cleanup := createTestState(t)
	defer cleanup()

	conn1 := NewConnection(TransportWebSocket)
	conn2 := NewConnection(TransportWebSocket)
	conn3 := NewConnection(TransportWebSocket)

	state.AddConnection(conn1)
	state.AddConnection(conn2)
	state.AddConnection(conn3)

	state.BroadcastExcept("test message", conn2.ID)

	if msg := <-conn1.SendChan; msg != "test message" {
		t.Errorf("conn1: expected 'test message', got '%s'", msg)
	}

	if msg := <-conn3.SendChan; msg != "test message" {
		t.Errorf("conn3: expected 'test message', got '%s'", msg)
	}

	select {
	case msg := <-conn2.SendChan:
		t.Errorf("conn2 should not receive message, got: %s", msg)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestChatStateAddMessage(t *testing.T) {
	state, cleanup := createTestState(t)
	defer cleanup()

	msg1 := state.AddMessage("alice", "hello")
	msg2 := state.AddMessage("bob", "world")

	if msg1.ID != 1 {
		t.Errorf("First message should have ID 1, got %d", msg1.ID)
	}
	if msg2.ID != 2 {
		t.Errorf("Second message should have ID 2, got %d", msg2.ID)
	}

	if msg1.Nickname != "alice" || msg1.Text != "hello" {
		t.Errorf("Message content incorrect: %v", msg1)
	}
}

func TestChatStateGetLastMessages(t *testing.T) {
	state, cleanup := createTestState(t)
	defer cleanup()

	for i := 0; i < 10; i++ {
		state.AddMessage("user", "message")
	}

	messages := state.GetLastN(5)
	if len(messages) != 5 {
		t.Errorf("Expected 5 messages, got %d", len(messages))
	}

	if messages[0].ID != 6 {
		t.Errorf("First message should be ID 6, got %d", messages[0].ID)
	}
	if messages[4].ID != 10 {
		t.Errorf("Last message should be ID 10, got %d", messages[4].ID)
	}
}

func TestChatStateGetHistory(t *testing.T) {
	state, cleanup := createTestState(t)
	defer cleanup()

	for i := 0; i < 20; i++ {
		state.AddMessage("user", "message")
	}

	messages := state.GetHistory(5, 3)
	if len(messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(messages))
	}

	if messages[0].ID != 13 {
		t.Errorf("First message should be ID 13, got %d", messages[0].ID)
	}
	if messages[2].ID != 15 {
		t.Errorf("Last message should be ID 15, got %d", messages[2].ID)
	}
}

func TestChatStateNicknameAllocation(t *testing.T) {
	state, cleanup := createTestState(t)
	defer cleanup()

	nick1 := state.AllocateNickname()
	nick2 := state.AllocateNickname()

	if nick1 == "" || nick2 == "" {
		t.Error("Nicknames should not be empty")
	}

	if nick1 == nick2 {
		t.Error("Nicknames should be unique")
	}

	conn := NewConnection(TransportWebSocket)
	conn.SetNickname(nick1)
	state.AddConnection(conn)

	state.RemoveConnection(conn.ID)

	available, used := state.NicknamePoolStats()
	if used != 1 {
		t.Errorf("Expected 1 nickname used (nick2), got %d", used)
	}
	if available < 50 {
		t.Errorf("Expected at least 50 available nicknames, got %d", available)
	}
}

func TestChatStateDoubleRemove(t *testing.T) {
	state, cleanup := createTestState(t)
	defer cleanup()

	conn := NewConnection(TransportWebSocket)
	conn.SetNickname("test")
	state.AddConnection(conn)

	nickname1 := state.RemoveConnection(conn.ID)
	if nickname1 != "test" {
		t.Errorf("First remove should return 'test', got '%s'", nickname1)
	}

	nickname2 := state.RemoveConnection(conn.ID)
	if nickname2 != "" {
		t.Errorf("Second remove should return empty string, got '%s'", nickname2)
	}
}

func TestChatStateConcurrentBroadcast(t *testing.T) {
	state, cleanup := createTestState(t)
	defer cleanup()

	const numConnections = 10
	const numMessages = 100

	conns := make([]*Connection, numConnections)
	var readersWg sync.WaitGroup

	for i := 0; i < numConnections; i++ {
		conn := NewConnection(TransportWebSocket)
		state.AddConnection(conn)
		conns[i] = conn

		readersWg.Add(1)
		go func(c *Connection) {
			defer readersWg.Done()
			for range c.SendChan {
			}
		}(conn)
	}

	var wg sync.WaitGroup
	for i := 0; i < numMessages; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			state.Broadcast("message")
		}(i)
	}

	wg.Wait()

	for _, conn := range conns {
		state.RemoveConnection(conn.ID)
	}

	readersWg.Wait()
}

func createTestState(t *testing.T) (*ChatState, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "chat-test-*.jsonl")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()

	state, err := NewChatState(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("Failed to create chat state: %v", err)
	}

	cleanup := func() {
		state.Close()
		os.Remove(tmpFile.Name())
	}

	return state, cleanup
}
