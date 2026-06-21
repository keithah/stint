package main

import (
	"context"
	"log"
	"os"
	_ "time/tzdata"

	"github.com/keithah/stint/internal/api"
	"github.com/keithah/stint/internal/config"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/workers"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	store, err := db.Open(ctx, cfg.DatabaseURL)
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
	if err := router.Start(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
