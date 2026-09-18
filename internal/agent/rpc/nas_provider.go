package rpc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ============ NAS Provider ============
//
// NAS 存储观测（见 nas-design.md）：统一快照模型，M1 linux 源 = /proc/mdstat
// + zpool + vgs（池）+ df（挂载容量）+ testparm/exportfs（SMB/NFS 共享）。
// 全部只读、argv 直调不经 shell；单源失败各自降级不影响其余源；
// 全部工具缺失时 available=false，巡检与前端跳过不报错（smart 同款）。

const (
	// nasCmdTimeout 单命令超时
	nasCmdTimeout = 5 * time.Second
	// nasTotalTimeout 整次 RPC 整体上限（smart 同款纪律）
	nasTotalTimeout = 30 * time.Second
	// nasMinMountGB 挂载表最小容量门槛（过滤 runc/临时小卷噪声）
	nasMinMountGB = 1.0
)

// 命令名做成变量仅为测试可注入（smart 同款）
var (
	nasZpoolBin    = "zpool"
	nasVgsBin      = "vgs"
	nasTestparmBin = "testparm"
	nasExportfsBin = "exportfs"
	nasDfBin       = "df"
	// nasMdstatPath 软件 RAID 状态文件路径，测试桩替换
	nasMdstatPath = "/proc/mdstat"
	// nasLookPath 工具探测，测试桩替换（假路径过不了真 LookPath）
	nasLookPath = exec.LookPath
	// nasDetectTools capability 探测的工具清单（任一存在即认为有观测价值）
	nasDetectTools = []string{"zpool", "vgs", "btrfs", "testparm", "exportfs"}
)

// NasPool 存储池 / RAID / 卷组
type NasPool struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"` // mdadm | zfs | lvm
	State   string   `json:"state"`
	TotalGB float64  `json:"totalGB"`
	UsedGB  float64  `json:"usedGB"`
	Devices []string `json:"devices"`
	Detail  string   `json:"detail"`
}

// NasMount 本地文件系统挂载容量
type NasMount struct {
	Device    string  `json:"device"`
	MountPath string  `json:"mountPath"`
	FsType    string  `json:"fsType"`
	TotalGB   float64 `json:"totalGB"`
	UsedGB    float64 `json:"usedGB"`
}

// NasShare SMB / NFS 共享导出
type NasShare struct {
	Protocol string `json:"protocol"` // smb | nfs
	Name     string `json:"name"`
	Path     string `json:"path"`
	Comment  string `json:"comment"`
	Hosts    string `json:"hosts"`
}

// NasSnapshot 统一快照（所有 provider——M2 的 dsm/truenas/omv——映射到此结构）
type NasSnapshot struct {
	Available bool       `json:"available"`
	Source    string     `json:"source"`
	Pools     []NasPool  `json:"pools"`
	Mounts    []NasMount `json:"mounts"`
	Shares    []NasShare `json:"shares"`
}

// NasProvider NAS 存储观测 Provider
type NasProvider struct {
	run Commander
	// readFile 文件读取器（/proc/mdstat、/etc/exports fallback），测试注入用
	readFile func(path string) ([]byte, error)
}

func NewNasProvider(run Commander) *NasProvider {
	if run == nil {
		run = defaultCommander
	}
	return &NasProvider{run: run, readFile: os.ReadFile}
}

func (p *NasProvider) Type() string { return "nas" }

func (p *NasProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	if action != "status" {
		return nil, fmt.Errorf("unknown nas action: %s", action)
	}
	return p.Snapshot()
}

// DetectNas 探测 NAS 观测价值：mdstat 存在（软件 RAID 可能有）或任一存储工具可用
func DetectNas() bool {
	if _, err := os.Stat(nasMdstatPath); err == nil {
		return true
	}
	for _, tool := range nasDetectTools {
		if _, err := nasLookPath(tool); err == nil {
			return true
		}
	}
	return false
}

