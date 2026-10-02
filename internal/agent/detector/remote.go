package detector

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"golang.org/x/crypto/ssh"
)

// RemoteServiceDetector 远程服务检测器
type RemoteServiceDetector struct {
	// 常用服务端口映射
	commonPorts map[protocol.RemoteProtocol][]int
}

// 远程服务探测是 Workbench 远控入口（SSH/RDP/VNC 面板 + 内置终端兜底）的数据源，
// 实现一直存在但从未注册——真机验收发现 remote-services capability 永不上报（2026-09-30）。
// 必须经 NewRemoteServiceDetector() 注册：直接 &RemoteServiceDetector{} 的 commonPorts 为 nil。
func init() {
	Register(NewRemoteServiceDetector())
}

// NewRemoteServiceDetector 创建远程服务检测器
func NewRemoteServiceDetector() *RemoteServiceDetector {
	return &RemoteServiceDetector{
		commonPorts: map[protocol.RemoteProtocol][]int{
			protocol.RemoteProtocolSSH:    {22},
			protocol.RemoteProtocolRDP:    {3389},
			protocol.RemoteProtocolVNC:    {5900, 5901, 5902, 5903, 5904, 5905},
			protocol.RemoteProtocolTelnet: {23},
			protocol.RemoteProtocolFTP:    {21, 2121},
		},
	}
}

func (d *RemoteServiceDetector) Name() string {
	return "remote-services"
}

func (d *RemoteServiceDetector) Priority() int {
	return 100
}

func (d *RemoteServiceDetector) Detect() (*protocol.Capability, error) {
	services := d.ScanRemoteServices()

	if len(services) == 0 {
		return nil, nil
	}

	// 构建 capability metadata
	metadata := make(map[string]interface{})
	for _, svc := range services {
		entry := map[string]interface{}{
			"host":    svc.Host,
			"port":    svc.Port,
			"name":    svc.Name,
			"running": svc.Running,
		}
		// SSH 探测支持的认证方式
		if svc.Protocol == protocol.RemoteProtocolSSH {
			if methods, err := probeSSHAuthMethods(svc.Host, svc.Port); err == nil {
				entry["authMethods"] = methods
			}
		}
		metadata[string(svc.Protocol)] = entry
	}

	return &protocol.Capability{
		Type:     "remote-services",
		Endpoint: "local",
		Version:  "1.0",
		Metadata: metadata,
	}, nil
}

// ScanRemoteServices 扫描远程服务
func (d *RemoteServiceDetector) ScanRemoteServices() []protocol.RemoteServiceInfo {
	var services []protocol.RemoteServiceInfo

	// 检测 localhost 上的常用端口
	hosts := []string{"127.0.0.1", "0.0.0.0", ""}

	for protocolType, ports := range d.commonPorts {
		for _, port := range ports {
			for _, host := range hosts {
				svc := d.checkService(host, port, protocolType)
				if svc != nil {
					services = append(services, *svc)
					break // 找到一个就停止检测该协议的其他端口
				}
			}
		}
	}

	return services
}

// checkService 检查单个服务
func (d *RemoteServiceDetector) checkService(host string, port int, protocolType protocol.RemoteProtocol) *protocol.RemoteServiceInfo {
	address := net.JoinHostPort(host, strconv.Itoa(port))
	if host == "" {
		address = ":" + strconv.Itoa(port)
	}

	// 尝试连接
	conn, err := net.DialTimeout("tcp", address, 500*time.Millisecond)
	if err != nil {
		return nil
	}
	conn.Close()

	// 服务运行中
	return &protocol.RemoteServiceInfo{
		Protocol: protocolType,
		Host:     host,
		Port:     port,
		Name:     d.getServiceName(protocolType),
		Running:  true,
	}
}

// getServiceName 获取服务名称
func (d *RemoteServiceDetector) getServiceName(protocolType protocol.RemoteProtocol) string {
	switch protocolType {
	case protocol.RemoteProtocolSSH:
		return "SSH Server"
	case protocol.RemoteProtocolRDP:
		return "RDP Server"
	case protocol.RemoteProtocolVNC:
		return "VNC Server"
	case protocol.RemoteProtocolTelnet:
		return "Telnet Server"
	case protocol.RemoteProtocolFTP:
		return "FTP Server"
	default:
		return string(protocolType)
	}
}

// ScanHost 扫描指定主机的端口
func (d *RemoteServiceDetector) ScanHost(host string, port int) bool {
	address := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", address, 1*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// ScanRange 扫描端口范围
func (d *RemoteServiceDetector) ScanRange(host string, startPort, endPort int) []int {
	var openPorts []int

	for port := startPort; port <= endPort; port++ {
		address := net.JoinHostPort(host, strconv.Itoa(port))
		conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
		if err == nil {
			openPorts = append(openPorts, port)
			conn.Close()
		}
	}

	return openPorts
}

// authMethodsRe 从 SSH 错误消息中提取认证方式列表。
// 错误格式: "ssh: unable to authenticate, attempted methods [password publickey]"
var authMethodsRe = regexp.MustCompile(`attempted methods \[([^\]]*)\]`)

// probeSSHAuthMethods 探测 SSH 服务器支持的认证方式。
// 用假用户名尝试认证，服务器会返回它支持的方法列表（如 password, publickey）。
func probeSSHAuthMethods(host string, port int) ([]string, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	config := &ssh.ClientConfig{
		User:            "probe-auth-check",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         3 * time.Second,
	}
	_, err := ssh.Dial("tcp", addr, config)
	if err == nil {
		return []string{"password", "publickey"}, nil
	}

	// 从错误消息中解析认证方式
	// "ssh: unable to authenticate, attempted methods [password publickey]"
	if m := authMethodsRe.FindStringSubmatch(err.Error()); len(m) > 1 {
		methods := strings.Fields(m[1])
		if len(methods) > 0 {
			return methods, nil
		}
	}

	return nil, fmt.Errorf("cannot determine auth methods: %w", err)
}

// RemoteCapabilityInfo 远程能力信息（供外部使用）
type RemoteCapabilityInfo struct {
	SSH *SSHInfo `json:"ssh,omitempty"`
	RDP *RDPInfo `json:"rdp,omitempty"`
	VNC *VNCInfo `json:"vnc,omitempty"`
}

type SSHInfo struct {
	Enabled bool   `json:"enabled"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
}

type RDPInfo struct {
	Enabled bool   `json:"enabled"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
}

type VNCInfo struct {
	Enabled bool `json:"enabled"`
	Display int  `json:"display"` // VNC 显示号
	Port    int  `json:"port"`
}

// GetRemoteCapability 从 capability 中提取远程能力信息
func GetRemoteCapability(cap protocol.Capability) *RemoteCapabilityInfo {
	if cap.Type != "remote-services" {
		return nil
	}

	info := &RemoteCapabilityInfo{}

	// 遍历 metadata
	for key, value := range cap.Metadata {
		if svcMap, ok := value.(map[string]interface{}); ok {
			running := false
			if r, ok := svcMap["running"].(bool); ok {
				running = r
			}

			host := ""
			if h, ok := svcMap["host"].(string); ok {
				host = h
			}

			port := 0
			if p, ok := svcMap["port"].(float64); ok {
				port = int(p)
			}

			if !running {
				continue
			}

			switch key {
			case "ssh":
				info.SSH = &SSHInfo{Enabled: true, Host: host, Port: port}
			case "rdp":
				info.RDP = &RDPInfo{Enabled: true, Host: host, Port: port}
			case "vnc":
				info.VNC = &VNCInfo{Enabled: true, Port: port}
			}
		}
	}

	return info
}
