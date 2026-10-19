package main

import (
	"errors"
	"os"
	"strings"
)

type Config struct {
	Port               string
	DatabaseURL        string
	BaseURL            string
	GoogleClientID     string
	GoogleClientSecret string
}

func loadConfig() (Config, error) {
	cfg := Config{
		Port:               envOr("PORT", "8080"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		BaseURL:            strings.TrimSuffix(os.Getenv("BASE_URL"), "/"),
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
	}
	if cfg.DatabaseURL == "" || cfg.BaseURL == "" {
		return cfg, errors.New("DATABASE_URL and BASE_URL are required")
	}
	return cfg, nil
}

func (c Config) GoogleSignInEnabled() bool {
	return c.GoogleClientID != "" && c.GoogleClientSecret != ""
}

func (c Config) SecureCookies() bool {
	return strings.HasPrefix(c.BaseURL, "https://")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
