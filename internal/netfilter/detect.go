package netfilter

import (
	"os/exec"
	"strings"
)

// Backend represents the firewall backend type.
type Backend int

const (
	BackendNone     Backend = iota
	BackendNftables
	BackendIptables
)

func (b Backend) String() string {
	switch b {
	case BackendNftables:
		return "nftables"
	case BackendIptables:
		return "iptables"
	default:
		return "none"
	}
}

// Detect returns the appropriate backend based on the forced string or auto-detection.
func Detect(forced string) Backend {
	switch forced {
	case "nftables":
		return BackendNftables
	case "iptables":
		return BackendIptables
	case "none":
		return BackendNone
	}
	// auto: prefer nftables
	if nftAvailable() {
		return BackendNftables
	}
	if iptAvailable() {
		return BackendIptables
	}
	return BackendNone
}

func nftAvailable() bool {
	path, err := exec.LookPath("nft")
	if err != nil {
		return false
	}
	out, err := exec.Command(path, "list", "tables").Output()
	if err != nil {
		return false
	}
	_ = out
	return true
}

func iptAvailable() bool {
	_, err := exec.LookPath("iptables")
	if err != nil {
		return false
	}
	out, err := exec.Command("iptables", "-L", "-n").CombinedOutput()
	if err != nil {
		return strings.Contains(string(out), "Chain")
	}
	return true
}
