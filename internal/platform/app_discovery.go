package platform

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// AppEntry contains only public launch metadata, never application settings or tokens.
type AppEntry struct {
	RecommendedMode string `json:"recommended_mode,omitempty"`
	App             string `json:"app"`
	Entry           string `json:"entry"`
	Name            string `json:"name"`
	Path            string `json:"path"`
	Target          string `json:"target"`
	GatewayURL      string `json:"gateway_url,omitempty"`
	Note            string `json:"note"`
	Existing        bool   `json:"existing"`
}

type AppDiscovery struct {
	Entries    []AppEntry `json:"entries"`
	Unreadable int        `json:"unreadable"`
}

// DiscoverAppEntries reads declared desktop entries. It does not scan ports,
// execute app scripts, inspect private config, or infer HTTP from service_port alone.
func DiscoverAppEntries(root string) (AppDiscovery, error) {
	result := AppDiscovery{Entries: []AppEntry{}}
	apps, err := os.ReadDir(root)
	if err != nil {
		return result, errors.New("无法读取飞牛应用目录，请确认在飞牛设备上运行且具有读取权限")
	}
	if len(apps) > 1024 {
		return result, errors.New("飞牛应用目录条目过多")
	}
	for _, app := range apps {
		if !app.IsDir() || app.Name() == "nginx-web" {
			continue
		}
		base := filepath.Join(root, app.Name())
		raw, err := readDiscoveryFile(filepath.Join(base, "manifest"), 64<<10)
		if err != nil {
			result.Unreadable++
			continue
		}
		manifest := map[string]string{}
		scanner := bufio.NewScanner(strings.NewReader(string(raw)))
		for scanner.Scan() {
			key, value, ok := strings.Cut(scanner.Text(), "=")
			if ok {
				manifest[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), "\"'")
			}
		}
		ui := manifest["desktop_uidir"]
		if ui == "" {
			ui = "ui"
		}
		if filepath.IsAbs(ui) || ui == ".." || strings.HasPrefix(filepath.Clean(ui), ".."+string(filepath.Separator)) {
			result.Unreadable++
			continue
		}
		target, err := filepath.EvalSymlinks(filepath.Join(base, "target"))
		if err != nil {
			result.Unreadable++
			continue
		}
		config, err := filepath.EvalSymlinks(filepath.Join(target, ui, "config"))
		if err != nil {
			if !os.IsNotExist(err) {
				result.Unreadable++
			}
			continue
		}
		rel, err := filepath.Rel(target, config)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			result.Unreadable++
			continue
		}
		raw, err = readDiscoveryFile(config, 1<<20)
		if err != nil {
			result.Unreadable++
			continue
		}
		var doc struct {
			URLs map[string]struct {
				Title         string          `json:"title"`
				Protocol      string          `json:"protocol"`
				Port          json.RawMessage `json:"port"`
				URL           string          `json:"url"`
				Type          string          `json:"type"`
				Hidden        bool            `json:"noDisplay"`
				GatewayPrefix string          `json:"gatewayPrefix"`
				GatewaySocket string          `json:"gatewaySocket"`
			} `json:".url"`
		}
		if json.Unmarshal(raw, &doc) != nil {
			result.Unreadable++
			continue
		}
		for id, entry := range doc.URLs {
			if entry.Hidden || (entry.Type != "iframe" && entry.Type != "url") {
				continue
			}
			name := entry.Title
			if name == "" || strings.Contains(name, "${") {
				name = manifest["display_name"]
			}
			if name == "" || strings.Contains(name, "${") {
				name = app.Name()
			}
			candidate := AppEntry{App: app.Name(), Entry: id, Name: name}
			if entry.GatewayPrefix != "" && entry.GatewaySocket != "" {
				prefix := entry.GatewayPrefix
				if strings.HasPrefix(prefix, "/app/") && !strings.ContainsAny(prefix, "?#\\") && !strings.Contains(prefix, "..") {
					candidate.GatewayURL = strings.TrimRight(prefix, "/") + "/"
					candidate.Note = "已有飞牛网关，可直接访问，无需重复接入"
					result.Entries = append(result.Entries, candidate)
				}
				continue
			}
			if strings.HasPrefix(entry.URL, "/cgi/") {
				continue
			}
			protocol := strings.ToLower(entry.Protocol)
			if protocol != "http" && protocol != "https" && protocol != "" {
				continue
			}
			if protocol == "" {
				protocol = "http"
			}
			port := strings.Trim(string(entry.Port), "\"")
			if port == "${TRIM_SERVICE_PORT}" {
				port = manifest["service_port"]
			}
			n, err := strconv.Atoi(port)
			if err == nil && n > 0 && n <= 65535 {
				candidate.Target = protocol + "://127.0.0.1:" + strconv.Itoa(n)
				candidate.Note = "应用声明的端口；保存前请确认服务已启动及子路径兼容性"
			} else if strings.Contains(port, "${") || strings.Contains(port, "$port") {
				candidate.Note = "端口由安装向导设置，需填写当前端口"
			} else {
				continue
			}
			// Absolute external URLs and credential-bearing launch URLs are not local services.
			if u, err := url.Parse(entry.URL); err != nil || u.IsAbs() || u.Host != "" || strings.Contains(entry.URL, "${") {
				continue
			}
			result.Entries = append(result.Entries, candidate)
		}
	}
	sort.Slice(result.Entries, func(i, j int) bool {
		a, b := result.Entries[i], result.Entries[j]
		if a.App == b.App {
			return a.Entry < b.Entry
		}
		return a.App < b.App
	})
	return result, nil
}

func readDiscoveryFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("应用声明文件无效或过大")
	}
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(raw)) > limit {
		return nil, errors.New("应用声明文件过大")
	}
	return raw, err
}
