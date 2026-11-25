package nginx

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	crossplane "github.com/nginxinc/nginx-go-crossplane"
)

var conflictWarning = regexp.MustCompile(`(?m)\[warn\] (?:[0-9]+#[0-9]+: )?conflicting server name "([^"\r\n]*)" on ([^,\r\n]+), ignored`)

type nameConflict struct{ name, address string }

// Only diagnose conflicts Nginx actually reported. Expand the installed master
// in include order; sorting the rule list or using the copied rule ID in a
// custom file can identify the wrong server.
func (m *Manager) nameWarnings(output string, rules []ProxyRule) []string {
	var warnings []string
	for _, conflict := range diagnoseNames(output, m.paths.NginxMaster, m.paths.NginxConfD, rules) {
		warnings = append(warnings, conflict.message)
	}
	return warnings
}

type serverSource struct {
	ruleID           string
	file             string
	line, occurrence int
	label            string
}

type conflictDetail struct {
	message         string
	winner, ignored serverSource
}

func diagnoseNames(output, master, confD string, rules []ProxyRule) []conflictDetail {
	matches := conflictWarning.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return nil
	}
	var order []nameConflict
	count := map[nameConflict]int{}
	for _, match := range matches {
		key := nameConflict{match[1], match[2]}
		if count[key] == 0 {
			order = append(order, key)
		}
		count[key]++
	}
	payload, err := crossplane.Parse(master, &crossplane.ParseOptions{
		CombineConfigs: true, StopParsingOnError: true, SkipDirectiveContextCheck: true, SkipDirectiveArgsCheck: true,
	})
	ignored := map[nameConflict][]conflictDetail{}
	if err == nil && len(payload.Config) == 1 && len(payload.Errors) == 0 {
		for _, ctx := range payload.Config[0].Parsed {
			if ctx.Directive != "http" && ctx.Directive != "stream" {
				continue
			}
			owners := map[nameBinding]serverSource{}
			occurrences := map[serverSource]int{}
			for _, server := range ctx.Block {
				if server.Directive == "server" {
					source := identifyServer(server, confD, rules)
					occurrences[source]++
					source.occurrence = occurrences[source]
					if source.occurrence > 1 {
						source.label = fmt.Sprintf("%s（该位置第 %d 个 server）", source.label, source.occurrence)
					}
					traceServerNames(server, owners, count, ignored, source)
				}
			}
		}
	}
	var warnings []conflictDetail
	for _, key := range order {
		prefix := fmt.Sprintf("域名 %q 在 %s 重复。", key.name, key.address)
		if len(ignored[key]) != count[key] {
			warnings = append(warnings, conflictDetail{message: prefix + "Nginx 已忽略重复匹配，无法确认全部来源。"})
			continue
		}
		for _, detail := range ignored[key] {
			detail.message = prefix + detail.message + "。"
			warnings = append(warnings, detail)
		}
	}
	return warnings
}

type serverListener struct {
	address  string
	datagram bool
}

type nameBinding struct {
	name     string
	listener serverListener
}

func traceServerNames(server *crossplane.Directive, owners map[nameBinding]serverSource, reported map[nameConflict]int, ignored map[nameConflict][]conflictDetail, source serverSource) {
	var names []string
	var listeners []serverListener
	for _, d := range server.Block {
		switch d.Directive {
		case "listen":
			if len(d.Args) > 0 {
				listeners = append(listeners, serverListener{listenAddress(d.Args[0]), slices.Contains(d.Args, "quic") || slices.Contains(d.Args, "udp")})
			}
		case "server_name":
			names = append(names, d.Args...)
		}
	}
	if len(listeners) == 0 {
		port := "80"
		if os.Geteuid() != 0 {
			port = "8000"
		}
		listeners = []serverListener{{address: "0.0.0.0:" + port}}
	}
	if len(names) == 0 {
		names = []string{""}
	}
	for _, listener := range listeners {
		for _, name := range names {
			if strings.HasPrefix(name, "~") {
				continue
			}
			if strings.EqualFold(name, "$hostname") {
				name, _ = os.Hostname()
			}
			name = strings.ToLower(name)
			winner := registerName(owners, listener, name, source)
			key := nameConflict{name, listener.address}
			if winner.file != "" && reported[key] > 0 {
				ignored[key] = append(ignored[key], conflictDetail{winner: winner, ignored: source, message: fmt.Sprintf("使用%s。%s的这项域名匹配被忽略", winner.label, source.label)})
			}
		}
	}
}

func registerName(owners map[nameBinding]serverSource, listener serverListener, name string, source serverSource) serverSource {
	keys := []string{name}
	if strings.HasPrefix(name, ".") {
		// Nginx reserves both the bare name and the leading wildcard.
		keys = []string{name[1:], "*" + name}
	}
	for _, key := range keys {
		if winner := owners[nameBinding{key, listener}]; winner.file != "" {
			return winner
		}
	}
	for _, key := range keys {
		owners[nameBinding{key, listener}] = source
	}
	return serverSource{}
}

func identifyServer(server *crossplane.Directive, confD string, rules []ProxyRule) serverSource {
	file := server.File
	if filepath.Dir(file) == confD {
		file = filepath.Base(file)
		if !strings.HasPrefix(file, "custom-") {
			for _, rule := range rules {
				if strings.HasSuffix(file, "-"+rule.ID+".conf") {
					return serverSource{ruleID: rule.ID, file: file, line: server.Line, label: fmt.Sprintf("规则「%s」（%s 第 %d 行）", rule.Name, file, server.Line)}
				}
			}
		}
	}
	return serverSource{file: file, line: server.Line, label: fmt.Sprintf("「%s」第 %d 行", file, server.Line)}
}

func listenAddress(value string) string {
	if strings.HasPrefix(value, "unix:") {
		return value
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		if _, err := strconv.ParseUint(value, 10, 16); err == nil {
			host, port = "*", value
		} else {
			host, port = strings.Trim(value, "[]"), "80"
		}
	}
	if host == "" || host == "*" {
		host = "0.0.0.0"
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return net.JoinHostPort(addr.String(), port)
	}
	return value
}
