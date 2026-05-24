package inventory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Inventory 基础设施清单
type Inventory struct {
	Version   string              `yaml:"version"`
	Metadata  Metadata            `yaml:"metadata,omitempty"`
	Regions   map[string]*Region  `yaml:"regions,omitempty"`
	Domains   map[string]*Domain  `yaml:"domains,omitempty"`
	Resources map[string]*Ref     `yaml:"resources,omitempty"`
	Templates map[string]*Template `yaml:"templates,omitempty"`
}

// Metadata 元数据
type Metadata struct {
	Name        string            `yaml:"name,omitempty"`
	Description string            `yaml:"description,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
}

// Region 地域
type Region struct {
	ID          string            `yaml:"id"`
	Name        string            `yaml:"name,omitempty"`
	Description string            `yaml:"description,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Zones       map[string]*Zone  `yaml:"zones,omitempty"`
}

// Zone 可用区
type Zone struct {
	ID          string            `yaml:"id"`
	Name        string            `yaml:"name,omitempty"`
	Description string            `yaml:"description,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Endpoints   []string          `yaml:"endpoints,omitempty"` // API 端点
	Agents      map[string]*Agent  `yaml:"agents,omitempty"`
}

// Agent Agent 定义
type Agent struct {
	ID          string            `yaml:"id"`
	Name        string            `yaml:"name,omitempty"`
	Hostname    string            `yaml:"hostname,omitempty"`
	IP          string            `yaml:"ip,omitempty"`
	Port        int               `yaml:"port,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Capabilities []string         `yaml:"capabilities,omitempty"` // pve, docker, openwrt, etc
	Config      map[string]any    `yaml:"config,omitempty"`      // Agent 特定配置
}

// Domain 域名定义
type Domain struct {
	ID          string            `yaml:"id"`
	Domain      string            `yaml:"domain"`
	Provider    string            `yaml:"provider,omitempty"` // cloudflare, godaddy, etc
	Agent       string            `yaml:"agent,omitempty"`    // 关联 Agent ID
	AutoRenew   bool              `yaml:"autoRenew,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Certificates []*Certificate   `yaml:"certificates,omitempty"`
}

// Certificate 证书定义
type Certificate struct {
	ID              string            `yaml:"id"`
	Domain          string            `yaml:"domain"` // 或引用 domains 中的 key
	Provider        string            `yaml:"provider,omitempty"` // letsencrypt, zerossl, etc
	Agent           string            `yaml:"agent,omitempty"`
	AutoRenew       bool              `yaml:"autoRenew,omitempty"`
	RenewBeforeDays int               `yaml:"renewBeforeDays,omitempty"`
	Labels          map[string]string `yaml:"labels,omitempty"`
}

// Ref 资源引用 (使用 Ref 关联其他资源)
type Ref struct {
	Ref string `yaml:"$ref"` // 引用路径，如 "regions.home.zone-a.servers.pve01"
}

// Template 模板定义
type Template struct {
	ID          string            `yaml:"id"`
	Description string            `yaml:"description,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Spec        map[string]any    `yaml:"spec"`
}

