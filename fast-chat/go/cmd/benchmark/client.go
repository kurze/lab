package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/webtransport-go"
)

type BenchClient struct {
	ws        *websocket.Conn
	wt        *webtransport.Session
	protocol  string
	nickname  string
	metrics   *Metrics
	stopChan  chan struct{}
	wg        sync.WaitGroup
	closed    uint32
	ctx       context.Context
	ctxCancel context.CancelFunc
}

func NewBenchClient(nickname string, protocol string, metrics *Metrics) *BenchClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &BenchClient{
		nickname:  nickname,
		protocol:  protocol,
		metrics:   metrics,
		stopChan:  make(chan struct{}),
		ctx:       ctx,
		ctxCancel: cancel,
	}
}

func (c *BenchClient) Connect(url string, insecure bool, certFile string) error {
	if c.protocol == "webtransport" || c.protocol == "http3" {
		return c.connectWebTransport(url, insecure, certFile)
	}
	return c.connectWebSocket(url, insecure, certFile)
}

func (c *BenchClient) connectWebSocket(url string, insecure bool, certFile string) error {
	startTime := time.Now()

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	if insecure {
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	} else if certFile != "" {
		certPool := x509.NewCertPool()
		certPEM, err := os.ReadFile(certFile)
		if err != nil {
			c.metrics.RecordConnectionFailure()
			return fmt.Errorf("failed to read cert file %s: %w", certFile, err)
		}
		if !certPool.AppendCertsFromPEM(certPEM) {
			c.metrics.RecordConnectionFailure()
			return fmt.Errorf("failed to parse cert from %s", certFile)
		}
		dialer.TLSClientConfig = &tls.Config{RootCAs: certPool}
	}

	ws, _, err := dialer.Dial(url, http.Header{})
	if err != nil {
		c.metrics.RecordConnectionFailure()
		return fmt.Errorf("dial failed: %w", err)
	}

	c.ws = ws
	duration := time.Since(startTime)
	c.metrics.RecordConnectionSuccess(duration)

	c.wg.Add(1)
	go c.readLoopWebSocket()

	return nil
}

func (c *BenchClient) connectWebTransport(url string, insecure bool, certFile string) error {
	startTime := time.Now()

	tlsConfig := &tls.Config{
		NextProtos: []string{"h3"},
	}
	if insecure {
		tlsConfig.InsecureSkipVerify = true
	} else if certFile != "" {
		certPool := x509.NewCertPool()
		certPEM, err := os.ReadFile(certFile)
		if err != nil {
			c.metrics.RecordConnectionFailure()
			return fmt.Errorf("failed to read cert file %s: %w", certFile, err)
		}
		if !certPool.AppendCertsFromPEM(certPEM) {
			c.metrics.RecordConnectionFailure()
			return fmt.Errorf("failed to parse cert from %s", certFile)
		}
		tlsConfig.RootCAs = certPool
	}

	dialer := webtransport.Dialer{
		TLSClientConfig: tlsConfig,
		QUICConfig: &quic.Config{
			MaxIdleTimeout:  60 * time.Second,
			EnableDatagrams: true,
		},
	}

	rsp, session, err := dialer.Dial(c.ctx, url, nil)
	if err != nil {
		c.metrics.RecordConnectionFailure()
		return fmt.Errorf("webtransport dial failed: %w", err)
	}
	if rsp.StatusCode != http.StatusOK {
		c.metrics.RecordConnectionFailure()
		return fmt.Errorf("webtransport upgrade failed: %d", rsp.StatusCode)
	}

	c.wt = session
	duration := time.Since(startTime)
	c.metrics.RecordConnectionSuccess(duration)

	c.wg.Add(1)
	go c.readLoopWebTransport()

	return nil
}

func (c *BenchClient) Join() error {
	msg := fmt.Sprintf("JOIN|%s", c.nickname)
	return c.sendRaw(msg)
}

func (c *BenchClient) SendMessage(text string) (string, error) {
	clientID := fmt.Sprintf("%s-%d", c.nickname, time.Now().UnixNano())
	msg := fmt.Sprintf("SEND|%s|%s|%s", c.nickname, text, clientID)

	if err := c.sendRaw(msg); err != nil {
		return "", err
	}

	c.metrics.RecordMessageSent(clientID, int64(len(msg)))
	return clientID, nil
}

