package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/darunshen/AIR/internal/agent"
	"github.com/darunshen/AIR/internal/guestapi"
)

func main() {
	network := flag.String("network", "unix", "listener network: unix, tcp, or vsock")
	address := flag.String("address", "/run/air-agent.sock", "listener address")
	port := flag.Uint("port", guestapi.DefaultVSockPort, "vsock port when --network=vsock")
	flag.Parse()

	listener, err := agent.Listen(*network, *address, uint32(*port))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	defer listener.Close()

	if *network == agent.NetworkVSock {
		fmt.Printf("air-agent listening on %s://%d\n", *network, *port)
	} else {
		fmt.Printf("air-agent listening on %s://%s\n", *network, *address)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	server := agent.NewServer(listener)
	if err := server.Serve(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
