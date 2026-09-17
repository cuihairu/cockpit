package rpc

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// ============ SMART Provider ============
//
// 磁盘健康只读观测（见 docs/guide/disk-health-design.md）：
// lsblk 发现物理盘 + 逐盘 smartctl --json -H -A 读白名单字段（D3/D4）。
// 无入参、只读、校验面为零（D2）；单盘失败不影响其余盘；
// 无 smartctl 时返回 available=false（D1，巡检与前端跳过不报错）。

const (
	// smartLsblkTimeout 盘发现命令超时
	smartLsblkTimeout = 5 * time.Second
	// smartCtlTimeout 单盘 smartctl 超时
	smartCtlTimeout = 10 * time.Second
	// smartTotalTimeout 整次 RPC 整体上限（D3）
	smartTotalTimeout = 30 * time.Second
)

// 命令名做成变量仅为测试可注入
var (
	smartLsblkBin = "lsblk"
	smartCtlBin   = "smartctl"
	// smartCtlLookPath smartctl 探测，测试桩替换（假路径过不了真 LookPath）
	smartCtlLookPath = exec.LookPath
)

// SmartProvider 磁盘健康观测，挂在 hardware-monitor capability 下（D1）
type SmartProvider struct {
	run Commander
}

// NewSmartProvider 创建 provider；run 为 nil 时使用真实命令执行
func NewSmartProvider(run Commander) *SmartProvider {
	if run == nil {
		run = defaultCommander
	}
	return &SmartProvider{run: run}
}

// Type RPC provider 类型（复用 hardware-monitor capability）
func (p *SmartProvider) Type() string { return "hardware-monitor" }

// Call RPC 分发
func (p *SmartProvider) Call(action string, _ map[string]interface{}) (interface{}, error) {
	if action != "status" {
		return nil, fmt.Errorf("unsupported action %q", action)
	}
	return p.readSmart()
}

// readSmart 盘发现 + 逐盘读数
func (p *SmartProvider) readSmart() (interface{}, error) {
	if _, err := smartCtlLookPath(smartCtlBin); err != nil {
		return map[string]interface{}{"available": false, "devices": []smartDevice{}}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), smartTotalTimeout)
	defer cancel()

	out, _, err := p.run(ctx, smartLsblkBin, "--json", "--paths", "-b",
		"-o", "NAME,TYPE,SIZE,MODEL,SERIAL")
	if err != nil {
		return nil, fmt.Errorf("lsblk failed: %w", err)
	}
	disks := parseLsblkDisks(out)

	devices := make([]smartDevice, 0, len(disks))
	for _, d := range disks {
		devices = append(devices, p.readOneDisk(ctx, d))
	}
	return map[string]interface{}{"available": true, "devices": devices}, nil
}

// readOneDisk 单盘 smartctl 读数；失败标 unknown + 摘要，不中断整体（D5/D3）
func (p *SmartProvider) readOneDisk(ctx context.Context, d lsblkDisk) smartDevice {
	cmdCtx, cancel := context.WithTimeout(ctx, smartCtlTimeout)
	defer cancel()

	out, stderr, err := p.run(cmdCtx, smartCtlBin, "--json", "-H", "-A", d.Name)
	if err == nil {
		return parseSmartctlDevice(d, out)
	}
	dev := smartDevice{
		Name: d.Name, Model: d.Model, Serial: d.Serial,
		SizeBytes: d.SizeBytes, Health: smartHealthUnknown,
	}
	dev.Error = smartErrSummary(stderr, err)
	return dev
}