// Snapshot 全量快照：各源独立降级，单源失败不拖垮整体
func (p *NasProvider) Snapshot() (interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), nasTotalTimeout)
	defer cancel()

	snap := NasSnapshot{Source: "linux"}
	snap.Pools = append(p.poolsFromMdstat(), p.poolsFromZfs(ctx)...)
	snap.Pools = append(snap.Pools, p.poolsFromLvm(ctx)...)
	snap.Mounts = p.mountsFromDf(ctx)
	snap.Shares = append(p.sharesFromSmb(ctx), p.sharesFromNfs(ctx)...)

	snap.Available = len(snap.Pools) > 0 || len(snap.Mounts) > 0 || len(snap.Shares) > 0
	return map[string]interface{}{
		"available": snap.Available,
		"source":    snap.Source,
		"pools":     poolsToMaps(snap.Pools),
		"mounts":    mountsToMaps(snap.Mounts),
		"shares":    sharesToMaps(snap.Shares),
	}, nil
}

// ============ mdadm（/proc/mdstat） ============

var (
	// mdDevLineRe md 设备行：md0 : active raid1 sda1[0] sdb1[2](F)
	mdDevLineRe = regexp.MustCompile(`^(\S+)\s*:\s*(\S+)\s*(.*)$`)
	// mdDevRe 成员盘：sda1[0] 或 sdb1[2](F)
	mdDevRe = regexp.MustCompile(`^(\S+?)\[\d+\](\(F\))?$`)
	// mdProgressRe 同步进度行（resync/recovery/check）
	mdProgressRe = regexp.MustCompile(`\b(resync|recovery|check|reshape)\b\s*=`)
	// mdBlocksRe 容量行：102396k blocks super 1.2 [2/1] [U_]
	mdBlocksRe = regexp.MustCompile(`(\d+)([kKmMgG]?) blocks`)
)

// poolsFromMdstat 解析 /proc/mdstat；文件不存在或无 md 设备返回空
func (p *NasProvider) poolsFromMdstat() []NasPool {
	raw, err := p.readFile(nasMdstatPath)
	if err != nil {
		return nil
	}
	var pools []NasPool
	var cur *NasPool
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimLeft(line, " \t")
		if line == "" || strings.HasPrefix(line, "Personalities") || strings.HasPrefix(line, "unused devices") {
			continue
		}
		if m := mdDevLineRe.FindStringSubmatch(line); m != nil && !strings.Contains(line, "blocks") {
			// 新 md 设备行
			if cur != nil {
				pools = append(pools, *cur)
			}
			state := "healthy"
			if m[2] == "inactive" {
				state = "failed"
			}
			cur = &NasPool{Name: m[1], Kind: "mdadm", State: state, Detail: m[2]}
			// 成员盘解析：(F) 标记失败盘 → degraded
			hasFailed := false
			for _, tok := range strings.Fields(m[3]) {
				if dm := mdDevRe.FindStringSubmatch(tok); dm != nil {
					cur.Devices = append(cur.Devices, dm[1])
					if dm[2] != "" {
						hasFailed = true
					}
				}
			}
			if hasFailed {
				cur.State = "degraded"
			}
			continue
		}
		if cur == nil {
			continue
		}
		// 容量行：取 blocks 数字（单位 k 为主）
		if m := mdBlocksRe.FindStringSubmatch(line); m != nil && cur.TotalGB == 0 {
			n, _ := strconv.ParseFloat(m[1], 64)
			cur.TotalGB = nasKBtoGB(n, m[2])
		}
		// 位图行 [U_] 缺 U → degraded（无失败盘标记但成员缺位）
		if strings.Contains(line, "[") && strings.Contains(line, "U") && strings.Contains(line, "_") {
			cur.State = "degraded"
		}
		// 进度行 → resync
		if mdProgressRe.MatchString(line) {
			cur.State = "resync"
			cur.Detail = strings.TrimSpace(line)
		}
	}
	if cur != nil {
		pools = append(pools, *cur)
	}
	return pools
}