func (c *BenchClient) RequestHistory(skip, take int) error {
	msg := fmt.Sprintf("HISTORY|%d|%d", skip, take)
	return c.sendRaw(msg)
}

func (c *BenchClient) Ping() error {
	return c.sendRaw("PING")
}

func (c *BenchClient) sendRaw(msg string) error {
	if atomic.LoadUint32(&c.closed) == 1 {
		return fmt.Errorf("client closed")
	}

	if c.protocol == "webtransport" || c.protocol == "http3" {
		return c.wt.SendDatagram([]byte(msg))
	}

	c.ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return c.ws.WriteMessage(websocket.TextMessage, []byte(msg))
}

func (c *BenchClient) readLoopWebSocket() {
	defer c.wg.Done()

	c.ws.SetReadDeadline(time.Time{})

	for {
		_, message, err := c.ws.ReadMessage()
		if err != nil {
			return
		}

		c.handleMessage(message)
	}
}

func (c *BenchClient) readLoopWebTransport() {
	defer c.wg.Done()

	for {
		data, err := c.wt.ReceiveDatagram(c.ctx)
		if err != nil {
			select {
			case <-c.stopChan:
				return
			case <-c.ctx.Done():
				return
			default:
				return
			}
		}

		c.handleMessage(data)
	}
}

func (c *BenchClient) handleMessage(data []byte) {
	c.metrics.RecordMessageReceived("", int64(len(data)))

	if c.metrics.ValidationMode {
		c.metrics.RecordRawMessage(data)
	}
}

func (c *BenchClient) Close() error {
	if atomic.SwapUint32(&c.closed, 1) == 1 {
		return nil
	}

	close(c.stopChan)
	c.ctxCancel()

	if c.ws != nil {
		c.ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		c.ws.Close()
	}
	if c.wt != nil {
		c.wt.CloseWithError(0, "")
	}

	c.wg.Wait()
	return nil
}

type ClientPool struct {
	clients  []*BenchClient
	metrics  *Metrics
	url      string
	protocol string
	insecure bool
	certFile string
	mu       sync.Mutex
}

func NewClientPool(url string, protocol string, insecure bool, certFile string, metrics *Metrics) *ClientPool {
	return &ClientPool{
		clients:  make([]*BenchClient, 0),
		metrics:  metrics,
		url:      url,
		protocol: protocol,
		insecure: insecure,
		certFile: certFile,
	}
}

func (p *ClientPool) CreateClients(count int) error {
	var wg sync.WaitGroup
	errChan := make(chan error, count)

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			nickname := fmt.Sprintf("bench-%d", idx)
			client := NewBenchClient(nickname, p.protocol, p.metrics)

			if err := client.Connect(p.url, p.insecure, p.certFile); err != nil {
				errChan <- err
				return
			}

			if err := client.Join(); err != nil {
				client.Close()
				errChan <- err
				return
			}

			p.mu.Lock()
			p.clients = append(p.clients, client)
			p.mu.Unlock()
		}(i)
	}

	wg.Wait()
	close(errChan)

	var errs []string
	for err := range errChan {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to create %d clients: %s", len(errs), strings.Join(errs[:min(5, len(errs))], "; "))
	}

	log.Printf("Created %d clients successfully", len(p.clients))
	return nil
}

func (p *ClientPool) GetClients() []*BenchClient {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.clients
}

func (p *ClientPool) CloseAll() {
	p.mu.Lock()
	clients := p.clients
	p.mu.Unlock()

	var wg sync.WaitGroup
	for _, client := range clients {
		wg.Add(1)
		go func(c *BenchClient) {
			defer wg.Done()
			c.Close()
		}(client)
	}
	wg.Wait()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func parseNicknameNumber(nickname string) int {
	parts := strings.Split(nickname, "-")
	if len(parts) < 2 {
		return 0
	}
	num, _ := strconv.Atoi(parts[len(parts)-1])
	return num
}
