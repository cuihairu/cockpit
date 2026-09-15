package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ============ Nginx Proxy Provider ============
//
// Nginx 站点可视化下发（见 proxy-design.md）：cockpit 只管理自己名下的
// conf 片段（confDir/cockpit-site-*.conf），用户已有配置零接触（D2）。
// 站点元数据内嵌在文件头注释里（D3），回读自己渲染的产物，无需解析
// nginx 语法。应用流程「nginx -t → 写 → reload，失败回滚」（D4）保证
// apply 失败站点照旧。

const (
	// nginxTestTimeout nginx -t / reload 单命令超时
	nginxTestTimeout = 10 * time.Second
	// nginxMetaPrefix 片段文件首行注释前缀
	nginxMetaPrefix = "# cockpit:meta "
	// nginxMaxExtra extra 指令直通上限
	nginxMaxExtra = 4 * 1024
)

var (
	// proxyNameRe 站点名（文件名一部分），同 backupNameRe
	proxyNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	// proxyDomainRe 域名/通配符（server_name 单值）
	proxyDomainRe = regexp.MustCompile(`^[A-Za-z0-9*.\-]{1,253}$`)
	// proxyUpstreamRe 上游 host:port
	proxyUpstreamRe = regexp.MustCompile(`^[A-Za-z0-9.:\-]{1,253}$`)
	// proxyFileRe 片段文件名（列目录后过滤）
	proxyFileRe = regexp.MustCompile(`^cockpit-site-([a-z0-9][a-z0-9_-]{0,63})\.conf$`)
)

// ProxySite 一个反代站点的声明
type ProxySite struct {
	Name        string   `json:"name"`
	ServerNames []string `json:"serverNames"`
	Upstream    string   `json:"upstream"`
	Scheme      string   `json:"scheme"` // http / https
	TLSCert     string   `json:"tlsCert,omitempty"`
	TLSKey      string   `json:"tlsKey,omitempty"`
	Websocket   bool     `json:"websocket,omitempty"`
	Extra       string   `json:"extra,omitempty"`
}

// validate 站点参数校验（server 侧同规则，双端防御）
func (s *ProxySite) validate() error {
	if !proxyNameRe.MatchString(s.Name) {
		return fmt.Errorf("invalid site name %q", s.Name)
	}
	if len(s.ServerNames) == 0 || len(s.ServerNames) > 16 {
		return fmt.Errorf("serverNames must contain 1-16 entries")
	}
	for _, d := range s.ServerNames {
		if !proxyDomainRe.MatchString(d) {
			return fmt.Errorf("invalid server name %q", d)
		}
	}
	if !proxyUpstreamRe.MatchString(s.Upstream) {
		return fmt.Errorf("invalid upstream %q", s.Upstream)
	}
	switch s.Scheme {
	case "http":
	case "https":
		if !strings.HasPrefix(s.TLSCert, "/") || !strings.HasPrefix(s.TLSKey, "/") {
			return fmt.Errorf("https requires absolute tlsCert and tlsKey paths")
		}
	default:
		return fmt.Errorf("scheme must be http or https")
	}
	if len(s.Extra) > nginxMaxExtra {
		return fmt.Errorf("extra directives too large (max %d bytes)", nginxMaxExtra)
	}
	return nil
}

// Commander 外部命令执行抽象（测试注入用）
type Commander func(ctx context.Context, name string, args ...string) (stdout []byte, stderr []byte, err error)

