package acme

import (
	"context"
	"net"
	"regexp"
	"strings"
	"time"
)

// Only bounded, application-owned fields are published. No TXT values, tokens,
// signed URLs or third-party error text enter the diagnostic pipe or API.
type DNSLogDetail struct {
	Domain    string `json:"domain,omitempty"`
	FQDN      string `json:"fqdn,omitempty"`
	Resolver  string `json:"resolver,omitempty"`
	Attempt   int    `json:"attempt,omitempty"`
	ElapsedMS int64  `json:"elapsed_ms"`
	Result    string `json:"result"`
}

func challengeFQDN(domain string) string {
	return "_acme-challenge." + strings.TrimSuffix(strings.TrimPrefix(domain, "*."), ".") + "."
}

type logSinkKey struct{}

func withLogSink(ctx context.Context, sink func(LogEntry)) context.Context {
	return context.WithValue(ctx, logSinkKey{}, sink)
}
func emitLog(ctx context.Context, entry LogEntry) {
	if sink, ok := ctx.Value(logSinkKey{}).(func(LogEntry)); ok {
		sink(entry)
	}
}

var safeDNSName = regexp.MustCompile(`^[a-zA-Z0-9_.*-]+(?:\.[a-zA-Z0-9_*-]+)*\.?$`)
var resolverPattern = regexp.MustCompile(`(?:\bNS |\[ns=)(\[[0-9a-fA-F:]+\]:[0-9]+|[^\s,\]]+)`)

func safeName(name string) string {
	if len(name) <= 254 && safeDNSName.MatchString(name) {
		return name
	}
	return ""
}
func failingResolver(err error) string {
	if err == nil {
		return ""
	}
	match := resolverPattern.FindStringSubmatch(err.Error())
	if len(match) < 2 {
		return ""
	}
	if addr, e := normalizeResolver(match[1]); e == nil {
		return addr
	}
	host, port, e := net.SplitHostPort(match[1])
	if e == nil && port == "53" && safeName(host) != "" {
		return net.JoinHostPort(host, port)
	}
	return ""
}
func dnsErrorCode(err error) string {
	if err == nil {
		return "dns_ready"
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "refused"):
		return "dns_query_refused"
	case strings.Contains(s, "servfail"):
		return "dns_servfail"
	case strings.Contains(s, "nxdomain"):
		return "dns_no_txt"
	case strings.Contains(s, "did not return the expected txt record"):
		// Inspect the suffix to classify an empty answer; never publish its content.
		if pos := strings.Index(s, "]: "); pos >= 0 && strings.TrimSpace(s[pos+3:]) == "" {
			return "dns_no_txt"
		}
		return "dns_txt_mismatch"
	case strings.Contains(s, "i/o timeout"), strings.Contains(s, "context deadline exceeded"):
		return "dns_query_timeout"
	default:
		return "dns_pending"
	}
}
func dnsResultLabel(code string) string {
	switch code {
	case "dns_ready":
		return "验证记录已匹配"
	case "dns_created":
		return "TXT 创建完成"
	case "dns_cleaned":
		return "TXT 清理完成"
	case "dns_wait":
		return "跳过本地预检查，按固定时间等待；CA 仍会独立验证"
	case "dns_resolver":
		return "使用 ACME DNS 服务器"
	case "dns_pending":
		return "等待 DNS 记录生效"
	default:
		return diagnostic(code).Message
	}
}
func dnsLog(ctx context.Context, domain, fqdn, resolver string, attempt int, started time.Time, code string) {
	emitLog(ctx, LogEntry{Time: time.Now().UTC(), Level: "info", Message: "DNS 查询与验证", Detail: &DNSLogDetail{
		Domain: safeName(domain), FQDN: safeName(fqdn), Resolver: resolver, Attempt: attempt, ElapsedMS: time.Since(started).Milliseconds(), Result: code,
	}})
}

func appendDiagnostic(r *Record, entry LogEntry) {
	if entry.Detail == nil {
		return
	}
	d := *entry.Detail
	switch d.Result {
	case "dns_ready", "dns_created", "dns_cleaned", "dns_wait", "dns_resolver", "dns_pending", "dns_query_timeout", "dns_query_refused", "dns_servfail", "dns_no_txt", "dns_txt_mismatch":
	default:
		return
	}
	d.Domain, d.FQDN = safeName(d.Domain), safeName(d.FQDN)
	if d.Resolver != "" {
		if resolver, err := normalizeResolver(d.Resolver); err == nil {
			d.Resolver = resolver
		} else {
			d.Resolver = ""
		}
	}
	if d.Attempt < 0 || d.Attempt > 10000 || d.ElapsedMS < 0 || d.ElapsedMS > 35*time.Minute.Milliseconds() {
		return
	}
	appendEntry(r, entry.Time, "info", dnsResultLabel(d.Result), nil)
	if len(r.Runs) > 0 {
		run := &r.Runs[len(r.Runs)-1]
		run.Entries[len(run.Entries)-1].Detail = &d
	}
}
