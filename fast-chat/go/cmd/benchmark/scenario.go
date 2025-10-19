package main

import (
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"
)

type Scenario interface {
	Name() string
	Description() string
	Run(pool *ClientPool, duration time.Duration) error
}

type ConnectionStormScenario struct{}

func (s *ConnectionStormScenario) Name() string {
	return "connection-storm"
}

func (s *ConnectionStormScenario) Description() string {
	return "All clients connect simultaneously and stay idle"
}

func (s *ConnectionStormScenario) Run(pool *ClientPool, duration time.Duration) error {
	log.Printf("Running connection storm scenario for %v", duration)

	time.Sleep(duration)

	log.Printf("Connection storm complete")
	return nil
}

type MessageFloodScenario struct {
	Rate int
}

func (s *MessageFloodScenario) Name() string {
	return "message-flood"
}

func (s *MessageFloodScenario) Description() string {
	return fmt.Sprintf("All clients send messages at maximum rate (%d msgs/sec per client)", s.Rate)
}

func (s *MessageFloodScenario) Run(pool *ClientPool, duration time.Duration) error {
	log.Printf("Running message flood scenario for %v (%d msgs/sec per client)", duration, s.Rate)

	clients := pool.GetClients()
	var wg sync.WaitGroup
	stopChan := make(chan struct{})

	for _, client := range clients {
		wg.Add(1)
		go func(c *BenchClient) {
			defer wg.Done()

			interval := time.Second / time.Duration(s.Rate)
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			msgNum := 0
			for {
				select {
				case <-stopChan:
					return
				case <-ticker.C:
					text := fmt.Sprintf("Flood message %d", msgNum)
					c.SendMessage(text)
					msgNum++
				}
			}
		}(client)
	}

	time.Sleep(duration)
	close(stopChan)
	wg.Wait()

	log.Printf("Message flood complete")
	return nil
}

type RealisticChatScenario struct {
	AvgMsgsPerMin int
}

func (s *RealisticChatScenario) Name() string {
	return "realistic-chat"
}

func (s *RealisticChatScenario) Description() string {
	return fmt.Sprintf("Clients send messages with random intervals (avg %d msgs/min per client)", s.AvgMsgsPerMin)
}

func (s *RealisticChatScenario) Run(pool *ClientPool, duration time.Duration) error {
	log.Printf("Running realistic chat scenario for %v (%d avg msgs/min per client)", duration, s.AvgMsgsPerMin)

	clients := pool.GetClients()
	var wg sync.WaitGroup
	stopChan := make(chan struct{})

	for _, client := range clients {
		wg.Add(1)
		go func(c *BenchClient) {
			defer wg.Done()

			rng := rand.New(rand.NewSource(time.Now().UnixNano()))
			avgInterval := time.Minute / time.Duration(s.AvgMsgsPerMin)
			msgNum := 0

			for {
				interval := time.Duration(float64(avgInterval) * rng.ExpFloat64())
				if interval > 5*time.Minute {
					interval = 5 * time.Minute
				}

				select {
				case <-stopChan:
					return
				case <-time.After(interval):
					text := fmt.Sprintf("Hello from %s - message %d", c.nickname, msgNum)
					c.SendMessage(text)
					msgNum++
				}
			}
		}(client)
	}

	time.Sleep(duration)
	close(stopChan)
	wg.Wait()

	log.Printf("Realistic chat complete")
	return nil
}

type HistoryLoadScenario struct {
	RequestsPerClient int
}

func (s *HistoryLoadScenario) Name() string {
	return "history-load"
}

func (s *HistoryLoadScenario) Description() string {
	return fmt.Sprintf("All clients request history (%d requests per client)", s.RequestsPerClient)
}

func (s *HistoryLoadScenario) Run(pool *ClientPool, duration time.Duration) error {
	log.Printf("Running history load scenario for %v (%d requests per client)", duration, s.RequestsPerClient)

	clients := pool.GetClients()
	var wg sync.WaitGroup

	for _, client := range clients {
		wg.Add(1)
		go func(c *BenchClient) {
			defer wg.Done()

			for i := 0; i < s.RequestsPerClient; i++ {
				skip := i * 10
				take := 10
				c.RequestHistory(skip, take)
				time.Sleep(100 * time.Millisecond)
			}
		}(client)
	}

	wg.Wait()
	log.Printf("History load complete")
	return nil
}

