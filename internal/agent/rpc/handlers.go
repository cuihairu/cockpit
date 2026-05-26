package rpc

import (
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/agent/provider"
	"github.com/cuihairu/cockpit/internal/docker"
	"github.com/cuihairu/cockpit/internal/openwrt"
	"github.com/cuihairu/cockpit/internal/pve"
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

type SystemProvider struct {
	startTime time.Time
}

func NewSystemProvider() *SystemProvider {
	return &SystemProvider{
		startTime: time.Now(),
	}
}

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
	// 计算 Agent 运行时间
	uptime := time.Since(p.startTime)

	return map[string]interface{}{
		"status":     "ok",
		"uptime":     uptime.String(),
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
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
	client *pve.Client
}

// NewPVEProvider 创建 PVE Provider
func NewPVEProvider(endpoint, tokenID, tokenSecret string) *PVEProvider {
	cfg := pve.Config{
		Endpoint:    endpoint,
		TokenID:     tokenID,
		TokenSecret: tokenSecret,
		InsecureTLS: true, // 默认允许自签名证书
	}
	return &PVEProvider{
		client: pve.NewClient(cfg),
	}
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
	case "vms.suspend":
		return p.SuspendVM(params)
	case "vms.resume":
		return p.ResumeVM(params)
	case "containers.list":
		return p.ListContainers(params)
	case "containers.get":
		return p.GetContainer(params)
	case "containers.start":
		return p.StartContainer(params)
	case "containers.stop":
		return p.StopContainer(params)
	case "containers.restart":
		return p.RestartContainer(params)
	case "nodes.list":
		return p.ListNodes(params)
	case "storage.list":
		return p.ListStorage(params)
	case "snapshots.list":
		return p.ListSnapshots(params)
	case "snapshots.create":
		return p.CreateSnapshot(params)
	case "snapshots.delete":
		return p.DeleteSnapshot(params)
	default:
		return nil, fmt.Errorf("unknown pve action: %s", action)
	}
}

func (p *PVEProvider) getNode(params map[string]interface{}) string {
	if node, ok := params["node"].(string); ok && node != "" {
		return node
	}
	return ""
}

func (p *PVEProvider) ListVMs(params map[string]interface{}) (interface{}, error) {
	node := p.getNode(params)
	vms, err := p.client.ListVMs(node)
	if err != nil {
		return nil, fmt.Errorf("list VMs: %w", err)
	}
	return vms, nil
}

func (p *PVEProvider) GetVM(params map[string]interface{}) (interface{}, error) {
	node := p.getNode(params)
	vmid, err := pve.GetVMID(params["vmid"])
	if err != nil {
		return nil, fmt.Errorf("invalid vmid: %w", err)
	}

	vm, err := p.client.GetVM(node, vmid)
	if err != nil {
		return nil, fmt.Errorf("get VM: %w", err)
	}
	return vm, nil
}

func (p *PVEProvider) StartVM(params map[string]interface{}) (interface{}, error) {
	node := p.getNode(params)
	vmid, err := pve.GetVMID(params["vmid"])
	if err != nil {
		return nil, fmt.Errorf("invalid vmid: %w", err)
	}

	if err := p.client.StartVM(node, vmid); err != nil {
		return nil, fmt.Errorf("start VM: %w", err)
	}
	return map[string]interface{}{"status": "started", "vmid": vmid}, nil
}

func (p *PVEProvider) StopVM(params map[string]interface{}) (interface{}, error) {
	node := p.getNode(params)
	vmid, err := pve.GetVMID(params["vmid"])
	if err != nil {
		return nil, fmt.Errorf("invalid vmid: %w", err)
	}

	if err := p.client.StopVM(node, vmid); err != nil {
		return nil, fmt.Errorf("stop VM: %w", err)
	}
	return map[string]interface{}{"status": "stopped", "vmid": vmid}, nil
}

func (p *PVEProvider) RestartVM(params map[string]interface{}) (interface{}, error) {
	node := p.getNode(params)
	vmid, err := pve.GetVMID(params["vmid"])
	if err != nil {
		return nil, fmt.Errorf("invalid vmid: %w", err)
	}

	if err := p.client.RestartVM(node, vmid); err != nil {
		return nil, fmt.Errorf("restart VM: %w", err)
	}
	return map[string]interface{}{"status": "restarted", "vmid": vmid}, nil
}

