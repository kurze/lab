# Fastest Chat App POC

A proof-of-concept chat application optimized for minimal latency and fastest time-to-interactive, featuring HTTP/3 + WebTransport with WebSocket fallback.

## Features

- ⚡ **HTTP/2 & HTTP/3 dual-stack** - Both protocols on same port (8443)
- 📢 **Alt-Svc header** - Automatic HTTP/3 upgrade for supporting browsers
- 🚀 **WebTransport** for modern browsers (with datagram support)
- 🔄 **WebSocket fallback** - Works on both HTTP/2 and HTTP/3
- 📦 **Server-side HTML rendering** (no client-side templating)
- 💾 **In-memory circular buffer** (100 messages max)
- 🔒 **HTML escaping** for XSS protection
- 📊 **Client-side latency measurement**
- 👥 **User count tracking**
- 💬 **System messages** (join/leave notifications)
- 📜 **Message history** (10 inline + 90 on-demand)

## Performance Targets

| Metric | Target |
|--------|--------|
| Time to Interactive | < 100ms |
| Message Round-Trip | < 50ms (localhost) |
| Server Memory | < 10MB for 1000 users |
| Concurrent Users | 1000+ on single instance |

## Quick Start

### 1. Add /etc/hosts entry

```bash
echo "127.0.0.1 chat.local" | sudo tee -a /etc/hosts
```

### 2. Build and run

```bash
cd /home/simon/code/lab/fast-chat
go build -o bin/server ./cmd/server
./bin/server
```

### 3. Open in browser

Navigate to: **https://chat.local:8443**

You'll need to accept the self-signed certificate warning in your browser.

### 4. Test with multiple tabs/browsers

Open multiple browser tabs or different browsers to see the real-time chat in action!

## Architecture

### Dual-Stack Server

The server runs **both HTTP/2 (TCP) and HTTP/3 (UDP) simultaneously on port 8443**:

```
Port 8443:
├── TCP → HTTP/2 server (TLS with h2 ALPN)
│   ├── Serves HTML pages
│   ├── WebSocket support
│   └── Sends Alt-Svc: h3=":8443"; ma=86400
│
└── UDP → HTTP/3 server (QUIC)
    ├── Serves HTML pages (faster!)
    ├── WebSocket over HTTP/3
    └── WebTransport with datagrams
```

### Connection Flow

```
1. First visit: Browser connects via HTTP/2 (TCP)
   ↓
2. Server responds with Alt-Svc header advertising HTTP/3
   ↓
3. Browser caches this for 24 hours (ma=86400)
   ↓
4. Next request: Browser tries HTTP/3 (UDP) first
   ↓
5. If HTTP/3 works: All future requests use HTTP/3
   If fails: Falls back to HTTP/2

WebSocket/WebTransport selection:
- Modern browsers (Chrome/Edge): Try WebTransport first
- Fallback: WebSocket (works on both HTTP/2 and HTTP/3)
```

### Why This Matters

- **Firefox users**: Get HTTP/2 immediately, upgrade to HTTP/3 after Alt-Svc
- **Chrome/Edge users**: Get HTTP/2 first visit, HTTP/3 + WebTransport after
- **No DNS changes needed**: Alt-Svc header handles HTTP/3 discovery
- **Same port**: Firewall-friendly, no additional ports to open

## Protocol

### Client → Server Messages

Plain text pipe-delimited format:

```
JOIN|{nickname}          - Join the chat
SEND|{nickname}|{text}   - Send a message
HISTORY|{count}          - Request history
PING                     - Keep-alive
```

### Server → Client Messages

HTML fragments with `data-*` attributes for DOM routing:

```html
<!-- New message (append) -->
<div data-target="#messages" data-action="append">
  <div class="msg" data-id="1234567890123">
    <span class="nick">Alice</span>
    <span class="text">Hello world</span>
    <time>14:23:45.123</time>
  </div>
</div>

<!-- User count update (replace) -->
<span data-target="#user-count" data-action="replace">42 users online</span>
```