func defaultCommander(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// NginxConfig provider 配置（测试注入）
type NginxConfig struct {
	ConfDir string    // 片段目录，默认 /etc/nginx/conf.d
	Run     Commander // 命令执行器，默认 exec
}

// NginxProvider Nginx 反代管理 Provider
type NginxProvider struct {
	confDir  string
	run      Commander
	baseline BaselineRecorder // 漂移基线挂钩（见 drift-design.md D7），nil 不记录
}

// SetBaseline 注入漂移基线挂钩（providers.go 接线用）
func (p *NginxProvider) SetBaseline(b BaselineRecorder) { p.baseline = b }

func NewNginxProvider(cfg NginxConfig) *NginxProvider {
	confDir := cfg.ConfDir
	if confDir == "" {
		if v := os.Getenv("COCKPIT_NGINX_CONF_DIR"); v != "" {
			confDir = v
		} else {
			confDir = "/etc/nginx/conf.d"
		}
	}
	run := cfg.Run
	if run == nil {
		run = defaultCommander
	}
	return &NginxProvider{confDir: confDir, run: run}
}

func (p *NginxProvider) Type() string { return "nginx" }

func (p *NginxProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "status":
		return p.Status()
	case "sites":
		return p.Sites()
	case "site.get":
		return p.GetSite(paramString(params, "name"))
	case "site.apply":
		site, err := siteFromParams(params)
		if err != nil {
			return nil, err
		}
		return p.ApplySite(site)
	case "site.delete":
		return p.DeleteSite(paramString(params, "name"))
	default:
		return nil, fmt.Errorf("unknown nginx action: %s", action)
	}
}

func siteFromParams(params map[string]interface{}) (*ProxySite, error) {
	raw, ok := params["site"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("site object required")
	}
	b, _ := json.Marshal(raw)
	var site ProxySite
	if err := json.Unmarshal(b, &site); err != nil {
		return nil, fmt.Errorf("bad site payload: %w", err)
	}
	return &site, nil
}

// DetectNginx 探测 nginx 可执行与版本；未安装返回空版本
func DetectNginx() (version string, ok bool) {
	if _, err := exec.LookPath("nginx"); err != nil {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, stderr, err := defaultCommander(ctx, "nginx", "-v")
	if err != nil && stderr == nil {
		return "", false
	}
	// nginx -v 把版本打到 stderr：nginx version: nginx/1.24.0
	line := strings.TrimSpace(string(stderr))
	if _, ver, found := strings.Cut(line, "nginx/"); found {
		return "nginx/" + strings.Fields(ver)[0], true
	}
	return "nginx", true
}

// ============ RPC 实现 ============

// Status nginx 安装状态概览
func (p *NginxProvider) Status() (interface{}, error) {
	sites, err := p.loadSites()
	if err != nil {
		return nil, err
	}
	reloadMode := "signal"
	if _, err := exec.LookPath("systemctl"); err == nil {
		reloadMode = "systemctl"
	}
	version, ok := DetectNginx()
	return map[string]interface{}{
		"installed":  ok,
		"version":    version,
		"confDir":    p.confDir,
		"siteCount":  len(sites),
		"reloadMode": reloadMode,
	}, nil
}

// Sites 列出 cockpit 名下站点（解析 meta 注释）
func (p *NginxProvider) Sites() (interface{}, error) {
	sites, err := p.loadSites()
	if err != nil {
		return nil, err
	}
	out := make([]map[string]interface{}, 0, len(sites))
	for i := range sites {
		s := &sites[i]
		out = append(out, map[string]interface{}{
			"name":        s.Name,
			"serverNames": s.ServerNames,
			"upstream":    s.Upstream,
			"scheme":      s.Scheme,
			"websocket":   s.Websocket,
		})
	}
	return map[string]interface{}{"sites": out}, nil
}

// GetSite 查看站点元数据与渲染后的配置全文
func (p *NginxProvider) GetSite(name string) (interface{}, error) {
	if !proxyNameRe.MatchString(name) {
		return nil, fmt.Errorf("invalid site name %q", name)
	}
	content, err := os.ReadFile(p.sitePath(name))
	if err != nil {
		return nil, fmt.Errorf("site not found: %s", name)
	}
	site, err := parseMeta(content)
	if err != nil {
		return nil, fmt.Errorf("corrupted meta: %w", err)
	}
	return map[string]interface{}{"name": name, "site": site, "content": string(content)}, nil
}

// ApplySite 校验 → nginx -t → 写片段 → reload；nginx -t 失败不落盘，
// reload 失败回滚旧内容（D4）。
func (p *NginxProvider) ApplySite(site *ProxySite) (interface{}, error) {
	if err := site.validate(); err != nil {
		return nil, err
	}
	if err := p.testConfig(); err != nil {
		return nil, err
	}

	path := p.sitePath(site.Name)
	var previous []byte // nil = 原本不存在
	if b, err := os.ReadFile(path); err == nil {
		previous = b
	}
	written := []byte(renderSite(site))
	if err := os.WriteFile(path, written, 0o644); err != nil {
		return nil, fmt.Errorf("write config: %w", err)
	}
	if err := p.reload(); err != nil {
		// 回滚：原来有内容恢复之，原本没有则删掉新文件
		if previous != nil {
			_ = os.WriteFile(path, previous, 0o644)
		} else {
			_ = os.Remove(path)
		}
		_ = p.reload() // 回滚后尽力恢复线上状态；失败也不再遮掩原错误
		return nil, fmt.Errorf("reload failed (config rolled back): %w", err)
	}
	// 写入并 reload 成功后登记基线（基线 = 线上实际生效的内容，D8）
	if p.baseline != nil {
		p.baseline.Record("nginx", site.Name, written)
	}
	return map[string]interface{}{"name": site.Name, "file": filepath.Base(path)}, nil
}

// DeleteSite 删片段文件 → reload；reload 失败恢复文件
func (p *NginxProvider) DeleteSite(name string) (interface{}, error) {
	if !proxyNameRe.MatchString(name) {
		return nil, fmt.Errorf("invalid site name %q", name)
	}
	path := p.sitePath(name)
	previous, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("site not found: %s", name)
	}
	if err := os.Remove(path); err != nil {
		return nil, fmt.Errorf("remove config: %w", err)
	}
	if err := p.reload(); err != nil {
		_ = os.WriteFile(path, previous, 0o644)
		_ = p.reload()
		return nil, fmt.Errorf("reload failed (site restored): %w", err)
	}
	if p.baseline != nil {
		p.baseline.Forget("nginx", name)
	}
	return map[string]interface{}{}, nil
}

