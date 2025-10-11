package handlers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/quic-go/webtransport-go"
	"github.com/kurze/lab/internal/chat"
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start writer goroutine for datagrams
	go wtWriter(ctx, session, conn)

	// Handle incoming datagrams
	wtReader(ctx, session, conn, state)
}

// wtReader reads datagrams from WebTransport
func wtReader(ctx context.Context, session *webtransport.Session, conn *chat.Connection, state *chat.ChatState) {
	defer func() {
		nickname := conn.GetNickname()
		state.RemoveConnection(conn.ID)

		if nickname != "" {
			// Broadcast user left
			sysMsg := chat.SystemMessage(nickname + " left the chat")
			state.Broadcast(sysMsg)

			// Update user count
			count := state.ConnectionCount()
			state.Broadcast(chat.UserCountHTML(count))
		}

		log.Printf("WebTransport connection closed: %s", conn.ID)
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
			log.Printf("WebTransport receive error: %v", err)
			return
		}

		conn.UpdateLastSeen()
		handleClientMessage(string(data), conn, state)
	}
}

// wtWriter writes datagrams to WebTransport
func wtWriter(ctx context.Context, session *webtransport.Session, conn *chat.Connection) {
	for {
		select {
		case message, ok := <-conn.SendChan:
			if !ok {
				return
			}

			err := session.SendDatagram([]byte(message))

			if err != nil {
				log.Printf("WebTransport send error: %v", err)
				return
			}

		case <-ctx.Done():
			return
		}
	}
}
