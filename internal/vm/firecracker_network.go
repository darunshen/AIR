package vm

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	firecrackerNetworkNone = "none"
	firecrackerNetworkFull = "full"
)

type firecrackerNetworkInterface struct {
	IfaceID       string `json:"iface_id"`
	GuestMAC      string `json:"guest_mac"`
	HostDevName   string `json:"host_dev_name"`
	RxRateLimiter any    `json:"rx_rate_limiter,omitempty"`
	TxRateLimiter any    `json:"tx_rate_limiter,omitempty"`
}

type firecrackerNetworkConfig struct {
	Mode           string `json:"mode"`
	TapName        string `json:"tap_name,omitempty"`
	HostIP         string `json:"host_ip,omitempty"`
	GuestIP        string `json:"guest_ip,omitempty"`
	GatewayIP      string `json:"gateway_ip,omitempty"`
	SubnetCIDR     string `json:"subnet_cidr,omitempty"`
	GuestMAC       string `json:"guest_mac,omitempty"`
	HostInterface  string `json:"host_interface,omitempty"`
	ResolvConfPath string `json:"resolv_conf_path,omitempty"`
	GuestIfaceName string `json:"guest_iface_name,omitempty"`
}

func normalizeNetworkMode(mode string) string {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		return defaultNetworkMode
	}
	switch mode {
	case firecrackerNetworkNone, firecrackerNetworkFull:
		return mode
	default:
		return mode
	}
}

func DefaultNetworkMode() string {
	return defaultNetworkMode
}

func ValidateNetworkMode(mode string) error {
	return validateNetworkMode(mode)
}

func validateNetworkMode(mode string) error {
	switch normalizeNetworkMode(mode) {
	case firecrackerNetworkNone, firecrackerNetworkFull:
		return nil
	default:
		return errUnsupportedNetworkMode(mode)
	}
}

func networkOrdinal(sessionID string) int {
	var total int
	for _, ch := range []byte(sessionID) {
		total += int(ch)
	}
	return total % 200
}

func hostTapName(sessionID string) string {
	suffix := sessionID
	if len(suffix) > 8 {
		suffix = suffix[len(suffix)-8:]
	}
	return "airtap" + suffix
}

func guestStaticNetworkConfig(sessionID string) firecrackerNetworkConfig {
	ordinal := networkOrdinal(sessionID) + 10
	subnetBase := ordinal
	subnetCIDR := fmt.Sprintf("172.22.%d.0/24", subnetBase)
	hostIP := fmt.Sprintf("172.22.%d.1", subnetBase)
	guestIP := fmt.Sprintf("172.22.%d.2", subnetBase)
	macTail := ordinal & 0xff
	return firecrackerNetworkConfig{
		Mode:           firecrackerNetworkFull,
		TapName:        hostTapName(sessionID),
		HostIP:         hostIP,
		GuestIP:        guestIP,
		GatewayIP:      hostIP,
		SubnetCIDR:     subnetCIDR,
		GuestMAC:       fmt.Sprintf("06:00:ac:16:%02x:02", macTail),
		GuestIfaceName: "eth0",
	}
}

func writeGuestNetworkConfig(path string, cfg firecrackerNetworkConfig) error {
	lines := []string{
		"AIR_NETWORK_MODE=" + cfg.Mode,
		"AIR_NETWORK_GUEST_IFACE=" + cfg.GuestIfaceName,
		"AIR_NETWORK_GUEST_IP=" + cfg.GuestIP,
		"AIR_NETWORK_GATEWAY_IP=" + cfg.GatewayIP,
		"AIR_NETWORK_SUBNET_CIDR=" + cfg.SubnetCIDR,
		"AIR_NETWORK_RESOLV_CONF=" + cfg.ResolvConfPath,
	}
	body := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(path, []byte(body), 0o644)
}

func discoverDefaultRouteInterface() (string, error) {
	file, err := os.Open("/proc/net/route")
	if err != nil {
		return "", errTapNetworkingUnavailable("read /proc/net/route", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	first := true
	for scanner.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		if fields[1] == "00000000" && fields[0] != "" {
			return fields[0], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", errTapNetworkingUnavailable("scan /proc/net/route", err)
	}
	return "", errTapNetworkingUnavailable("default route interface not found", nil)
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func runNetworkCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = err.Error()
		}
		return errTapNetworkingUnavailable(strings.Join(append([]string{name}, args...), " "), fmt.Errorf("%s", msg))
	}
	return nil
}

