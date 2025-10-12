package handlers

import (
	"crypto/md5"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kurze/lab/internal/chat"
)

//go:embed index.html
var indexTemplate string

//go:embed chat.js
var chatJS string

//go:embed chat.css
var chatCSS string

// Precompute ETags for static resources at startup
var (
	chatJSETag  = fmt.Sprintf(`"%x"`, md5.Sum([]byte(chatJS)))
	chatCSSETag = fmt.Sprintf(`"%x"`, md5.Sum([]byte(chatCSS)))
)

// IndexHandler serves the main HTML page with streaming, HTTP/2 push, and Early Hints
func IndexHandler(state *chat.ChatState) http.HandlerFunc {
	// Split template into head and body for streaming
	headEndIdx := strings.Index(indexTemplate, "</head>")
	templateHead := indexTemplate[:headEndIdx+7] // Include </head>
	templateBodyStart := indexTemplate[headEndIdx+7:]
	messagesIdx := strings.Index(templateBodyStart, "{{MESSAGES}}")
	templateBeforeMessages := templateBodyStart[:messagesIdx]
	templateAfterMessages := templateBodyStart[messagesIdx+12:] // Skip {{MESSAGES}}

	return func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()

		// Send Early Hints (HTTP 103) for resources
		if r.ProtoMajor >= 2 {
			w.Header().Add("Link", "</chat.css>; rel=preload; as=style")
			w.Header().Add("Link", "</chat.js>; rel=preload; as=script")
			w.WriteHeader(103) // Early Hints

			// Clear headers for actual response
			w.Header().Del("Link")
		}

		// HTTP/2 Server Push for CSS and JS
		if pusher, ok := w.(http.Pusher); ok {
			// Push CSS first (render-blocking)
			pusher.Push("/chat.css", &http.PushOptions{
				Header: http.Header{
					"Content-Type": []string{"text/css; charset=utf-8"},
				},
			})
			// Then push JS
			pusher.Push("/chat.js", nil)
		}

		// Allocate nickname
		nickname := state.AllocateNickname()

		// Start streaming response
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Send head immediately
		html := strings.ReplaceAll(templateHead, "{{NICKNAME}}", nickname)
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, html)

		// Flush to client
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}

		// Stream body start
		io.WriteString(w, templateBeforeMessages)

		// Stream messages
		messages := state.GetLastN(10)
		for _, msg := range messages {
			io.WriteString(w, `<div class="msg" data-id="`)
			io.WriteString(w, formatInt64(msg.ID))
			io.WriteString(w, `"><span class="nick">`)
			io.WriteString(w, msg.Nickname)
			io.WriteString(w, `</span><span class="text">`)
			io.WriteString(w, msg.Text)
			io.WriteString(w, `</span><time>`)
			io.WriteString(w, msg.Timestamp.Format("15:04:05.000"))
			io.WriteString(w, `</time></div>
`)
		}

		// Stream rest of body
		body := strings.ReplaceAll(templateAfterMessages, "{{NICKNAME}}", nickname)
		io.WriteString(w, body)

		// Add Server-Timing header
		duration := time.Since(startTime).Milliseconds()
		w.Header().Set("Server-Timing", fmt.Sprintf("total;dur=%d", duration))
	}
}

// ChatJSHandler serves the chat.js file with ETag and compression support
func ChatJSHandler(state *chat.ChatState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check ETag
		if match := r.Header.Get("If-None-Match"); match == chatJSETag {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600, immutable")
		w.Header().Set("ETag", chatJSETag)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(chatJS))
	}
}

// ChatCSSHandler serves the chat.css file with ETag and compression support
func ChatCSSHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check ETag
		if match := r.Header.Get("If-None-Match"); match == chatCSSETag {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600, immutable")
		w.Header().Set("ETag", chatCSSETag)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(chatCSS))
	}
}

// Helper to format int64 to string (unchanged)
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
