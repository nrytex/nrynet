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

	"github.com/nrytex/nrynet/internal/config"
	"github.com/nrytex/nrynet/server/app"
	"github.com/nrytex/nrynet/server/certbothelper"
)

var version = "1.0.0"

func main() {
	configPath := flag.String("config", "config.yaml", "path to YAML configuration")
	showVersion := flag.Bool("version", false, "print version and exit")
	runCertbotHelper := flag.Bool("certbot-helper", false, "run the privileged certbot helper and exit")
	runCertbotRenew := flag.Bool("certbot-renew", false, "renew the managed certbot certificate and exit")
	helperInstallDir := flag.String("certbot-helper-install-dir", "/opt/nrynet", "install directory used by the certbot helper")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if *runCertbotHelper || *runCertbotRenew {
		options := certbothelper.OptionsForInstallDir(*helperInstallDir)
		var err error
		if *runCertbotRenew {
			err = certbothelper.RunRenewWithOptions(context.Background(), certbothelper.ExecRunner{}, options)
		} else {
			err = certbothelper.RunHelperWithOptions(context.Background(), certbothelper.ExecRunner{}, options)
		}
		if err != nil {
			log.Fatal(err)
		}
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
		fmt.Fprintf(os.Stderr, "Nrynet administrator created\nusername: %s\npassword: %s\nserver secret: %s\n",
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