// ============ 内部 ============

func (p *NginxProvider) sitePath(name string) string {
	return filepath.Join(p.confDir, "cockpit-site-"+name+".conf")
}

// loadSites 扫描片段目录，解析每个文件头部的 meta；损坏条目跳过
func (p *NginxProvider) loadSites() ([]ProxySite, error) {
	matches, err := filepath.Glob(filepath.Join(p.confDir, "cockpit-site-*.conf"))
	if err != nil {
		return nil, err
	}
	sites := make([]ProxySite, 0, len(matches))
	for _, path := range matches {
		base := filepath.Base(path)
		m := proxyFileRe.FindStringSubmatch(base)
		if m == nil {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		site, err := parseMeta(b)
		if err != nil {
			continue // meta 损坏（可能被手工改过）不中断列表
		}
		sites = append(sites, *site)
	}
	return sites, nil
}

// testConfig nginx -t 语法校验；失败带 stderr 摘要返回（不落盘的前提）
func (p *NginxProvider) testConfig() error {
	ctx, cancel := context.WithTimeout(context.Background(), nginxTestTimeout)
	defer cancel()
	_, stderr, err := p.run(ctx, "nginx", "-t")
	if err == nil {
		return nil
	}
	return fmt.Errorf("nginx -t failed: %s", commandErrSummary(stderr, err))
}

// commandErrSummary 提取命令失败的可读摘要（stderr 优先，截断 2KB）
func commandErrSummary(stderr []byte, err error) string {
	summary := strings.TrimSpace(string(stderr))
	if len(summary) > 2048 {
		summary = summary[:2048]
	}
	if summary == "" {
		summary = err.Error()
	}
	return summary
}

// reload 平滑重载：systemctl 优先，失败 fallback nginx -s reload（D5）
func (p *NginxProvider) reload() error {
	ctx, cancel := context.WithTimeout(context.Background(), nginxTestTimeout)
	defer cancel()
	if _, err := exec.LookPath("systemctl"); err == nil {
		_, stderr, err := p.run(ctx, "systemctl", "reload", "nginx")
		if err == nil {
			return nil
		}
		// 「单元不存在 / 无 systemd 运行」时改走信号，其余失败直接报错
		if !systemctlMissing(string(stderr)) {
			return fmt.Errorf("systemctl reload nginx: %s", commandErrSummary(stderr, err))
		}
	}
	_, stderr, err := p.run(ctx, "nginx", "-s", "reload")
	if err != nil {
		return fmt.Errorf("nginx reload: %s", commandErrSummary(stderr, err))
	}
	return nil
}

// systemctlMissing systemctl reload 输出「没有该单元」类错误时改走信号
func systemctlMissing(stderr string) bool {
	s := strings.ToLower(stderr)
	return strings.Contains(s, "not found") || strings.Contains(s, "不存在") ||
		strings.Contains(s, "failed to connect") // 无 systemd 运行（容器内）
}

// parseMeta 从片段内容首行解析 meta 注释
func parseMeta(content []byte) (*ProxySite, error) {
	text := string(content)
	line := text
	if idx := strings.Index(text, "\n"); idx >= 0 {
		line = text[:idx]
	}
	if !strings.HasPrefix(line, nginxMetaPrefix) {
		return nil, fmt.Errorf("missing meta header")
	}
	var site ProxySite
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, nginxMetaPrefix)), &site); err != nil {
		return nil, err
	}
	return &site, nil
}

