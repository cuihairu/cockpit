package rpc

import (
	"fmt"

	"github.com/cuihairu/cockpit/internal/pve"
)

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
