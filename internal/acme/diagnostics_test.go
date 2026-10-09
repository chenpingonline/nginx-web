package acme

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	legoacme "github.com/go-acme/lego/v5/acme"
)

func TestDiagnosticClassificationNeverIncludesProviderResponses(t *testing.T) {
	cases := []struct {
		stage, code string
		err         error
	}{
		{"初始化 DNS 服务商", "dns_credentials", errors.New("InvalidAccessKeyId: secret-token")},
		{"DNS 验证与证书签发", "dns_permission", errors.New("Forbidden: Authorization=secret-token")},
		{"DNS 验证与证书签发", "dns_zone", errors.New("could not find zone: secret-token")},
		{"清理 DNS TXT 记录", "dns_propagation", errors.New("time limit exceeded, propagation: token=secret-token")},
		{"注册 ACME 账户", "ca_account", &legoacme.ProblemDetails{Type: "urn:ietf:params:acme:error:externalAccountRequired", Detail: "secret-token"}},
		{"DNS 验证与证书签发", "ca_rate_limit", fmt.Errorf("wrapped: %w", &legoacme.ProblemDetails{Type: "urn:ietf:params:acme:error:rateLimited", HTTPStatus: 429, Detail: "secret-token"})},
		{"DNS 验证与证书签发", "ca_validation", errors.Join(&legoacme.ProblemDetails{Type: "urn:ietf:params:acme:error:unauthorized", Detail: "secret-token"})},
		{"连接证书机构", "timeout", context.DeadlineExceeded},
		{"连接证书机构", "interrupted", context.Canceled},
		{"部署证书", "deployment", errors.New("raw nginx dump including secret-token")},
		{"初始化 DNS 服务商", "dns_config", errors.New("configuration=secret-token")},
	}
	for _, tc := range cases {
		d := classifyError(tc.err, tc.stage)
		if d.Code != tc.code {
			t.Errorf("stage %s: got %s want %s", tc.stage, d.Code, tc.code)
		}
		raw, _ := json.Marshal(d)
		if strings.Contains(string(raw), "secret-token") {
			t.Fatal("provider response leaked")
		}
	}
}

type timedDNS struct{ recordingDNS }

func (p *timedDNS) Timeout() (time.Duration, time.Duration) { return 15 * time.Minute, 3 * time.Second }
func TestDNSProgressPreservesTimeoutAndReportsSafeStages(t *testing.T) {
	provider := &timedDNS{}
	var stages []string
	wrapper := &progressDNS{Provider: provider, progress: func(stage string) { stages = append(stages, stage) }}
	timeout, interval := wrapper.Timeout()
	if timeout != 15*time.Minute || interval != 3*time.Second {
		t.Fatal("lost provider timeout")
	}
	if err := wrapper.Present(context.Background(), "example.com", "secret-token", "secret-auth"); err != nil {
		t.Fatal(err)
	}
	if err := wrapper.CleanUp(context.Background(), "example.com", "secret-token", "secret-auth"); err != nil {
		t.Fatal(err)
	}
	if len(stages) != 4 || provider.present != 1 || provider.cleanup != 1 {
		t.Fatal("missing lifecycle events")
	}
	if strings.Contains(strings.Join(stages, " "), "secret") {
		t.Fatal("challenge leaked")
	}
}
