package main

import (
	"sync"
	"sync/atomic"
	"time"
)

type Metrics struct {
	ConnectionsSucceeded int64
	ConnectionsFailed    int64
	MessagesSent         int64
	MessagesReceived     int64
	BytesSent            int64
	BytesReceived        int64
	ErrorCount           int64

	latencyMu        sync.Mutex
	latencies        []time.Duration
	connectDurations []time.Duration
	pendingRTT       map[string]time.Time

	StartTime time.Time
	EndTime   time.Time
}

func NewMetrics() *Metrics {
	return &Metrics{
		latencies:        make([]time.Duration, 0, 100000),
		connectDurations: make([]time.Duration, 0, 10000),
		pendingRTT:       make(map[string]time.Time),
		StartTime:        time.Now(),
	}
}

func (m *Metrics) RecordConnectionSuccess(duration time.Duration) {
	atomic.AddInt64(&m.ConnectionsSucceeded, 1)
	m.latencyMu.Lock()
	m.connectDurations = append(m.connectDurations, duration)
	m.latencyMu.Unlock()
}

func (m *Metrics) RecordConnectionFailure() {
	atomic.AddInt64(&m.ConnectionsFailed, 1)
}

func (m *Metrics) RecordMessageSent(clientID string, bytes int64) {
	atomic.AddInt64(&m.MessagesSent, 1)
	atomic.AddInt64(&m.BytesSent, bytes)

	if clientID != "" {
		m.latencyMu.Lock()
		m.pendingRTT[clientID] = time.Now()
		m.latencyMu.Unlock()
	}
}

func (m *Metrics) RecordMessageReceived(clientID string, bytes int64) {
	atomic.AddInt64(&m.MessagesReceived, 1)
	atomic.AddInt64(&m.BytesReceived, bytes)

	if clientID != "" {
		m.latencyMu.Lock()
		if sentTime, ok := m.pendingRTT[clientID]; ok {
			latency := time.Since(sentTime)
			m.latencies = append(m.latencies, latency)
			delete(m.pendingRTT, clientID)
		}
		m.latencyMu.Unlock()
	}
}

func (m *Metrics) RecordError() {
	atomic.AddInt64(&m.ErrorCount, 1)
}

func (m *Metrics) Stop() {
	m.EndTime = time.Now()
}

type Stats struct {
	Duration             time.Duration
	ConnectionsTotal     int64
	ConnectionsSucceeded int64
	ConnectionsFailed    int64
	ConnectionRate       float64
	MessagesTotal        int64
	MessageRate          float64
	BytesTotal           int64
	Throughput           float64

	LatencyP50  time.Duration
	LatencyP95  time.Duration
	LatencyP99  time.Duration
	LatencyP999 time.Duration
	LatencyMax  time.Duration
	LatencyMin  time.Duration
	LatencyAvg  time.Duration

	ConnectAvg time.Duration
	ConnectMax time.Duration

	ErrorRate float64
}

func (m *Metrics) CalculateStats() *Stats {
	duration := m.EndTime.Sub(m.StartTime)
	if duration == 0 {
		duration = 1
	}

	stats := &Stats{
		Duration:             duration,
		ConnectionsTotal:     m.ConnectionsSucceeded + m.ConnectionsFailed,
		ConnectionsSucceeded: m.ConnectionsSucceeded,
		ConnectionsFailed:    m.ConnectionsFailed,
		MessagesTotal:        m.MessagesSent,
		BytesTotal:           m.BytesSent,
	}

	seconds := duration.Seconds()
	if seconds > 0 {
		stats.ConnectionRate = float64(m.ConnectionsSucceeded) / seconds
		stats.MessageRate = float64(m.MessagesSent) / seconds
		stats.Throughput = float64(m.BytesSent) / seconds
	}

	if stats.ConnectionsTotal > 0 {
		stats.ErrorRate = float64(m.ErrorCount) / float64(stats.ConnectionsTotal)
	}

	m.latencyMu.Lock()
	defer m.latencyMu.Unlock()

	if len(m.latencies) > 0 {
		sorted := make([]time.Duration, len(m.latencies))
		copy(sorted, m.latencies)
		sortDurations(sorted)

		stats.LatencyMin = sorted[0]
		stats.LatencyMax = sorted[len(sorted)-1]
		stats.LatencyP50 = percentile(sorted, 0.50)
		stats.LatencyP95 = percentile(sorted, 0.95)
		stats.LatencyP99 = percentile(sorted, 0.99)
		stats.LatencyP999 = percentile(sorted, 0.999)

		var sum time.Duration
		for _, lat := range sorted {
			sum += lat
		}
		stats.LatencyAvg = sum / time.Duration(len(sorted))
	}

	if len(m.connectDurations) > 0 {
		sorted := make([]time.Duration, len(m.connectDurations))
		copy(sorted, m.connectDurations)
		sortDurations(sorted)

		stats.ConnectMax = sorted[len(sorted)-1]
		var sum time.Duration
		for _, d := range sorted {
			sum += d
		}
		stats.ConnectAvg = sum / time.Duration(len(sorted))
	}

	return stats
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)) * p)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func sortDurations(durations []time.Duration) {
	for i := 1; i < len(durations); i++ {
		for j := i; j > 0 && durations[j] < durations[j-1]; j-- {
			durations[j], durations[j-1] = durations[j-1], durations[j]
		}
	}
}
