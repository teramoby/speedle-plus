// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"bufio"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// maxHostIPs bounds the addresses reported in the host object.
const maxHostIPs = 16

// HostInfo describes the machine an instance runs on. It is sent in the hello
// and in every long-poll heartbeat for display only; the source never uses it
// to identify or authorize the instance. Every field is best effort: a value
// that cannot be collected is left empty.
type HostInfo struct {
	Hostname  string   `json:"hostname"`
	OS        string   `json:"os"`
	OSVersion string   `json:"osVersion"`
	Arch      string   `json:"arch"`
	IPs       []string `json:"ips"`
	GoVersion string   `json:"goVersion"`
	PID       int      `json:"pid"`
	StartedAt int64    `json:"startedAt"`
	Container bool     `json:"container"`
	Transport string   `json:"transport"`
}

// CollectHostInfo gathers the host object. It never fails; startedAt is the
// process start as the channel saw it.
func CollectHostInfo(startedAt time.Time) HostInfo {
	hostname, _ := os.Hostname()
	return HostInfo{
		Hostname:  hostname,
		OS:        runtime.GOOS,
		OSVersion: osVersion(),
		Arch:      runtime.GOARCH,
		IPs:       hostIPs(),
		GoVersion: runtime.Version(),
		PID:       os.Getpid(),
		StartedAt: startedAt.UnixMilli(),
		Container: inContainer(),
	}
}

// osVersion reads PRETTY_NAME from /etc/os-release, falling back to the
// kernel release.
func osVersion() string {
	if file, err := os.Open("/etc/os-release"); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if value, found := strings.CutPrefix(line, "PRETTY_NAME="); found {
				return strings.Trim(value, `"'`)
			}
		}
	}
	if runtime.GOOS == "windows" {
		return ""
	}
	output, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// hostIPs lists up to maxHostIPs non-loopback interface addresses.
func hostIPs() []string {
	ips := []string{}
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}
	for _, address := range addresses {
		network, ok := address.(*net.IPNet)
		if !ok || network.IP.IsLoopback() || network.IP.IsUnspecified() {
			continue
		}
		ips = append(ips, network.IP.String())
		if len(ips) == maxHostIPs {
			break
		}
	}
	return ips
}

// inContainer detects a container from /.dockerenv or the cgroup of PID 1.
func inContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	data, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	text := string(data)
	for _, marker := range []string{"docker", "kubepods", "containerd", "lxc", "podman"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
