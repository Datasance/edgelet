package dnsresolver

import (
	"context"
	"fmt"
	"net"
	"sort"
	"time"

	"github.com/miekg/dns"
)

const (
	forwardMaxAttempts = 3
	forwardBackoffBase = 250 * time.Millisecond
	forwardBackoffMax  = 5 * time.Second
)

type upstreamForwardState struct {
	lastSuccessUnix int64
	lastFailureUnix int64
	successStreak   uint32
	failureStreak   uint32
	cooldownUntil   int64
}

type forwardCandidate struct {
	addr string
	st   *upstreamForwardState
}

func defaultForwardResolvers() ([]string, error) {
	cfg, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil {
		return nil, fmt.Errorf("load resolv.conf: %w", err)
	}
	addrs := make([]string, 0, len(cfg.Servers))
	seen := make(map[string]struct{}, len(cfg.Servers))
	for _, ns := range cfg.Servers {
		addr := net.JoinHostPort(ns, cfg.Port)
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		addrs = append(addrs, addr)
	}
	return addrs, nil
}

func (r *Resolver) forwardExternal(req *dns.Msg) (*dns.Msg, error) {
	r.mu.RLock()
	resolve := r.forwardResolveFn
	nowFn := r.forwardNowFn
	r.mu.RUnlock()
	if resolve == nil {
		resolve = defaultForwardResolvers
	}
	if nowFn == nil {
		nowFn = time.Now
	}

	addrs, err := resolve()
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no upstream resolvers configured")
	}

	now := nowFn()
	candidates := r.rankForwardCandidates(addrs, now)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no upstream resolver candidates available")
	}

	client := &dns.Client{Timeout: defaultForwardTimeout}
	attempts := 0
	var lastErr error
	for _, cand := range candidates {
		if attempts >= forwardMaxAttempts {
			break
		}
		resp, _, exErr := client.ExchangeContext(context.Background(), req, cand.addr)
		attempts++
		if exErr == nil && resp != nil {
			r.recordForwardSuccess(cand.addr, nowFn())
			return resp, nil
		}
		lastErr = exErr
		r.recordForwardFailure(cand.addr, nowFn())
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("all upstream resolvers failed")
	}
	return nil, lastErr
}

func (r *Resolver) rankForwardCandidates(addrs []string, now time.Time) []forwardCandidate {
	r.mu.Lock()
	defer r.mu.Unlock()

	nowUnix := now.UnixMilli()
	active := make([]forwardCandidate, 0, len(addrs))
	backoff := make([]forwardCandidate, 0, len(addrs))
	for _, addr := range addrs {
		st, ok := r.forwardState[addr]
		if !ok {
			st = &upstreamForwardState{}
			r.forwardState[addr] = st
		}
		c := forwardCandidate{addr: addr, st: st}
		if st.cooldownUntil > nowUnix {
			backoff = append(backoff, c)
			r.forwardBackoffSkips.Add(1)
			continue
		}
		active = append(active, c)
	}

	sort.SliceStable(active, func(i, j int) bool {
		if active[i].st.failureStreak == active[j].st.failureStreak {
			return active[i].st.lastSuccessUnix > active[j].st.lastSuccessUnix
		}
		return active[i].st.failureStreak < active[j].st.failureStreak
	})
	sort.SliceStable(backoff, func(i, j int) bool {
		return backoff[i].st.cooldownUntil < backoff[j].st.cooldownUntil
	})

	out := make([]forwardCandidate, 0, len(addrs))
	out = append(out, active...)
	if len(out) < forwardMaxAttempts {
		out = append(out, backoff...)
	}
	return out
}

func (r *Resolver) recordForwardSuccess(addr string, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.forwardState[addr]
	if !ok {
		st = &upstreamForwardState{}
		r.forwardState[addr] = st
	}
	st.lastSuccessUnix = now.UnixMilli()
	st.successStreak++
	st.failureStreak = 0
	st.cooldownUntil = 0
	r.forwardDegraded = false
	r.forwardLastSuccessUnix = st.lastSuccessUnix
}

func (r *Resolver) recordForwardFailure(addr string, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.forwardState[addr]
	if !ok {
		st = &upstreamForwardState{}
		r.forwardState[addr] = st
	}
	st.lastFailureUnix = now.UnixMilli()
	st.failureStreak++
	st.successStreak = 0
	backoff := backoffDuration(st.failureStreak)
	st.cooldownUntil = st.lastFailureUnix + backoff.Milliseconds()
	r.forwardDegraded = true
	r.forwardLastFailureUnix = st.lastFailureUnix
}

func (r *Resolver) forwardHealthCountsLocked() (uint64, uint64) {
	total := uint64(len(r.forwardState))
	if total == 0 {
		return 0, 0
	}
	nowUnix := time.Now().UnixMilli()
	healthy := uint64(0)
	for _, st := range r.forwardState {
		if st == nil {
			continue
		}
		if st.cooldownUntil <= nowUnix {
			healthy++
		}
	}
	return total, healthy
}

func backoffDuration(failureStreak uint32) time.Duration {
	if failureStreak == 0 {
		return 0
	}
	backoff := forwardBackoffBase
	for i := uint32(1); i < failureStreak; i++ {
		backoff *= 2
		if backoff >= forwardBackoffMax {
			return forwardBackoffMax
		}
	}
	if backoff > forwardBackoffMax {
		return forwardBackoffMax
	}
	return backoff
}
