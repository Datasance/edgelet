package dnsresolver

import (
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/datasance/edgelet/internal/utils/logging"
)

const (
	defaultRateLimitRPS      = 200
	defaultRateLimitBurst    = 400
	defaultMaxRequestBytes   = 4096
	defaultMaxQNameBytes     = 253
	defaultBucketIdleTTL     = 10 * time.Minute
	defaultLogSampleInterval = 5 * time.Second
)

type sourceTokenBucket struct {
	tokens     float64
	lastRefill int64
	lastSeen   int64
}

type sourceRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*sourceTokenBucket
	rps     float64
	burst   float64
	ttlMs   int64
}

type sampledLogger struct {
	mu         sync.Mutex
	lastByKey  map[string]int64
	intervalMs int64
}

func newSourceRateLimiter(rps int, burst int, ttl time.Duration) *sourceRateLimiter {
	if rps <= 0 {
		rps = 1
	}
	if burst <= 0 {
		burst = 1
	}
	return &sourceRateLimiter{
		buckets: make(map[string]*sourceTokenBucket),
		rps:     float64(rps),
		burst:   float64(burst),
		ttlMs:   ttl.Milliseconds(),
	}
}

func (l *sourceRateLimiter) Allow(source string, now time.Time) bool {
	nowMs := now.UnixMilli()
	l.mu.Lock()
	defer l.mu.Unlock()

	l.cleanup(nowMs)
	b, ok := l.buckets[source]
	if !ok {
		b = &sourceTokenBucket{
			tokens:     l.burst,
			lastRefill: nowMs,
			lastSeen:   nowMs,
		}
		l.buckets[source] = b
	}
	elapsedSec := float64(nowMs-b.lastRefill) / float64(time.Second.Milliseconds())
	if elapsedSec > 0 {
		b.tokens += elapsedSec * l.rps
		if b.tokens > l.burst {
			b.tokens = l.burst
		}
		b.lastRefill = nowMs
	}
	b.lastSeen = nowMs
	if b.tokens < 1.0 {
		return false
	}
	b.tokens -= 1.0
	return true
}

func (l *sourceRateLimiter) cleanup(nowMs int64) {
	if l.ttlMs <= 0 {
		return
	}
	for key, b := range l.buckets {
		if b == nil {
			delete(l.buckets, key)
			continue
		}
		if nowMs-b.lastSeen > l.ttlMs {
			delete(l.buckets, key)
		}
	}
}

func newSampledLogger(interval time.Duration) *sampledLogger {
	return &sampledLogger{
		lastByKey:  make(map[string]int64),
		intervalMs: interval.Milliseconds(),
	}
}

func (s *sampledLogger) ShouldLog(key string, now time.Time) bool {
	nowMs := now.UnixMilli()
	s.mu.Lock()
	defer s.mu.Unlock()
	last := s.lastByKey[key]
	if nowMs-last < s.intervalMs {
		return false
	}
	s.lastByKey[key] = nowMs
	return true
}

func (r *Resolver) ensureGuardrailsInitialized() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rateLimiter != nil && r.logSampler != nil {
		return
	}
	r.applyGuardrailConfigFromEnvLocked()
}

func (r *Resolver) applyGuardrailConfigFromEnvLocked() {
	enabled := true
	if raw := strings.TrimSpace(os.Getenv("EDGELET_DNS_RATE_LIMIT_ENABLED")); raw != "" {
		enabled = !strings.EqualFold(raw, "false")
	}

	rps := parseIntWithDefault("EDGELET_DNS_RATE_LIMIT_RPS", defaultRateLimitRPS)
	burst := parseIntWithDefault("EDGELET_DNS_RATE_LIMIT_BURST", defaultRateLimitBurst)
	maxReqBytes := parseIntWithDefault("EDGELET_DNS_MAX_REQUEST_BYTES", defaultMaxRequestBytes)
	maxQNameBytes := parseIntWithDefault("EDGELET_DNS_MAX_QNAME_BYTES", defaultMaxQNameBytes)
	r.rateLimitEnabled = enabled
	r.rateLimitRPS = rps
	r.rateLimitBurst = burst
	r.maxRequestBytes = maxReqBytes
	r.maxQNameBytes = maxQNameBytes
	r.rateLimiter = newSourceRateLimiter(rps, burst, defaultBucketIdleTTL)
	if r.logSampler == nil {
		r.logSampler = newSampledLogger(defaultLogSampleInterval)
	}
}

func parseIntWithDefault(envKey string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(envKey))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		logging.LogWarn(moduleName, "invalid "+envKey+" value="+raw+", using default")
		return fallback
	}
	return v
}

func sourceKeyFromAddr(addr net.Addr) string {
	if addr == nil {
		return "unknown"
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		if strings.TrimSpace(addr.String()) == "" {
			return "unknown"
		}
		return addr.String()
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return "unknown"
	}
	return host
}

func (r *Resolver) warnSampled(key, msg string) {
	r.mu.RLock()
	s := r.logSampler
	r.mu.RUnlock()
	if s == nil {
		logging.LogWarn(moduleName, msg)
		return
	}
	if s.ShouldLog(key, time.Now()) {
		logging.LogWarn(moduleName, msg)
	}
}
