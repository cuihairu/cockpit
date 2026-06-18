package agent

import (
	"log"
	"os"
	"strconv"

	"github.com/cuihairu/cockpit/internal/agent/rpc"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// setupProviders 根据检测到的 capabilities 注册 RPC Provider。
//
// 注册规则：
//   - SystemProvider：始终注册（内置基础能力）
//   - DockerProvider：检测到 "docker-api" capability 时注册，host 取自 Capability.Endpoint
//   - PVEProvider：检测到 "pve-api" capability 且环境变量 PVE_TOKEN_ID / PVE_TOKEN_SECRET 同时存在时注册
//   - OpenWrtProvider：检测到 "openwrt" capability 且 OPENWRT_HOST/OPENWRT_USER/OPENWRT_PASS 存在时注册
//
// 单个 Provider 初始化失败仅记录日志，不影响 Agent 基础心跳。
func (a *Agent) setupProviders() {
	if a.rpc == nil {
		return
	}

	// 1. SystemProvider 始终注册
	a.rpc.RegisterProvider(rpc.NewSystemProvider())

	// 2. 按检测到的能力注册
	for _, cap := range a.capabilities {
		switch cap.Type {
		case "docker-api":
			a.registerDockerProvider(cap)
		case "pve-api":
			a.registerPVEProvider(cap)
		case "openwrt":
			a.registerOpenWrtProvider(cap)
		}
	}
}

// registerDockerProvider 注册 Docker Provider
func (a *Agent) registerDockerProvider(cap protocol.Capability) {
	host := cap.Endpoint
	if host == "" {
		host = os.Getenv("DOCKER_HOST")
	}
	if host == "" {
		log.Printf("Skip docker provider: no endpoint detected")
		return
	}

	p, err := rpc.NewDockerProvider(host)
	if err != nil {
		log.Printf("Failed to create docker provider: %v", err)
		return
	}
	a.rpc.RegisterProvider(p)
}

// registerPVEProvider 注册 PVE Provider
//
// PVE API 需要 token 认证；endpoint 在 capability 中，但 token 必须从环境变量读取
// （capability payload 不应包含敏感凭据）。
func (a *Agent) registerPVEProvider(cap protocol.Capability) {
	endpoint := cap.Endpoint
	if endpoint == "" {
		endpoint = os.Getenv("PVE_URL")
	}
	tokenID := os.Getenv("PVE_TOKEN_ID")
	tokenSecret := os.Getenv("PVE_TOKEN_SECRET")

	if endpoint == "" || tokenID == "" || tokenSecret == "" {
		log.Printf("Skip pve provider: missing endpoint or token (PVE_URL/PVE_TOKEN_ID/PVE_TOKEN_SECRET)")
		return
	}

	a.rpc.RegisterProvider(rpc.NewPVEProvider(endpoint, tokenID, tokenSecret))
}

// registerOpenWrtProvider 注册 OpenWrt Provider
//
// OpenWrt detector 只检测本地特征，但 HTTP API 访问需要凭据。
// 凭据来源：OPENWRT_HOST / OPENWRT_PORT / OPENWRT_USER / OPENWRT_PASS 环境变量。
func (a *Agent) registerOpenWrtProvider(cap protocol.Capability) {
	host := os.Getenv("OPENWRT_HOST")
	user := os.Getenv("OPENWRT_USER")
	pass := os.Getenv("OPENWRT_PASS")
	if host == "" || user == "" || pass == "" {
		log.Printf("Skip openwrt provider: missing OPENWRT_HOST/OPENWRT_USER/OPENWRT_PASS")
		return
	}

	port := 443
	if pStr := os.Getenv("OPENWRT_PORT"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			port = p
		}
	}

	a.rpc.RegisterProvider(rpc.NewOpenWrtProvider(host, port, user, pass))
}
