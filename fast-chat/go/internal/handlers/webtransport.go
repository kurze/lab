package handlers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/kurze/lab/internal/chat"
	"github.com/quic-go/webtransport-go"
)

// WebTransportHandler creates a WebTransport handler
func WebTransportHandler(state *chat.ChatState, wtServer *webtransport.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, err := wtServer.Upgrade(w, r)
		if err != nil {
			log.Printf("WebTransport upgrade failed: %v", err)
			http.Error(w, "WebTransport upgrade failed", http.StatusInternalServerError)
			return
		}

		HandleWebTransportSession(session, state)
	}
}

// HandleWebTransportSession handles a WebTransport session
func HandleWebTransportSession(session *webtransport.Session, state *chat.ChatState) {
	conn := chat.NewConnection(chat.TransportWebTransport)
	state.AddConnection(conn)

	log.Printf("New WebTransport connection: %s", conn.ID)

	// Use session context to detect when client disconnects
	ctx := session.Context()

	// Start writer goroutine for datagrams
	go wtWriter(ctx, session, conn)

	// Handle incoming datagrams (blocks until connection closes)
	wtReader(ctx, session, conn, state)
}

// wtReader reads datagrams from WebTransport
func wtReader(ctx context.Context, session *webtransport.Session, conn *chat.Connection, state *chat.ChatState) {
	defer func() {
		// Remove connection and get nickname if this is the first cleanup
		nickname := state.RemoveConnection(conn.ID)

		// Only broadcast if we actually removed it (prevents duplicates)
		if nickname != "" {
			// Broadcast user left (JSON)
			sysMsg := chat.SystemMessageJSON(nickname + " left the chat")
			state.Broadcast(sysMsg)

			// Update user count (JSON)
			count := state.ConnectionCount()
			state.Broadcast(chat.UserCountJSON(count))

			log.Printf("WebTransport user %s disconnected (%s)", nickname, conn.ID)
		} else {
			log.Printf("WebTransport connection already cleaned up: %s", conn.ID)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Read datagram with timeout
		readCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		data, err := session.ReceiveDatagram(readCtx)
		cancel()

		if err != nil {
			if err == context.DeadlineExceeded {
				// Timeout, check if connection is still alive
				continue
			}
			log.Printf("WebTransport receive error for %s: %v", conn.ID, err)
			return
		}

		conn.UpdateLastSeen()
		msg := string(data)
		log.Printf("WebTransport received from %s: %s", conn.ID, msg[:min(50, len(msg))])
		handleClientMessage(msg, conn, state)
	}
}

// wtWriter writes datagrams to WebTransport
func wtWriter(ctx context.Context, session *webtransport.Session, conn *chat.Connection) {
	const maxRetries = 3
	const retryDelay = 50 * time.Millisecond

	for {
		select {
		case message, ok := <-conn.SendChan:
			if !ok {
				log.Printf("WebTransport send channel closed for %s", conn.ID)
				return
			}

			log.Printf("WebTransport sending to %s: %s", conn.ID, message[:min(50, len(message))])

			isCritical := len(message) > 0 && (message[0] == '{' ||
				(len(message) > 4 && (message[:4] == "JOIN" || message[:4] == "HIST")))

			var err error
			if isCritical {
				for attempt := 0; attempt < maxRetries; attempt++ {
					err = session.SendDatagram([]byte(message))
					if err == nil {
						break
					}
					if attempt < maxRetries-1 {
						time.Sleep(retryDelay * time.Duration(attempt+1))
					}
				}
				if err != nil {
					log.Printf("WebTransport critical message failed after %d retries for %s: %v", maxRetries, conn.ID, err)
				}
			} else {
				err = session.SendDatagram([]byte(message))
				if err != nil {
					log.Printf("WebTransport send error for %s: %v", conn.ID, err)
				}
			}

		case <-ctx.Done():
			log.Printf("WebTransport writer context done for %s", conn.ID)
			return
		}
	}
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