// ComputeInstance 计算资源 (在 Zone 下定义)
type ComputeInstance struct {
	ID          string            `yaml:"id"`
	Name        string            `yaml:"name"`
	Type        string            `yaml:"type"` // vm, container, baremetal
	Agent       string            `yaml:"agent"`
	Template    string            `yaml:"template,omitempty"`
	CPU         int               `yaml:"cpu,omitempty"`
	Memory      int               `yaml:"memory,omitempty"` // MB
	Disk        int               `yaml:"disk,omitempty"`   // GB
	IPv4        string            `yaml:"ipv4,omitempty"`
	IPv6        string            `yaml:"ipv6,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Tags        []string          `yaml:"tags,omitempty"`
}

// Service 服务定义
type Service struct {
	ID         string            `yaml:"id"`
	Name       string            `yaml:"name"`
	Type       string            `yaml:"type"` // http, tcp, database
	Agent      string            `yaml:"agent,omitempty"`
	URL        string            `yaml:"url,omitempty"`
	Endpoint   *Endpoint         `yaml:"endpoint,omitempty"`
	Interval   int               `yaml:"interval,omitempty"` // 检查间隔（秒）
	Labels     map[string]string `yaml:"labels,omitempty"`
}

// Endpoint 端点定义
type Endpoint struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	Path string `yaml:"path,omitempty"`
}

// Gateway 网关定义
type Gateway struct {
	ID       string            `yaml:"id"`
	Name     string            `yaml:"name"`
	Type     string            `yaml:"type"` // openwrt, pfsense, etc
	Agent    string            `yaml:"agent"`
	IPv4     string            `yaml:"ipv4,omitempty"`
	IPv6     string            `yaml:"ipv6,omitempty"`
	Upstream string            `yaml:"upstream,omitempty"`
	Labels   map[string]string `yaml:"labels,omitempty"`
}

// Storage 存储定义
type Storage struct {
	ID     string            `yaml:"id"`
	Name   string            `yaml:"name"`
	Type   string            `yaml:"type"` // nfs, iscsi, local, ceph
	Agent  string            `yaml:"agent,omitempty"`
	Path   string            `yaml:"path,omitempty"`
	Labels map[string]string `yaml:"labels,omitempty"`
}

// ParseFile 解析 inventory 文件
func ParseFile(path string) (*Inventory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return Parse(data)
}

// Parse 解析 inventory YAML
func Parse(data []byte) (*Inventory, error) {
	var inv Inventory
	if err := yaml.Unmarshal(data, &inv); err != nil {
		return nil, fmt.Errorf("unmarshal yaml: %w", err)
	}

	// 验证
	if err := inv.Validate(); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	return &inv, nil
}

// Validate 验证 inventory 配置
func (i *Inventory) Validate() error {
	// 检查版本
	if i.Version == "" {
		return fmt.Errorf("version is required")
	}
	if !strings.HasPrefix(i.Version, "v1") {
		return fmt.Errorf("unsupported version: %s", i.Version)
	}

	// 验证域名
	for id, domain := range i.Domains {
		if domain == nil {
			continue
		}
		domain.ID = id
		if domain.Domain == "" {
			return fmt.Errorf("domain %s: domain name is required", id)
		}
	}

	// 验证地域
	for regionID, region := range i.Regions {
		if region == nil {
			continue
		}
		region.ID = regionID

		// 验证可用区
		for zoneID, zone := range region.Zones {
			if zone == nil {
				continue
			}
			zone.ID = zoneID

			// 验证 Agent
			for agentID, agent := range zone.Agents {
				if agent == nil {
					continue
				}
				agent.ID = agentID
				if agent.Hostname == "" && agent.IP == "" {
					return fmt.Errorf("region %s zone %s agent %s: hostname or ip required", regionID, zoneID, agentID)
				}
			}
		}
	}

	return nil
}

// ResolveRef 解析引用路径
// 支持的路径格式:
// - "regions.<region>.zones.<zone>.agents.<agent>"
// - "domains.<domain>"
func (i *Inventory) ResolveRef(refPath string) (any, error) {
	parts := strings.Split(refPath, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid ref path: %s", refPath)
	}

	switch parts[0] {
	case "regions":
		return i.resolveRegionRef(parts[1:])
	case "domains":
		if domain, ok := i.Domains[parts[1]]; ok {
			return domain, nil
		}
		return nil, fmt.Errorf("domain not found: %s", parts[1])
	default:
		return nil, fmt.Errorf("unsupported ref type: %s", parts[0])
	}
}

func (i *Inventory) resolveRegionRef(parts []string) (any, error) {
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid region ref")
	}

	region, ok := i.Regions[parts[0]]
	if !ok {
		return nil, fmt.Errorf("region not found: %s", parts[0])
	}

	if parts[1] == "zones" {
		if len(parts) < 3 {
			return nil, fmt.Errorf("invalid zone ref")
		}
		zone, ok := region.Zones[parts[2]]
		if !ok {
			return nil, fmt.Errorf("zone not found: %s", parts[2])
		}

		if len(parts) > 3 && parts[3] == "agents" {
			if len(parts) < 5 {
				return nil, fmt.Errorf("invalid agent ref")
			}
			agent, ok := zone.Agents[parts[4]]
			if !ok {
				return nil, fmt.Errorf("agent not found: %s", parts[4])
			}
			return agent, nil
		}
		return zone, nil
	}

	return region, nil
}

// GetAgents 获取所有 Agent
func (i *Inventory) GetAgents() map[string]*AgentLocation {
	agents := make(map[string]*AgentLocation)

	for regionID, region := range i.Regions {
		for zoneID, zone := range region.Zones {
			for agentID, agent := range zone.Agents {
				if agent == nil {
					continue
				}
				agents[agentID] = &AgentLocation{
					Agent:      agent,
					Region:     regionID,
					Zone:       zoneID,
					RegionName: region.Name,
					ZoneName:   zone.Name,
				}
			}
		}
	}

	return agents
}

// AgentLocation 带位置信息的 Agent
type AgentLocation struct {
	*Agent
	Region     string
	Zone       string
	RegionName string
	ZoneName   string
}

// GetDomains 获取所有域名
func (i *Inventory) GetDomains() []*Domain {
	domains := make([]*Domain, 0, len(i.Domains))
	for _, domain := range i.Domains {
		if domain != nil {
			domains = append(domains, domain)
		}
	}
	return domains
}

// GetCertificates 获取所有证书
func (i *Inventory) GetCertificates() []*Certificate {
	certs := make([]*Certificate, 0)
	for _, domain := range i.Domains {
		if domain == nil {
			continue
		}
		for _, cert := range domain.Certificates {
			if cert != nil {
				certs = append(certs, cert)
			}
		}
	}
	return certs
}

// Write 写入 inventory 到文件
func (i *Inventory) Write(path string) error {
	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	data, err := yaml.Marshal(i)
	if err != nil {
		return fmt.Errorf("marshal yaml: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// Merge 合并另一个 inventory
func (i *Inventory) Merge(other *Inventory) {
	if other == nil {
		return
	}
	if other.Regions != nil {
		if i.Regions == nil {
			i.Regions = make(map[string]*Region)
		}
		for k, v := range other.Regions {
			i.Regions[k] = v
		}
	}
	if other.Domains != nil {
		if i.Domains == nil {
			i.Domains = make(map[string]*Domain)
		}
		for k, v := range other.Domains {
			i.Domains[k] = v
		}
	}
	if other.Resources != nil {
		if i.Resources == nil {
			i.Resources = make(map[string]*Ref)
		}
		for k, v := range other.Resources {
			i.Resources[k] = v
		}
	}
}

// LoadDir 加载目录下的所有 inventory 文件
func LoadDir(dir string) (*Inventory, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	merged := &Inventory{Version: "v1"}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		inv, err := ParseFile(path)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", entry.Name(), err)
		}

		merged.Merge(inv)
	}

	return merged, nil
}
