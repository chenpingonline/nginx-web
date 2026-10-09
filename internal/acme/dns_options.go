package acme

import (
	"errors"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/go-acme/lego/v5/challenge/dns01"
	"github.com/miekg/dns"
)

func normalizeResolver(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if ip, err := netip.ParseAddr(raw); err == nil && ip.Zone() == "" {
		return net.JoinHostPort(ip.String(), "53"), nil
	}
	host, port, err := net.SplitHostPort(raw)
	ip, ipErr := netip.ParseAddr(host)
	p, portErr := strconv.Atoi(port)
	if err != nil || ipErr != nil || ip.Zone() != "" || portErr != nil || p < 1 || p > 65535 {
		return "", errors.New("DNS 服务器请填写 IP 或 IP:端口；IPv6 带端口时使用 [IPv6]:端口")
	}
	return net.JoinHostPort(ip.String(), strconv.Itoa(p)), nil
}

func validateDNSOptions(in *Input) error {
	if len(in.DNSResolvers) > 8 {
		return errors.New("最多配置 8 个 ACME DNS 服务器")
	}
	resolvers := []string{}
	seen := map[string]bool{}
	for _, raw := range in.DNSResolvers {
		resolver, err := normalizeResolver(raw)
		if err != nil {
			return err
		}
		if !seen[resolver] {
			resolvers = append(resolvers, resolver)
			seen[resolver] = true
		}
	}
	in.DNSResolvers = resolvers
	if in.DNSCheckMode == "" {
		in.DNSCheckMode = "auto"
	}
	if in.DNSCheckMode != "auto" && in.DNSCheckMode != "wait" {
		return errors.New("不支持的 DNS 预检查模式")
	}
	if in.DNSWaitSeconds == 0 {
		in.DNSWaitSeconds = 60
	}
	if in.DNSWaitSeconds < 30 || in.DNSWaitSeconds > 600 {
		return errors.New("固定 DNS 等待时间应为 30 至 600 秒")
	}
	return nil
}

func resolverAddresses(in Input) []string {
	if len(in.DNSResolvers) > 0 {
		return append([]string(nil), in.DNSResolvers...)
	}
	if config, err := dns.ClientConfigFromFile("/etc/resolv.conf"); err == nil && len(config.Servers) > 0 {
		out := []string{}
		for _, server := range config.Servers {
			out = append(out, net.JoinHostPort(server, config.Port))
		}
		return out
	}
	// Matches lego's fallback when the system resolver configuration is unavailable.
	return []string{"1.1.1.1:53", "1.0.0.1:53", "[2606:4700:4700::1111]:53", "[2606:4700:4700::1001]:53"}
}

// Called only inside the isolated worker. Never changes another job's DNS client.
func configureDNSClient(in Input) {
	opts := dns01.NewOptions()
	opts.RecursiveNameservers = resolverAddresses(in)
	opts.Timeout = 5 * time.Second
	dns01.SetDefaultClient(dns01.NewClient(opts))
}
