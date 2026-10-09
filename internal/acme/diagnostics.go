package acme

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"os"
	"strings"
	"time"

	legoacme "github.com/go-acme/lego/v5/acme"
)

// Only application-owned messages leave the issuer. Provider errors may contain
// credentials, signed URLs, DNS challenges and response bodies.
type Diagnostic struct {
	Code              string `json:"code"`
	Message           string `json:"message"`
	Hint              string `json:"hint"`
	Stage             string `json:"stage,omitempty"`
	RetryAfterSeconds int64  `json:"retry_after_seconds,omitempty"`
}

func (d *Diagnostic) Error() string { return d.Message + "；" + d.Hint }

func diagnostic(code string) *Diagnostic {
	d := &Diagnostic{Code: code}
	switch code {
	case "dns_credentials":
		d.Message, d.Hint = "DNS 服务商认证失败", "检查 API ID、密钥或 Token 是否正确、过期，以及所选服务商是否匹配"
	case "dns_permission":
		d.Message, d.Hint = "DNS API 权限不足", "为凭据授予目标 DNS Zone 的读取和 TXT 记录管理权限"
	case "dns_zone":
		d.Message, d.Hint = "找不到域名对应的 DNS Zone", "确认域名托管在所选服务商、Zone 可被凭据访问，且权威 DNS 配置正确"
	case "dns_propagation":
		d.Message, d.Hint = "DNS 验证记录未及时生效", "检查权威 DNS、CNAME 和 TXT 记录解析；必要时增加 DNS 等待时间"
	case "dns_query_timeout":
		d.Message, d.Hint = "DNS 查询超时", "检查 NAS 到日志中 DNS 服务器的 UDP/TCP 53 连通性；可配置独立 ACME DNS 服务器"
	case "dns_query_refused":
		d.Message, d.Hint = "DNS 服务器拒绝查询", "检查 DNS 访问策略；可改用允许递归查询的独立 ACME DNS 服务器"
	case "dns_servfail":
		d.Message, d.Hint = "DNS 服务器解析失败（SERVFAIL）", "检查上游 DNS、域名委派及 DNSSEC，或更换 ACME DNS 服务器后重试"
	case "dns_no_txt":
		d.Message, d.Hint = "未查询到 DNS TXT 验证记录", "检查公网 TXT 是否已创建，以及局域网通配符规则是否拦截 TXT 查询"
	case "dns_txt_mismatch":
		d.Message, d.Hint = "DNS TXT 记录与本次验证不匹配", "检查 DNS 缓存、CNAME 委派和传播状态；不要手工复制其他申请的验证值"
	case "ca_rate_limit":
		d.Message, d.Hint = "证书机构限制了申请频率", "等待证书机构限制恢复后重试，避免频繁重新签发"
	case "ca_account":
		d.Message, d.Hint = "ACME 账户注册或认证失败", "检查所选 CA、Directory 地址、EAB KID/HMAC 及服务条款"
	case "ca_validation":
		d.Message, d.Hint = "证书机构拒绝域名验证或签发", "检查域名、CAA、DNS 验证记录及证书机构政策"
	case "network":
		d.Message, d.Hint = "连接服务商或证书机构失败", "检查 NAS 的网络、DNS、代理和 TLS 信任配置"
	case "timeout":
		d.Message, d.Hint = "申请操作超时", "检查网络和 DNS 传播状态，调整等待时间后重试"
	case "interrupted":
		d.Message, d.Hint = "任务执行中断", "服务恢复后会继续检查；可查看下一次执行记录"
	case "storage":
		d.Message, d.Hint = "无法保存申请任务", "检查磁盘空间及应用私有目录的写入权限"
	case "deployment":
		d.Message, d.Hint = "证书部署失败", "旧证书继续使用；检查 Nginx 配置、证书目录权限和磁盘空间后重试部署"
	case "worker":
		d.Message, d.Hint = "DNS 申请子进程中断", "检查应用进程、资源和安装文件，服务恢复后重试"
	case "dns_config":
		d.Message, d.Hint = "DNS 服务商配置无效", "检查所选服务商的必填凭据、参数及凭据文件权限"
	default:
		d.Code, d.Message, d.Hint = "issuance", "证书申请失败", "按失败阶段检查 DNS 配置、域名、CA/EAB 和网络后重试"
	}
	return d
}

