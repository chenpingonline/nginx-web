package acme

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	legoacme "github.com/go-acme/lego/v5/acme"
	"github.com/go-acme/lego/v5/certificate"
	"github.com/go-acme/lego/v5/challenge/dns01"
	"github.com/miekg/dns"
)

func TestFixedWaitSkipsOnlyLocalCheckAndHonorsCancellation(t *testing.T) {
	check := fixedWaitCheck(time.Millisecond)
	called := false
	precheck := func(context.Context, string, string) (bool, error) {
		called = true
		return false, errors.New("local DNS unavailable")
	}
	ok, err := check(context.Background(), "example.com", challengeFQDN("example.com"), "private-token", precheck)
	if !ok || err != nil || called {
		t.Fatal("fixed wait did not bypass only the local precheck")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ok, err = fixedWaitCheck(time.Hour)(ctx, "example.com", challengeFQDN("example.com"), "private-token", precheck)
	if ok || !errors.Is(err, context.Canceled) || called {
		t.Fatal("fixed wait ignored cancellation")
	}
}

func TestExplicitDeploymentRetryReusesIssuedCertificate(t *testing.T) {
	now := time.Now().UTC()
	deployments := 0
	m, err := New(t.TempDir(), func(id, name string, resource *certificate.Resource) (string, error) {
		deployments++
		return id, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	m.now = func() time.Time { return now }
	job, err := m.Create(validInput())
	if err != nil {
		t.Fatal(err)
	}
	r := m.records[job.ID]
	r.Pending, r.Resource = true, testResource(t, now)
	if err = m.save(r); err != nil {
		t.Fatal(err)
	}
	m.issue = func(context.Context, *Record, func() error, func(string)) error {
		t.Fatal("deployment retry created an ACME order")
		return nil
	}
	if err = m.Action(job.ID, "retry_deploy"); err != nil {
		t.Fatal(err)
	}
	m.step(context.Background())
	if deployments != 1 || m.List()[0].DeploymentPending || m.List()[0].Status != "ready" {
		t.Fatal("deployment retry failed")
	}
	if err = m.Action(job.ID, "retry_deploy"); err == nil {
		t.Fatal("accepted deployment retry without a pending certificate")
	}
	r = m.records[job.ID]
	r.Pending, r.Resource = true, testResource(t, now.Add(-365*24*time.Hour))
	if err = m.save(r); err != nil {
		t.Fatal(err)
	}
	if err = m.Action(job.ID, "retry_deploy"); err == nil {
		t.Fatal("expired deployment retry could create an order")
	}
}

func TestDNSOptionsValidationAndPersistence(t *testing.T) {
	in := validInput()
	in.DNSResolvers = []string{"223.5.5.5", "223.5.5.5:53", "2001:db8::1", "[2001:db8::2]:5353"}
	if err := validate(&in); err != nil {
		t.Fatal(err)
	}
	if len(in.DNSResolvers) != 3 || in.DNSResolvers[0] != "223.5.5.5:53" || in.DNSCheckMode != "auto" || in.DNSWaitSeconds != 60 {
		t.Fatal("invalid normalization", in.DNSResolvers)
	}
	for _, bad := range []string{"dns.example.com", "https://user:secret@example.com", "1.1.1.1:0", "1.1.1.1:65536", "fe80::1%en0", "1.1.1.1\nAuthorization=secret"} {
		invalid := validInput()
		invalid.DNSResolvers = []string{bad}
		if validate(&invalid) == nil {
			t.Fatal("accepted resolver", bad)
		}
	}
	m, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	j, err := m.Create(in)
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := m.Settings(j.ID)
	settings.DNSCheckMode, settings.DNSWaitSeconds = "wait", 90
	if _, err := m.Update(j.ID, UpdateInput{Input: settings.Input, Revision: settings.Revision}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := New(m.dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	settings, _ = reloaded.Settings(j.ID)
	if len(settings.DNSResolvers) != 3 || settings.DNSCheckMode != "wait" || settings.DNSWaitSeconds != 90 {
		t.Fatal("lost DNS options")
	}
	for _, mutate := range []func(*Input){func(in *Input) { in.DNSCheckMode = "disabled" }, func(in *Input) { in.DNSWaitSeconds = 1 }, func(in *Input) { in.DNSWaitSeconds = 601 }} {
		invalid := validInput()
		mutate(&invalid)
		if validate(&invalid) == nil {
			t.Fatal("accepted invalid check options")
		}
	}
}

func TestDNSPropagationLogsAreSpecificAndNeverContainTXTValues(t *testing.T) {
	cases := []struct{ raw, code, resolver string }{
		{"recursive nameservers: NS 192.168.1.88:53 returned REFUSED for _acme-challenge.example.com.", "dns_query_refused", "192.168.1.88:53"},
		{"recursive nameservers: NS 223.5.5.5:53 returned SERVFAIL for _acme-challenge.example.com.", "dns_servfail", "223.5.5.5:53"},
		{"NS 192.168.1.88:53 did not return the expected TXT record [fqdn: _acme-challenge.example.com., value: secret-token]: ", "dns_no_txt", "192.168.1.88:53"},
		{"NS [2001:db8::1]:53 did not return the expected TXT record [fqdn: _acme-challenge.example.com., value: secret-token]: other-secret", "dns_txt_mismatch", "[2001:db8::1]:53"},
		{"DNS call error: read udp: i/o timeout [ns=223.5.5.5:53, question='_acme-challenge.example.com. IN TXT'] secret-token", "dns_query_timeout", "223.5.5.5:53"},
	}
	for _, tc := range cases {
		var entries []LogEntry
		ctx := withLogSink(context.Background(), func(entry LogEntry) { entries = append(entries, entry) })
		check := propagationCheck(func(string) {})
		_, err := check(ctx, "example.com", "_acme-challenge.example.com.", "secret-token", func(context.Context, string, string) (bool, error) { return false, errors.New(tc.raw) })
		if err == nil || len(entries) != 1 || entries[0].Detail.Result != tc.code || entries[0].Detail.Resolver != tc.resolver {
			t.Fatalf("wrong diagnostic: %#v", entries)
		}
		r := &Record{Runs: []Run{{}}}
		appendDiagnostic(r, entries[0])
		for range 5 {
			_, _ = check(ctx, "example.com", "_acme-challenge.example.com.", "secret-token", func(context.Context, string, string) (bool, error) { return false, errors.New(tc.raw) })
		}
		if len(entries) != 1 {
			t.Fatal("repeated errors flooded log")
		}
		_, _ = check(ctx, "example.com", "_acme-challenge.example.com.", "secret-token", func(context.Context, string, string) (bool, error) { return true, nil })
		if len(entries) != 2 || entries[1].Detail.Attempt != 7 || entries[1].Detail.Result != "dns_ready" {
			t.Fatal("missing recovery event")
		}
		raw, _ := json.Marshal(r)
		if strings.Contains(string(raw), "secret") {
			t.Fatal("TXT contents leaked")
		}
		if d := classifyError(errors.New("propagation: time limit exceeded: "+tc.raw), "等待 DNS TXT 记录生效"); d.Code != tc.code {
			t.Fatal("final error lost DNS cause", d)
		}
	}
}

func TestCleanupDoesNotReplaceFailureStageAndRateLimitCannotBeBypassed(t *testing.T) {
	m, _ := New(t.TempDir(), nil)
	now := time.Now()
	m.now = func() time.Time { return now }
	j, _ := m.Create(validInput())
	m.issue = func(_ context.Context, _ *Record, _ func() error, progress func(string)) error {
		progress("等待 DNS TXT 记录生效")
		progress("清理 DNS TXT 记录")
		progress("DNS TXT 记录已清理")
		return errors.New("propagation: time limit exceeded: NS 127.0.0.1:53 returned REFUSED")
	}
	m.step(context.Background())
	job := m.List()[0]
	if job.FailureStage != "等待 DNS TXT 记录生效" || job.FailureCode != "dns_query_refused" || job.CleanupMessage != "DNS TXT 记录已清理" || strings.HasPrefix(job.Message, "DNS TXT 记录已清理") {
		t.Fatal("misleading failure", job)
	}
	now = now.Add(time.Hour)
	m.issue = func(_ context.Context, _ *Record, _ func() error, progress func(string)) error {
		progress("DNS 验证与证书签发")
		return &legoacme.RateLimitedError{ProblemDetails: &legoacme.ProblemDetails{HTTPStatus: 429, Detail: "private-provider-message"}, RetryAfter: 3 * time.Hour}
	}
	m.step(context.Background())
	job = m.List()[0]
	if job.NextAttempt.Before(now.Add(3*time.Hour)) || job.RetryAfter == nil || job.RetryAfter.Before(now.Add(3*time.Hour)) {
		t.Fatal("ignored CA retry-after", job)
	}
	now = now.Add(2 * time.Minute)
	if m.Action(j.ID, "retry") == nil {
		t.Fatal("manual retry bypassed rate limit")
	}
	if m.Action(j.ID, "pause") != nil || m.Action(j.ID, "resume") != nil {
		t.Fatal("pause/resume failed")
	}
	if m.List()[0].NextAttempt.Before(job.NextAttempt) {
		t.Fatal("resume bypassed rate limit")
	}
	if _, err := m.Reissue(j.ID, []string{"new.example.com"}); err == nil {
		t.Fatal("reissue bypassed rate limit")
	}
	raw, _ := json.Marshal(m.List())
	if strings.Contains(string(raw), "private-provider-message") {
		t.Fatal("CA response leaked")
	}
}

func TestReadOnlyDiagnosticsDistinguishLocalNODATAAndPublicTXT(t *testing.T) {
	var writes atomic.Int32
	ca := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes.Add(1)
		}
		_, _ = w.Write([]byte(`{"newNonce":"/nonce","newAccount":"/account","newOrder":"/order"}`))
	}))
	defer ca.Close()
	resolver := func(txt bool) string {
		conn, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		server := &dns.Server{PacketConn: conn, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, q *dns.Msg) {
			answer := new(dns.Msg)
			answer.SetReply(q)
			if q.Opcode != dns.OpcodeQuery {
				writes.Add(1)
			}
			if txt {
				answer.Answer = []dns.RR{&dns.TXT{Hdr: dns.RR_Header{Name: q.Question[0].Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 60}, Txt: []string{"private-challenge-value"}}}
			}
			_ = w.WriteMsg(answer)
		})}
		go server.ActivateAndServe()
		t.Cleanup(func() { _ = server.Shutdown() })
		return conn.LocalAddr().String()
	}
	in := validInput()
	in.DirectoryURL = ca.URL
	in.Domains = []string{"example.com"}
	in.DNSResolvers = []string{resolver(false), resolver(true)}
	checks := diagnoseInput(context.Background(), in)
	if len(checks) != 4 || checks[1].Status != "ok" || checks[2].Status != "info" || checks[3].Status != "ok" || writes.Load() != 0 {
		t.Fatal("wrong checks", checks)
	}
	raw, _ := json.Marshal(checks)
	if strings.Contains(string(raw), "private-challenge-value") || strings.Contains(string(raw), "secret-token-test") {
		t.Fatal("diagnostics leaked private data")
	}
}

func TestIndependentResolverIsUsedForZoneDiscovery(t *testing.T) {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := &dns.Server{PacketConn: conn, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, q *dns.Msg) {
		calls.Add(1)
		answer := new(dns.Msg)
		answer.SetReply(q)
		rr, _ := dns.NewRR("example.com. 60 IN SOA ns.example.com. hostmaster.example.com. 1 60 60 60 60")
		answer.Answer = []dns.RR{rr}
		_ = w.WriteMsg(answer)
	})}
	go server.ActivateAndServe()
	defer server.Shutdown()
	previous := dns01.DefaultClient()
	defer dns01.SetDefaultClient(previous)
	configureDNSClient(Input{DNSResolvers: []string{conn.LocalAddr().String()}})
	zone, err := dns01.DefaultClient().FindZoneByFqdn(context.Background(), "_acme-challenge.example.com.")
	if err != nil || zone != "example.com." || calls.Load() == 0 {
		t.Fatal("custom DNS ignored", zone, err)
	}
}
