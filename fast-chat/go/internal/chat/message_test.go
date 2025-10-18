package chat

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewMessage(t *testing.T) {
	msg := NewMessage(1, "alice", "hello world")

	if msg.ID != 1 {
		t.Errorf("Expected ID 1, got %d", msg.ID)
	}
	if msg.Nickname != "alice" {
		t.Errorf("Expected nickname 'alice', got '%s'", msg.Nickname)
	}
	if msg.Text != "hello world" {
		t.Errorf("Expected text 'hello world', got '%s'", msg.Text)
	}
	if msg.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestMessageHTMLEscaping(t *testing.T) {
	msg := NewMessage(1, "<script>alert('xss')</script>", "<img src=x onerror=alert(1)>")

	if strings.Contains(msg.Nickname, "<script>") {
		t.Error("Nickname should be HTML escaped")
	}
	if strings.Contains(msg.Text, "<img") {
		t.Error("Text should be HTML escaped")
	}

	if !strings.Contains(msg.Nickname, "&lt;") || !strings.Contains(msg.Nickname, "&gt;") {
		t.Error("Nickname should contain HTML entities")
	}
}

func TestMessageToJSON(t *testing.T) {
	msg := NewMessage(42, "bob", "test message")

	jsonStr := msg.ToJSON()

	var parsed ClientMessage
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if parsed.Type != "message" {
		t.Errorf("Expected type 'message', got '%s'", parsed.Type)
	}
	if parsed.Action != "append" {
		t.Errorf("Expected action 'append', got '%s'", parsed.Action)
	}

	data, ok := parsed.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Data should be a map")
	}

	if data["id"] != "42" {
		t.Errorf("Expected ID '42', got '%v'", data["id"])
	}
	if data["nickname"] != "bob" {
		t.Errorf("Expected nickname 'bob', got '%v'", data["nickname"])
	}
	if data["text"] != "test message" {
		t.Errorf("Expected text 'test message', got '%v'", data["text"])
	}
}

func TestSystemMessageJSON(t *testing.T) {
	jsonStr := SystemMessageJSON("user joined")

	var parsed ClientMessage
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if parsed.Type != "message" {
		t.Errorf("Expected type 'message', got '%s'", parsed.Type)
	}

	data, ok := parsed.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Data should be a map")
	}

	if data["text"] != "user joined" {
		t.Errorf("Expected text 'user joined', got '%v'", data["text"])
	}

	if isSystem, ok := data["isSystem"].(bool); !ok || !isSystem {
		t.Error("isSystem should be true")
	}
}

func TestSystemMessageJSONEscaping(t *testing.T) {
	jsonStr := SystemMessageJSON("<script>alert('xss')</script>")

	if strings.Contains(jsonStr, "<script>") {
		t.Error("System message should be HTML escaped")
	}
}

func TestUserCountJSON(t *testing.T) {
	tests := []struct {
		count    int
		expected string
	}{
		{0, "0 users online"},
		{1, "1 user online"},
		{2, "2 users online"},
		{100, "100 users online"},
	}

	for _, tt := range tests {
		jsonStr := UserCountJSON(tt.count)

		var parsed ClientMessage
		if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
			t.Fatalf("Failed to parse JSON for count %d: %v", tt.count, err)
		}

		if parsed.Type != "usercount" {
			t.Errorf("Expected type 'usercount', got '%s'", parsed.Type)
		}
		if parsed.Action != "replace" {
			t.Errorf("Expected action 'replace', got '%s'", parsed.Action)
		}

		data, ok := parsed.Data.(string)
		if !ok {
			t.Fatal("Data should be a string")
		}

		if data != tt.expected {
			t.Errorf("For count %d: expected '%s', got '%s'", tt.count, tt.expected, data)
		}
	}
}

func TestMessageTimestamp(t *testing.T) {
	before := time.Now()
	msg := NewMessage(1, "user", "text")
	after := time.Now()

	if msg.Timestamp.Before(before) || msg.Timestamp.After(after) {
		t.Error("Timestamp should be between before and after")
	}
}

func TestClientMessageTimestamp(t *testing.T) {
	msg := NewMessage(1, "user", "text")
	jsonStr := msg.ToJSON()

	var parsed ClientMessage
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if parsed.Timestamp == 0 {
		t.Error("Timestamp should not be zero")
	}

	now := time.Now().UnixMilli()
	diff := now - parsed.Timestamp
	if diff < 0 || diff > 1000 {
		t.Errorf("Timestamp diff too large: %d ms", diff)
	}
}

func TestMessageDataFields(t *testing.T) {
	msg := NewMessage(123, "alice", "hello")
	jsonStr := msg.ToJSON()

	var parsed ClientMessage
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatal(err)
	}

	data, ok := parsed.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Data should be a map")
	}

	requiredFields := []string{"id", "nickname", "text", "time"}
	for _, field := range requiredFields {
		if _, exists := data[field]; !exists {
			t.Errorf("Missing required field: %s", field)
		}
	}
}

func TestFormatInt64(t *testing.T) {
	msg1 := NewMessage(0, "user", "text")
	jsonStr1 := msg1.ToJSON()
	if !strings.Contains(jsonStr1, `"id":"0"`) {
		t.Error("Should format 0 correctly")
	}

	msg2 := NewMessage(9999999999, "user", "text")
	jsonStr2 := msg2.ToJSON()
	if !strings.Contains(jsonStr2, `"id":"9999999999"`) {
		t.Error("Should format large numbers correctly")
	}

	msg3 := NewMessage(-1, "user", "text")
	jsonStr3 := msg3.ToJSON()
	if !strings.Contains(jsonStr3, `"id":"-1"`) {
		t.Error("Should format negative numbers correctly")
	}
}

func TestMessageToHTML(t *testing.T) {
	msg := NewMessage(42, "alice", "hello")
	html := msg.ToHTML()

	if !strings.Contains(html, `data-id="42"`) {
		t.Error("HTML should contain data-id")
	}
	if !strings.Contains(html, "alice") {
		t.Error("HTML should contain nickname")
	}
	if !strings.Contains(html, "hello") {
		t.Error("HTML should contain text")
	}
	if !strings.Contains(html, `class="msg"`) {
		t.Error("HTML should contain message class")
	}
}

func TestSystemMessage(t *testing.T) {
	html := SystemMessage("test system message")

	if !strings.Contains(html, `class="sys"`) {
		t.Error("System message should have sys class")
	}
	if !strings.Contains(html, "test system message") {
		t.Error("System message should contain text")
	}
	if strings.Contains(html, `data-id`) {
		t.Error("System message should not have data-id")
	}
}

func TestUserCountHTML(t *testing.T) {
	html1 := UserCountHTML(1)
	if !strings.Contains(html1, "1 user online") {
		t.Errorf("Expected '1 user online', got: %s", html1)
	}

	html2 := UserCountHTML(5)
	if !strings.Contains(html2, "5 users online") {
		t.Errorf("Expected '5 users online', got: %s", html2)
	}
}