// ============ ZFS ============

// poolsFromZfs zpool list -H -p -o name,size,alloc,health；无 zpool 跳过
func (p *NasProvider) poolsFromZfs(ctx context.Context) []NasPool {
	if _, err := nasLookPath(nasZpoolBin); err != nil {
		return nil
	}
	ctx2, cancel := context.WithTimeout(ctx, nasCmdTimeout)
	defer cancel()
	out, stderr, err := p.run(ctx2, nasZpoolBin, "list", "-H", "-p", "-o", "name,size,alloc,health")
	if err != nil {
		return nil // zpool 存在但不可用（权限/无池），静默降级
	}
	_ = stderr
	var pools []NasPool
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 4 {
			continue
		}
		size, _ := strconv.ParseFloat(fields[1], 64)
		alloc, _ := strconv.ParseFloat(fields[2], 64)
		state := strings.ToLower(fields[3])
		if state == "online" {
			state = "healthy"
		}
		pools = append(pools, NasPool{
			Name: fields[0], Kind: "zfs", State: state,
			TotalGB: nasBytesToGB(size), UsedGB: nasBytesToGB(alloc),
			Detail: fields[3],
		})
	}
	return pools
}

// ============ LVM ============

// poolsFromLvm vgs 卷组容量；无 vgs 跳过
func (p *NasProvider) poolsFromLvm(ctx context.Context) []NasPool {
	if _, err := nasLookPath(nasVgsBin); err != nil {
		return nil
	}
	ctx2, cancel := context.WithTimeout(ctx, nasCmdTimeout)
	defer cancel()
	out, _, err := p.run(ctx2, nasVgsBin, "--noheadings", "--units", "b", "--nosuffix",
		"-o", "vg_name,vg_size,vg_free")
	if err != nil {
		return nil
	}
	var pools []NasPool
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 3 {
			continue
		}
		size, _ := strconv.ParseFloat(fields[1], 64)
		free, _ := strconv.ParseFloat(fields[2], 64)
		pools = append(pools, NasPool{
			Name: fields[0], Kind: "lvm", State: "unknown",
			TotalGB: nasBytesToGB(size), UsedGB: nasBytesToGB(size - free),
		})
	}
	return pools
}

// ============ df 挂载容量 ============

// mountsFromDf df -k -P：排除伪与远端 FS（source 不以 / 开头或 // 远端前缀）
// 与 <1GB 小卷；同 (device, 容量) 去重防 bind mount 噪声
func (p *NasProvider) mountsFromDf(ctx context.Context) []NasMount {
	ctx2, cancel := context.WithTimeout(ctx, nasCmdTimeout)
	defer cancel()
	out, _, err := p.run(ctx2, nasDfBin, "-k", "-P")
	if err != nil {
		return nil
	}
	var mounts []NasMount
	seen := map[string]bool{}
	for i, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || i == 0 && strings.HasPrefix(line, "Filesystem") {
			continue
		}
		// POSIX df -P 固定 6 列（多空格对齐），Fields 按任意空白切
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		device := fields[0]
		if !strings.HasPrefix(device, "/") || strings.HasPrefix(device, "//") {
			continue
		}
		totalKB, _ := strconv.ParseFloat(fields[1], 64)
		usedKB, _ := strconv.ParseFloat(fields[2], 64)
		totalGB := nasKBtoGB(totalKB, "k")
		if totalGB < nasMinMountGB {
			continue
		}
		key := device + "|" + strconv.FormatFloat(totalGB, 'f', 1, 64)
		if seen[key] {
			continue
		}
		seen[key] = true
		mounts = append(mounts, NasMount{
			Device:    device,
			MountPath: strings.TrimSpace(fields[5]),
			TotalGB:   totalGB,
			UsedGB:    nasKBtoGB(usedKB, "k"),
		})
	}
	return mounts
}

// ============ SMB / NFS 共享 ============

