mod chat;
mod config;
mod handlers;

use anyhow::Result;
use axum::{
    routing::get,
    Router,
};
use std::sync::Arc;
use tower_http::compression::CompressionLayer;
use tower_http::cors::{Any, CorsLayer};
use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt};

#[tokio::main]
async fn main() -> Result<()> {
    tracing_subscriber::registry()
        .with(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| "info".into()),
        )
        .with(tracing_subscriber::fmt::layer())
        .init();

    let config = config::Config::load("rust-config.json")?;
    config.validate()?;

    tracing::info!("Fast-Chat Server (Rust)");
    tracing::info!("=======================");

    if let Err(e) = tokio::fs::create_dir_all("../logs").await {
        tracing::warn!("Failed to create logs directory: {}", e);
    }

    let state = Arc::new(
        chat::ChatState::new(
            &config.logging.message_log_file,
            config.limits.max_messages,
            config.limits.log_channel_buffer,
            config.logging.log_flush_interval,
        )
        .await?,
    );
    tracing::info!("Chat state initialized");

    let tls_config = load_tls_config(&config.tls)?;

    let cors = CorsLayer::new()
        .allow_origin(Any)
        .allow_methods(Any)
        .allow_headers(Any);

    let app = Router::new()
        .route("/", get(handlers::index_handler))
        .route("/chat.css", get(handlers::chat_css_handler))
        .route("/chat.js", get(handlers::chat_js_handler))
        .route("/favicon.ico", get(handlers::favicon_handler))
        .route("/ws", get(handlers::websocket_handler))
        .layer(cors)
        .layer(CompressionLayer::new())
        .with_state(state);

    let addr: std::net::SocketAddr = config.server.h2_addr.parse()?;
    
    tracing::info!("Starting HTTP/2 server on https://chat.local{}", config.server.h2_addr);
    tracing::info!("WebSocket available at wss://chat.local{}/ws", config.server.h2_addr);
    tracing::info!("Add '127.0.0.1 chat.local' to /etc/hosts if needed");

    let listener = tokio::net::TcpListener::bind(&addr).await?;

    axum_server::from_tcp_rustls(listener.into_std()?, axum_server::tls_rustls::RustlsConfig::from_config(Arc::new(tls_config)))
        .serve(app.into_make_service())
        .await?;

    Ok(())
}

fn load_tls_config(tls: &config::TLSConfig) -> Result<tokio_rustls::rustls::ServerConfig> {
    use std::io::BufReader;
    use tokio_rustls::rustls;

    let cert_file = std::fs::File::open(&tls.cert_file)?;
    let key_file = std::fs::File::open(&tls.key_file)?;

    let cert_chain = rustls_pemfile::certs(&mut BufReader::new(cert_file))
        .collect::<Result<Vec<_>, _>>()?;

    let mut key_reader = BufReader::new(key_file);
    let key = rustls_pemfile::private_key(&mut key_reader)?
        .ok_or_else(|| anyhow::anyhow!("No private key found in file"))?;

    let config = rustls::ServerConfig::builder()
        .with_no_client_auth()
        .with_single_cert(cert_chain, key)?;

    Ok(config)
}
