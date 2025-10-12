package handlers

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kurze/lab/internal/chat"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for POC
	},
}

// WebSocketHandler handles WebSocket connections
func WebSocketHandler(state *chat.ChatState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade error: %v", err)
			return
		}

		conn := chat.NewConnection(chat.TransportWebSocket)
		state.AddConnection(conn)

		log.Printf("New WebSocket connection: %s", conn.ID)

		// Start writer goroutine
		go wsWriter(ws, conn)

		// Handle incoming messages
		wsReader(ws, conn, state)
	}
}

// wsReader reads messages from the WebSocket
func wsReader(ws *websocket.Conn, conn *chat.Connection, state *chat.ChatState) {
	defer func() {
		// Remove connection and get nickname if this is the first cleanup
		nickname := state.RemoveConnection(conn.ID)
		ws.Close()

		// Only broadcast if we actually removed it (prevents duplicates)
		if nickname != "" {
			// Broadcast user left
			sysMsg := chat.SystemMessage(nickname + " left the chat")
			state.Broadcast(sysMsg)

			// Update user count
			count := state.ConnectionCount()
			state.Broadcast(chat.UserCountHTML(count))

			log.Printf("WebSocket user %s disconnected (%s)", nickname, conn.ID)
		} else {
			log.Printf("WebSocket connection already cleaned up: %s", conn.ID)
		}
	}()

	ws.SetReadDeadline(time.Now().Add(60 * time.Second))
	ws.SetPongHandler(func(string) error {
		ws.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		conn.UpdateLastSeen()
		handleClientMessage(string(message), conn, state)
	}
}

// wsWriter writes messages to the WebSocket
func wsWriter(ws *websocket.Conn, conn *chat.Connection) {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		ws.Close()
	}()

	for {
		select {
		case message, ok := <-conn.SendChan:
			if !ok {
				ws.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := ws.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
				return
			}

		case <-ticker.C:
			ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleClientMessage processes incoming client messages
func handleClientMessage(data string, conn *chat.Connection, state *chat.ChatState) {
	parts := strings.Split(data, "|")
	if len(parts) == 0 {
		return
	}

	command := parts[0]

	switch command {
	case "JOIN":
		if len(parts) < 2 {
			return
		}
		nickname := parts[1]
		conn.SetNickname(nickname)

		// Broadcast system message
		sysMsg := chat.SystemMessage(nickname + " joined the chat")
		state.Broadcast(sysMsg)

		// Update user count
		count := state.ConnectionCount()
		state.Broadcast(chat.UserCountHTML(count))

		log.Printf("User %s joined (conn: %s)", nickname, conn.ID)

	case "SEND":
		if len(parts) < 3 {
			return
		}
		nickname := parts[1]
		text := parts[2]

		// Add message and broadcast
		msg := state.AddMessage(nickname, text)
		html := msg.ToHTML()
		state.Broadcast(html)

		log.Printf("Message from %s: %s", nickname, text)

	case "HISTORY":
		if len(parts) < 2 {
			return
		}

		// Support both old format (HISTORY|count) and new format (HISTORY|skip|take)
		var skip, take int
		var err error

		if len(parts) >= 3 {
			// New format: HISTORY|skip|take
			skip, err = strconv.Atoi(parts[1])
			if err != nil || skip < 0 {
				return
			}
			take, err = strconv.Atoi(parts[2])
			if err != nil || take <= 0 {
				return
			}
		} else {
			// Old format: HISTORY|count (for backward compatibility)
			take, err = strconv.Atoi(parts[1])
			if err != nil || take <= 0 {
				return
			}
			skip = 10 // Skip initial 10
		}

		// Get history
		messages := state.GetHistory(skip, take)

		// Send history as prepend fragment
		if len(messages) > 0 {
			var html strings.Builder
			html.WriteString(`<div data-target="#messages" data-action="prepend">`)
			for _, msg := range messages {
				html.WriteString(`
  <div class="msg" data-id="`)
				html.WriteString(formatInt64(msg.ID))
				html.WriteString(`">
    <span class="nick">`)
				html.WriteString(msg.Nickname)
				html.WriteString(`</span>
    <span class="text">`)
				html.WriteString(msg.Text)
				html.WriteString(`</span>
    <time>`)
				html.WriteString(msg.Timestamp.Format("15:04:05.000"))
				html.WriteString(`</time>
  </div>`)
			}
			html.WriteString(`
</div>`)

			conn.Send(html.String())
		}

	case "PING":
		// Keep-alive ping, no response needed
	}
}