func hostHasAddress(devName, addressCIDR string) bool {
	cmd := exec.Command("ip", "-4", "addr", "show", "dev", devName)
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), addressCIDR)
}

func setupTapNetworking(cfg firecrackerNetworkConfig) error {
	if !commandExists("ip") {
		return errTapNetworkingUnavailable("ip command not found", nil)
	}
	if !commandExists("iptables") {
		return errTapNetworkingUnavailable("iptables command not found", nil)
	}
	if cfg.HostInterface == "" {
		iface, err := discoverDefaultRouteInterface()
		if err != nil {
			return err
		}
		cfg.HostInterface = iface
	}
	if err := runNetworkCommand("ip", "tuntap", "add", "dev", cfg.TapName, "mode", "tap"); err != nil {
		if !strings.Contains(err.Error(), "File exists") {
			return err
		}
	}
	hostCIDR := cfg.HostIP + "/24"
	if !hostHasAddress(cfg.TapName, hostCIDR) {
		if err := runNetworkCommand("ip", "addr", "add", hostCIDR, "dev", cfg.TapName); err != nil {
			if !strings.Contains(err.Error(), "File exists") {
				return err
			}
		}
	}
	if err := runNetworkCommand("ip", "link", "set", "dev", cfg.TapName, "up"); err != nil {
		return err
	}
	if err := runNetworkCommand("iptables", "-t", "nat", "-C", "POSTROUTING", "-s", cfg.SubnetCIDR, "-o", cfg.HostInterface, "-j", "MASQUERADE"); err != nil {
		if err := runNetworkCommand("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", cfg.SubnetCIDR, "-o", cfg.HostInterface, "-j", "MASQUERADE"); err != nil {
			return err
		}
	}
	if err := runNetworkCommand("iptables", "-C", "FORWARD", "-i", cfg.HostInterface, "-o", cfg.TapName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"); err != nil {
		if err := runNetworkCommand("iptables", "-A", "FORWARD", "-i", cfg.HostInterface, "-o", cfg.TapName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"); err != nil {
			return err
		}
	}
	if err := runNetworkCommand("iptables", "-C", "FORWARD", "-i", cfg.TapName, "-o", cfg.HostInterface, "-j", "ACCEPT"); err != nil {
		if err := runNetworkCommand("iptables", "-A", "FORWARD", "-i", cfg.TapName, "-o", cfg.HostInterface, "-j", "ACCEPT"); err != nil {
			return err
		}
	}
	if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1\n"), 0o644); err != nil {
		return errTapNetworkingUnavailable("enable net.ipv4.ip_forward", err)
	}
	return nil
}

func teardownTapNetworking(cfg firecrackerNetworkConfig) {
	if cfg.TapName == "" {
		return
	}
	if cfg.HostInterface != "" && commandExists("iptables") {
		_ = exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING", "-s", cfg.SubnetCIDR, "-o", cfg.HostInterface, "-j", "MASQUERADE").Run()
		_ = exec.Command("iptables", "-D", "FORWARD", "-i", cfg.HostInterface, "-o", cfg.TapName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT").Run()
		_ = exec.Command("iptables", "-D", "FORWARD", "-i", cfg.TapName, "-o", cfg.HostInterface, "-j", "ACCEPT").Run()
	}
	if commandExists("ip") {
		_ = exec.Command("ip", "link", "set", "dev", cfg.TapName, "down").Run()
		_ = exec.Command("ip", "tuntap", "del", "dev", cfg.TapName, "mode", "tap").Run()
	}
}

func resolveResolvConfPath() string {
	for _, candidate := range []string{"/etc/resolv.conf", "/run/systemd/resolve/resolv.conf"} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func parseNetworkConfigFile(path string) (firecrackerNetworkConfig, error) {
	cfg := firecrackerNetworkConfig{}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "AIR_NETWORK_MODE":
			cfg.Mode = value
		case "AIR_NETWORK_TAP_NAME":
			cfg.TapName = value
		case "AIR_NETWORK_HOST_IP":
			cfg.HostIP = value
		case "AIR_NETWORK_GUEST_IP":
			cfg.GuestIP = value
		case "AIR_NETWORK_GATEWAY_IP":
			cfg.GatewayIP = value
		case "AIR_NETWORK_SUBNET_CIDR":
			cfg.SubnetCIDR = value
		case "AIR_NETWORK_GUEST_MAC":
			cfg.GuestMAC = value
		case "AIR_NETWORK_HOST_INTERFACE":
			cfg.HostInterface = value
		case "AIR_NETWORK_RESOLV_CONF":
			cfg.ResolvConfPath = value
		case "AIR_NETWORK_GUEST_IFACE":
			cfg.GuestIfaceName = value
		}
	}
	return cfg, nil
}

