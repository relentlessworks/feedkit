package config

import (
	"crypto/rand"
	"flag"
	"fmt"
	"os"
)

// Config holds all service configuration.
type Config struct {
	Addr   string
	DB     string
	Secret string
}

// Load parses flags and env vars to produce a Config.
func Load() *Config {
	c := &Config{
		Addr:   ":8790",
		DB:     "feedkit.json",
		Secret: "",
	}
	// Env vars
	if v := os.Getenv("FEEDKIT_ADDR"); v != "" {
		c.Addr = v
	}
	if v := os.Getenv("FEEDKIT_DB"); v != "" {
		c.DB = v
	}
	if v := os.Getenv("FEEDKIT_SECRET"); v != "" {
		c.Secret = v
	}
	// Flags
	flag.StringVar(&c.Addr, "addr", c.Addr, "listen address")
	flag.StringVar(&c.DB, "db", c.DB, "database file path")
	flag.StringVar(&c.Secret, "secret", c.Secret, "token signing secret")
	flag.Parse()
	// Generate random secret if not provided
	if c.Secret == "" {
		c.Secret = randomSecret()
	}
	return c
}

func randomSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