// renderSite 渲染片段：首行 meta 注释 + server 块（https 时附带 80 跳转块）
func renderSite(s *ProxySite) string {
	meta, _ := json.Marshal(s)
	var b strings.Builder
	b.WriteString(nginxMetaPrefix)
	b.Write(meta)
	b.WriteString("\n\n")

	if s.Scheme == "https" {
		// 80 → https 301
		b.WriteString("server {\n    listen 80;\n")
		for _, d := range s.ServerNames {
			b.WriteString(fmt.Sprintf("    server_name %s;\n", d))
		}
		b.WriteString("    return 301 https://$host$request_uri;\n}\n\n")
	}

	b.WriteString("server {\n")
	if s.Scheme == "https" {
		b.WriteString("    listen 443 ssl;\n")
	} else {
		b.WriteString("    listen 80;\n")
	}
	for _, d := range s.ServerNames {
		b.WriteString(fmt.Sprintf("    server_name %s;\n", d))
	}
	if s.Scheme == "https" {
		b.WriteString(fmt.Sprintf("    ssl_certificate %s;\n", s.TLSCert))
		b.WriteString(fmt.Sprintf("    ssl_certificate_key %s;\n", s.TLSKey))
	}

	b.WriteString("\n    location / {\n")
	b.WriteString(fmt.Sprintf("        proxy_pass http://%s;\n", s.Upstream))
	b.WriteString("        proxy_set_header Host $host;\n")
	b.WriteString("        proxy_set_header X-Real-IP $remote_addr;\n")
	b.WriteString("        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n")
	b.WriteString("        proxy_set_header X-Forwarded-Proto $scheme;\n")
	if s.Websocket {
		b.WriteString("        proxy_http_version 1.1;\n")
		b.WriteString("        proxy_set_header Upgrade $http_upgrade;\n")
		b.WriteString("        proxy_set_header Connection \"upgrade\";\n")
	}
	b.WriteString("    }\n")

	if strings.TrimSpace(s.Extra) != "" {
		b.WriteString("\n    # extra directives\n")
		b.WriteString(strings.TrimRight(s.Extra, "\n"))
		b.WriteString("\n")
	}

	b.WriteString("}\n")
	return b.String()
}
