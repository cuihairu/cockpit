package detector

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cuihairu/cockpit/internal/protocol"
)

func init() {
	Register(&HardwareDetector{})
}

// HardwareDetector 硬件监控检测器
type HardwareDetector struct{}

// Name 检测器名称
func (d *HardwareDetector) Name() string {
	return "hardware-monitor"
}

// Priority 检测优先级
func (d *HardwareDetector) Priority() int {
	return 30
}

// Detect 检测硬件监控能力
func (d *HardwareDetector) Detect() (*protocol.Capability, error) {
	features := make(map[string]any)

	// 1. 检测 SMART 支持
	if d.hasSmartctl() {
		features["smart"] = true
	}

	// 2. 检测温度监控
	if d.hasTempSensors() {
		features["temperature"] = true
	}

	// 3. 检测 UPS 支持
	if d.hasUPS() {
		features["ups"] = true
	}

	if len(features) == 0 {
		return nil, nil
	}

	return &protocol.Capability{
		Type:     "hardware-monitor",
		Metadata: features,
	}, nil
}

// hasSmartctl 检测是否有 smartctl 工具
func (d *HardwareDetector) hasSmartctl() bool {
	_, err := exec.LookPath("smartctl")
	if err != nil {
		return false
	}
	// 尝试运行一次验证
	cmd := exec.Command("smartctl", "--version")
	return cmd.Run() == nil
}

// hasTempSensors 检测温度传感器
func (d *HardwareDetector) hasTempSensors() bool {
	// 检查 /sys/class/thermal
	thermalPath := "/sys/class/thermal"
	if entries, err := os.ReadDir(thermalPath); err == nil {
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "thermal_zone") {
				return true
			}
		}
	}

	// 检查 lm-sensors
	_, err := exec.LookPath("sensors")
	if err == nil {
		cmd := exec.Command("sensors")
		return cmd.Run() == nil
	}

	return false
}

// hasUPS 检测 UPS 支持
func (d *HardwareDetector) hasUPS() bool {
	// 检查 apcupsd
	_, err := exec.LookPath("apcaccess")
	if err == nil {
		return true
	}

	// 检查 NUT (Network UPS Tools)
	_, err = exec.LookPath("upsc")
	if err == nil {
		return true
	}

	// 检查 /dev/usb 设备
	usbPath := "/dev/usb"
	if _, err := os.Stat(usbPath); err == nil {
		entries, _ := os.ReadDir(usbPath)
		for _, entry := range entries {
			if strings.Contains(entry.Name(), "hiddev") ||
			   strings.Contains(entry.Name(), "ups") {
				return true
			}
		}
	}

	return false
}

// GetDisks 获取磁盘列表（用于 SMART 监控）
func GetDisks() ([]string, error) {
	var disks []string

	// 扫描 /sys/block
	blockPath := "/sys/block"
	entries, err := os.ReadDir(blockPath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		// 跳过 loop 设备和 CD-ROM
		if strings.HasPrefix(entry.Name(), "loop") ||
		   strings.HasPrefix(entry.Name(), "sr") {
			continue
		}

		// 检查是否是物理设备
		devicePath := filepath.Join(blockPath, entry.Name())
		if _, err := os.Stat(filepath.Join(devicePath, "device")); err == nil {
			disks = append(disks, "/dev/"+entry.Name())
		}
	}

	return disks, nil
}
