package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/nrytex/nrynet/relay/runtime"
)

var version = "1.0.0"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	server := flag.String("server", "http://127.0.0.1:7000", "central control API URL")
	id := flag.String("id", "", "relay node id")
	address := flag.String("address", "", "advertised public relay address")
	controlListen := flag.String("control-listen", "127.0.0.1:7100", "local relay control listener")
	controlAddress := flag.String("control-address", "", "control listener address reachable from central server")
	controlTLS := flag.Bool("control-tls", false, "serve relay control API over TLS 1.3")
	controlCertFile := flag.String("control-cert-file", "", "relay control TLS certificate file")
	controlKeyFile := flag.String("control-key-file", "", "relay control TLS private key file")
	bindHost := flag.String("bind-host", "0.0.0.0", "local host for assigned visitor ports")
	broker := flag.String("broker", "", "central broker TCP address")
	brokerTLS := flag.Bool("broker-tls", false, "use TLS 1.3 for the central broker")
	brokerServerName := flag.String("broker-server-name", "", "TLS server name for central broker validation")
	brokerCAFile := flag.String("broker-ca-file", "", "PEM CA file for a private broker certificate")
	token := flag.String("token", "", "relay control and data-plane token")
	interval := flag.Duration("heartbeat", 10*time.Second, "heartbeat interval")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	node, err := runtime.New(runtime.Config{ID: *id, Address: *address, ControlListen: *controlListen, ControlAddress: *controlAddress, BindHost: *bindHost, BrokerAddress: *broker, Token: *token, BrokerTLS: *brokerTLS, BrokerServerName: *brokerServerName, BrokerCAFile: *brokerCAFile, ControlTLS: *controlTLS, ControlCertFile: *controlCertFile, ControlKeyFile: *controlKeyFile})
	if err != nil {
		log.Fatal(err)
	}
	listener, err := net.Listen("tcp", *controlListen)
	if err != nil {
		log.Fatalf("control listener: %v", err)
	}
	defer listener.Close()
	go func() {
		if err := node.ServeControl(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("control listener: %v", err)
		}
	}()
	client := &http.Client{Timeout: 5 * time.Second}
	if err := node.Register(client, *server); err != nil {
		log.Fatal(err)
	}
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	for range ticker.C {
		if err := node.Heartbeat(client, *server); err != nil {
			log.Printf("heartbeat failed: %v", err)
			if err := node.Register(client, *server); err != nil {
				log.Printf("relay re-registration failed: %v", err)
			}
		}
	}
}
