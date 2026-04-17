package middleware

import (
	"net"
	"strings"
)

// RealIP extracts the most likely client IP address from an HTTP request.
// When trustedProxyCIDRs is non-empty and the direct peer (r.RemoteAddr) matches
// one of them, the function walks the X-Forwarded-For chain from right to left
// and returns the first entry NOT from a trusted proxy.
//
// When trustedProxyCIDRs is empty or the peer is not trusted, the function falls
// back to r.RemoteAddr (stripping the port).
//
// This prevents IP spoofing attacks where an untrusted client injects arbitrary
// X-Forwarded-For headers.
func RealIP(remoteAddr string, xForwardedFor string, trustedProxyCIDRs []*net.IPNet) net.IP {
	peerIP := parseHost(remoteAddr)
	if peerIP == nil {
		return nil
	}

	// No trusted proxies configured — always use direct peer.
	if len(trustedProxyCIDRs) == 0 {
		return peerIP
	}

	// Peer must be a trusted proxy for XFF to be consulted.
	if !isTrustedProxy(peerIP, trustedProxyCIDRs) {
		return peerIP
	}

	// Walk XFF right-to-left; return the first non-trusted entry.
	parts := strings.Split(xForwardedFor, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := net.ParseIP(strings.TrimSpace(parts[i]))
		if candidate == nil {
			continue
		}
		if !isTrustedProxy(candidate, trustedProxyCIDRs) {
			return candidate
		}
	}

	// All XFF entries are trusted proxies — fall back to peer.
	return peerIP
}

func parseHost(remoteAddr string) net.IP {
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}
	return net.ParseIP(host)
}

func isTrustedProxy(ip net.IP, trusted []*net.IPNet) bool {
	for _, cidr := range trusted {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}
