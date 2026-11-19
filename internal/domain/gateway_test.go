package domain

import "testing"

func TestGatewayRouteValidation(t *testing.T) {
	base := GatewayRoute{ID: RandomID(), Name: "Music", Path: "music", Target: "http://127.0.0.1:4533/base", Enabled: true}
	if err := ValidateGatewayRoutes([]GatewayRoute{base}); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"file:///etc/passwd", "http://user:password@localhost", "http://localhost:0", "http://localhost:65536", "http://localhost/a/../b", "http://localhost/%2e%2e", "http://localhost/?a=b", "http://localhost/#x"} {
		r := base
		r.Target = target
		if ValidateGatewayRoutes([]GatewayRoute{r}) == nil {
			t.Errorf("accepted target %q", target)
		}
	}
	for _, path := range []string{"../api", "foo/bar", "-music", "Music", "music?x=1", ""} {
		r := base
		r.Path = path
		if ValidateGatewayRoutes([]GatewayRoute{r}) == nil {
			t.Errorf("accepted path %q", path)
		}
	}
	other := base
	other.ID = RandomID()
	if ValidateGatewayRoutes([]GatewayRoute{base, other}) == nil {
		t.Fatal("duplicate path accepted")
	}
}

func TestGatewayTrustBoundToTarget(t *testing.T) {
	route := GatewayRoute{ID: RandomID(), Name: "router", Path: "router", Target: "https://192.168.1.1:9999", Enabled: true, TLSFingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", TLSTrustTarget: "https://192.168.1.1:9999"}
	if err := ValidateGatewayRoutes([]GatewayRoute{route}); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"https://192.168.1.2:9999", "https://192.168.1.1:9998", "https://192.168.1.1:9999/new-base", "http://192.168.1.1:9999"} {
		changed := route
		changed.Target = target
		if ValidateGatewayRoutes([]GatewayRoute{changed}) == nil {
			t.Fatalf("retained trust for changed target: %s", target)
		}
	}
	route.TLSFingerprint = "not-a-fingerprint"
	if ValidateGatewayRoutes([]GatewayRoute{route}) == nil {
		t.Fatal("invalid fingerprint accepted")
	}
}

func TestGatewayGenericCompatibilityAndLegacyAlias(t *testing.T) {
	for _, mode := range []string{"", "subpath", "ikuai-v4"} {
		route := GatewayRoute{ID: RandomID(), Name: "device", Path: "device", Target: "http://192.168.1.2:8080", Compatibility: mode}
		if err := ValidateGatewayRoutes([]GatewayRoute{route}); err != nil {
			t.Fatal(err)
		}
		if route.UsesSubpathCompatibility() != (mode != "") {
			t.Errorf("unexpected mode %s", mode)
		}
	}
}
