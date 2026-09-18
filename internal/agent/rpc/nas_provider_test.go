package rpc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// mockNasRunner 模拟 NAS 观测命令：按命令名返回预置输出；
// failBin 指定注入失败的命令（单源降级断言用）。
type mockNasRunner struct {
	mu      sync.Mutex
	outputs map[string]string
	failBin string
	called  []string
}

func (m *mockNasRunner) run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.called = append(m.called, name)
	if name == m.failBin {
		return nil, []byte(name + " failed"), errors.New("exit status 1")
	}
	// testparm 的配置从 stderr 出（samba 惯例），其余走 stdout
	if name == nasTestparmBin {
		return nil, []byte(m.outputs[name]), nil
	}
	return []byte(m.outputs[name]), nil, nil
}

const sampleMdstat = `Personalities : [raid1] [linear] [multipath] [raid0]
md0 : active raid1 sda1[0] sdb1[2](F)
      1023960 blocks super 1.2 [2/1] [U_]

md1 : active raid5 sdd1[0] sde1[1] sdf1[3] sdg1[4]
      2095841280 blocks super 1.2 level 5, 512k chunk, algorithm 2 [4/4] [UUUU]
      [=====>...............]  resync = 27.5% (144021504/523960320) finish=120.5min speed=52608K/sec

md2 : active (auto-read-only) raid1 sdh1[0] sdi1[1]
      8380416 blocks super 1.2 [2/2] [UU]

unused devices: <none>
`

const sampleZpoolList = `tank	21990232555520	10995116277760	ONLINE
backup	1099511627776	109951162777	DEGRADED
`

const sampleVgs = `vg0 2199023255552 1099511627776
`

const sampleDf = `Filesystem     1K-blocks      Used Available Capacity Mounted on
/dev/sda1        40831236  12231368  26463228      32% /
tmpfs             8192000         0   8192000       0% /dev/shm
overlay          40831236  12231368  26463228      32% /var/lib/docker/overlay2/abc/merged
//nas.local/media 102396000 12231368 90000000    12% /mnt/remote
/dev/sdb1      1953512264 1000203020 953309244      52% /mnt/data
/dev/sdb1      1953512264 1000203020 953309244      52% /mnt/data/bind-dup
/dev/loop0           63472     63472         0     100% /snap/core/123
`

const sampleTestparm = `Load smb config files from /etc/samba/smb.conf
Loaded services file OK.

[global]
	server string = %h server

[media]
	path = /mnt/data/media
	comment = Media Library

[public]
	path = /mnt/data/public
`

const sampleExportfs = `/srv/nfs/media	192.168.1.0/24(rw,async,wdelay, insecure)
/srv/nfs/backup	10.0.0.5(ro,async)
`

func newNasTestProvider(t *testing.T, outputs map[string]string, mdstat string, failBin string) (*NasProvider, *mockNasRunner) {
	m := &mockNasRunner{outputs: outputs, failBin: failBin}
	// 桩掉工具探测与 mdstat 路径：本机装没装 zpool/samba 不影响用例
	origLook, origMd := nasLookPath, nasMdstatPath
	t.Cleanup(func() { nasLookPath = origLook; nasMdstatPath = origMd })
	nasLookPath = func(name string) (string, error) { return "/usr/sbin/" + name, nil }
	nasMdstatPath = "/proc/mdstat.test"
	p := NewNasProvider(m.run)
	p.readFile = func(path string) ([]byte, error) {
		if path == nasMdstatPath {
			return []byte(mdstat), nil
		}
		return nil, errors.New("not found")
	}
	return p, m
}