// sharesFromSmb testparm -s（配置走 stderr）：解析非 global [share] 段
func (p *NasProvider) sharesFromSmb(ctx context.Context) []NasShare {
	if _, err := nasLookPath(nasTestparmBin); err != nil {
		return nil
	}
	ctx2, cancel := context.WithTimeout(ctx, nasCmdTimeout)
	defer cancel()
	_, stderr, err := p.run(ctx2, nasTestparmBin, "-s")
	if err != nil && len(stderr) == 0 {
		return nil
	}
	var shares []NasShare
	var cur *NasShare
	inGlobal := false
	for _, line := range strings.Split(string(stderr), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if cur != nil && cur.Name != "" {
				shares = append(shares, *cur)
			}
			name := strings.Trim(line, "[]")
			inGlobal = name == "global"
			cur = nil
			if !inGlobal {
				cur = &NasShare{Protocol: "smb", Name: name}
			}
			continue
		}
		if cur == nil {
			continue
		}
		if v, ok := strings.CutPrefix(line, "path = "); ok {
			cur.Path = strings.TrimSpace(v)
		} else if v, ok := strings.CutPrefix(line, "comment = "); ok {
			cur.Comment = strings.TrimSpace(v)
		}
	}
	if cur != nil && cur.Name != "" {
		shares = append(shares, *cur)
	}
	return shares
}

var nfsExportRe = regexp.MustCompile(`^(\S+)\s+(\S+?)\(([^)]*)\)$`)

// sharesFromNfs exportfs -v：`/path host(opts)` 一行一条；空输出 fallback /etc/exports
func (p *NasProvider) sharesFromNfs(ctx context.Context) []NasShare {
	if _, err := nasLookPath(nasExportfsBin); err != nil {
		return nil
	}
	ctx2, cancel := context.WithTimeout(ctx, nasCmdTimeout)
	defer cancel()
	out, _, err := p.run(ctx2, nasExportfsBin, "-v")
	if err != nil {
		return nil
	}
	return parseNfsExports(string(out))
}

func parseNfsExports(out string) []NasShare {
	var shares []NasShare
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if m := nfsExportRe.FindStringSubmatch(line); m != nil {
			shares = append(shares, NasShare{
				Protocol: "nfs", Path: m[1], Hosts: m[2],
				Name: m[1],
			})
		}
	}
	return shares
}

// ============ 工具函数 ============

// nasKBtoGB 1k 块数（mdstat 的 k/m/g 后缀）转 GB
func nasKBtoGB(n float64, unit string) float64 {
	switch strings.ToLower(unit) {
	case "g":
		return n * 1024 * 1024 * 1024 / 1e9
	case "m":
		return n * 1024 * 1024 / 1e9
	default: // k
		return n * 1024 / 1e9
	}
}

func nasBytesToGB(n float64) float64 { return n / 1e9 }

// RPC 边界统一为 map（与 protocol JSON 序列化对齐，测试断言方便）
func poolsToMaps(pools []NasPool) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(pools))
	for i := range pools {
		out = append(out, map[string]interface{}{
			"name": pools[i].Name, "kind": pools[i].Kind, "state": pools[i].State,
			"totalGB": pools[i].TotalGB, "usedGB": pools[i].UsedGB,
			"devices": pools[i].Devices, "detail": pools[i].Detail,
		})
	}
	return out
}

func mountsToMaps(mounts []NasMount) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(mounts))
	for i := range mounts {
		out = append(out, map[string]interface{}{
			"device": mounts[i].Device, "mountPath": mounts[i].MountPath,
			"fsType": mounts[i].FsType, "totalGB": mounts[i].TotalGB, "usedGB": mounts[i].UsedGB,
		})
	}
	return out
}

func sharesToMaps(shares []NasShare) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(shares))
	for i := range shares {
		out = append(out, map[string]interface{}{
			"protocol": shares[i].Protocol, "name": shares[i].Name, "path": shares[i].Path,
			"comment": shares[i].Comment, "hosts": shares[i].Hosts,
		})
	}
	return out
}
