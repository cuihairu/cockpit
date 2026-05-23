package rpc

import (
	"fmt"
	"log"
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
	rpcPayload, ok := msg.Payload["method"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid method")
	}

	method := rpcPayload
	params, _ := msg.Payload["params"].(map[string]interface{})

	// 解析方法格式: <provider>.<action>
	// 例如: pve.list_vms, docker.list_containers
	providerType, action, err := parseMethod(method)
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
	result, err := p.Call(action, params)
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

func parseMethod(method string) (string, string, error) {
	// 支持格式:
	// - "pve.list" -> provider=pve, action=list
	// - "docker.containers.list" -> provider=docker, action=containers.list
	// - "status" -> provider=system, action=status (默认)

	parts := splitMethod(method)
	if len(parts) == 1 {
		return "system", parts[0], nil
	}

	providerType := parts[0]
	action := joinMethod(parts[1:])

	return providerType, action, nil
}

func splitMethod(s string) []string {
	var parts []string
	var current string
	for _, ch := range s {
		if ch == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

func joinMethod(parts []string) string {
	result := ""
	for i, part := range parts {
		if i > 0 {
			result += "."
		}
		result += part
	}
	return result
}

// ============ System Provider (内置) ============

type SystemProvider struct{}

func (p *SystemProvider) Type() string { return "system" }

func (p *SystemProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "status":
		return p.Status(params)
	case "info":
		return p.Info(params)
	case "version":
		return map[string]interface{}{
			"version": "0.1.0",
			"build":   "dev",
		}, nil
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (p *SystemProvider) Status(params map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{
		"status": "ok",
		"uptime": "TODO",
	}, nil
}

func (p *SystemProvider) Info(params map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{
		"capabilities": []string{"pve", "docker", "openwrt"},
		"version":      "0.1.0",
	}, nil
}

// ============ PVE Provider ============

type PVEProvider struct {
	client *PVEClient
}

func NewPVEClient(endpoint, token string) *PVEClient {
	return &PVEClient{
		Endpoint: endpoint,
		Token:    token,
	}
}

type PVEClient struct {
	Endpoint string
	Token    string
}

func (p *PVEProvider) Type() string { return "pve" }

func (p *PVEProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "vms.list":
		return p.ListVMs(params)
	case "vms.get":
		return p.GetVM(params)
	case "vms.start":
		return p.StartVM(params)
	case "vms.stop":
		return p.StopVM(params)
	case "vms.restart":
		return p.RestartVM(params)
	case "containers.list":
		return p.ListContainers(params)
	case "nodes.list":
		return p.ListNodes(params)
	default:
		return nil, fmt.Errorf("unknown pve action: %s", action)
	}
}

func (p *PVEProvider) ListVMs(params map[string]interface{}) (interface{}, error) {
	// TODO: 调用 PVE API
	return []interface{}{}, nil
}

func (p *PVEProvider) GetVM(params map[string]interface{}) (interface{}, error) {
	vmID, ok := params["vmid"].(string)
	if !ok {
		return nil, fmt.Errorf("vmid required")
	}
	// TODO: 调用 PVE API
	return map[string]interface{}{
		"vmid": vmID,
		"name": "example-vm",
		"status": "running",
	}, nil
}

func (p *PVEProvider) StartVM(params map[string]interface{}) (interface{}, error) {
	// TODO: 调用 PVE API
	return map[string]interface{}{"status": "started"}, nil
}

func (p *PVEProvider) StopVM(params map[string]interface{}) (interface{}, error) {
	// TODO: 调用 PVE API
	return map[string]interface{}{"status": "stopped"}, nil
}

func (p *PVEProvider) RestartVM(params map[string]interface{}) (interface{}, error) {
	// TODO: 调用 PVE API
	return map[string]interface{}{"status": "restarted"}, nil
}

func (p *PVEProvider) ListContainers(params map[string]interface{}) (interface{}, error) {
	// TODO: 调用 PVE API
	return []interface{}{}, nil
}

func (p *PVEProvider) ListNodes(params map[string]interface{}) (interface{}, error) {
	// TODO: 调用 PVE API
	return []interface{}{}, nil
}

// ============ Docker Provider ============

type DockerProvider struct {
	socket string
}

func NewDockerProvider(socket string) *DockerProvider {
	return &DockerProvider{socket: socket}
}

func (p *DockerProvider) Type() string { return "docker" }

func (p *DockerProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "containers.list":
		return p.ListContainers(params)
	case "containers.get":
		return p.GetContainer(params)
	case "containers.start":
		return p.StartContainer(params)
	case "containers.stop":
		return p.StopContainer(params)
	case "containers.logs":
		return p.GetLogs(params)
	case "images.list":
		return p.ListImages(params)
	case "volumes.list":
		return p.ListVolumes(params)
	default:
		return nil, fmt.Errorf("unknown docker action: %s", action)
	}
}

func (p *DockerProvider) ListContainers(params map[string]interface{}) (interface{}, error) {
	// TODO: 调用 Docker API
	return []interface{}{}, nil
}

func (p *DockerProvider) GetContainer(params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id required")
	}
	return map[string]interface{}{
		"id":     id,
		"name":   "example-container",
		"image":  "nginx:latest",
		"status": "running",
	}, nil
}

func (p *DockerProvider) StartContainer(params map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"status": "started"}, nil
}

