package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"

	"github.com/chenpingonline/nginx-web/internal/domain"
)

// Nginx often renders only a stable reference (e.g. upstream fnproxy_up_ID).
// Check its definition too so the same text cannot silently select other nodes.
func formDependencies(st State, value any) (string, error) {
	deps := map[string]any{}
	addPool := func(id string) {
		for _, p := range st.UpstreamPools {
			if p.ID == id {
				deps["upstream/"+id] = struct {
					Protocol, Strategy, Hash           string
					Keepalive, Requests, Time, Timeout int
					Servers                            []domain.UpstreamServer
				}{p.Protocol, p.Strategy, p.HashKey, p.Keepalive, p.KeepaliveRequests, p.KeepaliveTime, p.KeepaliveTimeout, p.Servers}
				return
			}
		}
	}
	addCert := func(id string) {
		for _, c := range st.Certificates {
			if c.ID == id {
				deps["certificate/"+id] = c.Fingerprint
				return
			}
		}
	}
	switch v := value.(type) {
	case ProxyRule:
		addCert(v.CertificateID)
		addPool(v.UpstreamPoolID)
		addPool(v.RootLocation.UpstreamPoolID)
		for _, l := range v.Locations {
			addPool(l.Settings.UpstreamPoolID)
		}
		for _, g := range st.RuleGroups {
			if g.ID == v.GroupID {
				deps["group"] = g
			}
		}
		for _, p := range st.RateLimitPolicies {
			if p.ID == v.RateLimitPolicyID {
				deps["limits"] = p.Settings
			}
		}
		for _, p := range st.AuthProfiles {
			if p.ID == v.Authentication.ProfileID {
				users := map[string]bool{}
				for _, u := range p.Users {
					users[u.Username] = u.Enabled
				}
				deps["auth"] = struct {
					Realm string
					Users map[string]bool
				}{p.Realm, users}
			}
		}
		for _, w := range st.WAFInstances {
			if slices.Contains(w.Sites, v.ID) {
				deps["waf"] = struct {
					ID, Mode, Action string
					Limit            int
					Upload           bool
					Excluded         []int
				}{w.ID, w.Mode, w.BodyLimitAction, w.BodyLimitMiB, slices.Contains(w.UploadSites, v.ID), w.Exclusions[v.ID]}
			}
		}
	case StreamRule:
		addCert(v.CertificateID)
		addPool(v.UpstreamPoolID)
		for _, r := range v.SNIRoutes {
			addPool(r.UpstreamPoolID)
		}
	case domain.RuleGroup:
		addCert(v.CertificateID)
	}
	if len(deps) == 0 {
		return "", nil
	}
	data, err := json.Marshal(deps)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
