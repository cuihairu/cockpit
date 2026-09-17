package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ============ Overlay Provider ============
//
// 组网工具运行态只读观测（见 docs/guide/overlay-design.md）：
// ZeroTier / Tailscale / WireGuard 走本机 CLI，frp 走版本 + 进程 + 可选
// admin API（COCKPIT_FRPC_ADMIN / COCKPIT_FRPS_ADMIN，host:port）。
// 输出全部白名单构造（D6），单工具失败不影响其余（D5）。

const (
	// overlayCmdTimeout 单条 CLI 命令超时（D10）
	overlayCmdTimeout = 5 * time.Second
	// overlayMaxPeers 每工具 peers 输出上限（D5）
	overlayMaxPeers = 200
	// overlayMaxOutput 单命令输出上限
	overlayMaxOutput = 1 << 20
	// overlayAdminTimeout frp admin API 超时
	overlayAdminTimeout = 3 * time.Second
)

// overlay CLI 命令名做成变量仅为测试可注入
var (
	overlayZTCliBin     = "zerotier-cli"
	overlayTailscaleBin = "tailscale"
	overlayWGBin        = "wg"
	overlayFrpcBin      = "frpc"
	overlayFrpsBin      = "frps"
	overlayPgrepBin     = "pgrep"
)

// OverlayProvider 组网工具观测
type OverlayProvider struct {
	run Commander
}

// NewOverlayProvider 创建 provider；run 为 nil 时使用真实命令执行
func NewOverlayProvider(run Commander) *OverlayProvider {
	if run == nil {
		run = defaultCommander
	}
	return &OverlayProvider{run: run}
}

// Type RPC provider 类型
func (p *OverlayProvider) Type() string { return "overlay" }

// Call RPC 分发
func (p *OverlayProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "status":
		return p.Status()
	default:
		return nil, fmt.Errorf("unknown overlay action: %s", action)
	}
}

// overlayTool 单工具观测快照（设计文档数据模型）
type overlayTool struct {
	Tool       string                   `json:"tool"`
	Status     string                   `json:"status"` // ok | degraded | error | unavailable
	Version    string                   `json:"version,omitempty"`
	Error      string                   `json:"error,omitempty"`
	Networks   []map[string]interface{} `json:"networks,omitempty"`
	Peers      []map[string]interface{} `json:"peers,omitempty"`
	Interfaces []map[string]interface{} `json:"interfaces,omitempty"`
	Extra      map[string]interface{}   `json:"extra,omitempty"`
}

// Status 一次返回全部工具快照
func (p *OverlayProvider) Status() (interface{}, error) {
	tools := []overlayTool{
		*p.readZeroTier(),
		*p.readTailscale(),
		*p.readWireGuard(),
		*p.readFRP(),
	}
	return map[string]interface{}{"tools": tools}, nil
}

// toolCtx 单命令执行上下文
func toolCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), overlayCmdTimeout)
}

// degrade 将工具标记为 error 并附摘要
func degrade(t *overlayTool, what string, err error) *overlayTool {
	t.Status, t.Error = "error", what+": "+err.Error()
	return t
}

// ============ ZeroTier ============

// readZeroTier zerotier-cli -j info / listnetworks / listpeers。
// 版本与 daemon 在线态取自 info（无需单独 -v，且天然校验 daemon 可达）。
func (p *OverlayProvider) readZeroTier() *overlayTool {
	t := &overlayTool{Tool: "zerotier", Status: "ok"}

	ctx, cancel := toolCtx()
	defer cancel()
	out, _, err := p.run(ctx, overlayZTCliBin, "-j", "info")
	if err != nil {
		return toolUnavailable(t, err)
	}
	version, online, err := parseZeroTierInfo(clipOutput(out))
	if err != nil {
		return degrade(t, "parse info", err)
	}
	t.Version = version
	if !online {
		t.Status, t.Error = "degraded", "zerotier-core not online"
	}

	ctx2, cancel2 := toolCtx()
	defer cancel2()
	if out, _, err = p.run(ctx2, overlayZTCliBin, "-j", "listnetworks"); err == nil {
		t.Networks = clipPeers(parseZeroTierNetworks(clipOutput(out)), overlayMaxPeers)
	}

	ctx3, cancel3 := toolCtx()
	defer cancel3()
	if out, _, err = p.run(ctx3, overlayZTCliBin, "-j", "listpeers"); err != nil {
		if t.Status == "ok" {
			return degrade(t, "listpeers", err)
		}
		return t
	}
	peers := parseZeroTierPeers(clipOutput(out))
	if len(peers) > overlayMaxPeers {
		t.Error = strings.TrimSpace(t.Error + " peers truncated to " + itoa(overlayMaxPeers))
		peers = peers[:overlayMaxPeers]
	}
	t.Peers = toIfacePeers(peers)
	return t
}

