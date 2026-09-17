package rpc

import (
	"encoding/json"
	"strconv"
	"strings"
)

// SMART 磁盘健康解析（纯函数，见 docs/guide/disk-health-design.md D3/D4）。
// 只取白名单字段（D4），smartctl 的完整 JSON 绝不透传；
// 取不到的数值字段保持 nil（json omitempty），前端展示 "—"。

// smartHealth 健康三态（D5）
const (
	smartHealthPassed  = "passed"
	smartHealthFailed  = "failed"
	smartHealthUnknown = "unknown"
)

// smartDevice smart.status devices[] 元素
type smartDevice struct {
	Name               string  `json:"name"`
	Model              string  `json:"model,omitempty"`
	Serial             string  `json:"serial,omitempty"`
	SizeBytes          uint64  `json:"sizeBytes,omitempty"`
	Health             string  `json:"health"`
	TemperatureC       int     `json:"temperatureC,omitempty"`
	PowerOnHours       *uint64 `json:"powerOnHours,omitempty"`
	ReallocatedSectors *uint64 `json:"reallocatedSectors,omitempty"`
	PendingSectors     *uint64 `json:"pendingSectors,omitempty"`
	MediaErrors        *uint64 `json:"mediaErrors,omitempty"`
	PercentUsed        int     `json:"percentUsed,omitempty"`
	Error              string  `json:"error,omitempty"`
}

// lsblkDisk lsblk 发现的一块物理盘
type lsblkDisk struct {
	Name      string
	Model     string
	Serial    string
	SizeBytes uint64
}

// parseLsblkDisks 解析 lsblk --json 输出，过滤 TYPE=="disk"（排除 loop/ram/分区）
func parseLsblkDisks(out []byte) []lsblkDisk {
	var parsed struct {
		BlockDevices []struct {
			Name   string      `json:"name"`
			Type   string      `json:"type"`
			Size   json.Number `json:"size"`
			Model  string      `json:"model"`
			Serial string      `json:"serial"`
		} `json:"blockdevices"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil
	}
	var disks []lsblkDisk
	for _, d := range parsed.BlockDevices {
		if d.Type != "disk" || d.Name == "" {
			continue
		}
		disk := lsblkDisk{Name: d.Name, Model: d.Model, Serial: d.Serial}
		if n, err := d.Size.Int64(); err == nil && n > 0 {
			disk.SizeBytes = uint64(n)
		}
		disks = append(disks, disk)
	}
	return disks
}

// smartRawNumber smartctl raw.value 兼容解析：不同版本可能是数字或字符串
func smartRawNumber(v interface{}) *uint64 {
	switch n := v.(type) {
	case float64:
		if n < 0 {
			return nil
		}
		u := uint64(n)
		return &u
	case string:
		s := strings.TrimSpace(n)
		if u, err := strconv.ParseUint(s, 10, 64); err == nil {
			return &u
		}
	}
	return nil
}

// ataAttrByID 从 ata_smart_attributes.table 里按 id 找 raw.value
func ataAttrByID(table []interface{}, id float64) *uint64 {
	for _, item := range table {
		attr, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if aid, ok := attr["id"].(float64); !ok || aid != id {
			continue
		}
		raw, ok := attr["raw"].(map[string]interface{})
		if !ok {
			continue
		}
		return smartRawNumber(raw["value"])
	}
	return nil
}

// smartErrSummary 错误摘要：stderr 去 blank 后截断（前端提示用，不告警）
func smartErrSummary(stderr []byte, err error) string {
	s := strings.TrimSpace(string(stderr))
	if s == "" && err != nil {
		s = err.Error()
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// parseSmartctlDevice 解析单盘 smartctl --json -H -A 输出，填白名单字段（D4）。
// base 携带 lsblk 侧已知的名称/型号/容量；无 smart_status 时 health=unknown。
func parseSmartctlDevice(base lsblkDisk, out []byte) smartDevice {
	dev := smartDevice{
		Name:      base.Name,
		Model:     base.Model,
		Serial:    base.Serial,
		SizeBytes: base.SizeBytes,
		Health:    smartHealthUnknown,
	}
	var parsed struct {
		Device struct {
			Type string `json:"type"`
		} `json:"device"`
		SmartStatus *struct {
			Passed bool `json:"passed"`
		} `json:"smart_status"`
		Temperature struct {
			Current int `json:"current"`
		} `json:"temperature"`
		AtaAttrs struct {
			Table []interface{} `json:"table"`
		} `json:"ata_smart_attributes"`
		NVMeHealth struct {
			PowerOnHours  interface{} `json:"power_on_hours"`
			MediaErrors   interface{} `json:"media_errors"`
			CriticalWarn  interface{} `json:"critical_warning"`
			PercentageUsed interface{} `json:"percentage_used"`
		} `json:"nvme_smart_health_information_log"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		dev.Error = "smartctl 输出解析失败"
		return dev
	}

	if parsed.SmartStatus != nil {
		if parsed.SmartStatus.Passed {
			dev.Health = smartHealthPassed
		} else {
			dev.Health = smartHealthFailed
		}
	} else if dev.Error == "" {
		// 无健康结论（盘不支持 SMART / 权限受限等），给前端一句说明（D5）
		dev.Error = "SMART 状态不可用"
	}
	if parsed.Temperature.Current != 0 {
		dev.TemperatureC = parsed.Temperature.Current
	}

	// ATA 关键属性：5=Reallocated_Sector_Ct 197=Current_Pending_Sector 9=Power_On_Hours
	if len(parsed.AtaAttrs.Table) > 0 {
		dev.ReallocatedSectors = ataAttrByID(parsed.AtaAttrs.Table, 5)
		dev.PendingSectors = ataAttrByID(parsed.AtaAttrs.Table, 197)
		dev.PowerOnHours = ataAttrByID(parsed.AtaAttrs.Table, 9)
	}

	// NVMe：smart_health_information_log（critical_warning 非 0 记入 error 提示）
	if parsed.Device.Type == "nvme" {
		if dev.PowerOnHours == nil {
			dev.PowerOnHours = smartRawNumber(parsed.NVMeHealth.PowerOnHours)
		}
		if dev.MediaErrors == nil {
			dev.MediaErrors = smartRawNumber(parsed.NVMeHealth.MediaErrors)
		}
		if p := smartRawNumber(parsed.NVMeHealth.PercentageUsed); p != nil {
			dev.PercentUsed = int(*p)
		}
		if cw := smartRawNumber(parsed.NVMeHealth.CriticalWarn); cw != nil && *cw != 0 && dev.Error == "" {
			dev.Error = "NVMe critical_warning 非 0"
		}
	}
	return dev
}
