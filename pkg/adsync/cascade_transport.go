// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"fmt"
	"net/http"
	"net/url"
)

// Each configured authority owns an immutable TLS hostname verifier. In-flight
// forwarded requests may outlive a switch and must still verify their URL host.
type upstreamTransport map[string]*http.Transport

func (t upstreamTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport := t[request.URL.Host]
	if transport == nil {
		return nil, fmt.Errorf("adsync: request authority is not a configured upstream")
	}
	return transport.RoundTrip(request)
}
func (t upstreamTransport) CloseIdleConnections() {
	for _, transport := range t {
		transport.CloseIdleConnections()
	}
}
func cascadeTransport(config Config, base *http.Transport) http.RoundTripper {
	if config.tlsMaterial == nil || len(config.UpstreamURLs) < 2 {
		return base
	}
	result := upstreamTransport{}
	for _, entry := range config.UpstreamURLs {
		target, _ := url.Parse(entry) // Already validated by parseUpstreams.
		if result[target.Host] != nil {
			continue
		}
		transport := base.Clone()
		transport.TLSClientConfig = config.tlsMaterial.configForHost(target.Hostname())
		result[target.Host] = transport
	}
	return result
}
