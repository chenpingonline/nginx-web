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
	p.progress("创建 DNS TXT 记录")
	if err := p.Provider.Present(ctx, domain, token, keyAuth); err != nil {
		return classifyError(err, "初始化 DNS 服务商")
	}
	p.progress("DNS TXT 记录已创建")
	return nil
}
func (p *progressDNS) CleanUp(ctx context.Context, domain, token, keyAuth string) error {
	p.progress("清理 DNS TXT 记录")
	if err := p.Provider.CleanUp(ctx, domain, token, keyAuth); err != nil {
		p.progress("DNS TXT 清理失败，请检查服务商权限和残留记录")
		return classifyError(err, "初始化 DNS 服务商")
	}
	p.progress("DNS TXT 记录已清理")
	return nil
}

func propagationProgress(progress func(string)) dns01.ChallengeOption {
	var mu sync.Mutex
	waiting, ready := map[string]bool{}, map[string]bool{}
	return dns01.WrapPreCheck(func(ctx context.Context, domain, fqdn, value string, check dns01.PreCheckFunc) (bool, error) {
		mu.Lock()
		if !waiting[domain] {
			progress("等待 DNS TXT 记录生效")
			waiting[domain] = true
		}
		mu.Unlock()
		ok, err := check(ctx, fqdn, value)
		mu.Lock()
		if ok && err == nil && !ready[domain] {
			progress("DNS 记录已生效，提交 CA 验证")
			ready[domain] = true
		}
		mu.Unlock()
		return ok, err
	})
}