func (p *PVEProvider) SuspendVM(params map[string]interface{}) (interface{}, error) {
	node := p.getNode(params)
	vmid, err := pve.GetVMID(params["vmid"])
	if err != nil {
		return nil, fmt.Errorf("invalid vmid: %w", err)
	}

	if err := p.client.SuspendVM(node, vmid); err != nil {
		return nil, fmt.Errorf("suspend VM: %w", err)
	}
	return map[string]interface{}{"status": "suspended", "vmid": vmid}, nil
}

func (p *PVEProvider) ResumeVM(params map[string]interface{}) (interface{}, error) {
	node := p.getNode(params)
	vmid, err := pve.GetVMID(params["vmid"])
	if err != nil {
		return nil, fmt.Errorf("invalid vmid: %w", err)
	}

	if err := p.client.ResumeVM(node, vmid); err != nil {
		return nil, fmt.Errorf("resume VM: %w", err)
	}
	return map[string]interface{}{"status": "resumed", "vmid": vmid}, nil
}

func (p *PVEProvider) ListContainers(params map[string]interface{}) (interface{}, error) {
	node := p.getNode(params)
	containers, err := p.client.ListContainers(node)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	return containers, nil
}

func (p *PVEProvider) GetContainer(params map[string]interface{}) (interface{}, error) {
	node := p.getNode(params)
	vmid, err := pve.GetVMID(params["vmid"])
	if err != nil {
		return nil, fmt.Errorf("invalid vmid: %w", err)
	}

	container, err := p.client.GetContainer(node, vmid)
	if err != nil {
		return nil, fmt.Errorf("get container: %w", err)
	}
	return container, nil
}

func (p *PVEProvider) StartContainer(params map[string]interface{}) (interface{}, error) {
	node := p.getNode(params)
	vmid, err := pve.GetVMID(params["vmid"])
	if err != nil {
		return nil, fmt.Errorf("invalid vmid: %w", err)
	}

	if err := p.client.StartContainer(node, vmid); err != nil {
		return nil, fmt.Errorf("start container: %w", err)
	}
	return map[string]interface{}{"status": "started", "vmid": vmid}, nil
}

func (p *PVEProvider) StopContainer(params map[string]interface{}) (interface{}, error) {
	node := p.getNode(params)
	vmid, err := pve.GetVMID(params["vmid"])
	if err != nil {
		return nil, fmt.Errorf("invalid vmid: %w", err)
	}

	if err := p.client.StopContainer(node, vmid); err != nil {
		return nil, fmt.Errorf("stop container: %w", err)
	}
	return map[string]interface{}{"status": "stopped", "vmid": vmid}, nil
}

func (p *PVEProvider) RestartContainer(params map[string]interface{}) (interface{}, error) {
	node := p.getNode(params)
	vmid, err := pve.GetVMID(params["vmid"])
	if err != nil {
		return nil, fmt.Errorf("invalid vmid: %w", err)
	}

	if err := p.client.RestartContainer(node, vmid); err != nil {
		return nil, fmt.Errorf("restart container: %w", err)
	}
	return map[string]interface{}{"status": "restarted", "vmid": vmid}, nil
}

func (p *PVEProvider) ListNodes(params map[string]interface{}) (interface{}, error) {
	nodes, err := p.client.ListNodes()
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	return nodes, nil
}

func (p *PVEProvider) ListStorage(params map[string]interface{}) (interface{}, error) {
	node := p.getNode(params)
	storage, err := p.client.ListStorage(node)
	if err != nil {
		return nil, fmt.Errorf("list storage: %w", err)
	}
	return storage, nil
}

func (p *PVEProvider) ListSnapshots(params map[string]interface{}) (interface{}, error) {
	node := p.getNode(params)
	vmid, err := pve.GetVMID(params["vmid"])
	if err != nil {
		return nil, fmt.Errorf("invalid vmid: %w", err)
	}

	snapshots, err := p.client.ListVMSnapshots(node, vmid)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	return snapshots, nil
}

func (p *PVEProvider) CreateSnapshot(params map[string]interface{}) (interface{}, error) {
	node := p.getNode(params)
	vmid, err := pve.GetVMID(params["vmid"])
	if err != nil {
		return nil, fmt.Errorf("invalid vmid: %w", err)
	}

	name, _ := params["name"].(string)
	desc, _ := params["description"].(string)

	if err := p.client.CreateVMSnapshot(node, vmid, name, desc); err != nil {
		return nil, fmt.Errorf("create snapshot: %w", err)
	}
	return map[string]interface{}{"status": "created", "name": name}, nil
}

