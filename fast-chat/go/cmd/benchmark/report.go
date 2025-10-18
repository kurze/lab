package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Report struct {
	Scenario string        `json:"scenario"`
	Duration time.Duration `json:"duration"`
	Clients  int           `json:"clients"`

	ConnectionsSucceeded int64   `json:"connections_succeeded"`
	ConnectionsFailed    int64   `json:"connections_failed"`
	ConnectionRate       float64 `json:"connection_rate"`

	MessagesSent     int64   `json:"messages_sent"`
	MessagesReceived int64   `json:"messages_received"`
	MessageRate      float64 `json:"message_rate"`

	BytesSent     int64   `json:"bytes_sent"`
	BytesReceived int64   `json:"bytes_received"`
	Throughput    float64 `json:"throughput_mbps"`

	LatencyP50  float64 `json:"latency_p50_ms"`
	LatencyP95  float64 `json:"latency_p95_ms"`
	LatencyP99  float64 `json:"latency_p99_ms"`
	LatencyP999 float64 `json:"latency_p999_ms"`
	LatencyMax  float64 `json:"latency_max_ms"`
	LatencyMin  float64 `json:"latency_min_ms"`
	LatencyAvg  float64 `json:"latency_avg_ms"`

	ConnectAvg float64 `json:"connect_avg_ms"`
	ConnectMax float64 `json:"connect_max_ms"`

	ErrorRate float64 `json:"error_rate"`
}

func NewReport(scenario string, clients int, stats *Stats) *Report {
	return &Report{
		Scenario:             scenario,
		Duration:             stats.Duration,
		Clients:              clients,
		ConnectionsSucceeded: stats.ConnectionsSucceeded,
		ConnectionsFailed:    stats.ConnectionsFailed,
		ConnectionRate:       stats.ConnectionRate,
		MessagesSent:         stats.MessagesTotal,
		MessagesReceived:     stats.MessagesTotal,
		MessageRate:          stats.MessageRate,
		BytesSent:            stats.BytesTotal,
		BytesReceived:        stats.BytesTotal,
		Throughput:           stats.Throughput / 1024 / 1024,
		LatencyP50:           stats.LatencyP50.Seconds() * 1000,
		LatencyP95:           stats.LatencyP95.Seconds() * 1000,
		LatencyP99:           stats.LatencyP99.Seconds() * 1000,
		LatencyP999:          stats.LatencyP999.Seconds() * 1000,
		LatencyMax:           stats.LatencyMax.Seconds() * 1000,
		LatencyMin:           stats.LatencyMin.Seconds() * 1000,
		LatencyAvg:           stats.LatencyAvg.Seconds() * 1000,
		ConnectAvg:           stats.ConnectAvg.Seconds() * 1000,
		ConnectMax:           stats.ConnectMax.Seconds() * 1000,
		ErrorRate:            stats.ErrorRate,
	}
}

func (r *Report) ToJSON() string {
	bytes, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": "Failed to marshal report: %v"}`, err)
	}
	return string(bytes)
}

func (r *Report) ToText() string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString("=== Benchmark Results ===\n")
	sb.WriteString(fmt.Sprintf("Scenario:    %s\n", r.Scenario))
	sb.WriteString(fmt.Sprintf("Duration:    %v\n", r.Duration.Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("Clients:     %d\n", r.Clients))
	sb.WriteString("\n")

	sb.WriteString("Connections:\n")
	successRate := 100.0
	if r.ConnectionsSucceeded+r.ConnectionsFailed > 0 {
		successRate = float64(r.ConnectionsSucceeded) * 100.0 / float64(r.ConnectionsSucceeded+r.ConnectionsFailed)
	}
	sb.WriteString(fmt.Sprintf("  Successful:   %d (%.1f%%)\n", r.ConnectionsSucceeded, successRate))
	sb.WriteString(fmt.Sprintf("  Failed:       %d (%.1f%%)\n", r.ConnectionsFailed, 100.0-successRate))
	sb.WriteString(fmt.Sprintf("  Avg Connect:  %.2f ms\n", r.ConnectAvg))
	sb.WriteString(fmt.Sprintf("  Max Connect:  %.2f ms\n", r.ConnectMax))
	sb.WriteString("\n")

	sb.WriteString("Messages:\n")
	sb.WriteString(fmt.Sprintf("  Sent:         %s\n", formatNumber(r.MessagesSent)))
	sb.WriteString(fmt.Sprintf("  Received:     %s\n", formatNumber(r.MessagesReceived)))
	sb.WriteString(fmt.Sprintf("  Rate:         %.1f msgs/sec\n", r.MessageRate))
	sb.WriteString("\n")

	if r.LatencyP50 > 0 {
		sb.WriteString("Latency (ms):\n")
		sb.WriteString(fmt.Sprintf("  min:          %.2f\n", r.LatencyMin))
		sb.WriteString(fmt.Sprintf("  p50:          %.2f\n", r.LatencyP50))
		sb.WriteString(fmt.Sprintf("  p95:          %.2f\n", r.LatencyP95))
		sb.WriteString(fmt.Sprintf("  p99:          %.2f\n", r.LatencyP99))
		sb.WriteString(fmt.Sprintf("  p999:         %.2f\n", r.LatencyP999))
		sb.WriteString(fmt.Sprintf("  max:          %.2f\n", r.LatencyMax))
		sb.WriteString(fmt.Sprintf("  avg:          %.2f\n", r.LatencyAvg))
		sb.WriteString("\n")
	}

	sb.WriteString("Throughput:\n")
	sb.WriteString(fmt.Sprintf("  Sent:         %.2f MB/s\n", r.Throughput))
	sb.WriteString(fmt.Sprintf("  Total:        %s\n", formatBytes(r.BytesSent)))
	sb.WriteString("\n")

	if r.ErrorRate > 0 {
		sb.WriteString(fmt.Sprintf("Error Rate:   %.2f%%\n", r.ErrorRate*100))
		sb.WriteString("\n")
	}

	return sb.String()
}

