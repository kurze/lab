package main

import (
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
	"github.com/kurze/lab/internal/chat"
	"github.com/kurze/lab/internal/handlers"
)

const (
	h2Addr   = ":8443"  // HTTP/2 on TCP
	h3Addr   = ":8443"  // HTTP/3 on UDP (same port, different protocol)
	certFile = "certs/cert.pem"
	keyFile  = "certs/key.pem"
	logFile  = "logs/messages.jsonl" // Message log file
)

// altSvcMiddleware adds Alt-Svc header to advertise HTTP/3 availability
func altSvcMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Advertise HTTP/3 on same port with 24h max-age
		w.Header().Set("Alt-Svc", `h3=":8443"; ma=86400`)
		next.ServeHTTP(w, r)
	})
}

func main() {
	// Ensure logs directory exists
	if err := os.MkdirAll("logs", 0755); err != nil {
		log.Fatalf("Failed to create logs directory: %v", err)
	}

	// Create chat state with message logging
	state, err := chat.NewChatState(logFile)
	if err != nil {
		log.Fatalf("Failed to create chat state: %v", err)
	}
	defer state.Close()
	log.Println("Chat state initialized")

	// Load TLS certificate
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Fatalf("Failed to load TLS certificate: %v", err)
	}

	// TLS config for HTTP/2 (with h2 and http/1.1 ALPN)
	tlsConfigH2 := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h2", "http/1.1"},
	}

	// TLS config for HTTP/3
	tlsConfigH3 := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h3"},
	}

	// Create HTTP/3 server with WebTransport support
	quicConfig := &quic.Config{
		EnableDatagrams: true,
		MaxIdleTimeout:  60 * time.Second,
	}

	// Create WebTransport server
	wtServer := &webtransport.Server{
		H3: http3.Server{
			Addr:       h3Addr,
			TLSConfig:  tlsConfigH3,
			QUICConfig: quicConfig,
		},
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for POC
		},
	}

	// Create mux for HTTP handlers
	mux := http.NewServeMux()

	// Index handler (only on exact "/" path)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		handlers.IndexHandler(state)(w, r)
	})

	// Favicon handler
	mux.HandleFunc("/favicon.ico", handlers.FaviconHandler())

	// WebSocket handler (works on both HTTP/2 and HTTP/3)
	mux.HandleFunc("/ws", handlers.WebSocketHandler(state))

	// WebTransport handler (HTTP/3 only)
	mux.HandleFunc("/chat", handlers.WebTransportHandler(state, wtServer))

	// Wrap mux with Alt-Svc middleware
	handlerWithAltSvc := altSvcMiddleware(mux)

	// Set handler for both servers
	wtServer.H3.Handler = handlerWithAltSvc

	// Create HTTP/2 server
	h2Server := &http.Server{
		Addr:      h2Addr,
		TLSConfig: tlsConfigH2,
		Handler:   handlerWithAltSvc,
	}

	// Start both servers
	log.Printf("Starting HTTP/2 server (TCP) on https://chat.local%s", h2Addr)
	log.Printf("Starting HTTP/3 server (UDP) on https://chat.local%s", h3Addr)
	log.Printf("WebSocket available at wss://chat.local%s/ws", h2Addr)
	log.Printf("WebTransport available at https://chat.local%s/chat (HTTP/3 only)", h3Addr)
	log.Println("Add '127.0.0.1 chat.local' to /etc/hosts if needed")
	log.Println("Alt-Svc header will advertise HTTP/3 to browsers")

	serverErr := make(chan error, 2)

	// Start HTTP/2 server
	go func() {
		log.Println("HTTP/2 server starting...")
		serverErr <- h2Server.ListenAndServeTLS(certFile, keyFile)
	}()

	// Start HTTP/3 server (UDP - will work on same port as HTTP/2 TCP)
	go func() {
		log.Println("HTTP/3 server starting...")
		listener, err := quic.ListenAddr(h3Addr, tlsConfigH3, quicConfig)
		if err != nil {
			serverErr <- err
			return
		}
		serverErr <- wtServer.H3.ServeListener(listener)
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		log.Fatalf("Server error: %v", err)
	case <-sigChan:
		log.Println("Shutting down servers...")
		log.Println("Servers stopped")
	}
}
