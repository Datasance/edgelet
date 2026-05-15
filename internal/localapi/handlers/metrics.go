package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/dnsresolver"
	"github.com/eclipse-iofog/agent/internal/statusreporter"
)

// MetricsHandler handles /metrics — Prometheus-format metrics.
// Exposes basic agent metrics for enterprise observability.
func MetricsHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	var b []byte
	// Agent uptime in seconds
	uptime := time.Since(MetricsStartTime()).Seconds()
	b = append(b, "# HELP iofog_agent_uptime_seconds Agent process uptime in seconds\n"...)
	b = append(b, "# TYPE iofog_agent_uptime_seconds gauge\n"...)
	b = append(b, "iofog_agent_uptime_seconds "...)
	b = append(b, strconv.FormatFloat(uptime, 'f', -1, 64)...)
	b = append(b, "\n"...)

	// Provisioned (1 or 0)
	cfg := config.GetInstance()
	provisioned := 0
	if cfg != nil && cfg.IOFogUUID != "" {
		provisioned = 1
	}
	b = append(b, "# HELP iofog_agent_provisioned Whether the agent is provisioned (1) or not (0)\n"...)
	b = append(b, "# TYPE iofog_agent_provisioned gauge\n"...)
	b = append(b, "iofog_agent_provisioned "...)
	b = append(b, strconv.Itoa(provisioned)...)
	b = append(b, "\n"...)

	// Running microservices count
	sr := statusreporter.GetInstance()
	runningCount := 0
	if sr != nil {
		if pm := sr.GetProcessManagerStatus(); pm != nil {
			runningCount = pm.RunningMicroservicesCount
		}
	}
	b = append(b, "# HELP iofog_agent_running_microservices Number of running microservices\n"...)
	b = append(b, "# TYPE iofog_agent_running_microservices gauge\n"...)
	b = append(b, "iofog_agent_running_microservices "...)
	b = append(b, strconv.Itoa(runningCount)...)
	b = append(b, "\n"...)

	dnsSnapshot := dnsresolver.GetInstance().Snapshot()
	b = append(b, "# HELP iofog_dns_queries_total Total DNS queries handled by embedded resolver\n"...)
	b = append(b, "# TYPE iofog_dns_queries_total counter\n"...)
	b = append(b, "iofog_dns_queries_total "...)
	b = append(b, strconv.FormatUint(dnsSnapshot.QueriesTotal, 10)...)
	b = append(b, "\n"...)

	b = append(b, "# HELP iofog_dns_success_total Total successful internal DNS responses\n"...)
	b = append(b, "# TYPE iofog_dns_success_total counter\n"...)
	b = append(b, "iofog_dns_success_total "...)
	b = append(b, strconv.FormatUint(dnsSnapshot.SuccessTotal, 10)...)
	b = append(b, "\n"...)

	b = append(b, "# HELP iofog_dns_nxdomain_total Total NXDOMAIN responses\n"...)
	b = append(b, "# TYPE iofog_dns_nxdomain_total counter\n"...)
	b = append(b, "iofog_dns_nxdomain_total "...)
	b = append(b, strconv.FormatUint(dnsSnapshot.NXDomainTotal, 10)...)
	b = append(b, "\n"...)

	b = append(b, "# HELP iofog_dns_servfail_total Total SERVFAIL responses\n"...)
	b = append(b, "# TYPE iofog_dns_servfail_total counter\n"...)
	b = append(b, "iofog_dns_servfail_total "...)
	b = append(b, strconv.FormatUint(dnsSnapshot.ServFailTotal, 10)...)
	b = append(b, "\n"...)

	b = append(b, "# HELP iofog_dns_policy_denied_total Total policy-denied cross-scope DNS responses\n"...)
	b = append(b, "# TYPE iofog_dns_policy_denied_total counter\n"...)
	b = append(b, "iofog_dns_policy_denied_total "...)
	b = append(b, strconv.FormatUint(dnsSnapshot.PolicyDeniedTotal, 10)...)
	b = append(b, "\n"...)

	b = append(b, "# HELP iofog_dns_inactive_total Total known-name inactive DNS responses (NODATA style)\n"...)
	b = append(b, "# TYPE iofog_dns_inactive_total counter\n"...)
	b = append(b, "iofog_dns_inactive_total "...)
	b = append(b, strconv.FormatUint(dnsSnapshot.InactiveTotal, 10)...)
	b = append(b, "\n"...)

	b = append(b, "# HELP iofog_dns_forwarded_total Total forwarded external DNS responses\n"...)
	b = append(b, "# TYPE iofog_dns_forwarded_total counter\n"...)
	b = append(b, "iofog_dns_forwarded_total "...)
	b = append(b, strconv.FormatUint(dnsSnapshot.ForwardedTotal, 10)...)
	b = append(b, "\n"...)

	b = append(b, "# HELP iofog_dns_forward_err_total Total forwarding failures for external DNS\n"...)
	b = append(b, "# TYPE iofog_dns_forward_err_total counter\n"...)
	b = append(b, "iofog_dns_forward_err_total "...)
	b = append(b, strconv.FormatUint(dnsSnapshot.ForwardErrTotal, 10)...)
	b = append(b, "\n"...)

	b = append(b, "# HELP iofog_dns_forward_backoff_skips_total Total upstream selections skipped due to backoff cooldown\n"...)
	b = append(b, "# TYPE iofog_dns_forward_backoff_skips_total counter\n"...)
	b = append(b, "iofog_dns_forward_backoff_skips_total "...)
	b = append(b, strconv.FormatUint(dnsSnapshot.ForwardBackoffSkipTotal, 10)...)
	b = append(b, "\n"...)

	b = append(b, "# HELP iofog_dns_rate_limited_total Total DNS requests denied by per-source rate limit\n"...)
	b = append(b, "# TYPE iofog_dns_rate_limited_total counter\n"...)
	b = append(b, "iofog_dns_rate_limited_total "...)
	b = append(b, strconv.FormatUint(dnsSnapshot.RateLimitedTotal, 10)...)
	b = append(b, "\n"...)

	b = append(b, "# HELP iofog_dns_rejected_total Total DNS requests rejected by safety guards (size/question/type/class)\n"...)
	b = append(b, "# TYPE iofog_dns_rejected_total counter\n"...)
	b = append(b, "iofog_dns_rejected_total "...)
	b = append(b, strconv.FormatUint(dnsSnapshot.RejectedTotal, 10)...)
	b = append(b, "\n"...)

	dnsStarted := 0
	if dnsSnapshot.Started {
		dnsStarted = 1
	}
	b = append(b, "# HELP iofog_dns_started Whether embedded DNS resolver is started (1) or not (0)\n"...)
	b = append(b, "# TYPE iofog_dns_started gauge\n"...)
	b = append(b, "iofog_dns_started "...)
	b = append(b, strconv.Itoa(dnsStarted)...)
	b = append(b, "\n"...)

	managedListening := 0
	if dnsSnapshot.ScopeManaged.Listening {
		managedListening = 1
	}
	b = append(b, "# HELP iofog_dns_scope_listening Embedded DNS scope listener status by scope (1 listening, 0 not listening)\n"...)
	b = append(b, "# TYPE iofog_dns_scope_listening gauge\n"...)
	b = append(b, "iofog_dns_scope_listening{scope=\"iofog\"} "...)
	b = append(b, strconv.Itoa(managedListening)...)
	b = append(b, "\n"...)

	localListening := 0
	if dnsSnapshot.ScopeLocal.Listening {
		localListening = 1
	}
	b = append(b, "iofog_dns_scope_listening{scope=\"iofog-local\"} "...)
	b = append(b, strconv.Itoa(localListening)...)
	b = append(b, "\n"...)

	rateLimitEnabled := 0
	if dnsSnapshot.RateLimitEnabled {
		rateLimitEnabled = 1
	}
	b = append(b, "# HELP iofog_dns_rate_limit_enabled Whether DNS rate limiting is enabled (1) or disabled (0)\n"...)
	b = append(b, "# TYPE iofog_dns_rate_limit_enabled gauge\n"...)
	b = append(b, "iofog_dns_rate_limit_enabled "...)
	b = append(b, strconv.Itoa(rateLimitEnabled)...)
	b = append(b, "\n"...)

	b = append(b, "# HELP iofog_dns_rate_limit_rps Configured DNS rate limit requests per second per source\n"...)
	b = append(b, "# TYPE iofog_dns_rate_limit_rps gauge\n"...)
	b = append(b, "iofog_dns_rate_limit_rps "...)
	b = append(b, strconv.Itoa(dnsSnapshot.RateLimitRPS)...)
	b = append(b, "\n"...)

	b = append(b, "# HELP iofog_dns_rate_limit_burst Configured DNS token burst capacity per source\n"...)
	b = append(b, "# TYPE iofog_dns_rate_limit_burst gauge\n"...)
	b = append(b, "iofog_dns_rate_limit_burst "...)
	b = append(b, strconv.Itoa(dnsSnapshot.RateLimitBurst)...)
	b = append(b, "\n"...)

	b = append(b, "# HELP iofog_dns_max_request_bytes Configured max accepted DNS request size in bytes\n"...)
	b = append(b, "# TYPE iofog_dns_max_request_bytes gauge\n"...)
	b = append(b, "iofog_dns_max_request_bytes "...)
	b = append(b, strconv.Itoa(dnsSnapshot.MaxRequestBytes)...)
	b = append(b, "\n"...)

	b = append(b, "# HELP iofog_dns_max_qname_bytes Configured max accepted normalized DNS qname length\n"...)
	b = append(b, "# TYPE iofog_dns_max_qname_bytes gauge\n"...)
	b = append(b, "iofog_dns_max_qname_bytes "...)
	b = append(b, strconv.Itoa(dnsSnapshot.MaxQNameBytes)...)
	b = append(b, "\n"...)

	forwardingDegraded := 0
	if dnsSnapshot.ForwardingDegraded {
		forwardingDegraded = 1
	}
	b = append(b, "# HELP iofog_dns_forwarding_degraded Whether forwarding path is degraded (1) or healthy (0)\n"...)
	b = append(b, "# TYPE iofog_dns_forwarding_degraded gauge\n"...)
	b = append(b, "iofog_dns_forwarding_degraded "...)
	b = append(b, strconv.Itoa(forwardingDegraded)...)
	b = append(b, "\n"...)

	b = append(b, "# HELP iofog_dns_forward_upstreams_total Number of configured upstream resolvers currently tracked\n"...)
	b = append(b, "# TYPE iofog_dns_forward_upstreams_total gauge\n"...)
	b = append(b, "iofog_dns_forward_upstreams_total "...)
	b = append(b, strconv.FormatUint(dnsSnapshot.ForwardTotalUpstream, 10)...)
	b = append(b, "\n"...)

	b = append(b, "# HELP iofog_dns_forward_upstreams_healthy Number of upstream resolvers currently considered healthy\n"...)
	b = append(b, "# TYPE iofog_dns_forward_upstreams_healthy gauge\n"...)
	b = append(b, "iofog_dns_forward_upstreams_healthy "...)
	b = append(b, strconv.FormatUint(dnsSnapshot.ForwardHealthyUpstream, 10)...)
	b = append(b, "\n"...)

	b = append(b, "# HELP iofog_dns_forward_last_success_unix_ms Last successful forwarded DNS response timestamp (unix ms)\n"...)
	b = append(b, "# TYPE iofog_dns_forward_last_success_unix_ms gauge\n"...)
	b = append(b, "iofog_dns_forward_last_success_unix_ms "...)
	b = append(b, strconv.FormatInt(dnsSnapshot.ForwardLastSuccessUnix, 10)...)
	b = append(b, "\n"...)

	b = append(b, "# HELP iofog_dns_forward_last_failure_unix_ms Last failed forwarding attempt timestamp (unix ms)\n"...)
	b = append(b, "# TYPE iofog_dns_forward_last_failure_unix_ms gauge\n"...)
	b = append(b, "iofog_dns_forward_last_failure_unix_ms "...)
	b = append(b, strconv.FormatInt(dnsSnapshot.ForwardLastFailureUnix, 10)...)
	b = append(b, "\n"...)

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}
