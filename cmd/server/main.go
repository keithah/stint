package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/keithah/stint/internal/api"
	"github.com/keithah/stint/internal/config"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/workers"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	store, err := db.OpenWithPoolConfig(ctx, cfg.DatabaseURL, db.PoolConfig{
		MaxConns:        cfg.DBMaxConns,
		MinConns:        cfg.DBMinConns,
		MaxConnLifetime: cfg.DBMaxConnLifetime,
		MaxConnIdleTime: cfg.DBMaxConnIdleTime,
	})
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer store.Close()

	if err := store.RunMigrations(ctx); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	if len(os.Args) > 1 && os.Args[1] == "worker" {
		if err := workers.Run(ctx, cfg, store); err != nil && err != context.Canceled {
			log.Fatal(err)
		}
		return
	}

	router := api.NewRouter(cfg, store)
	server := router.Server
	server.Addr = ":" + cfg.Port
	server.ReadHeaderTimeout = 5 * time.Second
	server.ReadTimeout = 30 * time.Second
	server.WriteTimeout = 60 * time.Second
	server.IdleTimeout = 120 * time.Second
	errCh := make(chan error, 1)
	go func() {
		errCh <- router.StartServer(server)
	}()

	var serverErr error
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Fatal(err)
		}
		serverErr = <-errCh
	case err := <-errCh:
		serverErr = err
	}
	if serverErr != nil && serverErr != http.ErrServerClosed {
		log.Fatal(serverErr)
	}
}
