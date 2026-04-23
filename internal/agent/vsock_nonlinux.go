//go:build !linux

package agent

import (
	"fmt"
	"net"
)

func listenVSock(port uint32) (net.Listener, error) {
	return nil, fmt.Errorf("vsock is only supported on linux (requested port %d)", port)
}

func dialHostVSock(port uint32) (net.Conn, error) {
	return nil, fmt.Errorf("vsock is only supported on linux (requested host port %d)", port)
}