func (p *PVEProvider) DeleteSnapshot(params map[string]interface{}) (interface{}, error) {
	node := p.getNode(params)
	vmid, err := pve.GetVMID(params["vmid"])
	if err != nil {
		return nil, fmt.Errorf("invalid vmid: %w", err)
	}

	name, ok := params["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("snapshot name required")
	}

	if err := p.client.DeleteVMSnapshot(node, vmid, name); err != nil {
		return nil, fmt.Errorf("delete snapshot: %w", err)
	}
	return map[string]interface{}{"status": "deleted", "name": name}, nil
}

// ============ Docker Provider ============

type DockerProvider struct {
	client docker.DockerAPI
}

// NewDockerProvider 创建 Docker Provider
func NewDockerProvider(host string) (*DockerProvider, error) {
	cfg := docker.Config{
		Host:    host,
		Timeout: 30 * time.Second,
	}
	client, err := docker.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	return &DockerProvider{client: client}, nil
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
	case "containers.restart":
		return p.RestartContainer(params)
	case "containers.remove":
		return p.RemoveContainer(params)
	case "containers.pause":
		return p.PauseContainer(params)
	case "containers.unpause":
		return p.UnpauseContainer(params)
	case "containers.logs":
		return p.GetLogs(params)
	case "containers.stats":
		return p.GetStats(params)
	case "images.list":
		return p.ListImages(params)
	case "images.remove":
		return p.RemoveImage(params)
	case "images.pull":
		return p.PullImage(params)
	case "volumes.list":
		return p.ListVolumes(params)
	case "volumes.remove":
		return p.RemoveVolume(params)
	case "networks.list":
		return p.ListNetworks(params)
	case "system.info":
		return p.GetSystemInfo(params)
	default:
		return nil, fmt.Errorf("unknown docker action: %s", action)
	}
}

func (p *DockerProvider) ListContainers(params map[string]interface{}) (interface{}, error) {
	all, _ := params["all"].(bool)
	containers, err := p.client.ListContainers(all)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	return containers, nil
}

func (p *DockerProvider) GetContainer(params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id required")
	}
	container, err := p.client.GetContainer(id)
	if err != nil {
		return nil, fmt.Errorf("get container: %w", err)
	}
	return container, nil
}

func (p *DockerProvider) StartContainer(params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id required")
	}
	if err := p.client.StartContainer(id); err != nil {
		return nil, fmt.Errorf("start container: %w", err)
	}
	return map[string]interface{}{"status": "started", "id": id}, nil
}

func (p *DockerProvider) StopContainer(params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id required")
	}
	var timeout *int
	if t, ok := params["timeout"].(int); ok {
		timeout = &t
	}
	if err := p.client.StopContainer(id, timeout); err != nil {
		return nil, fmt.Errorf("stop container: %w", err)
	}
	return map[string]interface{}{"status": "stopped", "id": id}, nil
}

func (p *DockerProvider) RestartContainer(params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id required")
	}
	var timeout *int
	if t, ok := params["timeout"].(int); ok {
		timeout = &t
	}
	if err := p.client.RestartContainer(id, timeout); err != nil {
		return nil, fmt.Errorf("restart container: %w", err)
	}
	return map[string]interface{}{"status": "restarted", "id": id}, nil
}

func (p *DockerProvider) RemoveContainer(params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id required")
	}
	force, _ := params["force"].(bool)
	removeVolumes, _ := params["volumes"].(bool)
	if err := p.client.RemoveContainer(id, force, removeVolumes); err != nil {
		return nil, fmt.Errorf("remove container: %w", err)
	}
	return map[string]interface{}{"status": "removed", "id": id}, nil
}

func (p *DockerProvider) PauseContainer(params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id required")
	}
	if err := p.client.PauseContainer(id); err != nil {
		return nil, fmt.Errorf("pause container: %w", err)
	}
	return map[string]interface{}{"status": "paused", "id": id}, nil
}

func (p *DockerProvider) UnpauseContainer(params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id required")
	}
	if err := p.client.UnpauseContainer(id); err != nil {
		return nil, fmt.Errorf("unpause container: %w", err)
	}
	return map[string]interface{}{"status": "unpaused", "id": id}, nil
}