func serializeHostNetworkConfig(cfg firecrackerNetworkConfig) []byte {
	lines := []string{
		"AIR_NETWORK_MODE=" + cfg.Mode,
		"AIR_NETWORK_TAP_NAME=" + cfg.TapName,
		"AIR_NETWORK_HOST_IP=" + cfg.HostIP,
		"AIR_NETWORK_GUEST_IP=" + cfg.GuestIP,
		"AIR_NETWORK_GATEWAY_IP=" + cfg.GatewayIP,
		"AIR_NETWORK_SUBNET_CIDR=" + cfg.SubnetCIDR,
		"AIR_NETWORK_GUEST_MAC=" + cfg.GuestMAC,
		"AIR_NETWORK_HOST_INTERFACE=" + cfg.HostInterface,
		"AIR_NETWORK_RESOLV_CONF=" + cfg.ResolvConfPath,
		"AIR_NETWORK_GUEST_IFACE=" + cfg.GuestIfaceName,
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

func parseHostNetworkConfig(path string) (firecrackerNetworkConfig, error) {
	return parseNetworkConfigFile(path)
}

func hostNetworkConfigPath(configDir string) string {
	return filepath.Join(configDir, "network-host.env")
}

func guestNetworkConfigPath(configDir string) string {
	return filepath.Join(configDir, "network-guest.env")
}

func buildGuestNetworkCommand(cfg firecrackerNetworkConfig) string {
	if cfg.Mode != firecrackerNetworkFull {
		return ""
	}
	resolvStep := ""
	if cfg.ResolvConfPath != "" {
		resolvStep = "if [ -f '" + shellEscape(cfg.ResolvConfPath) + "' ]; then cp '" + shellEscape(cfg.ResolvConfPath) + "' /etc/resolv.conf 2>/dev/null || true; fi && "
	}
	return fmt.Sprintf(
		"mkdir -p /run/air && printf '%%s\\n' 'AIR_NETWORK_MODE=%s' 'AIR_NETWORK_GUEST_IFACE=%s' 'AIR_NETWORK_GUEST_IP=%s' 'AIR_NETWORK_GATEWAY_IP=%s' 'AIR_NETWORK_SUBNET_CIDR=%s' > /run/air/network.env && "+
			"%sip link set dev %s up && ip addr add %s/24 dev %s && ip route replace default via %s dev %s",
		cfg.Mode,
		cfg.GuestIfaceName,
		cfg.GuestIP,
		cfg.GatewayIP,
		cfg.SubnetCIDR,
		resolvStep,
		cfg.GuestIfaceName,
		cfg.GuestIP,
		cfg.GuestIfaceName,
		cfg.GatewayIP,
		cfg.GuestIfaceName,
	)
}

func shellEscape(value string) string {
	return strings.ReplaceAll(value, "'", "'\"'\"'")
}

func ipStringToInt(ip string) int {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return 0
	}
	total := 0
	for _, part := range parts {
		value, _ := strconv.Atoi(part)
		total = (total << 8) | value
	}
	return total
}

func validateStaticNetworkConfig(cfg firecrackerNetworkConfig) error {
	if cfg.Mode != firecrackerNetworkFull {
		return nil
	}
	for _, value := range []string{cfg.TapName, cfg.HostIP, cfg.GuestIP, cfg.GatewayIP, cfg.SubnetCIDR, cfg.GuestMAC, cfg.GuestIfaceName} {
		if strings.TrimSpace(value) == "" {
			return errTapNetworkingUnavailable("incomplete static network config", nil)
		}
	}
	if net.ParseIP(cfg.HostIP) == nil || net.ParseIP(cfg.GuestIP) == nil || net.ParseIP(cfg.GatewayIP) == nil {
		return errTapNetworkingUnavailable("invalid static IP configuration", nil)
	}
	if ipStringToInt(cfg.HostIP) == ipStringToInt(cfg.GuestIP) {
		return errTapNetworkingUnavailable("host and guest IP must differ", nil)
	}
	return nil
}
