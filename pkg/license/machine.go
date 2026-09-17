package license

import (
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const machineSalt = "Takealot-AutoBest-Salt-2025"

// GetMachineID returns a normalized, stable, salted machine identifier formatted as TK-XXXX-XXXX-XXXX-XXXX.
func GetMachineID() string {
	raw := getRawMachineID()
	if strings.TrimSpace(raw) == "" {
		raw = getFallbackID()
	}

	h := sha256.New()
	h.Write([]byte(raw))
	h.Write([]byte(machineSalt))
	sum := h.Sum(nil)

	// Take first 8 bytes (16 hex chars) and format as TK-XXXX-XXXX-XXXX-XXXX
	hexStr := fmt.Sprintf("%X", sum[:8])
	if len(hexStr) >= 16 {
		return fmt.Sprintf("TK-%s-%s-%s-%s", hexStr[0:4], hexStr[4:8], hexStr[8:12], hexStr[12:16])
	}
	return "TK-" + hexStr
}

func getRawMachineID() string {
	switch runtime.GOOS {
	case "darwin":
		return getDarwinUUID()
	case "windows":
		return getWindowsUUID()
	case "linux":
		return getLinuxUUID()
	default:
		return getFallbackID()
	}
}

func getDarwinUUID() string {
	// 1. Try ioreg for IOPlatformUUID
	cmd := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "IOPlatformUUID") {
				parts := strings.Split(line, "=")
				if len(parts) >= 2 {
					uuid := strings.TrimSpace(parts[1])
					uuid = strings.Trim(uuid, `" `)
					if uuid != "" {
						return uuid
					}
				}
			}
		}
	}

	// 2. Try sysctl kern.uuid
	cmd = exec.Command("sysctl", "-n", "kern.uuid")
	out, err = cmd.Output()
	if err == nil {
		id := strings.TrimSpace(string(out))
		if id != "" {
			return id
		}
	}

	return ""
}

func getWindowsUUID() string {
	// 1. Try querying registry MachineGuid
	cmd := exec.Command("reg", "query", `HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Cryptography`, "/v", "MachineGuid")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "MachineGuid") {
				fields := strings.Fields(line)
				if len(fields) >= 3 {
					return strings.TrimSpace(fields[len(fields)-1])
				}
			}
		}
	}

	// 2. Fallback to wmic csproduct get uuid
	cmd = exec.Command("wmic", "csproduct", "get", "uuid")
	out, err = cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.EqualFold(trimmed, "uuid") {
				return trimmed
			}
		}
	}

	return ""
}

func getLinuxUUID() string {
	files := []string{
		"/etc/machine-id",
		"/var/lib/dbus/machine-id",
		"/sys/class/dmi/id/product_uuid",
	}

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err == nil {
			id := strings.TrimSpace(string(data))
			if id != "" {
				return id
			}
		}
	}
	return ""
}

func getFallbackID() string {
	var parts []string
	if hostname, err := os.Hostname(); err == nil {
		parts = append(parts, hostname)
	}
	if home, err := os.UserHomeDir(); err == nil {
		parts = append(parts, home)
	}
	parts = append(parts, fmt.Sprintf("%d", runtime.NumCPU()))

	// Try first MAC address
	if ifaces, err := net.Interfaces(); err == nil {
		for _, iface := range ifaces {
			if len(iface.HardwareAddr) > 0 {
				parts = append(parts, iface.HardwareAddr.String())
				break
			}
		}
	}

	return strings.Join(parts, "::")
}
