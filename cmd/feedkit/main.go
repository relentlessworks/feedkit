package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/relentlessworks/feedkit/internal/api"
	"github.com/relentlessworks/feedkit/internal/auth"
	"github.com/relentlessworks/feedkit/internal/config"
	"github.com/relentlessworks/feedkit/internal/store"
)

func main() {
	cfg := config.Load()

	// Ensure data directory exists
	dir := filepath.Dir(cfg.DB)
	if dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0755)
	}

	s, err := store.New(cfg.DB)
	if err != nil {
		log.Fatalf("failed to open store: %v", err)
	}

	a := auth.New(s)
	h := api.New(s, a)

	log.Printf("[feedkit] listening on %s (db: %s)", cfg.Addr, cfg.DB)
	if err := http.ListenAndServe(cfg.Addr, h.Routes()); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