func (r *Report) ToMarkdown() string {
	var sb strings.Builder

	sb.WriteString("# Benchmark Results\n\n")
	sb.WriteString("## Configuration\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Scenario | %s |\n", r.Scenario))
	sb.WriteString(fmt.Sprintf("| Duration | %v |\n", r.Duration.Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("| Clients | %d |\n", r.Clients))
	sb.WriteString("\n")

	sb.WriteString("## Connections\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	successRate := 100.0
	if r.ConnectionsSucceeded+r.ConnectionsFailed > 0 {
		successRate = float64(r.ConnectionsSucceeded) * 100.0 / float64(r.ConnectionsSucceeded+r.ConnectionsFailed)
	}
	sb.WriteString(fmt.Sprintf("| Successful | %d (%.1f%%) |\n", r.ConnectionsSucceeded, successRate))
	sb.WriteString(fmt.Sprintf("| Failed | %d (%.1f%%) |\n", r.ConnectionsFailed, 100.0-successRate))
	sb.WriteString(fmt.Sprintf("| Avg Connect Time | %.2f ms |\n", r.ConnectAvg))
	sb.WriteString(fmt.Sprintf("| Max Connect Time | %.2f ms |\n", r.ConnectMax))
	sb.WriteString("\n")

	sb.WriteString("## Messages\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Sent | %s |\n", formatNumber(r.MessagesSent)))
	sb.WriteString(fmt.Sprintf("| Received | %s |\n", formatNumber(r.MessagesReceived)))
	sb.WriteString(fmt.Sprintf("| Rate | %.1f msgs/sec |\n", r.MessageRate))
	sb.WriteString("\n")

	if r.LatencyP50 > 0 {
		sb.WriteString("## Latency (ms)\n\n")
		sb.WriteString("| Percentile | Value |\n")
		sb.WriteString("|------------|-------|\n")
		sb.WriteString(fmt.Sprintf("| min | %.2f |\n", r.LatencyMin))
		sb.WriteString(fmt.Sprintf("| p50 | %.2f |\n", r.LatencyP50))
		sb.WriteString(fmt.Sprintf("| p95 | %.2f |\n", r.LatencyP95))
		sb.WriteString(fmt.Sprintf("| p99 | %.2f |\n", r.LatencyP99))
		sb.WriteString(fmt.Sprintf("| p999 | %.2f |\n", r.LatencyP999))
		sb.WriteString(fmt.Sprintf("| max | %.2f |\n", r.LatencyMax))
		sb.WriteString(fmt.Sprintf("| avg | %.2f |\n", r.LatencyAvg))
		sb.WriteString("\n")
	}

	sb.WriteString("## Throughput\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Sent | %.2f MB/s |\n", r.Throughput))
	sb.WriteString(fmt.Sprintf("| Total | %s |\n", formatBytes(r.BytesSent)))
	sb.WriteString("\n")

	if r.ErrorRate > 0 {
		sb.WriteString("## Errors\n\n")
		sb.WriteString(fmt.Sprintf("Error Rate: %.2f%%\n\n", r.ErrorRate*100))
	}

	return sb.String()
}

func formatNumber(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1000000)
}

func formatBytes(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	if b < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	if b < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(b)/1024/1024)
	}
	return fmt.Sprintf("%.1f GB", float64(b)/1024/1024/1024)
}
