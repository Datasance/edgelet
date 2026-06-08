package dnsresolver

import (
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

type testDNSWriter struct {
	msg    *dns.Msg
	remote net.Addr
}

func (w *testDNSWriter) LocalAddr() net.Addr { return &net.UDPAddr{} }
func (w *testDNSWriter) RemoteAddr() net.Addr {
	if w.remote != nil {
		return w.remote
	}
	return &net.UDPAddr{}
}
func (w *testDNSWriter) WriteMsg(m *dns.Msg) error   { w.msg = m; return nil }
func (w *testDNSWriter) Write(_ []byte) (int, error) { return 0, nil }
func (w *testDNSWriter) Close() error                { return nil }
func (w *testDNSWriter) TsigStatus() error           { return nil }
func (w *testDNSWriter) TsigTimersOnly(bool)         {}
func (w *testDNSWriter) Hijack()                     {}

func startTestDNSServer(t *testing.T, ip string) (string, func()) {
	t.Helper()
	ln, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	server := &dns.Server{
		PacketConn: ln,
		Handler: dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
			resp := new(dns.Msg)
			resp.SetReply(req)
			resp.Authoritative = false
			if len(req.Question) > 0 {
				q := req.Question[0]
				resp.Answer = append(resp.Answer, rrFromIP(normalizeName(q.Name), ip, q.Qtype)...)
			}
			_ = w.WriteMsg(resp)
		}),
	}
	go func() { _ = server.ActivateAndServe() }()
	return ln.LocalAddr().String(), func() {
		_ = server.Shutdown()
		_ = ln.Close()
	}
}

func TestForwardingBackoffAndHealthyPreference(t *testing.T) {
	r := newTestResolver()
	goodAddr, cleanup := startTestDNSServer(t, "1.2.3.4")
	defer cleanup()

	badAddr := "127.0.0.1:9"
	r.SetForwardResolverProvider(func() ([]string, error) {
		return []string{badAddr, goodAddr}, nil
	})

	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeA)

	w1 := &testDNSWriter{}
	r.handleDNSQuery(ScopeManaged, w1, req)
	if w1.msg == nil || w1.msg.Rcode != dns.RcodeSuccess || len(w1.msg.Answer) == 0 {
		t.Fatal("expected successful forwarded response on first query")
	}

	badState := r.forwardState[badAddr]
	if badState == nil || badState.failureStreak == 0 {
		t.Fatal("expected bad upstream failure state after first query")
	}
	firstFailures := badState.failureStreak

	w2 := &testDNSWriter{}
	r.handleDNSQuery(ScopeManaged, w2, req)
	if w2.msg == nil || w2.msg.Rcode != dns.RcodeSuccess || len(w2.msg.Answer) == 0 {
		t.Fatal("expected successful forwarded response on second query")
	}

	if r.forwardState[badAddr].failureStreak != firstFailures {
		t.Fatal("expected bad upstream not retried while in backoff")
	}
	if snap := r.Snapshot(); snap.ForwardBackoffSkipTotal == 0 {
		t.Fatal("expected backoff skip counter to increase")
	}
}

func TestForwardingDegradedSignalOnFailure(t *testing.T) {
	r := newTestResolver()
	r.SetForwardResolverProvider(func() ([]string, error) {
		return []string{"127.0.0.1:9"}, nil
	})
	req := new(dns.Msg)
	req.SetQuestion("external.invalid.", dns.TypeA)

	w := &testDNSWriter{}
	r.handleDNSQuery(ScopeManaged, w, req)
	if w.msg == nil || w.msg.Rcode != dns.RcodeServerFailure {
		t.Fatal("expected SERVFAIL on total forwarding failure")
	}
	snap := r.Snapshot()
	if !snap.ForwardingDegraded {
		t.Fatal("expected forwarding degraded state after failure")
	}
	if snap.ForwardErrTotal == 0 {
		t.Fatal("expected forward error counter increment")
	}
}

func TestInternalResolutionUnaffectedByForwardingFailures(t *testing.T) {
	r := newTestResolver()
	r.UpsertWorkload(WorkloadRecord{
		UUID:        "ms1",
		Application: "app",
		Name:        "svc",
		Scope:       ScopeManaged,
		IP:          "10.1.1.2",
		Active:      true,
	})
	r.recordForwardFailure("127.0.0.1:9", time.Now())

	req := new(dns.Msg)
	req.SetQuestion("app.svc.", dns.TypeA)
	w := &testDNSWriter{}
	r.handleDNSQuery(ScopeManaged, w, req)

	if w.msg == nil || w.msg.Rcode != dns.RcodeSuccess {
		t.Fatal("expected internal answer success despite forward degradation")
	}
	if len(w.msg.Answer) == 0 {
		t.Fatal("expected internal answer records")
	}
}
