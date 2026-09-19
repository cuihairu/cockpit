package rpc

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ============ Overlay 输出解析 ============
//
// 全部为纯函数，输入 CLI/admin API 原始输出，输出白名单构造的展示结构
// （D6）：wg dump 的私钥/预共享密钥列永不进入返回值。

// overlayPeer 对端节点（tools[].peers 与 interfaces[].peers 同构）
type overlayPeer struct {
	ID            string   `json:"id"`
	Name          string   `json:"name,omitempty"`
	VirtualIPs    []string `json:"virtualIps,omitempty"`
	Version       string   `json:"version,omitempty"`
	LatencyMs     int      `json:"latencyMs,omitempty"`
	Online        bool     `json:"online"`
	Endpoint      string   `json:"endpoint,omitempty"`
	Relay         string   `json:"relay,omitempty"`
	Role          string   `json:"role,omitempty"`
	LastHandshake string   `json:"lastHandshake,omitempty"` // RFC3339 UTC
}

// clipOutput 输出截断保护（按行边界，同 logs 的纪律）
func clipOutput(out []byte) []byte {
	if len(out) <= overlayMaxOutput {
		return out
	}
	cut := out[:overlayMaxOutput]
	if idx := strings.LastIndexByte(string(cut), '\n'); idx >= 0 {
		cut = cut[:idx+1]
	}
	return cut
}

// ============ ZeroTier ============

// parseZeroTierInfo 解析 zerotier-cli -j info
func parseZeroTierInfo(out []byte) (version string, online bool, err error) {
	var info struct {
		Version string `json:"version"`
		Online  bool   `json:"online"`
	}
	if err := json.Unmarshal(out, &info); err != nil {
		return "", false, fmt.Errorf("invalid zerotier info json: %w", err)
	}
	return info.Version, info.Online, nil
}

// parseZeroTierNetworks 解析 zerotier-cli -j listnetworks
func parseZeroTierNetworks(out []byte) []map[string]interface{} {
	var nets []struct {
		NWID   string   `json:"nwid"`
		Name   string   `json:"name"`
		Status string   `json:"status"`
		Type   string   `json:"type"`
		Dev    string   `json:"dev"`
		IPs    []string `json:"ips"`
	}
	if err := json.Unmarshal(out, &nets); err != nil {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(nets))
	for _, n := range nets {
		result = append(result, map[string]interface{}{
			"id":     n.NWID,
			"name":   n.Name,
			"status": n.Status,
			"type":   n.Type,
			"dev":    n.Dev,
			"ips":    n.IPs,
		})
	}
	return result
}

