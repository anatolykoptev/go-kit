// Package svcauth attaches the fleet's service-to-service credential to
// outbound requests, and only to the internal services it is meant for.
//
// Callers of internal services (ox-browser, go-wowa, memdb-go) each set
// X-Internal-Secret by hand, and shared libraries that also fetch arbitrary
// public URLs cannot set it at all without leaking it to third parties.
// Transport scopes the header by destination instead: wrap the client once,
// list the internal base URLs, and every request to one of them carries the
// secret while every other request, redirect hops included, carries nothing.
package svcauth

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// HeaderInternalSecret is the header internal services check.
const HeaderInternalSecret = "X-Internal-Secret"

// EnvInternalSecret names the env var holding the shared secret.
const EnvInternalSecret = "INTERNAL_SERVICE_SECRET"

// Route binds one internal service, by base URL, to the secret it accepts.
type Route struct {
	// BaseURL is the service origin, e.g. "http://ox-browser:8901". Only
	// scheme, host and port are used; an empty BaseURL is skipped, so a
	// route can be built straight from an optional env var.
	BaseURL string
	// Secret is sent as X-Internal-Secret. An empty Secret sends nothing.
	Secret string
}

// Transport is an http.RoundTripper that adds X-Internal-Secret to requests
// whose scheme, host and port match a Route.
type Transport struct {
	base   http.RoundTripper
	routes map[string]string // origin key -> secret
}

// New wraps base (http.DefaultTransport when nil) with the given routes.
// It returns an error for a BaseURL that does not parse to scheme://host,
// so a typo fails at startup instead of silently sending no credential.
func New(base http.RoundTripper, routes ...Route) (*Transport, error) {
	if base == nil {
		base = http.DefaultTransport
	}
	t := &Transport{base: base, routes: make(map[string]string, len(routes))}
	for _, r := range routes {
		if strings.TrimSpace(r.BaseURL) == "" || r.Secret == "" {
			continue
		}
		u, err := url.Parse(r.BaseURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, fmt.Errorf("svcauth: invalid base URL %q", r.BaseURL)
		}
		t.routes[originKey(u)] = r.Secret
	}
	return t, nil
}

// FromEnv wraps base with one route per non-empty base URL, all using
// INTERNAL_SERVICE_SECRET.
func FromEnv(base http.RoundTripper, baseURLs ...string) (*Transport, error) {
	secret := os.Getenv(EnvInternalSecret)
	routes := make([]Route, 0, len(baseURLs))
	for _, b := range baseURLs {
		routes = append(routes, Route{BaseURL: b, Secret: secret})
	}
	return New(base, routes...)
}

// WrapClient returns a shallow copy of c (a new client when nil) whose
// Transport is wrapped with routes. c itself is not modified.
func WrapClient(c *http.Client, routes ...Route) (*http.Client, error) {
	var cc http.Client
	if c != nil {
		cc = *c
	}
	t, err := New(cc.Transport, routes...)
	if err != nil {
		return nil, err
	}
	cc.Transport = t
	return &cc, nil
}

// RoundTrip implements http.RoundTripper. The request is cloned before the
// header is set, as the RoundTripper contract requires; http.Client calls
// RoundTrip again for every redirect hop, so a hop to another origin gets
// no credential.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if secret, ok := t.routes[originKey(req.URL)]; ok {
		req = req.Clone(req.Context())
		req.Header.Set(HeaderInternalSecret, secret)
	}
	return t.base.RoundTrip(req)
}

// originKey normalises scheme://host:port so "http://Ox-Browser:80" and
// "http://ox-browser" compare equal.
func originKey(u *url.URL) string {
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	return scheme + "://" + net.JoinHostPort(host, port)
}