## Project Structure

```
fast-chat/
├── cmd/
│   └── server/
│       └── main.go              # Server entry point
├── internal/
│   ├── chat/
│   │   ├── state.go            # In-memory circular buffer
│   │   ├── message.go          # Message types & HTML generation
│   │   └── connection.go       # Connection management
│   └── handlers/
│       ├── index.go            # Serve initial HTML
│       ├── index.html          # Chat UI template
│       ├── websocket.go        # WebSocket handler
│       └── webtransport.go     # WebTransport handler
├── certs/
│   ├── cert.pem                # Self-signed certificate
│   └── key.pem                 # Private key
├── bin/
│   └── server                  # Compiled binary
├── go.mod
└── README.md
```

## Technology Stack

- **Go 1.25+**
- **quic-go** - QUIC/HTTP3 implementation
- **webtransport-go** - WebTransport support
- **gorilla/websocket** - WebSocket fallback
- **google/uuid** - Connection IDs

## Testing

### Manual Testing

1. Open https://chat.local:8443 in multiple browser tabs
2. Enter different nicknames in each tab
3. Send messages and observe:
   - Real-time message delivery
   - Latency measurements (green text)
   - Join/leave notifications
   - User count updates

### Transport Detection

**First visit to https://chat.local:8443:**
```bash
# Check which protocol is used
curl -k -I https://chat.local:8443/ | grep -E "HTTP|Alt-Svc"
# Expected: HTTP/2 200
#           alt-svc: h3=":8443"; ma=86400

# Force HTTP/3 test
curl -k --http3 -I https://chat.local:8443/ | head -1
# Expected: HTTP/3 200
```

**Browser behavior:**
- **First connection**: Uses HTTP/2, receives Alt-Svc header
- **Second connection (within 24h)**: Automatically upgrades to HTTP/3
- **Chrome/Edge 97+**: Will also try WebTransport for /chat endpoint
- **Firefox/Safari**: Use HTTP/3 for pages, WebSocket for real-time
- Check connection status in the UI (bottom left corner)

### Performance Testing

Monitor server logs for connection types:
```
2025-10-11 16:50:01 New WebTransport connection: 123e4567-e89b-12d3-a456-426614174000
2025-10-11 16:50:02 User Alice joined (conn: 123e4567-e89b-12d3-a456-426614174000)
2025-10-11 16:50:03 Message from Alice: Hello!
```

## Browser Compatibility

| Browser | WebTransport | WebSocket | HTTP/3 |
|---------|--------------|-----------|--------|
| Chrome 97+ | ✅ | ✅ | ✅ |
| Edge 97+ | ✅ | ✅ | ✅ |
| Firefox | ❌ | ✅ | ✅ |
| Safari | ❌ | ✅ | ✅ |

## Security Notes

⚠️ **This is a POC** - not production ready:

- Self-signed certificate (browsers will warn)
- No authentication/authorization
- No rate limiting implemented
- CORS allows all origins
- No persistent storage

## Future Enhancements

- [ ] Multiple chat rooms
- [ ] Typing indicators
- [ ] Message editing/deletion
- [ ] File uploads
- [ ] Persistent storage (SQLite/PostgreSQL)
- [ ] Rate limiting per connection
- [ ] User authentication
- [ ] Message reactions
- [ ] Read receipts

## Performance Optimization Tips

1. **Enable HTTP/3** in your reverse proxy (nginx/caddy)
2. **Use QUIC 0-RTT** for even faster reconnections
3. **Implement connection pooling** for database operations
4. **Add Redis** for distributed state across multiple instances
5. **Use binary protocol** instead of text for lower overhead

## License

MIT

## References

- [HTTP/3 Explained](https://http3-explained.haxx.se/)
- [WebTransport Spec](https://www.w3.org/TR/webtransport/)
- [htmz Documentation](https://leanrada.com/htmz/)
- [Quinn QUIC Implementation](https://github.com/quinn-rs/quinn)
- [quic-go](https://github.com/quic-go/quic-go)
