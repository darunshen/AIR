package agent

import (
	"fmt"
	"io"
	"net"
)

const NetworkVSock = "vsock"
const HostVSockCID uint32 = 2

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

func ServeTCPToHostVSock(listener net.Listener, hostPort uint32) error {
	for {
		clientConn, err := listener.Accept()
		if err != nil {
			return err
		}
		go func() {
			defer clientConn.Close()
			hostConn, err := dialHostVSock(hostPort)
			if err != nil {
				return
			}
			defer hostConn.Close()
			bridgeConns(clientConn, hostConn)
		}()
	}
}

func bridgeConns(left, right net.Conn) {
	done := make(chan error, 2)
	go func() {
		_, err := io.Copy(left, right)
		done <- err
	}()
	go func() {
		_, err := io.Copy(right, left)
		done <- err
	}()
	<-done
	_ = left.Close()
	_ = right.Close()
	<-done
}
