package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/darunshen/AIR/internal/agent"
	"github.com/darunshen/AIR/internal/buildinfo"
	"github.com/darunshen/AIR/internal/guestapi"
)

func main() {
	network := flag.String("network", "unix", "listener network: unix, tcp, or vsock")
	address := flag.String("address", "/run/air-agent.sock", "listener address")
	port := flag.Uint("port", guestapi.DefaultVSockPort, "vsock port when --network=vsock")
	hostProxyListen := flag.String("host-proxy-listen", "", "optional local tcp address that relays to the host over vsock")
	hostProxyVSockPort := flag.Uint("host-proxy-vsock-port", 0, "host vsock port for the optional local proxy relay")
	version := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *version {
		fmt.Println(buildinfo.String())
		return
	}

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

	if *hostProxyListen != "" {
		if *hostProxyVSockPort == 0 {
			fmt.Fprintln(os.Stderr, "error: --host-proxy-vsock-port is required when --host-proxy-listen is set")
			os.Exit(1)
		}
		hostProxyListener, err := agent.Listen("tcp", *hostProxyListen, 0)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		defer hostProxyListener.Close()
		go func() {
			<-ctx.Done()
			_ = hostProxyListener.Close()
		}()
		go func() {
			if err := agent.ServeTCPToHostVSock(hostProxyListener, uint32(*hostProxyVSockPort)); err != nil && ctx.Err() == nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				stop()
			}
		}()
		fmt.Printf("air-agent host proxy relay on tcp://%s -> vsock://%d:%d\n", *hostProxyListen, agent.HostVSockCID, *hostProxyVSockPort)
	}

	server := agent.NewServer(listener)
	if err := server.Serve(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
