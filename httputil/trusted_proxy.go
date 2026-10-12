package httputil

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

const ipv4InIPv6Bits = 96

// TrustedProxyClientIP returns a client-IP extractor that only believes
// X-Forwarded-For when the direct peer is a trusted proxy.
//
//   - Peer (r.RemoteAddr host) not in trusted: the peer is returned and every
//     forwarding header is ignored.
//   - Peer trusted: X-Forwarded-For (all header lines) is walked right to
//     left and the first hop that is not in trusted is returned (the
//     "rightmost untrusted" rule, as in labstack/echo ExtractIPFromXFFHeader
//     and realclientip-go RightmostTrustedRangeStrategy). If XFF is absent,
//     every hop is trusted, or a hop does not parse before an untrusted one
//     is found, the peer is returned.
//
// X-Real-IP is never read. IPv4-mapped IPv6 addresses are unmapped before the
// prefix check. Unlike [ClientIP], a client cannot choose the result by
// sending its own headers.
func TrustedProxyClientIP(trusted []netip.Prefix) func(*http.Request) string {
	return func(r *http.Request) string {
		host := remoteHost(r.RemoteAddr)
		peer, err := netip.ParseAddr(host)
		if err != nil || !containsAddr(trusted, peer) {
			return host
		}
		lines := r.Header.Values("X-Forwarded-For")
		for i := len(lines) - 1; i >= 0; i-- {
			hops := strings.Split(lines[i], ",")
			for j := len(hops) - 1; j >= 0; j-- {
				hop, err := netip.ParseAddr(strings.TrimSpace(hops[j]))
				if err != nil {
					return host
				}
				if !containsAddr(trusted, hop) {
					return hop.Unmap().WithZone("").String()
				}
			}
		}
		return host
	}
}

// ParseTrustedProxies parses a comma-separated list of CIDRs or bare IPs
// (bare IPs become /32 or /128) for [TrustedProxyClientIP]. Empty entries are
// skipped; an invalid entry yields an error naming it.
func ParseTrustedProxies(csv string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, raw := range strings.Split(csv, ",") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		p, err := netip.ParsePrefix(entry)
		if err != nil {
			addr, aerr := netip.ParseAddr(entry)
			if aerr != nil {
				return nil, fmt.Errorf("httputil: invalid trusted proxy %q: %w", entry, err)
			}
			p = netip.PrefixFrom(addr, addr.BitLen())
		}
		if a := p.Addr(); a.Is4In6() && p.Bits() >= ipv4InIPv6Bits {
			p = netip.PrefixFrom(a.Unmap(), p.Bits()-ipv4InIPv6Bits)
		}
		out = append(out, p)
	}
	return out, nil
}

func remoteHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

func containsAddr(prefixes []netip.Prefix, a netip.Addr) bool {
	a = a.Unmap().WithZone("")
	for _, p := range prefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