func (p *DockerProvider) GetLogs(params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id required")
	}
	tail, _ := params["tail"].(string)
	since, _ := params["since"].(string)
	follow, _ := params["follow"].(bool)
	timestamps, _ := params["timestamps"].(bool)
	stdout, _ := params["stdout"].(bool)
	stderr, _ := params["stderr"].(bool)

	logs, err := p.client.GetLogs(id, tail, since, follow, timestamps, stdout, stderr)
	if err != nil {
		return nil, fmt.Errorf("get logs: %w", err)
	}
	return logs, nil
}

func (p *DockerProvider) GetStats(params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id required")
	}
	stats, err := p.client.GetContainerStats(id)
	if err != nil {
		return nil, fmt.Errorf("get stats: %w", err)
	}
	return stats, nil
}

func (p *DockerProvider) ListImages(params map[string]interface{}) (interface{}, error) {
	all, _ := params["all"].(bool)
	images, err := p.client.ListImages(all)
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}
	return images, nil
}

func (p *DockerProvider) RemoveImage(params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id required")
	}
	force, _ := params["force"].(bool)
	pruneChildren, _ := params["prune_children"].(bool)
	deleted, err := p.client.RemoveImage(id, force, pruneChildren)
	if err != nil {
		return nil, fmt.Errorf("remove image: %w", err)
	}
	return map[string]interface{}{"status": "removed", "deleted": deleted}, nil
}

func (p *DockerProvider) PullImage(params map[string]interface{}) (interface{}, error) {
	ref, ok := params["ref"].(string)
	if !ok {
		return nil, fmt.Errorf("ref required")
	}
	imageID, err := p.client.PullImage(ref)
	if err != nil {
		return nil, fmt.Errorf("pull image: %w", err)
	}
	return map[string]interface{}{"status": "pulled", "id": imageID}, nil
}

func (p *DockerProvider) ListVolumes(params map[string]interface{}) (interface{}, error) {
	volumes, err := p.client.ListVolumes()
	if err != nil {
		return nil, fmt.Errorf("list volumes: %w", err)
	}
	return volumes, nil
}

func (p *DockerProvider) RemoveVolume(params map[string]interface{}) (interface{}, error) {
	name, ok := params["name"].(string)
	if !ok {
		return nil, fmt.Errorf("name required")
	}
	force, _ := params["force"].(bool)
	if err := p.client.RemoveVolume(name, force); err != nil {
		return nil, fmt.Errorf("remove volume: %w", err)
	}
	return map[string]interface{}{"status": "removed", "name": name}, nil
}

func (p *DockerProvider) ListNetworks(params map[string]interface{}) (interface{}, error) {
	networks, err := p.client.ListNetworks()
	if err != nil {
		return nil, fmt.Errorf("list networks: %w", err)
	}
	return networks, nil
}

func (p *DockerProvider) GetSystemInfo(params map[string]interface{}) (interface{}, error) {
	info, err := p.client.Info()
	if err != nil {
		return nil, fmt.Errorf("get system info: %w", err)
	}
	return info, nil
}

// ============ OpenWrt Provider ============

type OpenWrtProvider struct {
	client *openwrt.Client
}

// NewOpenWrtProvider 创建 OpenWrt Provider
func NewOpenWrtProvider(host string, port int, username, password string) *OpenWrtProvider {
	cfg := openwrt.Config{
		Host:        host,
		Port:        port,
		Username:    username,
		Password:    password,
		InsecureTLS: true,
	}
	return &OpenWrtProvider{
		client: openwrt.NewClient(cfg),
	}
}

func (p *OpenWrtProvider) Type() string { return "openwrt" }

func (p *OpenWrtProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "system.info":
		return p.GetSystemInfo(params)
	case "interfaces.list":
		return p.ListInterfaces(params)
	case "interfaces.get":
		return p.GetInterface(params)
	case "routes.get":
		return p.GetRoutes(params)
	case "firewall.zones":
		return p.GetFirewallZones(params)
	case "firewall.rules":
		return p.GetFirewallRules(params)
	case "firewall.redirects":
		return p.GetFirewallRedirects(params)
	case "wireless.status":
		return p.GetWirelessStatus(params)
	case "dhcp.leases":
		return p.GetDHCPLoads(params)
	case "file.read":
		return p.ReadFile(params)
	case "file.write":
		return p.WriteFile(params)
	case "reboot":
		return p.Reboot(params)
	case "led.get":
		return p.GetLEDState(params)
	case "led.set":
		return p.SetLEDState(params)
	default:
		return nil, fmt.Errorf("unknown openwrt action: %s", action)
	}
}

