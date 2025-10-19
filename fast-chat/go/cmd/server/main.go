package main

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/kurze/lab/internal/chat"
	"github.com/kurze/lab/internal/handlers"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
)

const (
	defaultConfigFile = "go-config.json"
)

func checkOrigin(allowedOrigins []string) func(*http.Request) bool {
	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")

		if origin == "" {
			return true
		}

		for _, allowed := range allowedOrigins {
			if origin == allowed {
				return true
			}
		}

		log.Printf("Rejected connection from origin: %s", origin)
		return false
	}
}

func main() {
	config, err := LoadConfig(defaultConfigFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := config.Validate(); err != nil {
		log.Fatalf("%v", err)
	}

	if err := os.MkdirAll("logs", 0755); err != nil {
		log.Fatalf("Failed to create logs directory: %v", err)
	}

	state, err := chat.NewChatState(config.Logging.MessageLogFile, config.Logging.LogFlushInterval.ToDuration())
	if err != nil {
		log.Fatalf("Failed to create chat state: %v", err)
	}
	defer state.Close()
	log.Println("Chat state initialized")

	cert, err := tls.LoadX509KeyPair(config.TLS.CertFile, config.TLS.KeyFile)
	if err != nil {
		log.Fatalf("Failed to load TLS certificate: %v", err)
	}

	// TLS config for HTTP/2 (with h2 and http/1.1 ALPN)
	tlsConfigH2 := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h2", "http/1.1"},
	}

	// TLS config for HTTP/3 with session ticket caching for 0-RTT
	tlsConfigH3 := &tls.Config{
		Certificates:           []tls.Certificate{cert},
		NextProtos:             []string{"h3"},
		SessionTicketsDisabled: false, // Enable session tickets for 0-RTT
		ClientSessionCache:     tls.NewLRUClientSessionCache(128),
	}

	quicConfig := &quic.Config{
		EnableDatagrams: true,
		MaxIdleTimeout:  config.Timeouts.MaxIdleTimeout.ToDuration(),
		Allow0RTT:       true,
	}

	originChecker := checkOrigin(config.Security.AllowedOrigins)

	wtServer := &webtransport.Server{
		H3: http3.Server{
			Addr:       config.Server.H3Addr,
			TLSConfig:  tlsConfigH3,
			QUICConfig: quicConfig,
		},
		CheckOrigin: originChecker,
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

	// Static resources (embedded, with HTTP/2 push support, ETags, and compression)
	mux.HandleFunc("/chat.css", handlers.ChatCSSHandler())
	mux.HandleFunc("/chat.js", handlers.ChatJSHandler(state))

	// Favicon handler
	mux.HandleFunc("/favicon.ico", handlers.FaviconHandler())

	mux.HandleFunc("/ws", handlers.WebSocketHandler(state, originChecker, config.Logging.Quiet))

	// WebTransport handler (HTTP/3 only)
	// WebTransport uses CONNECT method and should NOT have compression middleware
	mux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		session, err := wtServer.Upgrade(w, r)
		if err != nil {
			log.Printf("WebTransport upgrade failed: %v", err)
			http.Error(w, "WebTransport upgrade failed", http.StatusInternalServerError)
			return
		}
		handlers.HandleWebTransportSession(session, state, config.Logging.Quiet)
	})

	// Wrap with Alt-Svc middleware (but NOT compression for WebTransport)
	// We need a selective middleware that skips compression for WebTransport
	handlerWithMiddleware := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always add Alt-Svc header
		w.Header().Set("Alt-Svc", `h3=":8443"; ma=86400`)

		// Skip compression for WebTransport CONNECT requests
		if r.Method == "CONNECT" && r.URL.Path == "/chat" {
			mux.ServeHTTP(w, r)
			return
		}

		// Apply compression for all other requests
		handlers.GzipMiddleware(mux).ServeHTTP(w, r)
	})

	// Set handler for HTTP/3 server
	wtServer.H3.Handler = handlerWithMiddleware

	h2Server := &http.Server{
		Addr:      config.Server.H2Addr,
		TLSConfig: tlsConfigH2,
		Handler:   handlerWithMiddleware,
	}

	log.Printf("Starting HTTP/2 server (TCP) on https://chat.local%s", config.Server.H2Addr)
	log.Printf("Starting HTTP/3 server (UDP) on https://chat.local%s", config.Server.H3Addr)
	log.Printf("WebSocket available at wss://chat.local%s/ws", config.Server.H2Addr)
	log.Printf("WebTransport available at https://chat.local%s/chat (HTTP/3 only)", config.Server.H3Addr)
	log.Println("Add '127.0.0.1 chat.local' to /etc/hosts if needed")
	log.Println("Alt-Svc header will advertise HTTP/3 to browsers")

	serverErr := make(chan error, 2)

	go func() {
		log.Println("HTTP/2 server starting...")
		serverErr <- h2Server.ListenAndServeTLS(config.TLS.CertFile, config.TLS.KeyFile)
	}()

	go func() {
		log.Println("HTTP/3 server starting...")
		serverErr <- wtServer.ListenAndServeTLS(config.TLS.CertFile, config.TLS.KeyFile)
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		log.Fatalf("Server error: %v", err)
	case <-sigChan:
		log.Println("Shutting down servers...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), config.Timeouts.ShutdownTimeout.ToDuration())
		defer cancel()

		if err := h2Server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP/2 server shutdown error: %v", err)
		}

		if err := wtServer.Close(); err != nil {
			log.Printf("HTTP/3 server shutdown error: %v", err)
		}

		log.Println("Servers stopped")
	}
}
