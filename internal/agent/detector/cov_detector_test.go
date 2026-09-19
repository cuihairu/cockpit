package detector

// cov_* 测试：覆盖率攻坚新增（不修改既有测试文件）。
// 通过包级路径变量的注入（默认值即生产路径）覆盖原本依赖真实
// /etc、/proc、/sys、/dev、/bin 环境的分支。

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// covSetStr 测试期内替换字符串型包级变量，结束时恢复。
func covSetStr(t *testing.T, dst *string, val string) {
	t.Helper()
	old := *dst
	*dst = val
	t.Cleanup(func() { *dst = old })
}

// covSetStrs 测试期内替换 []string 型包级变量，结束时恢复。
func covSetStrs(t *testing.T, dst *[]string, val []string) {
	t.Helper()
	old := *dst
	*dst = val
	t.Cleanup(func() { *dst = old })
}

// covWriteFile 在临时目录写文件并返回路径。
func covWriteFile(t *testing.T, name, content string) string {
	t.Helper()
	fp := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(fp, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", fp, err)
	}
	return fp
}

// covListenUnix 在临时目录起一个 unix socket listener。
func covListenUnix(t *testing.T, name string) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), name)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	return sock
}

// covEmptyPATH 把 PATH 指向空目录，使所有 LookPath 失败（跨环境确定）。
func covEmptyPATH(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

// ============ docker.go ============

func TestCovNormalizeUnixHostRelativePath(t *testing.T) {
	// unix:// 前缀 + 相对路径：规范化为根路径
	path, endpoint := normalizeUnixHost("unix://var/run/docker.sock")
	if path != "/var/run/docker.sock" {
		t.Errorf("path = %q, want /var/run/docker.sock", path)
	}
	if endpoint != "unix:///var/run/docker.sock" {
		t.Errorf("endpoint = %q, want unix:///var/run/docker.sock", endpoint)
	}
}

func TestCovDockerTestSocketNonSocketFile(t *testing.T) {
	d := &DockerDetector{}
	// 普通文件存在但不是 socket：stat 通过、Dial 失败
	fp := covWriteFile(t, "plain.txt", "hi")
	if d.testSocket(fp) {
		t.Error("testSocket should fail for a regular file")
	}
}

func TestCovDockerDetectViaCandidateSocket(t *testing.T) {
	sock := covListenUnix(t, "docker.sock")
	t.Setenv("DOCKER_HOST", "")
	covSetStrs(t, &dockerSocketCandidates, []string{sock})

	d := &DockerDetector{}
	cap, err := d.Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if cap == nil {
		t.Fatal("Detect() should report capability via candidate socket")
	}
	if cap.Type != "docker-api" || cap.Endpoint != "unix://"+sock {
		t.Errorf("capability = %+v", cap)
	}
}

// ============ hardware.go ============

func TestCovHardwareTempViaThermalZone(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "thermal_zone0"), 0755); err != nil {
		t.Fatal(err)
	}
	covSetStr(t, &thermalSysPath, dir)
	covEmptyPATH(t) // 确保走 thermal_zone 分支而非 sensors

	d := &HardwareDetector{}
	if !d.hasTempSensors() {
		t.Error("hasTempSensors should be true with thermal_zone0")
	}
}

func TestCovHardwareUPSViaApcaccess(t *testing.T) {
	fakeBinDir(t, map[string]string{"apcaccess": "exit 0"})
	d := &HardwareDetector{}
	if !d.hasUPS() {
		t.Error("hasUPS should be true with stubbed apcaccess")
	}
}

