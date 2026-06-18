package rpc

import (
	"fmt"

	"github.com/cuihairu/cockpit/internal/openwrt"
)

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
