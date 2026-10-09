// Package version exposes the single application version embedded in every build.
package version

import (
	_ "embed"
	"regexp"
	"strings"
)

//go:embed VERSION
var source string

var Version = readVersion(source)

// BuildVersion is a distinctive marker in packaged binaries so cross-compiled
// artifacts can be verified without executing them. VERSION is authoritative.
var BuildVersion string

func init() {
	if BuildVersion != "" && BuildVersion != "nginx-web-version/"+Version+"/" {
		panic("build version does not match embedded VERSION")
	}
}

func readVersion(text string) string {
	value := strings.TrimSpace(text)
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(value) {
		panic("VERSION must contain a single major.minor.patch version")
	}
	return value
}
