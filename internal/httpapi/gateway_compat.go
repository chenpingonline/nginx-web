package httpapi

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/transform"
)

//go:embed gateway_compat_bootstrap.js
var gatewayBootstrap string

var gatewayCSSURL = regexp.MustCompile(`(?i)(url\(\s*["']?)([^\s"')]+)`)
var gatewayCSSImport = regexp.MustCompile(`(?i)(@import\s+["'])([^"']+)`)

// Only URL-bearing operations, never arbitrary strings, API data or route definitions.
var gatewayJSLocation = regexp.MustCompile(`((?:window\.)?location\.(?:href\s*=\s*|(?:assign|replace)\s*\(\s*))(["'])((?:https?:)?/[^"'\r\n]*)(["'])`)
var gatewayJSModule = regexp.MustCompile(`(\b(?:from\s*|import\s*\(\s*|import\s*))(["'])((?:https?:)?/[^"'\r\n]*)(["'])`)

func serveGatewayBootstrap(w http.ResponseWriter, prefix, cookiePrefix, target string) {
	script := gatewayBootstrap
	for placeholder, value := range map[string]string{"__GATEWAY_PREFIX__": prefix, "__GATEWAY_COOKIE_PREFIX__": cookiePrefix, "__GATEWAY_TARGET__": target} {
		encoded, _ := json.Marshal(value)
		script = strings.ReplaceAll(script, placeholder, string(encoded))
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.WriteString(w, script)
}

// Localize only root URLs or absolute URLs belonging to this configured upstream.
// Relative URLs retain their directory semantics; fragments and external origins stay intact.
func gatewayResourceURL(value, prefix, target string, entry ...string) string {
	if value == "" || strings.HasPrefix(value, "#") || strings.HasPrefix(value, "?") {
		return value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return value
	}
	path, escaped := parsed.Path, parsed.EscapedPath()
	upstream, err := url.Parse(target)
	if err != nil {
		return value
	}
	if parsed.Host != "" {
		matchesOrigin := func(origin *url.URL) bool {
			return strings.EqualFold(parsed.Host, origin.Host) && (parsed.Scheme == "" || parsed.Scheme == origin.Scheme)
		}
		if !matchesOrigin(upstream) {
			if len(entry) == 0 {
				return value
			}
			public, err := url.Parse(entry[0])
			if err != nil || !matchesOrigin(public) {
				return value
			}
		}
		if parsed.User != nil {
			return value
		}

		parsed.Host, parsed.Scheme = "", ""
	} else if parsed.Scheme != "" || !strings.HasPrefix(path, "/") {
		return value
	}
	if path == prefix || strings.HasPrefix(path, prefix+"/") {
		return parsed.String()
	}
	base := strings.TrimRight(upstream.Path, "/")
	if base != "" && (path == base || strings.HasPrefix(path, base+"/")) {
		path = strings.TrimPrefix(path, base)
		escaped = strings.TrimPrefix(escaped, base)
	}
	parsed.Path = prefix + "/" + strings.TrimPrefix(path, "/")
	parsed.RawPath = prefix + "/" + strings.TrimPrefix(escaped, "/")
	return parsed.String()
}

func rewriteGatewayCSS(source, prefix, target string, entry ...string) string {
	for _, pattern := range []*regexp.Regexp{gatewayCSSURL, gatewayCSSImport} {
		source = pattern.ReplaceAllStringFunc(source, func(match string) string {
			parts := pattern.FindStringSubmatch(match)
			return parts[1] + gatewayResourceURL(parts[2], prefix, target, entry...)
		})
	}
	return source
}

func rewriteGatewayJS(source, prefix, target string, entry ...string) string {
	for _, pattern := range []*regexp.Regexp{gatewayJSLocation, gatewayJSModule} {
		matches := pattern.FindAllStringSubmatchIndex(source, -1)
		if len(matches) == 0 {
			continue
		}
		code := gatewayJSCodeOffsets(source, matches)
		var out strings.Builder
		last := 0
		for i, m := range matches {
			if !code[i] {
				continue
			}
			parts := pattern.FindStringSubmatch(source[m[0]:m[1]])
			if parts[2] != parts[4] || strings.Contains(parts[3], "\\") {
				continue
			}
			out.WriteString(source[last:m[0]])
			out.WriteString(parts[1] + parts[2] + gatewayResourceURL(parts[3], prefix, target, entry...) + parts[4])
			last = m[1]
		}
		out.WriteString(source[last:])
		source = out.String()
	}

	return source
}

func rewriteGatewayResponse(response *http.Response, prefix, target string, entry ...string) error {
	kind := strings.ToLower(strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0]))
	switch kind {
	case "application/x-javascript", "text/ecmascript", "application/ecmascript":
		kind = "application/javascript"
	}
	if response.Body == nil || response.StatusCode == http.StatusNotModified || response.StatusCode == http.StatusNoContent || response.StatusCode == http.StatusPartialContent || (response.Request != nil && response.Request.Method == http.MethodHead) {
		return nil
	}
	if kind != "text/html" && kind != "text/css" && kind != "text/javascript" && kind != "application/javascript" {
		return nil
	}
	var reader io.Reader = response.Body
	defer response.Body.Close()
	if encoding := strings.ToLower(strings.TrimSpace(response.Header.Get("Content-Encoding"))); encoding == "gzip" {
		compressed, err := gzip.NewReader(response.Body)
		if err != nil {
			return err
		}
		defer compressed.Close()
		reader = compressed
	} else if encoding != "" && encoding != "identity" {
		return fmt.Errorf("unsupported gateway content encoding")
	}
	const limit = 16 << 20
	raw, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return err
	}
	if len(raw) > limit {
		return fmt.Errorf("gateway text response exceeds rewrite limit")
	}
	// Re-serialization emits UTF-8. Decode legacy device encodings first.
	var decoded io.Reader = bytes.NewReader(raw)
	if kind == "text/html" {
		decoded, err = charset.NewReader(decoded, response.Header.Get("Content-Type"))
		if err != nil {
			return err
		}
	} else if _, params, parseErr := mime.ParseMediaType(response.Header.Get("Content-Type")); parseErr == nil && params["charset"] != "" {
		encoding, _ := charset.Lookup(params["charset"])
		if encoding == nil {
			return fmt.Errorf("unsupported gateway text charset")
		}
		decoded = transform.NewReader(decoded, encoding.NewDecoder())
	}
	raw, err = io.ReadAll(io.LimitReader(decoded, limit+1))
	if err != nil {
		return err
	}
	if len(raw) > limit {
		return fmt.Errorf("gateway decoded text exceeds rewrite limit")
	}
	text := string(raw)
	switch kind {
	case "text/html":
		text, err = rewriteGatewayHTML(text, prefix, target, entry...)
	case "text/css":
		text = rewriteGatewayCSS(text, prefix, target, entry...)
	default:
		text = rewriteGatewayJS(text, prefix, target, entry...)
	}
	if err != nil {
		return err
	}
	data := []byte(text)
	response.Body = io.NopCloser(bytes.NewReader(data))
	response.ContentLength = int64(len(data))
	response.Header.Set("Content-Length", strconv.Itoa(len(data)))
	response.Header.Set("Content-Type", kind+"; charset=utf-8")
	for _, key := range []string{"Content-Encoding", "ETag", "Content-MD5", "Last-Modified", "Accept-Ranges"} {
		response.Header.Del(key)
	}
	response.Header.Set("Cache-Control", "no-store")
	return nil
}

