package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kurze/lab/internal/chat"
)

func createEchoMessage(msg *chat.Message, clientID string) string {
	echoData := chat.MessageData{
		ID:       strconv.FormatInt(msg.ID, 10),
		Nickname: msg.Nickname,
		Text:     msg.Text,
		Time:     msg.Timestamp.Format("15:04:05.000"),
		ClientID: clientID,
		ServerID: strconv.FormatInt(msg.ID, 10),
	}

	echoMsg := chat.ClientMessage{
		Type:      "message",
		Action:    "append",
		Timestamp: time.Now().UnixMilli(),
		Data:      echoData,
	}

	bytes, err := json.Marshal(echoMsg)
	if err != nil {
		return `{"type":"error","data":"Failed to encode echo message"}`
	}
	return string(bytes)
}

func WebSocketHandler(state *chat.ChatState, checkOrigin func(*http.Request) bool, quiet bool) http.HandlerFunc {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     checkOrigin,
	}

	return func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade error: %v", err)
			return
		}

		conn := chat.NewConnection(chat.TransportWebSocket)
		state.AddConnection(conn)

		if !quiet {
			log.Printf("New WebSocket connection: %s", conn.ID)
		}

		go wsWriter(ws, conn)

		wsReader(ws, conn, state, quiet)
	}
}

// wsReader reads messages from the WebSocket
func wsReader(ws *websocket.Conn, conn *chat.Connection, state *chat.ChatState, quiet bool) {
	defer func() {
		// Remove connection and get nickname if this is the first cleanup
		nickname := state.RemoveConnection(conn.ID)
		ws.Close()

		// Only broadcast if we actually removed it (prevents duplicates)
		if nickname != "" {
			// Broadcast user left (JSON)
			sysMsg := chat.SystemMessageJSON(nickname + " left the chat")
			state.Broadcast(sysMsg)

			// Update user count (JSON)
			count := state.ConnectionCount()
			state.Broadcast(chat.UserCountJSON(count))

			if !quiet {
				log.Printf("WebSocket user %s disconnected (%s)", nickname, conn.ID)
			}
		} else {
			if !quiet {
				log.Printf("WebSocket connection already cleaned up: %s", conn.ID)
			}
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
		handleClientMessage(string(message), conn, state, quiet)
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
func handleClientMessage(data string, conn *chat.Connection, state *chat.ChatState, quiet bool) {
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

		// Broadcast system message (JSON)
		sysMsg := chat.SystemMessageJSON(nickname + " joined the chat")
		state.Broadcast(sysMsg)

		// Update user count (JSON)
		count := state.ConnectionCount()
		state.Broadcast(chat.UserCountJSON(count))

		if !quiet {
			log.Printf("User %s joined (conn: %s)", nickname, conn.ID)
		}

	case "SEND":
		if len(parts) < 3 {
			return
		}
		nickname := parts[1]
		text := parts[2]

		var clientID string
		if len(parts) >= 4 {
			clientID = parts[3]
		}

		msg := state.AddMessage(nickname, text)

		if clientID != "" {
			echoMsg := createEchoMessage(msg, clientID)
			conn.Send(echoMsg)
		}

		jsonMsg := msg.ToJSON()
		state.BroadcastExcept(jsonMsg, conn.ID)

		if !quiet {
			log.Printf("Message from %s: %s", nickname, text)
		}

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

		const maxHistoryTake = 100
		const maxHistorySkip = 10000
		if take > maxHistoryTake {
			take = maxHistoryTake
		}
		if skip > maxHistorySkip {
			skip = maxHistorySkip
		}

		// Get history
		messages := state.GetHistory(skip, take)

		// Send history as JSON with prepend action
		if len(messages) > 0 {
			historyData := make([]chat.MessageData, len(messages))
			for i, msg := range messages {
				historyData[i] = chat.MessageData{
					ID:       strconv.FormatInt(msg.ID, 10),
					Nickname: msg.Nickname,
					Text:     msg.Text,
					Time:     msg.Timestamp.Format("15:04:05.000"),
				}
			}

			historyMsg := chat.ClientMessage{
				Type:      "history",
				Action:    "prepend",
				Timestamp: time.Now().UnixMilli(),
				Data:      historyData,
			}

			jsonBytes, _ := json.Marshal(historyMsg)
			conn.Send(string(jsonBytes))
		}

	case "PING":
		// Keep-alive ping, no response needed
	}
}
