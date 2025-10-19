use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use std::path::Path;
use std::time::Duration;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Config {
    pub server: ServerConfig,
    pub tls: TLSConfig,
    pub security: SecurityConfig,
    pub logging: LoggingConfig,
    pub timeouts: TimeoutsConfig,
    pub limits: LimitsConfig,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ServerConfig {
    pub h2_addr: String,
    pub h3_addr: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TLSConfig {
    pub cert_file: String,
    pub key_file: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SecurityConfig {
    pub allowed_origins: Vec<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LoggingConfig {
    pub message_log_file: String,
    #[serde(deserialize_with = "deserialize_duration_nanos")]
    pub log_flush_interval: Duration,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TimeoutsConfig {
    #[serde(deserialize_with = "deserialize_duration_nanos")]
    pub max_idle_timeout: Duration,
    #[serde(deserialize_with = "deserialize_duration_nanos")]
    pub read_deadline: Duration,
    #[serde(deserialize_with = "deserialize_duration_nanos")]
    pub write_deadline: Duration,
    #[serde(deserialize_with = "deserialize_duration_nanos")]
    pub ping_interval: Duration,
    #[serde(deserialize_with = "deserialize_duration_nanos")]
    pub shutdown_timeout: Duration,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LimitsConfig {
    pub max_messages: usize,
    pub send_channel_buffer: usize,
    pub log_channel_buffer: usize,
    pub max_history_take: usize,
    pub max_history_skip: usize,
}

fn deserialize_duration_nanos<'de, D>(deserializer: D) -> Result<Duration, D::Error>
where
    D: serde::Deserializer<'de>,
{
    let nanos = u64::deserialize(deserializer)?;
    Ok(Duration::from_nanos(nanos))
}

impl Config {
    pub fn load(path: &str) -> Result<Self> {
        if Path::new(path).exists() {
            let content = std::fs::read_to_string(path)
                .context("Failed to read config file")?;
            serde_json::from_str(&content)
                .context("Failed to parse config JSON")
        } else {
            Ok(Self::default())
        }
    }

    pub fn validate(&self) -> Result<()> {
        if !Path::new(&self.tls.cert_file).exists() {
            anyhow::bail!(
                "TLS certificate file not found: {}\n\
                Generate a self-signed certificate with:\n\n\
                  openssl req -x509 -newkey rsa:4096 \\\n\
                    -keyout certs/key.pem \\\n\
                    -out certs/cert.pem \\\n\
                    -days 365 -nodes \\\n\
                    -subj \"/CN=chat.local\"\n",
                self.tls.cert_file
            );
        }

        if !Path::new(&self.tls.key_file).exists() {
            anyhow::bail!(
                "TLS private key file not found: {}\nEnsure the key file exists at the configured path",
                self.tls.key_file
            );
        }

        if self.security.allowed_origins.is_empty() {
            anyhow::bail!(
                "No allowed origins configured\nAdd at least one origin, e.g.: [\"https://chat.local:8443\"]"
            );
        }

        Ok(())
    }
}

impl Default for Config {
    fn default() -> Self {
        Self {
            server: ServerConfig {
                h2_addr: ":8443".to_string(),
                h3_addr: ":8443".to_string(),
            },
            tls: TLSConfig {
                cert_file: "../certs/cert.pem".to_string(),
                key_file: "../certs/key.pem".to_string(),
            },
            security: SecurityConfig {
                allowed_origins: vec!["https://chat.local:8443".to_string()],
            },
            logging: LoggingConfig {
                message_log_file: "../logs/messages.jsonl".to_string(),
                log_flush_interval: Duration::from_secs(1),
            },
            timeouts: TimeoutsConfig {
                max_idle_timeout: Duration::from_secs(60),
                read_deadline: Duration::from_secs(60),
                write_deadline: Duration::from_secs(10),
                ping_interval: Duration::from_secs(30),
                shutdown_timeout: Duration::from_secs(10),
            },
            limits: LimitsConfig {
                max_messages: 1000,
                send_channel_buffer: 256,
                log_channel_buffer: 100,
                max_history_take: 100,
                max_history_skip: 10000,
            },
        }
    }
}