// ============ Tailscale ============

// readTailscale tailscale status --json --peers（旧版不认 --peers 时回退）。
// 版本/本机网络取自同一份输出，Self 映射为一条 network。
func (p *OverlayProvider) readTailscale() *overlayTool {
	t := &overlayTool{Tool: "tailscale", Status: "ok"}

	ctx, cancel := toolCtx()
	defer cancel()
	out, _, err := p.run(ctx, overlayTailscaleBin, "status", "--json", "--peers")
	if err != nil {
		if !errors.Is(err, exec.ErrNotFound) {
			// 旧版不认 --peers，回退基础形式再试一次
			ctx2, cancel2 := toolCtx()
			defer cancel2()
			out, _, err = p.run(ctx2, overlayTailscaleBin, "status", "--json")
		}
	}
	if err != nil {
		return toolUnavailable(t, err)
	}
	version, networks, peers, err := parseTailscaleStatus(clipOutput(out))
	if err != nil {
		return degrade(t, "parse status", err)
	}
	t.Version = version
	t.Networks = networks
	if len(peers) > overlayMaxPeers {
		t.Error = "peers truncated to " + itoa(overlayMaxPeers)
		peers = peers[:overlayMaxPeers]
	}
	t.Peers = toIfacePeers(peers)
	return t
}

// ============ WireGuard ============

// readWireGuard wg show all dump。接口与 peer 全部来自同一份 dump。
func (p *OverlayProvider) readWireGuard() *overlayTool {
	t := &overlayTool{Tool: "wireguard", Status: "ok"}

	ctx, cancel := toolCtx()
	defer cancel()
	out, _, err := p.run(ctx, overlayWGBin, "--version")
	if err != nil {
		return toolUnavailable(t, err)
	}
	// wireguard-tools 输出形如 "wireguard-tools v1.0.2020511"（Windows）或
	// "wireguard-tools v1.0.2"（Linux），去掉前缀保留版本
	if v := strings.TrimSpace(string(out)); v != "" {
		t.Version = strings.TrimPrefix(v, "wireguard-tools ")
	}

	ctx2, cancel2 := toolCtx()
	defer cancel2()
	out, _, err = p.run(ctx2, overlayWGBin, "show", "all", "dump")
	if err != nil {
		return degrade(t, "wg show", err)
	}
	t.Interfaces = parseWGDump(clipOutput(out), time.Now())
	return t
}

// ============ frp ============

