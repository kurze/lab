package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	url := flag.String("url", "", "Server URL (auto-detected based on protocol)")
	protocol := flag.String("protocol", "websocket", "Protocol: websocket, webtransport (http3)")
	scenario := flag.String("scenario", "realistic", "Scenario: storm, flood, realistic, history, mixed, burst")
	clients := flag.Int("clients", 100, "Number of concurrent clients")
	duration := flag.Duration("duration", 30*time.Second, "Test duration")
	rate := flag.Int("rate", 10, "Message rate (msgs/sec for flood, msgs/min for realistic)")
	output := flag.String("output", "text", "Output format: text, json, markdown")
	insecure := flag.Bool("insecure", false, "Skip TLS certificate verification")
	certFile := flag.String("cert", "", "Path to TLS certificate file (e.g., certs/cert.pem)")
	help := flag.Bool("help", false, "Show help")

	flag.Parse()

	if *help {
		printHelp()
		os.Exit(0)
	}

	if *url == "" {
		if *protocol == "webtransport" || *protocol == "http3" {
			*url = "https://chat.local:8443/chat"
		} else {
			*url = "wss://chat.local:8443/ws"
		}
	}

	log.SetFlags(log.Ltime)
	log.Printf("Fast-Chat Benchmark Tool")
	log.Printf("========================")
	log.Printf("Protocol: %s", *protocol)
	log.Printf("Server:   %s", *url)
	log.Printf("Scenario: %s", *scenario)
	log.Printf("Clients:  %d", *clients)
	log.Printf("Duration: %v", *duration)
	log.Printf("")

	metrics := NewMetrics()
	if *scenario == "validation" {
		metrics.ValidationMode = true
		if *clients < 3 {
			*clients = 3
			log.Printf("Validation mode requires at least 3 clients, using 3")
		}
	}
	pool := NewClientPool(*url, *protocol, *insecure, *certFile, metrics)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Printf("\nReceived interrupt, shutting down...")
		pool.CloseAll()
		metrics.Stop()
		stats := metrics.CalculateStats()
		report := NewReport(*scenario, *clients, stats)
		printReport(report, *output)
		os.Exit(0)
	}()

	log.Printf("Creating %d clients...", *clients)
	if err := pool.CreateClients(*clients); err != nil {
		log.Fatalf("Failed to create clients: %v", err)
	}

	time.Sleep(1 * time.Second)

	scenarioRunner := GetScenario(*scenario, *rate)
	log.Printf("Running scenario: %s", scenarioRunner.Description())
	log.Printf("Start time: %s", time.Now().Format("15:04:05"))
	log.Printf("")

	if err := scenarioRunner.Run(pool, *duration); err != nil {
		log.Fatalf("Scenario failed: %v", err)
	}

	log.Printf("")
	log.Printf("Scenario complete, shutting down clients...")
	pool.CloseAll()
	metrics.Stop()

	stats := metrics.CalculateStats()
	report := NewReport(*scenario, *clients, stats)
	printReport(report, *output)
}

func printReport(report *Report, format string) {
	switch format {
	case "json":
		fmt.Println(report.ToJSON())
	case "markdown", "md":
		fmt.Println(report.ToMarkdown())
	default:
		fmt.Println(report.ToText())
	}
}

func printHelp() {
	fmt.Println("Fast-Chat Benchmark Tool")
	fmt.Println("========================")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  benchmark [flags]")
	fmt.Println("")
	fmt.Println("Flags:")
	fmt.Println("  -protocol string")
	fmt.Println("        Protocol: websocket, webtransport (default: websocket)")
	fmt.Println("  -url string")
	fmt.Println("        Server URL (auto-detected: wss://chat.local:8443/ws for websocket,")
	fmt.Println("        https://chat.local:8443/chat for webtransport)")
	fmt.Println("  -scenario string")
	fmt.Println("        Test scenario (default: realistic)")
	fmt.Println("        Options: storm, flood, realistic, history, mixed, burst")
	fmt.Println("  -clients int")
	fmt.Println("        Number of concurrent clients (default: 100)")
	fmt.Println("  -duration duration")
	fmt.Println("        Test duration (default: 30s)")
	fmt.Println("  -rate int")
	fmt.Println("        Message rate - msgs/sec for flood, msgs/min for realistic (default: 10)")
	fmt.Println("  -output string")
	fmt.Println("        Output format: text, json, markdown (default: text)")
	fmt.Println("  -cert string")
	fmt.Println("        Path to TLS certificate file (e.g., certs/cert.pem)")
	fmt.Println("  -insecure")
	fmt.Println("        Skip TLS certificate verification (for self-signed certs)")
	fmt.Println("  -help")
	fmt.Println("        Show this help message")
	fmt.Println("")
	fmt.Println("Scenarios:")
	fmt.Println("  validation - Validates all functionality (JOIN, SEND, HISTORY, PING, broadcast)")
	fmt.Println("  storm      - All clients connect simultaneously and stay idle")
	fmt.Println("  flood      - All clients send messages at maximum rate")
	fmt.Println("  realistic  - Clients send messages with random intervals (Poisson)")
	fmt.Println("  history    - All clients request message history")
	fmt.Println("  mixed      - Combination of messaging and history requests")
	fmt.Println("  burst      - Periodic bursts of messages")
	fmt.Println("  extreme    - Connection churn, base rate, surge cycles (deterministic)")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  # Test with 100 clients for 30 seconds (realistic chat)")
	fmt.Println("  benchmark")
	fmt.Println("")
	fmt.Println("  # Connection storm with 500 clients")
	fmt.Println("  benchmark -scenario storm -clients 500 -duration 60s")
	fmt.Println("")
	fmt.Println("  # Message flood at 50 msgs/sec per client")
	fmt.Println("  benchmark -scenario flood -clients 50 -rate 50 -duration 30s")
	fmt.Println("")
	fmt.Println("  # Use self-signed certificate")
	fmt.Println("  benchmark -cert certs/cert.pem")
	fmt.Println("")
	fmt.Println("  # Skip certificate verification (insecure)")
	fmt.Println("  benchmark -insecure")
	fmt.Println("")
	fmt.Println("  # Test against remote server")
	fmt.Println("  benchmark -url wss://chat.example.com/ws -clients 200")
	fmt.Println("")
	fmt.Println("  # Output results as JSON")
	fmt.Println("  benchmark -output json > results.json")
	fmt.Println("")
}
