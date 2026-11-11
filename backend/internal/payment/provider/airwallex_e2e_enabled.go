//go:build e2e_payment

package provider

import (
	"net/http"
	"net/url"
	"os"
	"strings"
)

// This opt-in transport is compiled only into the isolated payment E2E binary.
// It never changes provider configuration validation or production builds.
func applyAirwallexE2ETransport(req *http.Request) {
	if req == nil || req.URL == nil || req.URL.Hostname() != "api-demo.airwallex.com" {
		return
	}
	raw := strings.TrimSpace(os.Getenv("SUB2API_E2E_AIRWALLEX_API_BASE"))
	if raw == "" {
		return
	}
	target, err := url.Parse(raw)
	if err != nil || target.Scheme != "http" || (target.Hostname() != "127.0.0.1" && target.Hostname() != "localhost") {
		return
	}
	req.URL.Scheme = target.Scheme
	req.URL.Host = target.Host
	req.URL.Path = strings.TrimRight(target.Path, "/") + strings.TrimPrefix(req.URL.Path, "/api/v1")
}