func TestCovHardwareUPSViaUsbDev(t *testing.T) {
	covEmptyPATH(t) // apcaccess/upsc 都不可见

	// 目录里没有匹配项：遍历后返回 false
	emptyDir := t.TempDir()
	covSetStr(t, &usbDevPath, emptyDir)
	d := &HardwareDetector{}
	if d.hasUPS() {
		t.Error("hasUPS should be false for empty usb dir")
	}

	// hiddev 设备：返回 true
	if err := os.WriteFile(filepath.Join(emptyDir, "hiddev0"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if !d.hasUPS() {
		t.Error("hasUPS should be true with hiddev device")
	}
}

func TestCovGetDisksPathMissing(t *testing.T) {
	covSetStr(t, &sysBlockPath, filepath.Join(t.TempDir(), "no-such-block"))
	if _, err := GetDisks(); err == nil {
		t.Error("GetDisks() should fail when block path is missing")
	}
}

// ============ openwrt.go ============

func TestCovOpenWrtDetectViaRelease(t *testing.T) {
	covSetStr(t, &openwrtReleasePath, covWriteFile(t, "openwrt_release",
		`DISTRIB_ID="OpenWrt"
DISTRIB_RELEASE="23.05.0"
DISTRIB_TARGET="x86/64"
malformed line without equals
`))
	d := &OpenWrtDetector{}
	cap, err := d.Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if cap == nil || cap.Type != "openwrt" {
		t.Fatalf("capability = %+v, want openwrt", cap)
	}
	if cap.Metadata["DISTRIB_ID"] != "OpenWrt" {
		t.Errorf("DISTRIB_ID = %v, want OpenWrt", cap.Metadata["DISTRIB_ID"])
	}
	if cap.Metadata["DISTRIB_RELEASE"] != "23.05.0" {
		t.Errorf("DISTRIB_RELEASE = %v, want 23.05.0", cap.Metadata["DISTRIB_RELEASE"])
	}
	if cap.Metadata["DISTRIB_TARGET"] != "x86/64" {
		t.Errorf("DISTRIB_TARGET = %v, want x86/64", cap.Metadata["DISTRIB_TARGET"])
	}
}

func TestCovOpenWrtDetectViaUbus(t *testing.T) {
	covSetStr(t, &openwrtReleasePath, filepath.Join(t.TempDir(), "no-release"))
	covSetStr(t, &ubusBinPath, covWriteFile(t, "ubus", ""))
	covSetStr(t, &opkgBinPath, filepath.Join(t.TempDir(), "no-opkg"))

	d := &OpenWrtDetector{}
	cap, err := d.Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if cap == nil || cap.Metadata["detection"] != "ubus" {
		t.Fatalf("capability = %+v, want detection=ubus", cap)
	}
}

func TestCovOpenWrtDetectViaOpkg(t *testing.T) {
	covSetStr(t, &openwrtReleasePath, filepath.Join(t.TempDir(), "no-release"))
	covSetStr(t, &ubusBinPath, filepath.Join(t.TempDir(), "no-ubus"))
	covSetStr(t, &opkgBinPath, covWriteFile(t, "opkg", ""))

	d := &OpenWrtDetector{}
	cap, err := d.Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if cap == nil || cap.Metadata["detection"] != "opkg" {
		t.Fatalf("capability = %+v, want detection=opkg", cap)
	}
}

func TestCovOpenWrtGetSystemInfoWithStubUbus(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "ubus")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nprintf '{\"uptime\":123}'"), 0755); err != nil {
		t.Fatal(err)
	}
	covSetStr(t, &ubusBinPath, stub)

	info, err := GetSystemInfo()
	if err != nil {
		t.Fatalf("GetSystemInfo() error = %v", err)
	}
	if info["raw_output"] != `{"uptime":123}` {
		t.Errorf("raw_output = %v", info["raw_output"])
	}
}

// ============ pve.go ============

func TestCovPVEDetectViaDefaultEndpoints(t *testing.T) {
	// 候选地址注入为本地 TLS 服务（客户端 InsecureSkipVerify）
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("PVE_URL", "")
	covSetStrs(t, &pveDefaultEndpoints, []string{srv.URL})

	d := &PVEDetector{}
	cap, err := d.Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if cap == nil {
		t.Fatal("Detect() should report capability via default endpoints")
	}
	if cap.Endpoint != srv.URL || cap.Version != "detected" {
		t.Errorf("capability = %+v", cap)
	}
}
