package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ============ Traefik Proxy Provider ============
//
// Traefik 站点可视化下发（见 proxy-design.md M2 D11-D16）：只走 file
// provider 动态目录，每站点一个 YAML 片段文件，与 nginx conf.d 片段
// 模式同构，D2「自己名下片段、用户配置零接触」原样成立。
// 无 nginx -t/reload 等价物——渲染后 yaml.Unmarshal 自检（失败不落盘），
// file provider 热加载、坏文件局部隔离（D13）。

const (
	// traefikVersionTimeout traefik version 探测超时
	traefikVersionTimeout = 5 * time.Second
	// traefikDynamicDirDefault file provider 动态目录缺省值
	traefikDynamicDirDefault = "/etc/traefik/dynamic"
	// traefikStaticConfig 静态配置文件（读 providers.file.directory）
	traefikStaticConfig = "/etc/traefik/traefik.yml"
	// traefikStaticConfigYaml 同上的 .yaml 拼写
	traefikStaticConfigYaml = "/etc/traefik/traefik.yaml"
)

// traefikFileRe 片段文件名（与 nginx 同构，扩展名 .yml）
var traefikFileRe = regexp.MustCompile(`^cockpit-site-([a-z0-9][a-z0-9_-]{0,63})\.yml$`)

// traefikStaticConfigs 静态配置候选路径（测试注入用）
var traefikStaticConfigs = []string{traefikStaticConfig, traefikStaticConfigYaml}

// TraefikProvider Traefik 动态配置管理 Provider
type TraefikProvider struct {
	dir      string // file provider 动态目录
	run      Commander
	baseline BaselineRecorder // 漂移基线挂钩（见 drift-design.md D7），nil 不记录
}

// SetBaseline 注入漂移基线挂钩（providers.go 接线用）
func (p *TraefikProvider) SetBaseline(b BaselineRecorder) { p.baseline = b }

// NewTraefikProvider dir 传 capability metadata 里的 dynamicDir（探测阶段定），
// 空值回退缺省目录；run 为 nil 用 exec
func NewTraefikProvider(dir string, run Commander) *TraefikProvider {
	if dir == "" {
		dir = traefikDynamicDirDefault
	}
	if run == nil {
		run = defaultCommander
	}
	return &TraefikProvider{dir: dir, run: run}
}

func (p *TraefikProvider) Type() string { return "traefik" }

func (p *TraefikProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
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
		return nil, fmt.Errorf("unknown traefik action: %s", action)
	}
}

// DetectTraefik 探测动态目录（D12）：COCKPIT_TRAEFIK_DIR 覆盖（同
// COCKPIT_NGINX_CONF_DIR 惯例）→ 静态配置的 providers.file.directory →
// 缺省 /etc/traefik/dynamic；目录存在即通过。不依赖 LookPath——Traefik
// 多为容器化宿主无二进制，目录（含 docker 挂载的宿主侧路径）才是事实源；
// 写权限失败在 apply 时诚实报错。返回目录供 capability metadata。
func DetectTraefik() (dir string, ok bool) {
	if v := os.Getenv("COCKPIT_TRAEFIK_DIR"); v != "" {
		if st, err := os.Stat(v); err == nil && st.IsDir() {
			return v, true
		}
		return "", false
	}
	dir = traefikDirFromStatic(traefikStaticConfigs)
	if dir == "" {
		dir = traefikDynamicDirDefault
	}
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		return dir, true
	}
	return "", false
}

// traefikDirFromStatic 依次读候选静态配置，取 providers.file.directory；
// 均缺失或解析失败返回空串
func traefikDirFromStatic(paths []string) string {
	for _, cfg := range paths {
		b, err := os.ReadFile(cfg)
		if err != nil {
			continue
		}
		var m struct {
			Providers struct {
				File struct {
					Directory string `yaml:"directory"`
				} `yaml:"file"`
			} `yaml:"providers"`
		}
		if err := yaml.Unmarshal(b, &m); err == nil && m.Providers.File.Directory != "" {
			return m.Providers.File.Directory
		}
	}
	return ""
}

// ============ RPC 实现 ============

// Status 概览：动态目录 + 站点数。version 仅当宿主机恰有 traefik 二进制
// 时非空（容器化部署常见为空，面板显示「-」，D16）
func (p *TraefikProvider) Status() (interface{}, error) {
	sites, err := p.loadSites()
	if err != nil {
		return nil, fmt.Errorf("scan dynamic dir: %w", err)
	}
	return map[string]interface{}{
		"backend":    "traefik",
		"installed":  true,
		"version":    p.detectVersion(),
		"confDir":    p.dir,
		"siteCount":  len(sites),
		"reloadMode": "hot", // file provider 热加载，无 reload 命令
	}, nil
}

