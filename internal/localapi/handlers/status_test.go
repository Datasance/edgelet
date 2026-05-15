package handlers

import "testing"

func TestAugmentWithDNSStatusAddsKeys(t *testing.T) {
	m := map[string]string{
		"iofogDaemon": "running",
	}
	augmentWithDNSStatus(m)

	requiredKeys := []string{
		"dnsStarted",
		"dnsCompatAliasesEnabled",
		"dnsRateLimitEnabled",
		"dnsRateLimitRPS",
		"dnsRateLimitBurst",
		"dnsMaxRequestBytes",
		"dnsMaxQNameBytes",
		"dnsScopeManagedListening",
		"dnsScopeManagedAddress",
		"dnsScopeLocalListening",
		"dnsScopeLocalAddress",
		"dnsQueriesTotal",
		"dnsSuccessTotal",
		"dnsNXDomainTotal",
		"dnsServFailTotal",
		"dnsPolicyDeniedTotal",
		"dnsInactiveTotal",
		"dnsForwardedTotal",
		"dnsForwardErrTotal",
		"dnsForwardingDegraded",
		"dnsForwardTotalUpstream",
		"dnsForwardHealthyUpstream",
		"dnsForwardLastSuccessUnix",
		"dnsForwardLastFailureUnix",
		"dnsForwardBackoffSkipTotal",
		"dnsRateLimitedTotal",
		"dnsRejectedTotal",
		"dnsHealth",
	}
	for _, key := range requiredKeys {
		if _, ok := m[key]; !ok {
			t.Fatalf("missing expected key %q", key)
		}
	}
}
