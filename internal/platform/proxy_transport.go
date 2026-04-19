package platform

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// ProxyTransport wraps a standard http.Transport with a dynamically
// configurable proxy function. All market-data HTTP clients share one
// instance so that changing the proxy setting takes effect globally.
type ProxyTransport struct {
	mu   sync.RWMutex
	mode string   // "system" | "custom" | "none"
	url  *url.URL // non-nil only when mode == "custom"

	inner *http.Transport
}

// NewProxyTransport builds a ProxyTransport initialised to the given mode/URL.
// Pass mode "system", "custom", or "none".
func NewProxyTransport(mode, rawURL string) *ProxyTransport {
	pt := &ProxyTransport{mode: mode}
	if mode == "custom" && rawURL != "" {
		pt.url, _ = url.Parse(rawURL)
	}
	pt.inner = &http.Transport{
		Proxy:                 pt.proxyFunc,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig:      &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:  8 * time.Second,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       60 * time.Second,
		ResponseHeaderTimeout: 12 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return pt
}

// Update changes the proxy configuration at runtime and closes idle
// connections so that subsequent requests use the new settings.
func (pt *ProxyTransport) Update(mode, rawURL string) {
	pt.mu.Lock()
	pt.mode = mode
	pt.url = nil
	if mode == "custom" && rawURL != "" {
		pt.url, _ = url.Parse(rawURL)
	}
	pt.mu.Unlock()
	pt.inner.CloseIdleConnections()
}

// RoundTrip implements http.RoundTripper.
func (pt *ProxyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return pt.inner.RoundTrip(req)
}

// proxyFunc is the dynamic proxy resolution callback used by the inner transport.
func (pt *ProxyTransport) proxyFunc(req *http.Request) (*url.URL, error) {
	pt.mu.RLock()
	mode := pt.mode
	u := pt.url
	pt.mu.RUnlock()

	switch mode {
	case "none":
		return nil, nil
	case "custom":
		if u != nil {
			return u, nil
		}
		return nil, nil
	default: // "system"
		return http.ProxyFromEnvironment(req)
	}
}

// NewHTTPClient returns an *http.Client that uses the given ProxyTransport.
func NewHTTPClient(pt *ProxyTransport) *http.Client {
	return &http.Client{
		Timeout:   15 * time.Second,
		Transport: pt,
	}
}
