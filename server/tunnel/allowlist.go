package tunnel

import (
	"net"
	"strings"
)

func visitorAllowed(remoteAddr net.Addr, allowlist []string) bool {
	if len(allowlist) == 0 {
		return true
	}
	host, _, err := net.SplitHostPort(remoteAddr.String())
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, entry := range allowlist {
		if addressMatches(ip, strings.TrimSpace(entry)) {
			return true
		}
	}
	return false
}

func addressMatches(ip net.IP, entry string) bool {
	if allowed := net.ParseIP(entry); allowed != nil {
		return allowed.Equal(ip)
	}
	_, network, err := net.ParseCIDR(entry)
	return err == nil && network.Contains(ip)
}