// detectVersion traefik version 首行（容器化宿主常无此命令，失败返回空）。
// v2 输出首行形如 "Version:      v2.9.6"，v3 为裸版本号，两种都兼容。
func (p *TraefikProvider) detectVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), traefikVersionTimeout)
	defer cancel()
	out, _, err := p.run(ctx, "traefik", "version")
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(out))
	if idx := strings.Index(line, "\n"); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	if _, v, found := strings.Cut(line, "Version:"); found {
		return strings.TrimSpace(v)
	}
	return line
}

// Sites 列出 cockpit 名下站点（解析 meta；损坏条目跳过）
func (p *TraefikProvider) Sites() (interface{}, error) {
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
func (p *TraefikProvider) GetSite(name string) (interface{}, error) {
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

// ApplySite 校验 → 渲染 → yaml 自检（失败不落盘）→ 原子写（D13）。
// file provider 热加载无 reload 分支：坏文件由 Traefik 拒载、其余片段照常，
// 无「apply 失败站点照旧」的回滚需求。
func (p *TraefikProvider) ApplySite(site *ProxySite) (interface{}, error) {
	if err := site.validate(); err != nil {
		return nil, err
	}
	// extra 是 nginx 特有直通口；traefik 拒绝渲染任意 YAML 片段（D14 注入面）
	if strings.TrimSpace(site.Extra) != "" {
		return nil, fmt.Errorf("extra directives are not supported by the traefik backend")
	}

	written := renderTraefikSite(site)
	// 渲染后自检（D13）：渲染器保证输出合法，注入点供测试覆盖防御分支
	if err := traefikSelfCheck(written); err != nil {
		return nil, fmt.Errorf("rendered config failed self-check: %w", err)
	}

	path := p.sitePath(site.Name)
	// 临时文件 + rename 原子写：热加载 watch 到的片段要么旧要么新，无半写状态
	tmp := path + ".cockpit-tmp"
	if err := os.WriteFile(tmp, written, 0o644); err != nil {
		return nil, fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return nil, fmt.Errorf("write config: %w", err)
	}
	// 落盘成功即生效（热加载），登记基线（基线 = 线上实际生效的内容）
	if p.baseline != nil {
		p.baseline.Record("traefik", site.Name, written)
	}
	return map[string]interface{}{"name": site.Name, "file": filepath.Base(path)}, nil
}

// DeleteSite 删片段文件；热加载无回滚分支，删除失败原样返回错误
func (p *TraefikProvider) DeleteSite(name string) (interface{}, error) {
	if !proxyNameRe.MatchString(name) {
		return nil, fmt.Errorf("invalid site name %q", name)
	}
	path := p.sitePath(name)
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("site not found: %s", name)
	}
	if err := os.Remove(path); err != nil {
		return nil, fmt.Errorf("remove config: %w", err)
	}
	if p.baseline != nil {
		p.baseline.Forget("traefik", name)
	}
	return map[string]interface{}{}, nil
}

// ============ 内部 ============

func (p *TraefikProvider) sitePath(name string) string {
	return filepath.Join(p.dir, "cockpit-site-"+name+".yml")
}

// loadSites 扫描动态目录的 cockpit 片段，解析每个文件头部的 meta；损坏条目跳过
func (p *TraefikProvider) loadSites() ([]ProxySite, error) {
	matches, err := filepath.Glob(filepath.Join(p.dir, "cockpit-site-*.yml"))
	if err != nil {
		return nil, err
	}
	sites := make([]ProxySite, 0, len(matches))
	for _, path := range matches {
		base := filepath.Base(path)
		if traefikFileRe.FindStringSubmatch(base) == nil {
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

// ============ 渲染（D14 映射）============

type traefikDynamicConfig struct {
	HTTP *traefikHTTP `yaml:"http,omitempty"`
	TLS  *traefikTLS  `yaml:"tls,omitempty"`
}

type traefikHTTP struct {
	Routers     map[string]traefikRouter     `yaml:"routers"`
	Middlewares map[string]traefikMiddleware `yaml:"middlewares,omitempty"`
	Services    map[string]traefikService    `yaml:"services"`
}

type traefikRouter struct {
	Rule        string    `yaml:"rule"`
	EntryPoints []string  `yaml:"entryPoints,omitempty"`
	Middlewares []string  `yaml:"middlewares,omitempty"`
	Service     string    `yaml:"service"`
	TLS         *struct{} `yaml:"tls,omitempty"`
}

type traefikRedirectScheme struct {
	Scheme    string `yaml:"scheme"`
	Permanent bool   `yaml:"permanent"`
}

type traefikMiddleware struct {
	RedirectScheme traefikRedirectScheme `yaml:"redirectScheme"`
}

type traefikServer struct {
	URL string `yaml:"url"`
}

type traefikService struct {
	LoadBalancer struct {
		Servers []traefikServer `yaml:"servers"`
	} `yaml:"loadBalancer"`
}

type traefikTLS struct {
	Certificates []traefikCertificate `yaml:"certificates"`
}

type traefikCertificate struct {
	CertFile string `yaml:"certFile"`
	KeyFile  string `yaml:"keyFile"`
}

// renderTraefikSite 渲染片段：首行 meta 注释 + 动态配置 YAML。
// router/service/middleware 命名 cockpit-<site>——冲突域在 Traefik 全局
// 命名空间，加前缀避免与用户动态文件撞名。websocket 字段 no-op
// （Traefik 原生透传 WS，字段保留兼容面板）。
func renderTraefikSite(s *ProxySite) []byte {
	meta, _ := json.Marshal(s)
	hostRule := fmt.Sprintf("Host(`%s`)", s.ServerNames[0])
	for _, d := range s.ServerNames[1:] {
		hostRule += fmt.Sprintf(" || Host(`%s`)", d)
	}

	cfg := traefikDynamicConfig{HTTP: &traefikHTTP{
		Routers:  map[string]traefikRouter{},
		Services: map[string]traefikService{},
	}}
	svc := cfg.HTTP.Services["cockpit-"+s.Name]
	svc.LoadBalancer.Servers = []traefikServer{{URL: "http://" + s.Upstream}}
	cfg.HTTP.Services["cockpit-"+s.Name] = svc

	if s.Scheme == "https" {
		// 80 router：同 rule 挂 redirectScheme 永久跳转（不碰静态配置）
		cfg.HTTP.Routers["cockpit-"+s.Name+"-web"] = traefikRouter{
			Rule:        hostRule,
			EntryPoints: []string{"web"},
			Middlewares: []string{"cockpit-" + s.Name + "-redirect"},
			Service:     "cockpit-" + s.Name,
		}
		cfg.HTTP.Middlewares = map[string]traefikMiddleware{
			"cockpit-" + s.Name + "-redirect": {
				RedirectScheme: traefikRedirectScheme{Scheme: "https", Permanent: true},
			},
		}
		// 443 router：tls 开启；证书文件引用声明在同一文件的顶层 tls 段
		cfg.HTTP.Routers["cockpit-"+s.Name+"-websecure"] = traefikRouter{
			Rule:        hostRule,
			EntryPoints: []string{"websecure"},
			Service:     "cockpit-" + s.Name,
			TLS:         &struct{}{},
		}
		cfg.TLS = &traefikTLS{Certificates: []traefikCertificate{
			{CertFile: s.TLSCert, KeyFile: s.TLSKey},
		}}
	} else {
		cfg.HTTP.Routers["cockpit-"+s.Name] = traefikRouter{
			Rule:        hostRule,
			EntryPoints: []string{"web"},
			Service:     "cockpit-" + s.Name,
		}
	}

	// yaml.Marshal 对纯结构体恒成功
	b, _ := yaml.Marshal(&cfg)
	return append([]byte(nginxMetaPrefix+string(meta)+"\n"), b...)
}

// traefikSelfCheck 可注入（测试覆盖渲染后自检失败的防御分支）
var traefikSelfCheck = checkTraefikYAML

// checkTraefikYAML 自检（D13）：剥离 meta 行后必须解析为合法动态配置，
// 且每个 router 引用的 service 存在
func checkTraefikYAML(content []byte) error {
	text := string(content)
	if idx := strings.Index(text, "\n"); idx >= 0 {
		text = text[idx+1:]
	}
	var cfg traefikDynamicConfig
	if err := yaml.Unmarshal([]byte(text), &cfg); err != nil {
		return err
	}
	if cfg.HTTP == nil {
		return fmt.Errorf("no http section")
	}
	if len(cfg.HTTP.Routers) == 0 || len(cfg.HTTP.Services) == 0 {
		return fmt.Errorf("no routers or services")
	}
	for name, r := range cfg.HTTP.Routers {
		if _, ok := cfg.HTTP.Services[r.Service]; !ok {
			return fmt.Errorf("router %s references missing service %s", name, r.Service)
		}
	}
	return nil
}
