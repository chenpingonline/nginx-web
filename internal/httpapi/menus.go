package httpapi

import "strings"

func menuForEndpoint(path string) string {
	resource, _, _ := strings.Cut(strings.TrimPrefix(path, "/api/"), "/")
	switch resource {
	case "gateway-apps", "gateway-probe", "gateway-routes":
		return "gateway"
	case "rules", "rule-groups", "auth-profiles":
		return "rules"
	case "streams", "upstreams", "logs", "backup", "revisions", "config", "topology", "waf":
		return resource
	case "rate-limit-policies":
		return "rate-limits"
	case "custom-configs":
		return "config"
	case "certificates", "acme":
		return "certificates"
	}
	return ""
}
