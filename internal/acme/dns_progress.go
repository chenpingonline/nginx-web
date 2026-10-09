package acme

import (
	"context"
	"sync"
	"time"

	"github.com/go-acme/lego/v5/challenge"
	"github.com/go-acme/lego/v5/challenge/dns01"
)

// Keep the provider's propagation timeout while reporting application-owned
// events. Challenge tokens and provider response bodies are never recorded.
type progressDNS struct {
	challenge.Provider
	progress func(string)
}

func (p *progressDNS) Timeout() (time.Duration, time.Duration) {
	if native, ok := p.Provider.(challenge.ProviderTimeout); ok {
		return native.Timeout()
	}
	return dns01.DefaultPropagationTimeout, dns01.DefaultPollingInterval
}
func (p *progressDNS) Present(ctx context.Context, domain, token, keyAuth string) error {
	started := time.Now()
	p.progress("创建 DNS TXT 记录")
	if err := p.Provider.Present(ctx, domain, token, keyAuth); err != nil {
		return classifyError(err, "创建 DNS TXT 记录")
	}
	p.progress("DNS TXT 记录已创建")
	dnsLog(ctx, domain, challengeFQDN(domain), "", 0, started, "dns_created")
	return nil
}
func (p *progressDNS) CleanUp(ctx context.Context, domain, token, keyAuth string) error {
	started := time.Now()
	p.progress("清理 DNS TXT 记录")
	if err := p.Provider.CleanUp(ctx, domain, token, keyAuth); err != nil {
		p.progress("DNS TXT 清理失败，请检查服务商权限和残留记录")
		return classifyError(err, "初始化 DNS 服务商")
	}
	p.progress("DNS TXT 记录已清理")
	dnsLog(ctx, domain, "", "", 0, started, "dns_cleaned")
	return nil
}

func propagationProgress(progress func(string)) dns01.ChallengeOption {
	return dns01.WrapPreCheck(propagationCheck(progress))
}

func fixedWaitCheck(wait time.Duration) dns01.WrapPreCheckFunc {
	return func(ctx context.Context, domain, fqdn, value string, _ dns01.PreCheckFunc) (bool, error) {
		started := time.Now()
		dnsLog(ctx, domain, fqdn, "", 0, started, "dns_wait")
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-timer.C:
			return true, nil
		}
	}
}

func propagationCheck(progress func(string)) dns01.WrapPreCheckFunc {
	var mu sync.Mutex
	waiting, ready := map[string]bool{}, map[string]bool{}
	attempts, lastCode := map[string]int{}, map[string]string{}
	started, lastLog := map[string]time.Time{}, map[string]time.Time{}
	return func(ctx context.Context, domain, fqdn, value string, check dns01.PreCheckFunc) (bool, error) {
		mu.Lock()
		if !waiting[domain] {
			progress("等待 DNS TXT 记录生效")
			waiting[domain] = true
			started[domain] = time.Now()
		}
		attempts[domain]++
		attempt := attempts[domain]
		mu.Unlock()
		ok, err := check(ctx, fqdn, value)
		mu.Lock()
		code := dnsErrorCode(err)
		if !ok && err == nil {
			code = "dns_pending"
		}
		if attempt == 1 || code != lastCode[domain] || time.Since(lastLog[domain]) >= 10*time.Second {
			dnsLog(ctx, domain, fqdn, failingResolver(err), attempt, started[domain], code)
			lastLog[domain], lastCode[domain] = time.Now(), code
		}
		if ok && err == nil && !ready[domain] {
			progress("DNS 记录已生效，提交 CA 验证")
			ready[domain] = true
		}
		mu.Unlock()
		return ok, err
	}
}
