use tokio::net::{TcpListener, UdpSocket, TcpStream};
use tokio::io::AsyncReadExt;
use tokio::sync::Semaphore;
use tokio::time::{timeout, Duration};
use std::sync::Arc;
use std::net::SocketAddr;
use clap::Parser;

#[derive(Parser)]
#[command(name = "rfc863")]
#[command(about = "RFC 863 Discard Protocol - Security PoC")]
struct Args {
    /// Port to bind (default: 9009)
    #[arg(short, long, default_value_t = 9009)]
    port: u16,
    
    /// Run in vulnerable mode (no protections)
    #[arg(long)]
    vulnerable: bool,
    
    /// Maximum concurrent connections (hardened mode)
    #[arg(long, default_value_t = 1000)]
    max_connections: usize,
    
    /// Connection idle timeout in seconds (hardened mode)
    #[arg(long, default_value_t = 30)]
    timeout: u64,
    
    /// Maximum connections per IP (hardened mode)
    #[arg(long, default_value_t = 10)]
    max_per_ip: usize,
}

#[derive(Clone)]
struct SecurityConfig {
    vulnerable_mode: bool,
    max_connections: usize,
    timeout_secs: u64,
    max_per_ip: usize,
}

async fn handle_tcp_connection(
    mut stream: TcpStream,
    peer: SocketAddr,
    config: SecurityConfig,
) {
    const BUF_SIZE: usize = 8192;
    let mut buf = [0u8; BUF_SIZE];
    let mut total_bytes = 0u64;
    
    let timeout_duration = if config.vulnerable_mode {
        None
    } else {
        Some(Duration::from_secs(config.timeout_secs))
    };
    
    loop {
        let read_result = if let Some(duration) = timeout_duration {
            // Hardened: with timeout
            match timeout(duration, stream.read(&mut buf)).await {
                Ok(result) => result,
                Err(_) => {
                    eprintln!("[SECURITY] Timeout on connection from {}", peer);
                    break;
                }
            }
        } else {
            // Vulnerable: no timeout
            stream.read(&mut buf).await
        };
        
        match read_result {
            Ok(0) => break,  // Clean EOF
            Ok(n) => {
                total_bytes += n as u64;
                // Data discarded
            }
            Err(e) => {
                eprintln!("Read error from {}: {}", peer, e);
                break;
            }
        }
    }
    
    println!("Connection from {} closed. Discarded {} bytes", peer, total_bytes);
}

async fn run_tcp_server(port: u16, config: SecurityConfig) {
    let listener = TcpListener::bind(format!("0.0.0.0:{}", port))
        .await
        .expect("Failed to bind TCP");
    
    if config.vulnerable_mode {
        println!("⚠️  TCP server in VULNERABLE mode on port {}", port);
        println!("⚠️  No connection limits, timeouts, or rate limiting!");
    } else {
        println!("🔒 TCP server in HARDENED mode on port {}", port);
        println!("🔒 Max connections: {}", config.max_connections);
        println!("🔒 Timeout: {}s", config.timeout_secs);
        println!("🔒 Max per IP: {}", config.max_per_ip);
    }
    
    let semaphore = if config.vulnerable_mode {
        None
    } else {
        Some(Arc::new(Semaphore::new(config.max_connections)))
    };
    
    loop {
        let permit = if let Some(ref sem) = semaphore {
            match sem.clone().acquire_owned().await {
                Ok(p) => Some(p),
                Err(_) => continue,
            }
        } else {
            None
        };
        
        match listener.accept().await {
            Ok((stream, addr)) => {
                println!("New connection from {}", addr);
                let config = config.clone();
                
                tokio::spawn(async move {
                    let _permit = permit;  // Hold until connection closes
                    handle_tcp_connection(stream, addr, config).await;
                });
            }
            Err(e) => eprintln!("Accept error: {}", e),
        }
    }
}

async fn run_udp_server(port: u16, config: SecurityConfig) {
    let socket = UdpSocket::bind(format!("0.0.0.0:{}", port))
        .await
        .expect("Failed to bind UDP");
    
    if config.vulnerable_mode {
        println!("⚠️  UDP server in VULNERABLE mode on port {}", port);
    } else {
        println!("🔒 UDP server in HARDENED mode on port {}", port);
    }
    
    const BUF_SIZE: usize = 65536;
    let mut buf = [0u8; BUF_SIZE];
    
    loop {
        match socket.recv_from(&mut buf).await {
            Ok((len, addr)) => {
                println!("UDP datagram from {}: {} bytes (discarded)", addr, len);
            }
            Err(e) => eprintln!("UDP receive error: {}", e),
        }
    }
}

#[tokio::main]
async fn main() {
    let args = Args::parse();
    
    let config = SecurityConfig {
        vulnerable_mode: args.vulnerable,
        max_connections: args.max_connections,
        timeout_secs: args.timeout,
        max_per_ip: args.max_per_ip,
    };
    
    println!("\n🎯 RFC 863 Discard Protocol - Security PoC\n");
    
    if config.vulnerable_mode {
        println!("⚠️  WARNING: Running in VULNERABLE mode!");
        println!("⚠️  This server is intentionally insecure for demonstration.\n");
    } else {
        println!("🔒 Running in HARDENED mode with security protections.\n");
    }
    
    let tcp_config = config.clone();
    let _tcp_task = tokio::spawn(async move {
        run_tcp_server(args.port, tcp_config).await;
    });
    
    let udp_config = config.clone();
    let _udp_task = tokio::spawn(async move {
        run_udp_server(args.port, udp_config).await;
    });
    
    // Wait for Ctrl+C
    tokio::signal::ctrl_c()
        .await
        .expect("Failed to listen for Ctrl+C");
    
    println!("\n🛑 Shutting down...");
}
