package rpc

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/cuihairu/cockpit/internal/agent/provider"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// Handler RPC 处理器
type Handler struct {
	mu        sync.RWMutex
	providers map[string]provider.Provider // provider type -> provider instance
}

// NewHandler 创建 RPC 处理器
func NewHandler() *Handler {
	return &Handler{
		providers: make(map[string]provider.Provider),
	}
}

// RegisterProvider 注册 Provider
func (h *Handler) RegisterProvider(p provider.Provider) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.providers[p.Type()] = p
	log.Printf("Registered provider: %s", p.Type())
}

// Handle 处理 RPC 请求
func (h *Handler) Handle(msg *protocol.Message) (*protocol.Message, error) {
	req, err := protocol.DecodeRPCRequest(msg)
	if err != nil {
		return nil, fmt.Errorf("invalid rpc request: %w", err)
	}
	if req.Method == "" {
		return nil, fmt.Errorf("invalid method")
	}

	// 解析方法格式: <provider>.<action>
	// 例如: pve.list_vms, docker.list_containers
	providerType, action, err := parseMethod(req.Method)
	if err != nil {
		return nil, err
	}

	h.mu.RLock()
	p, ok := h.providers[providerType]
	h.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", providerType)
	}

	// 调用 Provider
	result, err := p.Call(action, req.Params)
	if err != nil {
		return nil, err
	}

	resp := protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]interface{}{
		"status": "success",
		"data":   result,
	})
	resp.ID = msg.ID // 关联请求 ID
	return resp, nil
}

// parseMethod 解析 RPC 方法字符串
//
// 支持格式：
//   - "pve.list" -> provider=pve, action=list
//   - "docker.containers.list" -> provider=docker, action=containers.list
//   - "status" -> provider=system, action=status (默认)
func parseMethod(method string) (string, string, error) {
	parts := splitMethod(method)
	if len(parts) == 1 {
		return "system", parts[0], nil
	}

	providerType := parts[0]
	action := joinMethod(parts[1:])

	return providerType, action, nil
}

// splitMethod 按点分隔方法字符串，忽略空段
func splitMethod(s string) []string {
	parts := strings.Split(s, ".")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// joinMethod 用点连接方法段
func joinMethod(parts []string) string {
	return strings.Join(parts, ".")
}
