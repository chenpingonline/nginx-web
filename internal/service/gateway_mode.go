package service

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chenpingonline/nginx-web/internal/domain"
	"github.com/chenpingonline/nginx-web/internal/gatewayclient"
	"github.com/chenpingonline/nginx-web/internal/platform"
	"golang.org/x/net/html"
)

// Inspect only declared local web services. Discovery has a shared deadline
// so an offline app cannot hold up the whole gateway page.
func recommendGatewayModes(parent context.Context, entries []platform.AppEntry) {
	var candidates []*platform.AppEntry
	for i := range entries {
		e := &entries[i]
		if e.Existing || e.GatewayURL != "" {
			continue
		}
		e.RecommendedMode = "subpath"
		if e.Target != "" {
			candidates = append(candidates, e)
		}
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	jobs := make(chan *platform.AppEntry)
	var workers sync.WaitGroup
	for range min(6, len(candidates)) {
		workers.Go(func() {
			for e := range jobs {
				if nativeGatewayPage(ctx, e.Target, "/app/nginx-web/proxy/"+e.Path+"/") {
					e.RecommendedMode = "native"
				}
			}
		})
	}
	for _, e := range candidates {
		select {
		case jobs <- e:
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			return
		}
	}
	close(jobs)
	workers.Wait()
}

func nativeGatewayPage(ctx context.Context, target, prefix string) bool {
	u, err := url.Parse(target)
	if err != nil || u.Hostname() != "127.0.0.1" || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return false
	}
	req.Header.Set("X-Forwarded-Prefix", strings.TrimSuffix(prefix, "/"))
	client := http.Client{
		Transport:     gatewayclient.Transport(domain.GatewayRoute{}),
		Timeout:       1200 * time.Millisecond,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	kind, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if resp.StatusCode != http.StatusOK || kind != "text/html" {
		return false
	}
	const limit = 256 << 10
	body := &io.LimitedReader{R: resp.Body, N: limit + 1}
	tokens := html.NewTokenizer(body)
	base := &url.URL{Path: prefix}
	evidence := false
	for {
		switch tokens.Next() {
		case html.ErrorToken:
			return errors.Is(tokens.Err(), io.EOF) && body.N > 0 && evidence
		case html.StartTagToken, html.SelfClosingTagToken:
			tag := tokens.Token()
			for _, a := range tag.Attr {
				if a.Key != "src" && a.Key != "href" && a.Key != "action" {
					continue
				}
				value := strings.TrimSpace(a.Val)
				if value == "" || strings.HasPrefix(value, "#") || strings.HasPrefix(value, "?") {
					continue
				}
				ref, err := url.Parse(value)
				if err != nil {
					return false
				}
				if ref.Host != "" || ref.Scheme != "" {
					if strings.EqualFold(ref.Host, u.Host) || tag.Data == "base" {
						return false
					}
					continue
				}
				resolved := base.ResolveReference(ref)
				if !strings.HasPrefix(resolved.Path, prefix) {
					return false
				}
				if tag.Data == "base" && a.Key == "href" {
					base = resolved
				}
				if a.Key == "src" || tag.Data == "base" || tag.Data == "link" {
					evidence = true
				}
			}
		}
	}
}
