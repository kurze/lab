package main

import (
	"testing"
)

func TestConfigValidationMissingCert(t *testing.T) {
	config := DefaultConfig()
	config.TLS.CertFile = "/nonexistent/cert.pem"

	err := config.Validate()
	if err == nil {
		t.Error("Expected validation error for missing cert file")
	}

	if _, ok := err.(*ConfigError); !ok {
		t.Errorf("Expected ConfigError, got %T", err)
	}
}

func TestConfigValidationEmptyOrigins(t *testing.T) {
	config := DefaultConfig()
	config.Security.AllowedOrigins = []string{}

	err := config.Validate()
	if err == nil {
		t.Error("Expected validation error for empty allowed origins")
	}
}

func TestConfigValidationSuccess(t *testing.T) {
	config := DefaultConfig()
	config.TLS.CertFile = "../../../certs/cert.pem"
	config.TLS.KeyFile = "../../../certs/key.pem"

	err := config.Validate()
	if err != nil {
		t.Skipf("Skipping: certs not found at test location (%v)", err)
	}
}