func (p *DockerProvider) StopContainer(params map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"status": "stopped"}, nil
}

func (p *DockerProvider) GetLogs(params map[string]interface{}) (interface{}, error) {
	// TODO: 调用 Docker API
	return "", nil
}

func (p *DockerProvider) ListImages(params map[string]interface{}) (interface{}, error) {
	// TODO: 调用 Docker API
	return []interface{}{}, nil
}

func (p *DockerProvider) ListVolumes(params map[string]interface{}) (interface{}, error) {
	// TODO: 调用 Docker API
	return []interface{}{}, nil
}

// ============ OpenWrt Provider ============

type OpenWrtProvider struct {
	host     string
	port     int
	user     string
	password string
}

func NewOpenWrtProvider(host string, port int, user, password string) *OpenWrtProvider {
	return &OpenWrtProvider{
		host:     host,
		port:     port,
		user:     user,
		password: password,
	}
}

func (p *OpenWrtProvider) Type() string { return "openwrt" }

func (p *OpenWrtProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "status":
		return p.GetStatus(params)
	case "interfaces.list":
		return p.ListInterfaces(params)
	case "routes.get":
		return p.GetRoutes(params)
	case "firewall.get":
		return p.GetFirewallRules(params)
	default:
		return nil, fmt.Errorf("unknown openwrt action: %s", action)
	}
}

func (p *OpenWrtProvider) GetStatus(params map[string]interface{}) (interface{}, error) {
	// TODO: 调用 OpenWrt RPC API
	return map[string]interface{}{
		"uptime": "1h23m45s",
		"load":   []float64{0.1, 0.2, 0.15},
		"memory": map[string]interface{}{
			"total": 256000000,
			"free":  128000000,
		},
	}, nil
}

func (p *OpenWrtProvider) ListInterfaces(params map[string]interface{}) (interface{}, error) {
	// TODO: 调用 OpenWrt RPC API
	return []interface{}{
		map[string]interface{}{
			"name":  "br-lan",
			"type":  "bridge",
			"up":    true,
			"ipv4":  "192.168.1.1",
		},
	}, nil
}

func (p *OpenWrtProvider) GetRoutes(params map[string]interface{}) (interface{}, error) {
	// TODO: 调用 OpenWrt RPC API
	return []interface{}{}, nil
}

func (p *OpenWrtProvider) GetFirewallRules(params map[string]interface{}) (interface{}, error) {
	// TODO: 调用 OpenWrt RPC API
	return []interface{}{}, nil
}