type MixedLoadScenario struct {
	MessageRate     int
	HistoryRequests int
	ConnectionChurn bool
	ChurnRate       int
}

func (s *MixedLoadScenario) Name() string {
	return "mixed-load"
}

func (s *MixedLoadScenario) Description() string {
	return fmt.Sprintf("Mixed workload: %d msgs/sec, %d history requests, churn=%v",
		s.MessageRate, s.HistoryRequests, s.ConnectionChurn)
}

func (s *MixedLoadScenario) Run(pool *ClientPool, duration time.Duration) error {
	log.Printf("Running mixed load scenario for %v", duration)

	clients := pool.GetClients()
	var wg sync.WaitGroup
	stopChan := make(chan struct{})

	numMessagers := len(clients) * 70 / 100
	numHistoryLoaders := len(clients) * 20 / 100

	for i, client := range clients {
		if i < numMessagers {
			wg.Add(1)
			go func(c *BenchClient) {
				defer wg.Done()

				interval := time.Second / time.Duration(s.MessageRate)
				ticker := time.NewTicker(interval)
				defer ticker.Stop()

				msgNum := 0
				for {
					select {
					case <-stopChan:
						return
					case <-ticker.C:
						text := fmt.Sprintf("Mixed load msg %d", msgNum)
						c.SendMessage(text)
						msgNum++
					}
				}
			}(client)
		} else if i < numMessagers+numHistoryLoaders {
			wg.Add(1)
			go func(c *BenchClient) {
				defer wg.Done()

				for j := 0; j < s.HistoryRequests; j++ {
					select {
					case <-stopChan:
						return
					default:
					}

					skip := j * 10
					take := 10
					c.RequestHistory(skip, take)
					time.Sleep(time.Duration(1000/s.HistoryRequests) * time.Millisecond)
				}
			}(client)
		}
	}

	time.Sleep(duration)
	close(stopChan)
	wg.Wait()

	log.Printf("Mixed load complete")
	return nil
}

type BurstScenario struct {
	BurstSize     int
	BurstInterval time.Duration
}

func (s *BurstScenario) Name() string {
	return "burst"
}

func (s *BurstScenario) Description() string {
	return fmt.Sprintf("Periodic bursts of %d messages every %v", s.BurstSize, s.BurstInterval)
}

func (s *BurstScenario) Run(pool *ClientPool, duration time.Duration) error {
	log.Printf("Running burst scenario for %v (burst: %d msgs every %v)", duration, s.BurstSize, s.BurstInterval)

	clients := pool.GetClients()
	stopTime := time.Now().Add(duration)

	burstNum := 0
	for time.Now().Before(stopTime) {
		log.Printf("Burst %d: sending %d messages", burstNum, s.BurstSize*len(clients))

		var wg sync.WaitGroup
		for _, client := range clients {
			wg.Add(1)
			go func(c *BenchClient) {
				defer wg.Done()
				for i := 0; i < s.BurstSize; i++ {
					text := fmt.Sprintf("Burst %d msg %d", burstNum, i)
					c.SendMessage(text)
				}
			}(client)
		}
		wg.Wait()

		burstNum++
		time.Sleep(s.BurstInterval)
	}

	log.Printf("Burst scenario complete")
	return nil
}

type ExtremeLoadScenario struct {
	MessageRate int
}

func (s *ExtremeLoadScenario) Name() string {
	return "extreme"
}

func (s *ExtremeLoadScenario) Description() string {
	return fmt.Sprintf("Extreme load: connection churn, base rate, surge cycles (%d msgs/sec base)", s.MessageRate)
}

