package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nat-link/nat-link/internal/config"
	"github.com/nat-link/nat-link/server/app"
)

var version = "1.0.0"

func main() {
	configPath := flag.String("config", "config.yaml", "path to YAML configuration")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	application, bootstrap, err := app.New(context.Background(), cfg)
	if err != nil {
		log.Fatal(err)
	}
	if bootstrap.Created {
		fmt.Fprintf(os.Stderr, "NAT-Link administrator created\nusername: %s\npassword: %s\nserver secret: %s\n",
			bootstrap.Username, bootstrap.Password, bootstrap.ServerSecret)
	}
	runError := make(chan error, 1)
	go func() { runError <- application.Run() }()
	err = waitForStop(runError)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := application.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	if err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

func waitForStop(runError <-chan error) error {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case <-signals:
		return nil
	case err := <-runError:
		return err
	}
}
