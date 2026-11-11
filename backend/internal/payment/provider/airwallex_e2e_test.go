//go:build e2e_payment

package provider

import (
	"net/http"
	"testing"
)

func TestE2ETransportOnlyRewritesExplicitLoopbackTarget(t *testing.T) {
	t.Setenv("SUB2API_E2E_AIRWALLEX_API_BASE", "http://127.0.0.1:18921/api/v1")
	req, err := http.NewRequest(http.MethodGet, "https://api-demo.airwallex.com/api/v1/pa/refunds/create", nil)
	if err != nil {
		t.Fatal(err)
	}
	applyAirwallexE2ETransport(req)
	if got, want := req.URL.String(), "http://127.0.0.1:18921/api/v1/pa/refunds/create"; got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}

	productionReq, err := http.NewRequest(http.MethodGet, "https://api.airwallex.com/api/v1/pa/refunds/create", nil)
	if err != nil {
		t.Fatal(err)
	}
	applyAirwallexE2ETransport(productionReq)
	if productionReq.URL.Scheme != "https" || productionReq.URL.Host != "api.airwallex.com" {
		t.Fatalf("non-demo endpoint was rewritten: %s", productionReq.URL)
	}
}

func TestE2ETransportRejectsNonLoopbackTarget(t *testing.T) {
	t.Setenv("SUB2API_E2E_AIRWALLEX_API_BASE", "http://example.com")
	req, err := http.NewRequest(http.MethodGet, "https://api-demo.airwallex.com/api/v1/pa/refunds/create", nil)
	if err != nil {
		t.Fatal(err)
	}
	applyAirwallexE2ETransport(req)
	if req.URL.Host != "api-demo.airwallex.com" || req.URL.Scheme != "https" {
		t.Fatalf("non-loopback target was accepted: %s", req.URL)
	}
}