func (s *ExtremeLoadScenario) Run(pool *ClientPool, duration time.Duration) error {
	log.Printf("Running extreme load scenario for %v", duration)
	log.Printf("Base rate: %d msgs/sec, Surge: 3x, Churn: 10%% every 10s", s.MessageRate)

	clients := pool.GetClients()
	var wg sync.WaitGroup
	stopChan := make(chan struct{})

	numMessagers := len(clients) * 70 / 100
	numHistoryLoaders := len(clients) * 20 / 100

	surgeActive := false
	surgeMu := sync.RWMutex{}

	for i, client := range clients {
		if i < numMessagers {
			wg.Add(1)
			go func(c *BenchClient, idx int) {
				defer wg.Done()

				baseInterval := time.Second / time.Duration(s.MessageRate)
				ticker := time.NewTicker(baseInterval)
				defer ticker.Stop()

				msgNum := 0
				for {
					select {
					case <-stopChan:
						return
					case <-ticker.C:
						surgeMu.RLock()
						active := surgeActive
						surgeMu.RUnlock()

						if active {
							ticker.Reset(baseInterval / 3)
						} else {
							ticker.Reset(baseInterval)
						}

						text := fmt.Sprintf("msg-%d-%d", idx, msgNum)
						c.SendMessage(text)
						msgNum++
					}
				}
			}(client, i)
		} else if i < numMessagers+numHistoryLoaders {
			wg.Add(1)
			go func(c *BenchClient, idx int) {
				defer wg.Done()

				ticker := time.NewTicker(3 * time.Second)
				defer ticker.Stop()

				reqNum := 0
				for {
					select {
					case <-stopChan:
						return
					case <-ticker.C:
						skip := (reqNum * 10) % 100
						take := 10
						c.RequestHistory(skip, take)
						reqNum++
					}
				}
			}(client, i)
		}
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		surgeTicker := time.NewTicker(20 * time.Second)
		defer surgeTicker.Stop()

		churnTicker := time.NewTicker(10 * time.Second)
		defer churnTicker.Stop()

		progressTicker := time.NewTicker(5 * time.Second)
		defer progressTicker.Stop()

		startTime := time.Now()
		churnCycle := 0

		for {
			select {
			case <-stopChan:
				return
			case <-surgeTicker.C:
				surgeMu.Lock()
				surgeActive = !surgeActive
				surgeMu.Unlock()
				if surgeActive {
					log.Printf("SURGE: Message rate increased to %dx", 3)
				} else {
					log.Printf("NORMAL: Message rate back to base")
				}
			case <-churnTicker.C:
				churnCycle++
				churnCount := len(clients) / 10
				startIdx := (churnCycle * churnCount) % len(clients)
				endIdx := (startIdx + churnCount) % len(clients)

				if endIdx > startIdx {
					for i := startIdx; i < endIdx; i++ {
						clients[i].Close()
						newClient := NewBenchClient(clients[i].nickname, pool.protocol, pool.metrics)
						if err := newClient.Connect(pool.url, pool.insecure, pool.certFile); err == nil {
							newClient.Join()
							clients[i] = newClient
						}
					}
				} else {
					for i := startIdx; i < len(clients); i++ {
						clients[i].Close()
						newClient := NewBenchClient(clients[i].nickname, pool.protocol, pool.metrics)
						if err := newClient.Connect(pool.url, pool.insecure, pool.certFile); err == nil {
							newClient.Join()
							clients[i] = newClient
						}
					}
					for i := 0; i < endIdx; i++ {
						clients[i].Close()
						newClient := NewBenchClient(clients[i].nickname, pool.protocol, pool.metrics)
						if err := newClient.Connect(pool.url, pool.insecure, pool.certFile); err == nil {
							newClient.Join()
							clients[i] = newClient
						}
					}
				}
				log.Printf("CHURN: Reconnected %d clients (cycle %d)", churnCount, churnCycle)
			case <-progressTicker.C:
				elapsed := time.Since(startTime)
				log.Printf("Progress: %.0f%% complete, surge: %v", float64(elapsed)/float64(duration)*100, surgeActive)
			}
		}
	}()

	time.Sleep(duration)
	close(stopChan)
	wg.Wait()

	log.Printf("Extreme load complete")
	return nil
}

func GetScenario(name string, rate int) Scenario {
	switch name {
	case "storm":
		return &ConnectionStormScenario{}
	case "flood":
		return &MessageFloodScenario{Rate: rate}
	case "realistic":
		return &RealisticChatScenario{AvgMsgsPerMin: rate}
	case "history":
		return &HistoryLoadScenario{RequestsPerClient: 10}
	case "mixed":
		return &MixedLoadScenario{
			MessageRate:     rate,
			HistoryRequests: 5,
			ConnectionChurn: false,
		}
	case "burst":
		return &BurstScenario{
			BurstSize:     10,
			BurstInterval: 5 * time.Second,
		}
	case "validation":
		return &ValidationScenario{}
	case "extreme":
		return &ExtremeLoadScenario{MessageRate: rate}
	default:
		return &RealisticChatScenario{AvgMsgsPerMin: 10}
	}
}