// readFRP frpc/frps 版本 + 进程存活 + 可选 admin API（D7）。
// 零配置即有基本观测；配置 COCKPIT_FRPC_ADMIN / COCKPIT_FRPS_ADMIN 后
// 追加隧道级状态。
func (p *OverlayProvider) readFRP() *overlayTool {
	t := &overlayTool{Tool: "frp", Status: "ok"}
	extra := map[string]interface{}{}

	ctx, cancel := toolCtx()
	defer cancel()
	out, _, frpcErr := p.run(ctx, overlayFrpcBin, "-v")
	if frpcErr != nil {
		// frpc 不可用，看 frps；两者都缺失时以 frpc 的错误分流 unavailable/error
		ctxF, cancelF := toolCtx()
		defer cancelF()
		out2, _, frpsErr := p.run(ctxF, overlayFrpsBin, "-v")
		if frpsErr != nil {
			return toolUnavailable(t, frpcErr)
		}
		t.Version = strings.TrimSpace(string(out2))
	} else {
		t.Version = strings.TrimSpace(string(out))
	}

	extra["frpc"] = map[string]interface{}{
		"running": p.processRunning(overlayFrpcBin),
	}
	extra["frps"] = map[string]interface{}{
		"running": p.processRunning(overlayFrpsBin),
	}

	if admin := os.Getenv("COCKPIT_FRPC_ADMIN"); admin != "" {
		tunnels, err := fetchFRPAdminTunnelCount(admin)
		if err != nil {
			t.Status = "degraded"
			extra["frpc"].(map[string]interface{})["adminError"] = err.Error()
		} else {
			extra["frpc"].(map[string]interface{})["tunnels"] = tunnels
		}
	}
	if admin := os.Getenv("COCKPIT_FRPS_ADMIN"); admin != "" {
		proxies, err := fetchFRPAdminTunnelCount(admin)
		if err != nil {
			t.Status = "degraded"
			extra["frps"].(map[string]interface{})["adminError"] = err.Error()
		} else {
			extra["frps"].(map[string]interface{})["proxies"] = proxies
		}
	}

	t.Extra = extra
	return t
}

// processRunning pgrep -x 进程存活检查；pgrep 不存在或无匹配均视为未运行
func (p *OverlayProvider) processRunning(name string) bool {
	ctx, cancel := toolCtx()
	defer cancel()
	_, _, err := p.run(ctx, overlayPgrepBin, "-x", name)
	return err == nil
}

// fetchFRPAdminTunnelCount 从 frp admin API /api/status 统计隧道/代理条数。
// 各版本返回结构有差异（顶层按类型分组的列表），此处只数行不透传原始结构。
func fetchFRPAdminTunnelCount(addr string) (int, error) {
	client := &http.Client{Timeout: overlayAdminTimeout}
	url := "http://" + strings.TrimPrefix(strings.TrimPrefix(addr, "http://"), "https://") + "/api/status"
	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, overlayMaxOutput))
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("admin api status %d", resp.StatusCode)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		return 0, fmt.Errorf("invalid admin api json: %w", err)
	}
	count := 0
	for _, raw := range doc {
		var rows []json.RawMessage
		if json.Unmarshal(raw, &rows) == nil {
			count += len(rows)
		}
	}
	return count, nil
}

// ============ 小工具 ============

// toolUnavailable 命令不存在 → unavailable（其余 → error）
func toolUnavailable(t *overlayTool, err error) *overlayTool {
	if errors.Is(err, exec.ErrNotFound) || isNotFoundErr(err) {
		t.Status = "unavailable"
		return t
	}
	return degrade(t, "execute", err)
}

// isNotFoundErr 判定注入 Commander 返回的 exec.Error（exec.LookPath 失败形态）
func isNotFoundErr(err error) bool {
	var execErr *exec.Error
	return errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound)
}

// toIfacePeers overlayPeer 切片 → 通用 map 切片（wireguard interfaces peers 复用结构）
func toIfacePeers(peers []overlayPeer) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(peers))
	for _, p := range peers {
		m := map[string]interface{}{
			"id":     p.ID,
			"online": p.Online,
		}
		if p.Name != "" {
			m["name"] = p.Name
		}
		if len(p.VirtualIPs) > 0 {
			m["virtualIps"] = p.VirtualIPs
		}
		if p.Version != "" {
			m["version"] = p.Version
		}
		if p.LatencyMs > 0 {
			m["latencyMs"] = p.LatencyMs
		}
		if p.Endpoint != "" {
			m["endpoint"] = p.Endpoint
		}
		if p.Relay != "" {
			m["relay"] = p.Relay
		}
		if p.Role != "" {
			m["role"] = p.Role
		}
		if p.LastHandshake != "" {
			m["lastHandshake"] = p.LastHandshake
		}
		out = append(out, m)
	}
	return out
}

// clipPeers 截断 map 型 peers 列表
func clipPeers(peers []map[string]interface{}, max int) []map[string]interface{} {
	if len(peers) > max {
		return peers[:max]
	}
	return peers
}

// itoa 小整数转字符串（避免仅为一处引入 strconv）
func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
