package acme

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/miekg/dns"
)

type CheckResult struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	Domain    string `json:"domain,omitempty"`
	Resolver  string `json:"resolver,omitempty"`
	ElapsedMS int64  `json:"elapsed_ms"`
}

// Diagnose does not create orders, register accounts, write DNS or deploy anything.
// API permissions can only be confirmed by the provider during a real operation.
func (m *Manager) Diagnose(ctx context.Context, id string) ([]CheckResult, error) {
	m.mu.Lock()
	r, ok := m.records[id]
	if !ok {
		m.mu.Unlock()
		return nil, ErrNotFound
	}
	in := clone(r).Input
	m.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return diagnoseInput(ctx, in), nil
}

func diagnoseInput(ctx context.Context, in Input) []CheckResult {
	out := []CheckResult{}
	started := time.Now()
	credentials := CheckResult{Name: "DNS 凭据配置", Status: "ok", Message: "配置格式通过；API 凭据是否有效及 TXT 写入权限需在实际申请中确认"}
	if err := validateDNS(&in); err != nil {
		credentials.Status, credentials.Message = "error", "DNS 服务商配置不完整，请编辑配置检查必填凭据"
	}
	credentials.ElapsedMS = time.Since(started).Milliseconds()
	out = append(out, credentials)
	started = time.Now()
	ca := CheckResult{Name: "证书机构连通性", Status: "error", Message: "无法读取 ACME Directory，请检查网络、DNS 和 TLS 信任"}
	caCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	req, err := http.NewRequestWithContext(caCtx, http.MethodGet, in.DirectoryURL, nil)
	if err == nil {
		client := &http.Client{Timeout: 6 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		response, requestErr := client.Do(req)
		if requestErr == nil {
			var directory struct {
				NewNonce   string `json:"newNonce"`
				NewAccount string `json:"newAccount"`
				NewOrder   string `json:"newOrder"`
			}
			decodeErr := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&directory)
			_ = response.Body.Close()
			if response.StatusCode == 200 && decodeErr == nil && directory.NewOrder != "" && directory.NewAccount != "" && directory.NewNonce != "" {
				ca.Status, ca.Message = "ok", "ACME Directory 可访问；账户与 EAB 验证将在实际申请中进行"
			} else {
				ca.Message = fmt.Sprintf("ACME Directory 响应无效（HTTP %d）", response.StatusCode)
			}
		}
	}
	cancel()
	ca.ElapsedMS = time.Since(started).Milliseconds()
	out = append(out, ca)
	resolvers := resolverAddresses(in)
	for _, domain := range in.Domains {
		if len(out) >= 42 {
			out = append(out, CheckResult{Name: "诊断范围", Status: "info", Message: "本次最多检查 40 组域名与 DNS 服务器；其余域名请结合执行日志检查"})
			break
		}
		fqdn := challengeFQDN(domain)
		for _, resolver := range resolvers {
			if len(out) >= 42 {
				break
			}
			if ctx.Err() != nil {
				out = append(out, CheckResult{Name: "DNS 查询", Status: "error", Message: "诊断已超时或取消；可减少域名或 DNS 服务器后重试"})
				return out
			}
			started = time.Now()
			result := CheckResult{Name: "TXT 查询通路", Domain: fqdn, Resolver: resolver, Status: "error"}
			query := new(dns.Msg)
			query.SetQuestion(fqdn, dns.TypeTXT)
			client := &dns.Client{Timeout: 2 * time.Second}
			response, _, err := client.ExchangeContext(ctx, query, resolver)
			if err == nil && response.Truncated {
				client.Net = "tcp"
				response, _, err = client.ExchangeContext(ctx, query, resolver)
			}
			if err != nil {
				result.Message = "DNS 查询连接失败或超时，请检查 UDP/TCP 53 连通性"
			} else {
				switch response.Rcode {
				case dns.RcodeSuccess:
					count := 0
					for _, answer := range response.Answer {
						if _, ok := answer.(*dns.TXT); ok {
							count++
						}
					}
					if count > 0 {
						result.Status, result.Message = "ok", fmt.Sprintf("查询到 %d 条 TXT；本诊断不校验其是否匹配申请", count)
					} else {
						result.Status, result.Message = "info", "没有返回 TXT；申请结束后记录可能已清理，申请中出现此结果需检查本地覆盖、CNAME 和 DNS 传播"
					}
				case dns.RcodeNameError:
					result.Status, result.Message = "info", "域名不存在（NXDOMAIN）；申请结束后记录可能已清理"
				case dns.RcodeRefused:
					result.Message = "DNS 服务器拒绝查询（REFUSED）"
				case dns.RcodeServerFailure:
					result.Message = "DNS 解析失败（SERVFAIL），请检查上游及 DNSSEC"
				default:
					result.Message = "DNS 返回异常状态"
				}
			}
			result.ElapsedMS = time.Since(started).Milliseconds()
			out = append(out, result)
		}
	}
	return out
}
