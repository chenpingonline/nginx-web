package service

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chenpingonline/nginx-web/internal/platform"
)

// Relocate only paths owned by the application, never user-supplied static
// directories, htpasswd files or CA bundles. No source path is opened.
func formPaths(p platform.Paths, st State, content string) map[string]string {
	known := map[string]string{"auth": p.SocketPath, "waf": p.WAFSocket(), "topology": p.TopologySocket(), "pid": p.NginxPID, "mime": p.MimeTypes, "error-log": p.NginxErrorLog, "access-log": p.NginxAccessLog, "metrics": p.MetricsLog(), "stream-log": p.NginxStreamLog, "stream-metrics": p.StreamMetricsLog(), "http-include": filepath.Join(p.NginxConfD, "*.conf"), "stream-include": filepath.Join(p.NginxConfD, "*.stream")}
	for _, d := range []string{"body", "proxy", "fastcgi", "scgi", "uwsgi"} {
		known["temp/"+d] = filepath.Join(p.NginxTempDir, d)
	}
	for _, c := range st.Certificates {
		for _, f := range []string{"fullchain.pem", "privkey.pem"} {
			known[c.ID+"/"+f] = filepath.Join(p.CertificateDir, c.ID, f)
		}
	}
	for k, v := range known {
		if !strings.Contains(content, v) {
			delete(known, k)
		}
	}
	return known
}
func originalFormText(text string, local, origin map[string]string) (string, error) {
	if len(local) != len(origin) {
		return "", errors.New("配置使用的应用文件或证书不匹配")
	}
	keys := make([]string, 0, len(local))
	for k := range local {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(local[keys[i]]) > len(local[keys[j]]) })
	pairs := make([]string, 0, len(keys)*2)
	for _, k := range keys {
		v := origin[k]
		if !filepath.IsAbs(v) || strings.ContainsAny(v, "\x00\r\n") {
			return "", errors.New("配置中的应用路径无效")
		}
		pairs = append(pairs, local[k], v)
	}
	return strings.NewReplacer(pairs...).Replace(text), nil
}
