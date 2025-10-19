package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

type ValidationScenario struct{}

func (s *ValidationScenario) Name() string {
	return "validation"
}

func (s *ValidationScenario) Description() string {
	return "Validates all chat functionality (JOIN, SEND, HISTORY, PING, message broadcast)"
}

func (s *ValidationScenario) Run(pool *ClientPool, duration time.Duration) error {
	clients := pool.GetClients()
	if len(clients) < 3 {
		return fmt.Errorf("validation requires at least 3 clients, got %d", len(clients))
	}

	log.Printf("Starting validation tests...")

	if err := s.validateJoin(clients); err != nil {
		return fmt.Errorf("JOIN validation failed: %w", err)
	}

	if err := s.validateSend(clients); err != nil {
		return fmt.Errorf("SEND validation failed: %w", err)
	}

	if err := s.validateBroadcast(clients); err != nil {
		return fmt.Errorf("BROADCAST validation failed: %w", err)
	}

	if err := s.validateHistory(clients[0]); err != nil {
		return fmt.Errorf("HISTORY validation failed: %w", err)
	}

	if err := s.validatePing(clients[0]); err != nil {
		return fmt.Errorf("PING validation failed: %w", err)
	}

	log.Printf("✓ All validation tests passed")
	return nil
}

func (s *ValidationScenario) validateJoin(clients []*BenchClient) error {
	log.Printf("Testing JOIN...")

	client := clients[0]
	pool := client.metrics

	pool.validationMu.Lock()
	pool.rawMessages = pool.rawMessages[:0]
	pool.validationMu.Unlock()

	time.Sleep(500 * time.Millisecond)

	pool.validationMu.Lock()
	messages := pool.rawMessages
	pool.validationMu.Unlock()

	foundUserList := false
	for _, msg := range messages {
		var envelope struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(msg, &envelope); err != nil {
			continue
		}

		if envelope.Type == "userList" {
			var data struct {
				Users []string `json:"users"`
			}
			if err := json.Unmarshal(envelope.Data, &data); err == nil {
				if len(data.Users) > 0 {
					foundUserList = true
					log.Printf("  ✓ Received user list with %d users", len(data.Users))
					break
				}
			}
		}
	}

	if !foundUserList {
		return fmt.Errorf("did not receive user list after JOIN")
	}

	return nil
}

func (s *ValidationScenario) validateSend(clients []*BenchClient) error {
	log.Printf("Testing SEND...")

	client := clients[0]
	pool := client.metrics

	pool.validationMu.Lock()
	pool.rawMessages = pool.rawMessages[:0]
	pool.validationMu.Unlock()

	testMessage := fmt.Sprintf("validation-test-%d", time.Now().UnixNano())
	clientID, err := client.SendMessage(testMessage)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	time.Sleep(500 * time.Millisecond)

	pool.validationMu.Lock()
	messages := pool.rawMessages
	pool.validationMu.Unlock()

	foundEcho := false
	for _, msg := range messages {
		var envelope struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(msg, &envelope); err != nil {
			continue
		}

		if envelope.Type == "message" {
			var data struct {
				ClientID string `json:"clientId"`
				Text     string `json:"text"`
			}
			if err := json.Unmarshal(envelope.Data, &data); err == nil {
				if data.ClientID == clientID && strings.Contains(data.Text, testMessage) {
					foundEcho = true
					log.Printf("  ✓ Received echo of sent message (clientId: %s)", clientID)
					break
				}
			}
		}
	}

	if !foundEcho {
		return fmt.Errorf("did not receive echo of sent message")
	}

	return nil
}

func (s *ValidationScenario) validateBroadcast(clients []*BenchClient) error {
	log.Printf("Testing BROADCAST...")

	sender := clients[0]
	receiver := clients[1]
	pool := sender.metrics

	pool.validationMu.Lock()
	pool.rawMessages = pool.rawMessages[:0]
	pool.validationMu.Unlock()

	testMessage := fmt.Sprintf("broadcast-test-%d", time.Now().UnixNano())
	clientID, err := sender.SendMessage(testMessage)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	time.Sleep(500 * time.Millisecond)

	pool.validationMu.Lock()
	messages := pool.rawMessages
	pool.validationMu.Unlock()

	foundBroadcast := false
	for _, msg := range messages {
		var envelope struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(msg, &envelope); err != nil {
			continue
		}

		if envelope.Type == "message" {
			var data struct {
				ClientID string `json:"clientId"`
				Text     string `json:"text"`
				Nickname string `json:"nickname"`
			}
			if err := json.Unmarshal(envelope.Data, &data); err == nil {
				if data.ClientID == clientID && data.Nickname == sender.nickname {
					foundBroadcast = true
					log.Printf("  ✓ Message broadcast to all clients (sender: %s, receiver: %s)", sender.nickname, receiver.nickname)
					break
				}
			}
		}
	}

	if !foundBroadcast {
		return fmt.Errorf("message was not broadcast to other clients")
	}

	return nil
}

func (s *ValidationScenario) validateHistory(client *BenchClient) error {
	log.Printf("Testing HISTORY...")

	pool := client.metrics

	pool.validationMu.Lock()
	pool.rawMessages = pool.rawMessages[:0]
	pool.validationMu.Unlock()

	if err := client.RequestHistory(0, 10); err != nil {
		return fmt.Errorf("failed to request history: %w", err)
	}

	time.Sleep(500 * time.Millisecond)

	pool.validationMu.Lock()
	messages := pool.rawMessages
	pool.validationMu.Unlock()

	foundHistory := false
	for _, msg := range messages {
		var envelope struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(msg, &envelope); err != nil {
			continue
		}

		if envelope.Type == "history" {
			var data struct {
				Messages []interface{} `json:"messages"`
				Skip     int           `json:"skip"`
				Take     int           `json:"take"`
			}
			if err := json.Unmarshal(envelope.Data, &data); err == nil {
				foundHistory = true
				log.Printf("  ✓ Received history response (skip: %d, take: %d, messages: %d)",
					data.Skip, data.Take, len(data.Messages))
				break
			}
		}
	}

	if !foundHistory {
		return fmt.Errorf("did not receive history response")
	}

	return nil
}

func (s *ValidationScenario) validatePing(client *BenchClient) error {
	log.Printf("Testing PING...")

	pool := client.metrics

	pool.validationMu.Lock()
	pool.rawMessages = pool.rawMessages[:0]
	pool.validationMu.Unlock()

	if err := client.Ping(); err != nil {
		return fmt.Errorf("failed to send ping: %w", err)
	}

	time.Sleep(500 * time.Millisecond)

	pool.validationMu.Lock()
	messages := pool.rawMessages
	pool.validationMu.Unlock()

	foundPong := false
	for _, msg := range messages {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(msg, &envelope); err != nil {
			continue
		}

		if envelope.Type == "pong" {
			foundPong = true
			log.Printf("  ✓ Received PONG response")
			break
		}
	}

	if !foundPong {
		return fmt.Errorf("did not receive PONG response")
	}

	return nil
}
