package pve

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Client Proxmox VE API 客户端
type Client struct {
	endpoint    string
	tokenID     string
	tokenSecret string
	httpClient  *http.Client
	node        string // 默认节点，可选
}

// Config PVE 配置
type Config struct {
	Endpoint    string
	TokenID     string
	TokenSecret string
	Node        string // 可选，用于简化调用
	InsecureTLS bool   // 是否跳过 TLS 验证
}

// NewClient 创建 PVE 客户端
func NewClient(cfg Config) *Client {
	return &Client{
		endpoint:    strings.TrimSuffix(cfg.Endpoint, "/"),
		tokenID:     cfg.TokenID,
		tokenSecret: cfg.TokenSecret,
		node:        cfg.Node,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: cfg.InsecureTLS,
				},
			},
		},
	}
}

// doRequest 执行 HTTP 请求
func (c *Client) doRequest(method, path string, body []byte) ([]byte, error) {
	url := c.endpoint + path

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// 设置认证头
	req.Header.Set("Authorization", "PVEAPIToken="+c.tokenID+"="+c.tokenSecret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("PVE API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// get 执行 GET 请求
func (c *Client) get(path string) ([]byte, error) {
	return c.doRequest("GET", path, nil)
}

// post 执行 POST 请求
func (c *Client) post(path string, body interface{}) ([]byte, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}
	return c.doRequest("POST", path, jsonBody)
}

// put 执行 PUT 请求
func (c *Client) put(path string, body interface{}) ([]byte, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}
	return c.doRequest("PUT", path, jsonBody)
}

// del 执行 DELETE 请求
func (c *Client) del(path string) ([]byte, error) {
	return c.doRequest("DELETE", path, nil)
}

// ============ 节点相关 ============

// Node 节点信息
type Node struct {
	Node   string `json:"node"`
	Status string `json:"status"`
	IP     string `json:"ip"`
	CPU    float64
	MaxCPU int    `json:"maxcpu"`
	Mem    int64
	MaxMem int64 `json:"maxmem"`
	Disk   int64
	MaxDisk int64 `json:"maxdisk"`
	Uptime int64
	Level  string
}

// ListNodes 列出所有节点
func (c *Client) ListNodes() ([]Node, error) {
	body, err := c.get("/api2/json/nodes")
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data []Node `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return resp.Data, nil
}

// GetNodeStatus 获取节点状态
func (c *Client) GetNodeStatus(node string) (*Node, error) {
	body, err := c.get(fmt.Sprintf("/api2/json/nodes/%s/status", node))
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data Node `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &resp.Data, nil
}

// ============ 虚拟机相关 ============

// VM 虚拟机信息
type VM struct {
	VMID       int    `json:"vmid"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	CPU        float64
	Mem        int64
	MaxMem     int64 `json:"maxmem"`
	Disk       int64
	MaxDisk    int64 `json:"maxdisk"`
	Netout     int64
	Netin      int64
	DiskWrite  int64 `json:"diskwrite"`
	DiskRead   int64 `json:"diskread"`
	Uptime     int64
	CPUs       int    `json:"cpus"`
	Lock       string
	Tag        string
	MaxCPU     int    `json:"maxcpu"`
	Template   int    `json:"template"`
	QMPStatus  string `json:"qmpstatus"`
	Agent      int
	PoolID     string `json:"pool"`
}

// ListVMs 列出虚拟机
func (c *Client) ListVMs(node string) ([]VM, error) {
	if node == "" {
		node = c.node
	}
	if node == "" {
		return nil, fmt.Errorf("node required")
	}

	body, err := c.get(fmt.Sprintf("/api2/json/nodes/%s/qemu", node))
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data []VM `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return resp.Data, nil
}

// GetVM 获取虚拟机详情
func (c *Client) GetVM(node string, vmid int) (*VMConfig, error) {
	if node == "" {
		node = c.node
	}
	if node == "" {
		return nil, fmt.Errorf("node required")
	}

	// 获取当前状态
	body, err := c.get(fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/status/current", node, vmid))
	if err != nil {
		return nil, err
	}

	var statusResp struct {
		Data struct {
			VM       VM    `json:"vm"`
			Lock     string
			QMPStatus string `json:"qmpstatus"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &statusResp); err != nil {
		return nil, fmt.Errorf("parse status: %w", err)
	}

	// 获取配置
	configBody, err := c.get(fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/config", node, vmid))
	if err != nil {
		return nil, err
	}

	var configResp struct {
		Data VMConfigData `json:"data"`
	}
	if err := json.Unmarshal(configBody, &configResp); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return &VMConfig{
		VM:     statusResp.Data.VM,
		Config: configResp.Data,
		Lock:   statusResp.Data.Lock,
		QMPStatus: statusResp.Data.QMPStatus,
	}, nil
}

// VMConfig 虚拟机完整配置
type VMConfig struct {
	VM         VM
	Config     VMConfigData
	Lock       string
	QMPStatus  string
}

// VMConfigData 虚拟机配置数据
type VMConfigData struct {
	Cores    int    `json:"cores"`
	CPUType  string `json:"cpu,omitempty"`
	Memory   int    `json:"memory,omitempty"`
	Name     string `json:"name,omitempty"`
	Onboot   string `json:"onboot,omitempty"`
	BootDisk string `json:"bootdisk,omitempty"`
	BootOrder string `json:"boot,omitempty"`
	OSType   string `json:"ostype,omitempty"`
	SCSIHW   string `json:"scsihw,omitempty"`
	Sockets  int    `json:"sockets,omitempty"`
}

// StartVM 启动虚拟机
func (c *Client) StartVM(node string, vmid int) error {
	if node == "" {
		node = c.node
	}
	if node == "" {
		return fmt.Errorf("node required")
	}

	_, err := c.post(fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/status/start", node, vmid), nil)
	return err
}

// StopVM 停止虚拟机
func (c *Client) StopVM(node string, vmid int) error {
	if node == "" {
		node = c.node
	}
	if node == "" {
		return fmt.Errorf("node required")
	}

	_, err := c.post(fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/status/stop", node, vmid), nil)
	return err
}

// RestartVM 重启虚拟机
func (c *Client) RestartVM(node string, vmid int) error {
	if node == "" {
		node = c.node
	}
	if node == "" {
		return fmt.Errorf("node required")
	}

	_, err := c.post(fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/status/reboot", node, vmid), nil)
	return err
}

// SuspendVM 暂停虚拟机
func (c *Client) SuspendVM(node string, vmid int) error {
	if node == "" {
		node = c.node
	}
	if node == "" {
		return fmt.Errorf("node required")
	}

	_, err := c.post(fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/status/suspend", node, vmid), nil)
	return err
}

// ResumeVM 恢复虚拟机
func (c *Client) ResumeVM(node string, vmid int) error {
	if node == "" {
		node = c.node
	}
	if node == "" {
		return fmt.Errorf("node required")
	}

	_, err := c.post(fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/status/resume", node, vmid), nil)
	return err
}

// ============ 容器相关 ============

// Container LXC 容器信息
type Container struct {
	VMID      int    `json:"vmid"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CPU       float64
	Mem       int64
	MaxMem    int64 `json:"maxmem"`
	Disk      int64
	MaxDisk   int64 `json:"maxdisk"`
	Uptime    int64
	MaxCPU    int    `json:"maxcpu"`
	CPUs      int    `json:"cpus"`
	Lock      string
	Templates int    `json:"template"`
}

// ListContainers 列出容器
func (c *Client) ListContainers(node string) ([]Container, error) {
	if node == "" {
		node = c.node
	}
	if node == "" {
		return nil, fmt.Errorf("node required")
	}

	body, err := c.get(fmt.Sprintf("/api2/json/nodes/%s/lxc", node))
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data []Container `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return resp.Data, nil
}

// GetContainer 获取容器详情
func (c *Client) GetContainer(node string, vmid int) (*ContainerConfig, error) {
	if node == "" {
		node = c.node
	}
	if node == "" {
		return nil, fmt.Errorf("node required")
	}

	// 获取当前状态
	body, err := c.get(fmt.Sprintf("/api2/json/nodes/%s/lxc/%d/status/current", node, vmid))
	if err != nil {
		return nil, err
	}

	var statusResp struct {
		Data Container `json:"data"`
	}
	if err := json.Unmarshal(body, &statusResp); err != nil {
		return nil, fmt.Errorf("parse status: %w", err)
	}

	// 获取配置
	configBody, err := c.get(fmt.Sprintf("/api2/json/nodes/%s/lxc/%d/config", node, vmid))
	if err != nil {
		return nil, err
	}

	var configResp struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(configBody, &configResp); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return &ContainerConfig{
		Container: statusResp.Data,
		Config:    configResp.Data,
	}, nil
}

// ContainerConfig 容器完整配置
type ContainerConfig struct {
	Container Container
	Config    map[string]string
}

// StartContainer 启动容器
func (c *Client) StartContainer(node string, vmid int) error {
	if node == "" {
		node = c.node
	}
	if node == "" {
		return fmt.Errorf("node required")
	}

	_, err := c.post(fmt.Sprintf("/api2/json/nodes/%s/lxc/%d/status/start", node, vmid), nil)
	return err
}

// StopContainer 停止容器
func (c *Client) StopContainer(node string, vmid int) error {
	if node == "" {
		node = c.node
	}
	if node == "" {
		return fmt.Errorf("node required")
	}

	_, err := c.post(fmt.Sprintf("/api2/json/nodes/%s/lxc/%d/status/stop", node, vmid), nil)
	return err
}

// RestartContainer 重启容器
func (c *Client) RestartContainer(node string, vmid int) error {
	if node == "" {
		node = c.node
	}
	if node == "" {
		return fmt.Errorf("node required")
	}

	_, err := c.post(fmt.Sprintf("/api2/json/nodes/%s/lxc/%d/status/reboot", node, vmid), nil)
	return err
}

// ShutdownContainer 关闭容器
func (c *Client) ShutdownContainer(node string, vmid int) error {
	if node == "" {
		node = c.node
	}
	if node == "" {
		return fmt.Errorf("node required")
	}

	_, err := c.post(fmt.Sprintf("/api2/json/nodes/%s/lxc/%d/status/shutdown", node, vmid), nil)
	return err
}

// ============ 存储相关 ============

// Storage 存储信息
type Storage struct {
	Storage     string `json:"storage"`
	Node        string `json:"node"`
	Content     string `json:"content"`
	Type        string `json:"type"`
	Shared      int
	Used        int64
	Avail       int64
	Total       int64
	Plugin      string `json:"plugin"`
	Active      int
	Enabled     int
	ReadOnly    int    `json:"read_only"`
	Status      string
}

// ListStorage 列出存储
func (c *Client) ListStorage(node string) ([]Storage, error) {
	if node == "" {
		node = c.node
	}
	if node == "" {
		return nil, fmt.Errorf("node required")
	}

	body, err := c.get(fmt.Sprintf("/api2/json/nodes/%s/storage", node))
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data []Storage `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return resp.Data, nil
}

// GetStorage 获取存储详情
func (c *Client) GetStorage(node, storage string) (*StorageStatus, error) {
	if node == "" {
		node = c.node
	}
	if node == "" {
		return nil, fmt.Errorf("node required")
	}

	body, err := c.get(fmt.Sprintf("/api2/json/nodes/%s/storage/%s/status", node, storage))
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data StorageStatus `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &resp.Data, nil
}

// StorageStatus 存储状态
type StorageStatus struct {
	Used  int64
	Avail int64
	Total int64
}

// ============ 快照相关 ============

// Snapshot 快照信息
type Snapshot struct {
	Name        string
	Snapshot    string
	Digest      string
	VmState     int     `json:"vmstate"`
	Description string
	Parent      string
	Snaptime    int64
	SnapTime    int64 `json:"snaptime"`
}

// ListVMSnapshots 列出虚拟机快照
func (c *Client) ListVMSnapshots(node string, vmid int) ([]Snapshot, error) {
	if node == "" {
		node = c.node
	}
	if node == "" {
		return nil, fmt.Errorf("node required")
	}

	body, err := c.get(fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/snapshot", node, vmid))
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data []Snapshot `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return resp.Data, nil
}

// CreateVMSnapshot 创建虚拟机快照
func (c *Client) CreateVMSnapshot(node string, vmid int, name, desc string) error {
	if node == "" {
		node = c.node
	}
	if node == "" {
		return fmt.Errorf("node required")
	}

	params := map[string]string{}
	if name != "" {
		params["snapname"] = name
	}
	if desc != "" {
		params["description"] = desc
	}

	_, err := c.post(fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/snapshot", node, vmid), params)
	return err
}

// DeleteVMSnapshot 删除虚拟机快照
func (c *Client) DeleteVMSnapshot(node string, vmid int, name string) error {
	if node == "" {
		node = c.node
	}
	if node == "" {
		return fmt.Errorf("node required")
	}

	_, err := c.del(fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/snapshot/%s", node, vmid, name))
	return err
}

// ============ 网络相关 ============

// Version 获取版本信息
func (c *Client) Version() (*VersionInfo, error) {
	body, err := c.get("/api2/json/version")
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data VersionInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &resp.Data, nil
}

// VersionInfo 版本信息
type VersionInfo struct {
	Version string `json:"version"`
	Release string `json:"release"`
	Repoid  string `json:"repoid"`
}

// GetVMID 从字符串解析 VM ID
func GetVMID(vmid interface{}) (int, error) {
	switch v := vmid.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	case string:
		return strconv.Atoi(v)
	default:
		return 0, fmt.Errorf("invalid vmid type: %T", vmid)
	}
}
