package chat

import (
	"encoding/json"
	"html"
	"strconv"
	"time"
)

// Message represents a single chat message
type Message struct {
	ID        int64     `json:"id"`
	Nickname  string    `json:"nickname"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

// NewMessage creates a new message with HTML-escaped content
func NewMessage(id int64, nickname, text string) *Message {
	return &Message{
		ID:        id,
		Nickname:  html.EscapeString(nickname),
		Text:      html.EscapeString(text),
		Timestamp: time.Now(),
	}
}

// ClientMessage is the JSON wire format sent to clients
type ClientMessage struct {
	Type      string      `json:"type"`   // "message", "system", "usercount", "history"
	Action    string      `json:"action"` // "append", "prepend", "replace"
	Data      interface{} `json:"data"`
	Timestamp int64       `json:"timestamp"` // Unix milliseconds for client latency measurement
}

type MessageData struct {
	ID       string `json:"id"`
	Nickname string `json:"nickname"`
	Text     string `json:"text"`
	Time     string `json:"time"`
	IsSystem bool   `json:"isSystem,omitempty"`
	ClientID string `json:"clientId,omitempty"`
	ServerID string `json:"serverId,omitempty"`
}

// ToJSON converts the message to JSON format
func (m *Message) ToJSON() string {
	msg := ClientMessage{
		Type:      "message",
		Action:    "append",
		Timestamp: time.Now().UnixMilli(),
		Data: MessageData{
			ID:       strconv.FormatInt(m.ID, 10),
			Nickname: m.Nickname,
			Text:     m.Text,
			Time:     m.Timestamp.Format("15:04:05.000"),
		},
	}

	bytes, err := json.Marshal(msg)
	if err != nil {
		return `{"type":"error","data":"Failed to encode message"}`
	}
	return string(bytes)
}

// ToHTML renders the message as an HTML fragment (kept for compatibility)
func (m *Message) ToHTML() string {
	return `<div data-target="#messages" data-action="append">
  <div class="msg" data-id="` + formatInt64(m.ID) + `">
    <span class="nick">` + m.Nickname + `</span>
    <span class="text">` + m.Text + `</span>
    <time>` + m.Timestamp.Format("15:04:05.000") + `</time>
  </div>
</div>`
}

// SystemMessageJSON creates a system message in JSON format
func SystemMessageJSON(text string) string {
	msg := ClientMessage{
		Type:      "message",
		Action:    "append",
		Timestamp: time.Now().UnixMilli(),
		Data: MessageData{
			Text:     html.EscapeString(text),
			IsSystem: true,
		},
	}

	bytes, err := json.Marshal(msg)
	if err != nil {
		return `{"type":"error","data":"Failed to encode system message"}`
	}
	return string(bytes)
}

// SystemMessage creates a system message HTML fragment (kept for compatibility)
func SystemMessage(text string) string {
	return `<div data-target="#messages" data-action="append">
  <div class="sys">` + html.EscapeString(text) + `</div>
</div>`
}

// UserCountJSON creates a user count update in JSON format
func UserCountJSON(count int) string {
	plural := ""
	if count != 1 {
		plural = "s"
	}

	msg := ClientMessage{
		Type:      "usercount",
		Action:    "replace",
		Timestamp: time.Now().UnixMilli(),
		Data:      strconv.Itoa(count) + " user" + plural + " online",
	}

	bytes, err := json.Marshal(msg)
	if err != nil {
		return `{"type":"error","data":"Failed to encode user count"}`
	}
	return string(bytes)
}

// UserCountHTML creates a user count update HTML fragment (kept for compatibility)
func UserCountHTML(count int) string {
	plural := ""
	if count != 1 {
		plural = "s"
	}
	return `<span data-target="#user-count" data-action="replace">` +
		formatInt(count) + ` user` + plural + ` online</span>`
}

// Helper to format int64 to string without imports
func formatInt64(n int64) string {
	if n == 0 {
		return "0"
	}

	negative := n < 0
	if negative {
		n = -n
	}

	var buf [20]byte
	i := len(buf) - 1

	for n > 0 {
		buf[i] = byte('0' + n%10)
		n /= 10
		i--
	}

	if negative {
		buf[i] = '-'
		i--
	}

	return string(buf[i+1:])
}

// Helper to format int to string
func formatInt(n int) string {
	return formatInt64(int64(n))
}
