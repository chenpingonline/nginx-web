// Package waf inspects HTTP requests forwarded by Nginx over a private Unix
// socket. The listener has no upstream dialer and never serves the admin API.
package waf

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chenpingonline/nginx-web/internal/crscatalog"
	"github.com/chenpingonline/nginx-web/internal/domain"
	crs "github.com/corazawaf/coraza-coreruleset/v4"
	"github.com/corazawaf/coraza/v3"
	"github.com/corazawaf/coraza/v3/types"
)

const requestBodyLimit = 8 << 20

type Event struct {
	BodyLimit bool      `json:"body_limit,omitempty"`
	Time      time.Time `json:"time"`
	Client    string    `json:"client"`
	Host      string    `json:"host"`
	Path      string    `json:"path"`
	Instance  string    `json:"instance"`
	Site      string    `json:"site"`
	Rules     []int     `json:"rules"`
	Blocked   bool      `json:"blocked"`
}
type Inspector struct {
	engines [2]struct {
		once   sync.Once
		engine coraza.WAF
		err    error
	}
	mu          sync.Mutex
	events      [256]Event
	next, count int
}

func (i *Inspector) Events() []Event {
	i.mu.Lock()
	defer i.mu.Unlock()
	out := make([]Event, 0, i.count)
	for n := 0; n < i.count; n++ {
		out = append(out, i.events[(i.next+len(i.events)-1-n)%len(i.events)])
	}
	return out
}
func (i *Inspector) store(e Event) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.events[i.next] = e
	i.next = (i.next + 1) % len(i.events)
	if i.count < len(i.events) {
		i.count++
	}
}
func (i *Inspector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mode, excluded, hasExclusions := strings.Cut(r.Header.Get("X-Fnproxy-Mode"), ":")
	instance := r.Header.Get("X-Fnproxy-Instance")
	site := r.Header.Get("X-Fnproxy-Site")
	if !domain.ValidID(instance) || !domain.ValidID(site) || (mode != "block" && mode != "detect") {
		http.Error(w, "无效的检测请求", 400)
		return
	}
	resume := "@fnproxy_checked"
	if target := r.Header.Get("X-Fnproxy-Target"); target != "generic" {
		branch, err := strconv.ParseUint(target, 10, 32)
		if err != nil {
			http.Error(w, "无效的代理目标", 400)
			return
		}
		resume += "_" + strconv.FormatUint(branch, 10)
	}
	engine, err := i.engine(mode)
	if err != nil {
		http.Error(w, "WAF 引擎不可用", 503)
		return
	}
	client := r.Header.Get("X-Fnproxy-Client")
	tx := engine.NewTransaction()
	defer tx.Close()
	if hasExclusions {
		// The pinned Coraza transaction exposes per-request rule removal.
		scoped, ok := tx.(interface{ RemoveRuleByID(int) })
		if !ok {
			http.Error(w, "WAF 不支持站点规则排除", 503)
			return
		}
		for _, raw := range strings.Split(excluded, ",") {
			id, err := strconv.Atoi(raw)
			if err != nil || !crscatalog.Known(id) {
				http.Error(w, "无效的站点规则排除", 400)
				return
			}
			scoped.RemoveRuleByID(id)
		}
	}
	tx.ProcessConnection(client, 0, "", 0)
	tx.ProcessURI(r.RequestURI, r.Method, r.Header.Get("X-Fnproxy-Protocol"))
	tx.SetServerName(r.Host)
	tx.AddRequestHeader("Host", r.Host)
	for k, vs := range r.Header {
		switch k {
		case "X-Fnproxy-Mode", "X-Fnproxy-Instance", "X-Fnproxy-Site", "X-Fnproxy-Target", "X-Fnproxy-Client", "X-Fnproxy-Protocol":
			continue
		}
		for _, v := range vs {
			tx.AddRequestHeader(k, v)
		}
	}
	interrupt := tx.ProcessRequestHeaders()
	if interrupt == nil && r.ContentLength >= requestBodyLimit {
		interrupt = &types.Interruption{Status: http.StatusRequestEntityTooLarge}
	}
	if interrupt == nil && r.Body != nil && r.Body != http.NoBody {
		var err error
		interrupt, _, err = tx.ReadRequestBodyFrom(r.Body)
		if err != nil {
			http.Error(w, "请求体检测失败", 503)
			return
		}
	}
	if interrupt == nil {
		var err error
		interrupt, err = tx.ProcessRequestBody()
		if err != nil {
			http.Error(w, "规则执行失败", 503)
			return
		}
	}
	oversize := interrupt != nil && interrupt.Status == http.StatusRequestEntityTooLarge
	// Nginx has already buffered the original request. Drain the inspection
	// copy so that closing the Unix connection cannot interrupt its final reply.
	if interrupt != nil && r.Body != nil {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			http.Error(w, "读取请求体失败", 503)
			return
		}
	}
	ids := map[int]bool{}
	for _, m := range tx.MatchedRules() {
		if m.Message() != "" {
			ids[m.Rule().ID()] = true
		}
	}
	blocked := interrupt != nil && mode == "block"
	if len(ids) > 0 || blocked || oversize {
		rules := make([]int, 0, len(ids))
		for id := range ids {
			rules = append(rules, id)
		}
		sort.Ints(rules)
		path := r.URL.EscapedPath()
		if len(path) > 512 {
			path = path[:512]
		}
		i.store(Event{BodyLimit: oversize, Time: time.Now().UTC(), Client: client, Host: r.Host, Path: path, Instance: instance, Site: site, Rules: rules, Blocked: blocked})
	}
	if blocked {
		status := interrupt.Status
		if status < 400 || status > 599 {
			status = 403
		}
		http.Error(w, "请求被 WAF 拦截", status)
		return
	}
	w.Header().Set("X-Accel-Redirect", resume)
	w.WriteHeader(http.StatusOK)
}

func (i *Inspector) engine(mode string) (coraza.WAF, error) {
	slot := 0
	engineMode := "DetectionOnly"
	if mode == "block" {
		slot = 1
		engineMode = "On"

	}
	e := &i.engines[slot]
	e.once.Do(func() {
		directives := fmt.Sprintf(`Include @coraza.conf-recommended
SecRuleEngine %s
SecRequestBodyAccess On
SecRequestBodyLimit %d
SecRequestBodyLimitAction Reject
SecResponseBodyAccess Off
SecAuditEngine Off
Include @crs-setup.conf.example
Include @owasp_crs/*.conf`, engineMode, requestBodyLimit)
		e.engine, e.err = coraza.NewWAF(coraza.NewWAFConfig().WithRootFS(crs.FS).WithDirectives(directives))
		if e.err != nil {
			log.Printf("WAF 引擎初始化失败: %v", e.err)
		}
	})
	return e.engine, e.err
}
