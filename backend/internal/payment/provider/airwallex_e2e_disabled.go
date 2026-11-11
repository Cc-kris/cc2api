//go:build !e2e_payment

package provider

import "net/http"

// Production builds retain the configured, allow-listed HTTPS endpoint.
func applyAirwallexE2ETransport(_ *http.Request) {}
