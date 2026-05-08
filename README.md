# LAB

Collection of small projects and PoCs to try new tech or ideas.

- **agentcore** — Shared Go library for building LLM-powered agents: HTTP client, agentic loop with tool dispatch, standard tools (read_file, grep, list_dir, git_log), path safety, tracing, and JSON repair.
- **fast-chat** — The fastest chat app ever, optimized for message latency and time to interactivity. Dual Go + Rust implementations with HTTP/3, WebTransport, and WebSocket support.
- **gitlab-reviewer** — CLI tool that scans open GitLab merge requests, reviews them via a pluggable engine (in-process LLM agent or external command), and posts findings as comments.
- **review-agent** — AI-powered code review agent built in Go, with context compaction, diff review, and artifact comparison. Uses agentcore.
- **rfc863** — Security-focused PoC of RFC 863 (Discard Protocol) in Rust. Demonstrates DoS attacks (connection exhaustion, slowloris, bandwidth flood) and their mitigations.
