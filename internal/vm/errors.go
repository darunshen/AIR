package vm

import (
	"errors"
	"fmt"
)

var (
	ErrFirecrackerBinaryNotFound = errors.New("firecracker binary not found")
	ErrFirecrackerKernelRequired = errors.New("AIR_FIRECRACKER_KERNEL is required for firecracker runtime")
	ErrFirecrackerKernelNotFound = errors.New("firecracker kernel image not found")
	ErrFirecrackerRootfsRequired = errors.New("AIR_FIRECRACKER_ROOTFS is required for firecracker runtime")
	ErrFirecrackerRootfsNotFound = errors.New("firecracker rootfs image not found")
	ErrKVMDeviceNotAvailable     = errors.New("kvm device is unavailable for firecracker runtime")
	ErrGuestAgentNotReady        = errors.New("guest agent is not ready")
	ErrUnsupportedNetworkMode    = errors.New("unsupported network mode")
	ErrTapNetworkingUnavailable  = errors.New("firecracker tap networking is unavailable")
)

type unsupportedProviderError struct {
	provider string
}

func (e unsupportedProviderError) Error() string {
	return fmt.Sprintf("unsupported vm provider: %s", e.provider)
}

func ErrUnsupportedProvider(provider string) error {
	return unsupportedProviderError{provider: provider}
}

func errFirecrackerBinaryNotFound(binary string, err error) error {
	return fmt.Errorf("%w: %s (%v)", ErrFirecrackerBinaryNotFound, binary, err)
}

func errFirecrackerKernelNotFound(path string, err error) error {
	return fmt.Errorf("%w: %s (%v)", ErrFirecrackerKernelNotFound, path, err)
}

func errFirecrackerRootfsNotFound(path string, err error) error {
	return fmt.Errorf("%w: %s (%v)", ErrFirecrackerRootfsNotFound, path, err)
}

func errKVMDeviceNotAvailable(path string, err error) error {
	return fmt.Errorf("%w: %s (%v)", ErrKVMDeviceNotAvailable, path, err)
}

func errGuestAgentNotReady(sessionID, vsockPath string) error {
	return fmt.Errorf("%w for session %s: vsock path %s is not serving requests yet", ErrGuestAgentNotReady, sessionID, vsockPath)
}

func errGuestAgentTransport(sessionID, vsockPath string, err error) error {
	return fmt.Errorf("%w for session %s via %s: %v", ErrGuestAgentNotReady, sessionID, vsockPath, err)
}

func errUnsupportedNetworkMode(mode string) error {
	return fmt.Errorf("%w: %s", ErrUnsupportedNetworkMode, mode)
}

func errTapNetworkingUnavailable(message string, err error) error {
	if err == nil {
		return fmt.Errorf("%w: %s", ErrTapNetworkingUnavailable, message)
	}
	return fmt.Errorf("%w: %s (%v)", ErrTapNetworkingUnavailable, message, err)
}