// parseZeroTierPeers 解析 zerotier-cli -j listpeers。
// online = 存在 active path；endpoint 取 preferred 且 active 的一条，
// 否则取任一 active。
func parseZeroTierPeers(out []byte) []overlayPeer {
	var peers []struct {
		Address string `json:"address"`
		Version string `json:"version"`
		Latency int    `json:"latency"`
		Role    string `json:"role"`
		Paths   []struct {
			Address   string `json:"address"`
			Active    bool   `json:"active"`
			Preferred bool   `json:"preferred"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(out, &peers); err != nil {
		return nil
	}
	result := make([]overlayPeer, 0, len(peers))
	for _, p := range peers {
		peer := overlayPeer{
			ID:      p.Address,
			Version: p.Version,
			Role:    p.Role,
		}
		if p.Latency > 0 {
			peer.LatencyMs = p.Latency
		}
		var activeEndpoint string
		for _, path := range p.Paths {
			if !path.Active {
				continue
			}
			peer.Online = true
			if path.Preferred || activeEndpoint == "" {
				activeEndpoint = path.Address
			}
		}
		peer.Endpoint = activeEndpoint
		result = append(result, peer)
	}
	return result
}

// ============ Tailscale ============

// parseTailscaleStatus 解析 tailscale status --json。
// Self 映射为一条 network，Peer map 展开为 peers。
func parseTailscaleStatus(out []byte) (version string, networks []map[string]interface{}, peers []overlayPeer, err error) {
	var st struct {
		Version      string `json:"Version"`
		BackendState string `json:"BackendState"`
		Self         struct {
			HostName     string   `json:"HostName"`
			TailscaleIPs []string `json:"TailscaleIPs"`
			Online       bool     `json:"Online"`
		} `json:"Self"`
		Peer map[string]struct {
			HostName      string    `json:"HostName"`
			DNSName       string    `json:"DNSName"`
			TailscaleIPs  []string  `json:"TailscaleIPs"`
			Online        bool      `json:"Online"`
			LastHandshake time.Time `json:"LastHandshake"`
			CurAddr       string    `json:"CurAddr"`
			Relay         string    `json:"Relay"`
		} `json:"Peer"`
	}
	if err := json.Unmarshal(out, &st); err != nil {
		return "", nil, nil, fmt.Errorf("invalid tailscale status json: %w", err)
	}

	networks = []map[string]interface{}{{
		"id":     "tailscale",
		"name":   st.Self.HostName,
		"status": st.BackendState,
		"online": st.Self.Online,
		"ips":    st.Self.TailscaleIPs,
	}}
	peers = make([]overlayPeer, 0, len(st.Peer))
	for _, p := range st.Peer {
		peer := overlayPeer{
			Name:       p.HostName,
			VirtualIPs: p.TailscaleIPs,
			Online:     p.Online,
			Endpoint:   p.CurAddr,
			Relay:      p.Relay,
		}
		if p.DNSName != "" {
			peer.ID = strings.TrimSuffix(p.DNSName, ".")
		} else {
			peer.ID = p.HostName
		}
		// zero time = 从未握手，不输出
		if !p.LastHandshake.IsZero() && p.LastHandshake.Unix() > 0 {
			peer.LastHandshake = p.LastHandshake.UTC().Format(time.RFC3339)
		}
		peers = append(peers, peer)
	}
	return st.Version, networks, peers, nil
}

// ============ WireGuard ============

// wgHandshakeTime 最近握手被视为在线的窗口
var wgHandshakeOnline = 3 * time.Minute

// parseWGDump 解析 wg show all dump。
//
// dump 行格式（tab 分隔，首列为 interface 名）：
//
//	interface 行: <iface> <private-key> <listen-port> <fwmark>
//	peer 行:      <iface> <public-key> <preshared-key> <endpoint>
//	              <allowed-ips> <latest-handshake> <rx> <tx> <keepalive>
//
// 私钥与预共享密钥列（各行的敏感列）不进入返回值：interface 行只取
// listen-port（字段数 >= 9 视为 peer 行，其余按 interface 行处理）。
func parseWGDump(out []byte, now time.Time) []map[string]interface{} {
	type wgIface struct {
		name   string
		listen string
		peers  []overlayPeer
	}
	ifaces := map[string]*wgIface{}
	var order []string

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}
		ifaceName := fields[0]
		iface, ok := ifaces[ifaceName]
		if !ok {
			iface = &wgIface{name: ifaceName}
			ifaces[ifaceName] = iface
			order = append(order, ifaceName)
		}
		if len(fields) >= 9 {
			// peer 行：白名单取 [1]pubkey [3]endpoint [4]allowed [5]handshake [6]rx [7]tx
			peer := overlayPeer{ID: fields[1], Endpoint: fields[3]}
			if ips := fields[4]; ips != "" && ips != "(none)" {
				peer.VirtualIPs = strings.Split(ips, ",")
			}
			if hs, err := strconv.ParseInt(fields[5], 10, 64); err == nil && hs > 0 {
				t := time.Unix(hs, 0)
				peer.LastHandshake = t.UTC().Format(time.RFC3339)
				peer.Online = now.Sub(t) < wgHandshakeOnline
			}
			iface.peers = append(iface.peers, peer)
			continue
		}
		// interface 行：只取 listen-port（fields[2]），private-key（fields[1]）丢弃
		if len(fields) >= 3 {
			iface.listen = fields[2]
		}
	}

	result := make([]map[string]interface{}, 0, len(order))
	for _, name := range order {
		ifc := ifaces[name]
		peers := ifc.peers
		if len(peers) > overlayMaxPeers {
			peers = peers[:overlayMaxPeers]
		}
		result = append(result, map[string]interface{}{
			"name":       ifc.name,
			"listenPort": ifc.listen,
			"peerCount":  len(ifc.peers),
			"peers":      peers,
		})
	}
	return result
}
