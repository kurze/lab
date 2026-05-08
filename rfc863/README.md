# RFC 863 Discard Protocol - Security PoC

A security-focused implementation of RFC 863 (Discard Protocol) that demonstrates common network service vulnerabilities and their mitigations.

## What is RFC 863?

The Discard Protocol is the simplest network protocol: **accept data and throw it away**.

From RFC 863:
> "A discard service simply throws away any data it receives."

## Purpose

Despite its simplicity, this implementation demonstrates:
- **DoS attacks** (resource exhaustion, connection flooding)
- **Slowloris attacks** (slow data transmission to exhaust connections)
- **Bandwidth exhaustion** (flooding with data)
- **Resource management** (connection limits, timeouts, memory)

## Features

This PoC implements **both vulnerable and hardened modes**:

### Vulnerable Mode
- No connection limits
- No timeouts
- No rate limiting
- Perfect for demonstrating attacks

### Hardened Mode
- Connection limits using Semaphore
- Configurable idle timeouts
- Per-IP connection limits (configurable)
- Fixed-size buffers to prevent memory exhaustion

## Building

```bash
cd rfc863
cargo build --release
```

## Usage

### Run in Vulnerable Mode (for attack demonstrations)

```bash
cargo run --release -- --vulnerable
```

### Run in Hardened Mode (with security protections)

```bash
# Default settings
cargo run --release

# Custom settings
cargo run --release -- --max-connections 100 --timeout 10 --max-per-ip 5
```

### Command-line Options

```
Options:
  -p, --port <PORT>                  Port to bind [default: 9009]
      --vulnerable                   Run in vulnerable mode (no protections)
      --max-connections <NUM>        Maximum concurrent connections [default: 1000]
      --timeout <SECONDS>            Connection idle timeout in seconds [default: 30]
      --max-per-ip <NUM>            Maximum connections per IP [default: 10]
  -h, --help                         Print help
```

## Attack Demonstrations

All attack scripts are in the `attacks/` directory.

### Attack 1: Connection Exhaustion

Opens thousands of connections to exhaust server resources.

```bash
# Start vulnerable server
cargo run --release -- --vulnerable

# In another terminal, run attack
./attacks/connection_exhaustion.sh

# Monitor connection count
watch -n 1 'ss -tan | grep 9009 | wc -l'
```

**Effect on vulnerable mode:** Server accepts all connections until system limits are hit.

**Effect on hardened mode:** Server stops accepting after max-connections limit is reached.

### Attack 2: Slowloris

Opens many connections and sends data very slowly to hold them open indefinitely.

```bash
# Start vulnerable server
cargo run --release -- --vulnerable

# In another terminal, run attack
./attacks/slowloris.py

# Or with custom parameters
./attacks/slowloris.py localhost 9009 1000
```

**Effect on vulnerable mode:** Connections stay open forever, exhausting connection pool.

**Effect on hardened mode:** Connections are closed after timeout period (default 30s).

### Attack 3: Bandwidth Flood

Floods the server with data to exhaust network bandwidth.

```bash
# Start vulnerable server
cargo run --release -- --vulnerable

# In another terminal, run attack
./attacks/bandwidth_flood.sh

# Monitor bandwidth
iftop  # or nethogs
```

**Effect on vulnerable mode:** Server processes data at maximum speed, consuming all available bandwidth.

**Effect on hardened mode:** Connection limits prevent unlimited resource consumption.

## Monitoring During Attacks

```bash
# Watch connection count
watch -n 1 'ss -tan | grep 9009 | wc -l'

# Watch resource usage
htop

# Check open file descriptors
lsof -p $(pgrep rfc863) | wc -l

# Monitor network bandwidth
iftop
# or
nethogs
```

## Security Issues Demonstrated

### Issue 1: Unlimited Connections (Connection Exhaustion)
**Problem:** Accepting unlimited connections exhausts file descriptors.
**Solution:** Use tokio Semaphore to limit concurrent connections.

### Issue 2: No Timeouts (Slowloris Attack)
**Problem:** Clients can hold connections open indefinitely with minimal data.
**Solution:** Implement idle timeouts on read operations.

### Issue 3: No Rate Limiting (Bandwidth Exhaustion)
**Problem:** Clients can send data as fast as possible, consuming all bandwidth.
**Solution:** Connection limits and timeouts reduce impact (full rate limiting could be added).

### Issue 4: Unbounded Memory Usage
**Problem:** Large buffers per connection can cause out-of-memory.
**Solution:** Use fixed-size, stack-allocated buffers (8KB).

### Issue 5: No IP-based Restrictions
**Problem:** Single IP can open unlimited connections.
**Solution:** Track and limit connections per IP address (configurable).

## Security Lessons

1. **Connection limits prevent resource exhaustion**
2. **Timeouts prevent slowloris attacks**
3. **Per-IP limits prevent single-source DoS**
4. **Fixed buffers prevent memory exhaustion**
5. **Simple protocols still need security**

## Project Structure

```
rfc863/
├── Cargo.toml
├── src/
│   └── main.rs           # Server implementation
├── attacks/
│   ├── connection_exhaustion.sh
│   ├── slowloris.py
│   └── bandwidth_flood.sh
└── README.md
```

## Educational Use

This project is perfect for:
- Security training and workshops
- Demonstrating attack/defense concepts
- Understanding network protocol security
- Learning async Rust with tokio

## License

Educational/demonstration purposes.

## Warning

The vulnerable mode is **intentionally insecure**. Only run attack scripts against servers you own or have permission to test.
