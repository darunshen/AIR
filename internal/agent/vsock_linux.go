//go:build linux

package agent

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
	"unsafe"
)

const (
	vsockAF     = 40
	vsockCIDAny = ^uint32(0)
)

type rawSockaddrVM struct {
	Family    uint16
	Reserved1 uint16
	Port      uint32
	CID       uint32
	Zero      [4]byte
}

type vsockAddr struct {
	cid  uint32
	port uint32
}

func (a vsockAddr) Network() string {
	return NetworkVSock
}

func (a vsockAddr) String() string {
	return fmt.Sprintf("vsock:%d:%d", a.cid, a.port)
}

type vsockListener struct {
	fd   int
	addr vsockAddr
}

func listenVSock(port uint32) (net.Listener, error) {
	fd, err := syscall.Socket(vsockAF, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, fmt.Errorf("open vsock listener: %w", err)
	}

	addr := rawSockaddrVM{
		Family: vsockAF,
		Port:   port,
		CID:    vsockCIDAny,
	}

	if err := bindVSock(fd, &addr); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	if err := syscall.Listen(fd, 16); err != nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("listen on vsock port %d: %w", port, err)
	}

	return &vsockListener{
		fd: fd,
		addr: vsockAddr{
			cid:  vsockCIDAny,
			port: port,
		},
	}, nil
}

func dialHostVSock(port uint32) (net.Conn, error) {
	fd, err := syscall.Socket(vsockAF, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, fmt.Errorf("open host vsock dialer: %w", err)
	}
	addr := rawSockaddrVM{
		Family: vsockAF,
		Port:   port,
		CID:    HostVSockCID,
	}
	if err := connectVSock(fd, &addr); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	file := os.NewFile(uintptr(fd), fmt.Sprintf("vsock-host-conn-%d", fd))
	return &vsockConn{
		file: file,
		localAddr: vsockAddr{
			cid:  vsockCIDAny,
			port: port,
		},
		remoteAddr: vsockAddr{
			cid:  HostVSockCID,
			port: port,
		},
	}, nil
}

func connectVSock(fd int, addr *rawSockaddrVM) error {
	_, _, errno := syscall.Syscall(
		syscall.SYS_CONNECT,
		uintptr(fd),
		uintptr(unsafe.Pointer(addr)),
		unsafe.Sizeof(*addr),
	)
	if errno != 0 {
		return fmt.Errorf("connect vsock %d:%d: %w", addr.CID, addr.Port, errno)
	}
	return nil
}

func bindVSock(fd int, addr *rawSockaddrVM) error {
	_, _, errno := syscall.Syscall(
		syscall.SYS_BIND,
		uintptr(fd),
		uintptr(unsafe.Pointer(addr)),
		unsafe.Sizeof(*addr),
	)
	if errno != 0 {
		return fmt.Errorf("bind vsock port %d: %w", addr.Port, errno)
	}
	return nil
}

func (l *vsockListener) Accept() (net.Conn, error) {
	nfd, err := acceptFD(l.fd)
	if err != nil {
		return nil, err
	}

	file := os.NewFile(uintptr(nfd), fmt.Sprintf("vsock-conn-%d", nfd))
	return &vsockConn{
		file:      file,
		localAddr: l.addr,
	}, nil
}

func acceptFD(fd int) (int, error) {
	nfd, _, errno := syscall.Syscall(syscall.SYS_ACCEPT, uintptr(fd), 0, 0)
	if errno != 0 {
		return 0, errno
	}
	return int(nfd), nil
}

func (l *vsockListener) Close() error {
	return syscall.Close(l.fd)
}

func (l *vsockListener) Addr() net.Addr {
	return l.addr
}

type vsockConn struct {
	file       *os.File
	localAddr  net.Addr
	remoteAddr net.Addr
}

func (c *vsockConn) Read(p []byte) (int, error) {
	return c.file.Read(p)
}

func (c *vsockConn) Write(p []byte) (int, error) {
	return c.file.Write(p)
}

func (c *vsockConn) Close() error {
	return c.file.Close()
}

func (c *vsockConn) LocalAddr() net.Addr {
	return c.localAddr
}

func (c *vsockConn) RemoteAddr() net.Addr {
	if c.remoteAddr != nil {
		return c.remoteAddr
	}
	return vsockAddr{}
}

func (c *vsockConn) SetDeadline(t time.Time) error {
	return c.file.SetDeadline(t)
}

func (c *vsockConn) SetReadDeadline(t time.Time) error {
	return c.file.SetReadDeadline(t)
}

func (c *vsockConn) SetWriteDeadline(t time.Time) error {
	return c.file.SetWriteDeadline(t)
}
