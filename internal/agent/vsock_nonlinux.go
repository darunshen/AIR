//go:build !linux

package agent

import (
	"fmt"
	"net"
)

func listenVSock(port uint32) (net.Listener, error) {
	return nil, fmt.Errorf("vsock is only supported on linux (requested port %d)", port)
}
