package detector

import (
	"bytes"
	"os"
	"os/exec"
	"strings"

	"github.com/cuihairu/cockpit/internal/protocol"
)

func init() {
	Register(&OpenWrtDetector{})
}

// OpenWrtDetector OpenWrt 检测器
type OpenWrtDetector struct{}

// Name 检测器名称
func (d *OpenWrtDetector) Name() string {
	return "openwrt"
}

// Priority 检测优先级（最先检测）
func (d *OpenWrtDetector) Priority() int {
	return 5
}

// Detect 检测 OpenWrt 环境
func (d *OpenWrtDetector) Detect() (*protocol.Capability, error) {
	// 1. 检查 /etc/openwrt_release
	if _, err := os.Stat("/etc/openwrt_release"); err == nil {
		return d.readOpenWrtRelease()
	}

	// 2. 检查 ubus 工具
	if _, err := os.Stat("/bin/ubus"); err == nil {
		return &protocol.Capability{
			Type: "openwrt",
			Metadata: map[string]any{
				"detection": "ubus",
			},
		}, nil
	}

	// 3. 检查 opkg 包管理器
	if _, err := os.Stat("/bin/opkg"); err == nil {
		return &protocol.Capability{
			Type: "openwrt",
			Metadata: map[string]any{
				"detection": "opkg",
			},
		}, nil
	}

	return nil, nil
}

// readOpenWrtRelease 读取 OpenWrt 版本信息
func (d *OpenWrtDetector) readOpenWrtRelease() (*protocol.Capability, error) {
	data, err := os.ReadFile("/etc/openwrt_release")
	if err != nil {
		return nil, err
	}

	metadata := make(map[string]any)
	lines := bytes.Split(data, []byte("\n"))
	for _, line := range lines {
		parts := bytes.SplitN(line, []byte("="), 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(string(parts[0]))
			value := strings.Trim(strings.TrimSpace(string(parts[1])), `"`)
			metadata[key] = value
		}
	}

	return &protocol.Capability{
		Type:     "openwrt",
		Metadata: metadata,
	}, nil
}

// GetSystemInfo 获取 OpenWrt 系统信息
func GetSystemInfo() (map[string]any, error) {
	// 使用 ubus 获取系统信息
	cmd := exec.Command("/bin/ubus", "call", "system", "info")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	info := make(map[string]any)
	// 简单解析 JSON（生产环境应该用 json.Unmarshal）
	info["raw_output"] = string(output)

	return info, nil
}
