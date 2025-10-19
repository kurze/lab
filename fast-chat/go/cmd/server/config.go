package main

import (
	"encoding/json"
	"os"
	"time"
)

type Config struct {
	Server   ServerConfig   `json:"server"`
	TLS      TLSConfig      `json:"tls"`
	Security SecurityConfig `json:"security"`
	Logging  LoggingConfig  `json:"logging"`
	Timeouts TimeoutsConfig `json:"timeouts"`
	Limits   LimitsConfig   `json:"limits"`
}

type ServerConfig struct {
	H2Addr string `json:"h2_addr"`
	H3Addr string `json:"h3_addr"`
}

type TLSConfig struct {
	CertFile string `json:"cert_file"`
	KeyFile  string `json:"key_file"`
}

type SecurityConfig struct {
	AllowedOrigins []string `json:"allowed_origins"`
}

type LoggingConfig struct {
	MessageLogFile string `json:"message_log_file"`
}

type TimeoutsConfig struct {
	MaxIdleTimeout  Duration `json:"max_idle_timeout"`
	ReadDeadline    Duration `json:"read_deadline"`
	WriteDeadline   Duration `json:"write_deadline"`
	PingInterval    Duration `json:"ping_interval"`
	ShutdownTimeout Duration `json:"shutdown_timeout"`
}

type Duration time.Duration

func (d *Duration) UnmarshalJSON(b []byte) error {
	var nanos int64
	if err := json.Unmarshal(b, &nanos); err != nil {
		return err
	}
	*d = Duration(nanos)
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(int64(d))
}

func (d Duration) ToDuration() time.Duration {
	return time.Duration(d)
}

type LimitsConfig struct {
	MaxMessages       int `json:"max_messages"`
	SendChannelBuffer int `json:"send_channel_buffer"`
	LogChannelBuffer  int `json:"log_channel_buffer"`
	MaxHistoryTake    int `json:"max_history_take"`
	MaxHistorySkip    int `json:"max_history_skip"`
}

func LoadConfig(filename string) (*Config, error) {
	config := DefaultConfig()

	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return config, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, config); err != nil {
		return nil, err
	}

	return config, nil
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			H2Addr: ":8443",
			H3Addr: ":8443",
		},
		TLS: TLSConfig{
			CertFile: "certs/cert.pem",
			KeyFile:  "certs/key.pem",
		},
		Security: SecurityConfig{
			AllowedOrigins: []string{"https://chat.local:8443"},
		},
		Logging: LoggingConfig{
			MessageLogFile: "logs/messages.jsonl",
		},
		Timeouts: TimeoutsConfig{
			MaxIdleTimeout:  Duration(60 * time.Second),
			ReadDeadline:    Duration(60 * time.Second),
			WriteDeadline:   Duration(10 * time.Second),
			PingInterval:    Duration(30 * time.Second),
			ShutdownTimeout: Duration(10 * time.Second),
		},
		Limits: LimitsConfig{
			MaxMessages:       1000,
			SendChannelBuffer: 256,
			LogChannelBuffer:  100,
			MaxHistoryTake:    100,
			MaxHistorySkip:    10000,
		},
	}
}

func (c *Config) Validate() error {
	if _, err := os.Stat(c.TLS.CertFile); os.IsNotExist(err) {
		return &ConfigError{
			Field:   "tls.cert_file",
			Value:   c.TLS.CertFile,
			Message: "TLS certificate file not found",
			Hint: `Generate a self-signed certificate with:

  openssl req -x509 -newkey rsa:4096 \
    -keyout certs/key.pem \
    -out certs/cert.pem \
    -days 365 -nodes \
    -subj "/CN=chat.local"

Or use Let's Encrypt for production:
  https://letsencrypt.org/getting-started/`,
		}
	}

	if _, err := os.Stat(c.TLS.KeyFile); os.IsNotExist(err) {
		return &ConfigError{
			Field:   "tls.key_file",
			Value:   c.TLS.KeyFile,
			Message: "TLS private key file not found",
			Hint:    "Ensure the key file exists at the configured path",
		}
	}

	if len(c.Security.AllowedOrigins) == 0 {
		return &ConfigError{
			Field:   "security.allowed_origins",
			Value:   "[]",
			Message: "No allowed origins configured",
			Hint:    `Add at least one origin, e.g.: ["https://chat.local:8443"]`,
		}
	}

	return nil
}

type ConfigError struct {
	Field   string
	Value   string
	Message string
	Hint    string
}

func (e *ConfigError) Error() string {
	msg := "Configuration error: " + e.Message + "\n"
	msg += "  Field: " + e.Field + "\n"
	msg += "  Value: " + e.Value + "\n"
	if e.Hint != "" {
		msg += "\n" + e.Hint
	}
	return msg
}

func (c *Config) SaveExample(filename string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}