func rewriteGatewayHTML(source, prefix, target string, entry ...string) (string, error) {
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return "", err
	}
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode {
			var refresh bool
			for _, attr := range node.Attr {
				if attr.Key == "http-equiv" && strings.EqualFold(attr.Val, "refresh") {
					refresh = true
				}
			}
			for i, attr := range node.Attr {
				switch attr.Key {
				case "src", "href", "action", "poster", "data", "formaction":
					node.Attr[i].Val = gatewayResourceURL(attr.Val, prefix, target, entry...)
				case "style":
					node.Attr[i].Val = rewriteGatewayCSS(attr.Val, prefix, target, entry...)
				case "srcset":
					// Data URLs contain commas; leave those lists untouched.
					if !strings.Contains(strings.ToLower(attr.Val), "data:") {
						candidates := strings.Split(attr.Val, ",")
						for j, candidate := range candidates {
							parts := strings.Fields(candidate)
							if len(parts) > 0 {
								parts[0] = gatewayResourceURL(parts[0], prefix, target, entry...)
								candidates[j] = strings.Join(parts, " ")
							}
						}
						node.Attr[i].Val = strings.Join(candidates, ", ")
					}
				case "charset":
					if node.Data == "meta" {
						node.Attr[i].Val = "utf-8"
					}
				case "content":
					if refresh {
						delay, location, found := strings.Cut(attr.Val, ";")
						key, dest, hasURL := strings.Cut(strings.TrimSpace(location), "=")
						if found && hasURL && strings.EqualFold(key, "url") {
							node.Attr[i].Val = delay + ";url=" + gatewayResourceURL(strings.Trim(dest, "\"'"), prefix, target, entry...)
						}
					}
				}
			}
			if node.Data == "meta" {
				for _, attr := range node.Attr {
					if attr.Key == "http-equiv" && strings.EqualFold(attr.Val, "content-type") {
						for i := range node.Attr {
							if node.Attr[i].Key == "content" {
								node.Attr[i].Val = "text/html; charset=utf-8"
							}
						}
					}
				}
			}
			if node.Data == "head" {
				script := &html.Node{Type: html.ElementNode, Data: "script", Attr: []html.Attribute{{Key: "src", Val: prefix + "/__gateway_compat.js"}}}
				node.InsertBefore(script, node.FirstChild)
			}
			if node.Data == "style" || node.Data == "script" {
				scriptType := ""
				for _, attr := range node.Attr {
					if attr.Key == "type" {
						scriptType = strings.ToLower(attr.Val)
					}
				}
				for child := node.FirstChild; child != nil; child = child.NextSibling {
					if child.Type != html.TextNode {
						continue
					}
					if node.Data == "style" {
						child.Data = rewriteGatewayCSS(child.Data, prefix, target, entry...)
					} else if scriptType == "" || scriptType == "module" || scriptType == "text/javascript" || scriptType == "application/javascript" {
						child.Data = rewriteGatewayJS(child.Data, prefix, target, entry...)
					}
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(doc)
	var out bytes.Buffer
	err = html.Render(&out, doc)
	return out.String(), err
}

// Conservative lexical guard: quoted business strings, templates and comments are
// not executable URL operations. Template expressions are intentionally untouched.
func gatewayJSCodeOffsets(source string, matches [][]int) []bool {
	result := make([]bool, len(matches))
	state := byte(0)
	match := 0
	for i := 0; i < len(source); i++ {
		for match < len(matches) && matches[match][0] == i {
			result[match] = state == 0
			match++
		}
		c := source[i]
		switch state {
		case '\'', '"', '`':
			if c == '\\' {
				i++
			} else if c == state {
				state = 0
			}
		case 'L':
			if c == '\n' || c == '\r' {
				state = 0
			}
		case 'C':
			if c == '*' && i+1 < len(source) && source[i+1] == '/' {
				i++
				state = 0
			}
		default:
			if c == '\'' || c == '"' || c == '`' {
				state = c
			} else if c == '/' && i+1 < len(source) {
				if source[i+1] == '/' {
					state = 'L'
					i++
				} else if source[i+1] == '*' {
					state = 'C'
					i++
				}
			}
		}
		for match < len(matches) && matches[match][0] <= i {
			match++
		}
	}
	return result
}