func (p *OpenWrtProvider) GetSystemInfo(params map[string]interface{}) (interface{}, error) {
	info, err := p.client.GetSystemInfo()
	if err != nil {
		return nil, fmt.Errorf("get system info: %w", err)
	}
	return info, nil
}

func (p *OpenWrtProvider) ListInterfaces(params map[string]interface{}) (interface{}, error) {
	interfaces, err := p.client.ListInterfaces()
	if err != nil {
		return nil, fmt.Errorf("list interfaces: %w", err)
	}
	return interfaces, nil
}

func (p *OpenWrtProvider) GetInterface(params map[string]interface{}) (interface{}, error) {
	name, ok := params["name"].(string)
	if !ok {
		return nil, fmt.Errorf("name required")
	}
	iface, err := p.client.GetInterface(name)
	if err != nil {
		return nil, fmt.Errorf("get interface: %w", err)
	}
	return iface, nil
}

func (p *OpenWrtProvider) GetRoutes(params map[string]interface{}) (interface{}, error) {
	routes, err := p.client.ListRoutes()
	if err != nil {
		return nil, fmt.Errorf("get routes: %w", err)
	}
	return routes, nil
}

func (p *OpenWrtProvider) GetFirewallZones(params map[string]interface{}) (interface{}, error) {
	zones, err := p.client.GetFirewallZones()
	if err != nil {
		return nil, fmt.Errorf("get firewall zones: %w", err)
	}
	return zones, nil
}

func (p *OpenWrtProvider) GetFirewallRules(params map[string]interface{}) (interface{}, error) {
	rules, err := p.client.GetFirewallRules()
	if err != nil {
		return nil, fmt.Errorf("get firewall rules: %w", err)
	}
	return rules, nil
}

func (p *OpenWrtProvider) GetFirewallRedirects(params map[string]interface{}) (interface{}, error) {
	redirects, err := p.client.GetFirewallRedirects()
	if err != nil {
		return nil, fmt.Errorf("get firewall redirects: %w", err)
	}
	return redirects, nil
}

func (p *OpenWrtProvider) GetWirelessStatus(params map[string]interface{}) (interface{}, error) {
	status, err := p.client.GetWirelessStatus()
	if err != nil {
		return nil, fmt.Errorf("get wireless status: %w", err)
	}
	return status, nil
}

func (p *OpenWrtProvider) GetDHCPLoads(params map[string]interface{}) (interface{}, error) {
	leases, err := p.client.GetDHCPLoads()
	if err != nil {
		return nil, fmt.Errorf("get DHCP leases: %w", err)
	}
	return leases, nil
}

func (p *OpenWrtProvider) ReadFile(params map[string]interface{}) (interface{}, error) {
	path, ok := params["path"].(string)
	if !ok {
		return nil, fmt.Errorf("path required")
	}
	content, err := p.client.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return map[string]interface{}{"content": content}, nil
}

func (p *OpenWrtProvider) WriteFile(params map[string]interface{}) (interface{}, error) {
	path, ok1 := params["path"].(string)
	data, ok2 := params["data"].(string)
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("path and data required")
	}
	mode, _ := params["mode"].(string)
	if mode == "" {
		mode = "0644"
	}
	if err := p.client.WriteFile(path, data, mode); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}
	return map[string]interface{}{"status": "written"}, nil
}

func (p *OpenWrtProvider) Reboot(params map[string]interface{}) (interface{}, error) {
	if err := p.client.Reboot(); err != nil {
		return nil, fmt.Errorf("reboot: %w", err)
	}
	return map[string]interface{}{"status": "rebooting"}, nil
}

func (p *OpenWrtProvider) GetLEDState(params map[string]interface{}) (interface{}, error) {
	name, ok := params["name"].(string)
	if !ok {
		return nil, fmt.Errorf("name required")
	}
	state, err := p.client.GetLEDState(name)
	if err != nil {
		return nil, fmt.Errorf("get LED state: %w", err)
	}
	return state, nil
}

func (p *OpenWrtProvider) SetLEDState(params map[string]interface{}) (interface{}, error) {
	name, ok1 := params["name"].(string)
	state, ok2 := params["state"].(string)
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("name and state required")
	}
	if err := p.client.SetLEDState(name, state); err != nil {
		return nil, fmt.Errorf("set LED state: %w", err)
	}
	return map[string]interface{}{"status": "set"}, nil
}
