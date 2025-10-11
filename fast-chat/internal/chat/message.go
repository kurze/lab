package chat

import (
	"html"
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

// ToHTML renders the message as an HTML fragment
func (m *Message) ToHTML() string {
	return `<div data-target="#messages" data-action="append">
  <div class="msg" data-id="` + formatInt64(m.ID) + `">
    <span class="nick">` + m.Nickname + `</span>
    <span class="text">` + m.Text + `</span>
    <time>` + m.Timestamp.Format("15:04:05.000") + `</time>
  </div>
</div>`
}

// SystemMessage creates a system message HTML fragment
func SystemMessage(text string) string {
	return `<div data-target="#messages" data-action="append">
  <div class="sys">` + html.EscapeString(text) + `</div>
</div>`
}

// UserCountHTML creates a user count update HTML fragment
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
