package agent

import (
	"fmt"
	"net"
)

const NetworkVSock = "vsock"

func Listen(network, address string, port uint32) (net.Listener, error) {
	switch network {
	case "unix", "tcp":
		return net.Listen(network, address)
	case NetworkVSock:
		return listenVSock(port)
	default:
		return nil, fmt.Errorf("unsupported listener network: %s", network)
	}
}
