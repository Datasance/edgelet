package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsHandlerIncludesDNSMetrics(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)

	MetricsHandler(rr, req)
	body := rr.Body.String()

	expected := []string{
		"iofog_dns_queries_total",
		"iofog_dns_success_total",
		"iofog_dns_nxdomain_total",
		"iofog_dns_servfail_total",
		"iofog_dns_policy_denied_total",
		"iofog_dns_inactive_total",
		"iofog_dns_forwarded_total",
		"iofog_dns_forward_err_total",
		"iofog_dns_forward_backoff_skips_total",
		"iofog_dns_rate_limited_total",
		"iofog_dns_rejected_total",
		"iofog_dns_started",
		"iofog_dns_scope_listening{scope=\"iofog\"}",
		"iofog_dns_scope_listening{scope=\"iofog-local\"}",
		"iofog_dns_rate_limit_enabled",
		"iofog_dns_rate_limit_rps",
		"iofog_dns_rate_limit_burst",
		"iofog_dns_max_request_bytes",
		"iofog_dns_max_qname_bytes",
		"iofog_dns_forwarding_degraded",
		"iofog_dns_forward_upstreams_total",
		"iofog_dns_forward_upstreams_healthy",
		"iofog_dns_forward_last_success_unix_ms",
		"iofog_dns_forward_last_failure_unix_ms",
	}
	for _, metric := range expected {
		if !strings.Contains(body, metric) {
			t.Fatalf("metrics output missing %q", metric)
		}
	}
}
