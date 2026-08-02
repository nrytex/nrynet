package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/nat-link/nat-link/client/agent"
	"github.com/nat-link/nat-link/internal/config"
)

const version = "0.1.0"

func main() {
	configPath := flag.String("config", "config.yaml", "path to YAML configuration")
	flag.Parse()
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	options := agent.NewOptions(cfg, version)
	client, err := agent.New(options, slog.Default())
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := client.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