func classifyError(err error, stage string) (result *Diagnostic) {
	defer func() {
		if result != nil && result.Stage == "" {
			result.Stage = safeStage(stage)
		}
	}()
	var known *Diagnostic
	if errors.As(err, &known) {
		d := diagnostic(known.Code)
		if known.Stage != "" {
			d.Stage = safeStage(known.Stage)
		}
		if known.RetryAfterSeconds > 0 && known.RetryAfterSeconds <= 365*24*60*60 {
			d.RetryAfterSeconds = known.RetryAfterSeconds
		}
		return d
	}
	if errors.Is(err, context.Canceled) {
		return diagnostic("interrupted")
	}
	if stage == "部署证书" {
		return diagnostic("deployment")
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return diagnostic("storage")
	}
	var problem *legoacme.ProblemDetails
	var rateLimit *legoacme.RateLimitedError
	if errors.As(err, &rateLimit) {
		d := diagnostic("ca_rate_limit")
		if rateLimit.RetryAfter > 0 && rateLimit.RetryAfter <= 365*24*time.Hour {
			d.RetryAfterSeconds = int64((rateLimit.RetryAfter + time.Second - 1) / time.Second)
		}
		return d
	}
	if errors.As(err, &problem) {
		switch {
		case problem.HTTPStatus == 429 || strings.HasSuffix(problem.Type, ":rateLimited"):
			return diagnostic("ca_rate_limit")
		case strings.HasSuffix(problem.Type, ":externalAccountRequired") || strings.HasSuffix(problem.Type, ":accountDoesNotExist") || stage == "注册 ACME 账户":
			return diagnostic("ca_account")
		default:
			return diagnostic("ca_validation")
		}
	}
	// Match known error signatures, never publish the text being inspected.
	s := strings.ToLower(err.Error())
	for _, token := range []string{"invalidaccesskey", "invalidtoken", "invalid token", "invalid api", "invalidsecret", "signaturedoesnotmatch", "signature mismatch", "authentication", "authenticate", "invalid credentials", "unauthorized", "401", "6003", "9109"} {
		if strings.Contains(s, token) {
			return diagnostic("dns_credentials")
		}
	}
	for _, token := range []string{"permission", "forbidden", "accessdenied", "access denied", "403"} {
		if strings.Contains(s, token) {
			return diagnostic("dns_permission")
		}
	}
	if strings.Contains(s, "rate limited") || strings.Contains(s, "ratelimited") || strings.Contains(s, "429") {
		return diagnostic("ca_rate_limit")
	}
	for _, token := range []string{"could not find zone", "zone not found", "no matching zone", "failed to find zone", "could not find the start of authority"} {
		if strings.Contains(s, token) {
			return diagnostic("dns_zone")
		}
	}
	if strings.Contains(s, "propagation") || strings.Contains(s, "time limit exceeded") {
		if code := dnsErrorCode(err); code != "dns_pending" {
			return diagnostic(code)
		}
		return diagnostic("dns_propagation")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return diagnostic("timeout")
	}
	var netErr net.Error
	var trustErr x509.UnknownAuthorityError
	if errors.As(err, &netErr) || errors.As(err, &trustErr) {
		return diagnostic("network")
	}
	if stage == "注册 ACME 账户" {
		return diagnostic("ca_account")
	}
	if stage == "初始化 DNS 服务商" {
		return diagnostic("dns_config")
	}
	return diagnostic("issuance")
}

type LogEntry struct {
	Time    time.Time     `json:"time"`
	Level   string        `json:"level"`
	Message string        `json:"message"`
	Code    string        `json:"code,omitempty"`
	Hint    string        `json:"hint,omitempty"`
	Detail  *DNSLogDetail `json:"detail,omitempty"`
}
type Run struct {
	ID             string     `json:"id"`
	Kind           string     `json:"kind"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	Status         string     `json:"status"`
	Entries        []LogEntry `json:"entries"`
	OmittedEntries int        `json:"omitted_entries,omitempty"`
}

const maxRuns = 20
const maxEntries = 200

func safeStage(message string) string {
	switch message {
	case "准备申请", "准备部署", "连接证书机构", "注册 ACME 账户", "检查续期时间", "初始化 DNS 服务商", "DNS 验证与证书签发", "创建 DNS TXT 记录", "DNS TXT 记录已创建", "等待 DNS TXT 记录生效", "DNS 记录已生效，提交 CA 验证", "清理 DNS TXT 记录", "DNS TXT 记录已清理", "DNS TXT 清理失败，请检查服务商权限和残留记录", "证书签发成功", "部署证书":
		return message
	default:
		return "申请处理中"
	}
}

func cleanupStage(stage string) bool {
	return stage == "清理 DNS TXT 记录" || stage == "DNS TXT 记录已清理" || stage == "DNS TXT 清理失败，请检查服务商权限和残留记录"
}

func appendEntry(r *Record, at time.Time, level, message string, d *Diagnostic) {
	if len(r.Runs) == 0 {
		return
	}
	run := &r.Runs[len(r.Runs)-1]
	entry := LogEntry{Time: at.UTC(), Level: level, Message: message}
	if d != nil {
		entry.Code, entry.Hint = d.Code, d.Hint
	}
	if len(run.Entries) >= maxEntries {
		run.OmittedEntries++
		// Keep the beginning and the most recent events, including the outcome.
		run.Entries = append(run.Entries[:1], run.Entries[2:]...)
	}
	run.Entries = append(run.Entries, entry)
}