func TestNasSnapshotMerge(t *testing.T) {
	outputs := map[string]string{
		"zpool":    sampleZpoolList,
		"vgs":      sampleVgs,
		"df":       sampleDf,
		"testparm": sampleTestparm,
		"exportfs": sampleExportfs,
	}
	p, _ := newNasTestProvider(t, outputs, sampleMdstat, "")

	res, err := p.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	snap := res.(map[string]interface{})
	if !snap["available"].(bool) || snap["source"] != "linux" {
		t.Fatalf("header = %+v", snap)
	}

	pools := snap["pools"].([]map[string]interface{})
	if len(pools) != 6 { // md0 md1 md2 + tank backup + vg0
		t.Fatalf("pools = %d, want 6: %+v", len(pools), pools)
	}
	byName := map[string]map[string]interface{}{}
	for _, pool := range pools {
		byName[pool["name"].(string)] = pool
	}

	// md0：成员 (F) 失败盘 + 位图缺 U → degraded，设备名剥离 [n]/(F)
	md0 := byName["md0"]
	if md0["state"] != "degraded" {
		t.Errorf("md0 state = %v", md0["state"])
	}
	devices := fmt.Sprint(md0["devices"])
	if !strings.Contains(devices, "sda1") || !strings.Contains(devices, "sdb1") || strings.Contains(devices, "(F)") {
		t.Errorf("md0 devices = %s", devices)
	}
	// 容量：1023960k blocks ≈ 1.05GB
	if md0["totalGB"].(float64) < 1.0 || md0["totalGB"].(float64) > 1.1 {
		t.Errorf("md0 totalGB = %v", md0["totalGB"])
	}

	// md1：resync 进度行 → resync + detail 保留进度
	md1 := byName["md1"]
	if md1["state"] != "resync" || !strings.Contains(fmt.Sprint(md1["detail"]), "27.5%") {
		t.Errorf("md1 = %+v", md1)
	}

	// md2：[UU] 完整、无失败盘 → healthy
	if md2 := byName["md2"]; md2["state"] != "healthy" {
		t.Errorf("md2 state = %v", md2["state"])
	}

	// zfs：ONLINE → healthy、DEGRADED 保留原词小写、字节换算
	if tank := byName["tank"]; tank["state"] != "healthy" || tank["totalGB"].(float64) < 21990 || tank["usedGB"].(float64) < 10995 {
		t.Errorf("tank = %+v", tank)
	}
	if bk := byName["backup"]; bk["state"] != "degraded" {
		t.Errorf("backup state = %v", bk["state"])
	}

	// mounts：过滤 tmpfs/overlay/<1GB loop/网络挂载 //；bind mount 去重
	mounts := snap["mounts"].([]map[string]interface{})
	if len(mounts) != 2 { // / 与 /mnt/data（去重后）
		t.Fatalf("mounts = %d: %+v", len(mounts), mounts)
	}
	byPath := map[string]map[string]interface{}{}
	for _, m := range mounts {
		byPath[m["mountPath"].(string)] = m
	}
	if m := byPath["/mnt/data"]; m == nil || m["totalGB"].(float64) < 1953 || m["usedGB"].(float64) < 1000 {
		t.Errorf("/mnt/data = %+v", m)
	}
	for path := range byPath {
		if strings.HasPrefix(path, "/var/lib/docker") || path == "/mnt/remote" || path == "/snap/core/123" {
			t.Errorf("pseudo/remote/small mount leaked: %s", path)
		}
	}

	// shares：SMB 两段（global 跳过）+ NFS 两条
	shares := snap["shares"].([]map[string]interface{})
	if len(shares) != 4 {
		t.Fatalf("shares = %d: %+v", len(shares), shares)
	}
	smbCount, nfsWithHosts := 0, 0
	for _, s := range shares {
		switch s["protocol"] {
		case "smb":
			smbCount++
		case "nfs":
			if s["hosts"] == "192.168.1.0/24" {
				nfsWithHosts++
			}
		}
	}
	if smbCount != 2 || nfsWithHosts != 1 {
		t.Errorf("smb = %d, nfs-with-hosts = %d", smbCount, nfsWithHosts)
	}
}

func TestNasSingleSourceFailureDegrades(t *testing.T) {
	outputs := map[string]string{
		"zpool": sampleZpoolList, "vgs": sampleVgs, "df": sampleDf,
		"testparm": sampleTestparm, "exportfs": sampleExportfs,
	}
	// zpool 注入失败：池少两个但其余源（md 3 个 + lvm 1 个）照常
	p, _ := newNasTestProvider(t, outputs, sampleMdstat, "zpool")
	res, _ := p.Snapshot()
	snap := res.(map[string]interface{})
	pools := snap["pools"].([]map[string]interface{})
	if len(pools) != 4 {
		t.Errorf("pools after zpool failure = %d, want 4 (md+lvm)", len(pools))
	}
	if !snap["available"].(bool) {
		t.Error("available should stay true")
	}
}

func TestNasAllSourcesMissing(t *testing.T) {
	outputs := map[string]string{"zpool": "", "vgs": "", "df": "Filesystem 1K-blocks Used Available Capacity Mounted on\n", "testparm": "", "exportfs": ""}
	p, _ := newNasTestProvider(t, outputs, "", "")
	res, err := p.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	snap := res.(map[string]interface{})
	if snap["available"].(bool) {
		t.Error("available should be false when all sources empty")
	}
}

func TestNasCallUnknownAction(t *testing.T) {
	p := NewNasProvider(nil)
	if _, err := p.Call("reboot", nil); err == nil {
		t.Error("unknown action should fail")
	}
}

func TestDetectNasToolScan(t *testing.T) {
	// 桩替换 LookPath 与 mdstat 路径：任一工具存在即检出
	origLook, origMd := nasLookPath, nasMdstatPath
	t.Cleanup(func() { nasLookPath = origLook; nasMdstatPath = origMd })

	nasMdstatPath = "/proc/mdstat.does-not-exist"
	nasLookPath = func(string) (string, error) { return "", errors.New("not found") }
	if DetectNas() {
		t.Error("no tools should not detect")
	}
	nasLookPath = func(name string) (string, error) {
		if name == "zpool" {
			return "/usr/sbin/zpool", nil
		}
		return "", errors.New("not found")
	}
	if !DetectNas() {
		t.Error("zpool present should detect")
	}
}
