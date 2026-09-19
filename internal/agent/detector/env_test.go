package detector

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// ============ Docker detector ============

func TestDockerTestSocket(t *testing.T) {
	d := &DockerDetector{}

	if d.testSocket(filepath.Join(t.TempDir(), "missing.sock")) {
		t.Error("testSocket should fail for a missing path")
	}

	// A real listening unix socket must pass
	sockPath := filepath.Join(t.TempDir(), "docker.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	defer ln.Close()
	if !d.testSocket(sockPath) {
		t.Error("testSocket should succeed for a listening socket")
	}
}

func TestDockerDetectWithEnvSocket(t *testing.T) {
	d := &DockerDetector{}

	sockPath := filepath.Join(t.TempDir(), "docker.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	defer ln.Close()

	t.Setenv("DOCKER_HOST", sockPath)
	cap, err := d.Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if cap == nil {
		t.Fatal("Detect() should report capability for reachable socket")
	}
	// 裸 socket 路径规范化为 unix:// endpoint（docker 库 WithHost 要求 scheme）
	if cap.Type != "docker-api" || cap.Endpoint != "unix://"+sockPath {
		t.Errorf("capability = %+v", cap)
	}
}

func TestDockerDetectWithUnixSchemeEnv(t *testing.T) {
	// docker 官方惯例格式 DOCKER_HOST=unix:///path 之前无法检出
	// （testSocket 直接 stat 带前缀的字符串），这里固定该行为。
	d := &DockerDetector{}

	sockPath := filepath.Join(t.TempDir(), "docker.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	defer ln.Close()

	t.Setenv("DOCKER_HOST", "unix://"+sockPath)
	cap, err := d.Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if cap == nil {
		t.Fatal("Detect() should report capability for reachable unix:// socket")
	}
	if cap.Type != "docker-api" || cap.Endpoint != "unix://"+sockPath {
		t.Errorf("capability = %+v", cap)
	}
}

func TestDockerDetectWithUnreachableEnvSocket(t *testing.T) {
	d := &DockerDetector{}
	t.Setenv("DOCKER_HOST", filepath.Join(t.TempDir(), "nope.sock"))

	cap, err := d.Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if cap != nil {
		t.Errorf("Detect() should return nil for unreachable socket, got %+v", cap)
	}
}

// ============ PVE detector ============

func TestPVEDetectWithEnvURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/version" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":{"version":"8.2.4"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	d := &PVEDetector{}
	t.Setenv("PVE_URL", srv.URL)

	cap, err := d.Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if cap == nil {
		t.Fatal("Detect() should report capability for reachable API")
	}
	if cap.Endpoint != srv.URL {
		t.Errorf("Endpoint = %q, want %q", cap.Endpoint, srv.URL)
	}
	if cap.Version != "8.2.4" {
		t.Errorf("Version = %q, want 8.2.4", cap.Version)
	}
}

func TestPVEDetectWithUnreachableURL(t *testing.T) {
	d := &PVEDetector{}
	t.Setenv("PVE_URL", "http://127.0.0.1:1")

	cap, err := d.Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if cap != nil {
		t.Errorf("Detect() should return nil for unreachable API, got %+v", cap)
	}
}

func TestPVEGetVersionBadPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	d := &PVEDetector{}
	if v := d.getVersion(srv.URL); v != "unknown" {
		t.Errorf("getVersion() = %q, want unknown for bad payload", v)
	}
}

func TestPVEGetVersionUnreachable(t *testing.T) {
	d := &PVEDetector{}
	if v := d.getVersion("http://127.0.0.1:1"); v != "unknown" {
		t.Errorf("getVersion() = %q, want unknown for unreachable host", v)
	}
}

// ============ Remote service detector ============

func TestRemoteServiceDetectViaFTPPort(t *testing.T) {
	// 2121 is a non-privileged FTP fallback port checked by the detector
	ln, err := net.Listen("tcp", "127.0.0.1:2121")
	if err != nil {
		t.Skipf("cannot bind 127.0.0.1:2121: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	d := NewRemoteServiceDetector()
	cap, err := d.Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if cap == nil {
		t.Fatal("Detect() should find the listening FTP fallback port")
	}
	if cap.Type != "remote-services" {
		t.Errorf("Type = %q, want remote-services", cap.Type)
	}
	ftp, ok := cap.Metadata["ftp"].(map[string]interface{})
	if !ok {
		t.Fatalf("metadata missing ftp entry: %v", cap.Metadata)
	}
	var ftpPort int
	switch v := ftp["port"].(type) {
	case int:
		ftpPort = v
	case float64:
		ftpPort = int(v)
	default:
		t.Fatalf("unexpected port type %T", ftp["port"])
	}
	if ftpPort != 2121 {
		t.Errorf("ftp port = %d, want 2121", ftpPort)
	}
}

func TestRemoteServiceScanHost(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	d := NewRemoteServiceDetector()
	if !d.ScanHost("127.0.0.1", port) {
		t.Error("ScanHost should find the open port")
	}
	if d.ScanHost("127.0.0.1", 1) {
		t.Error("ScanHost should not report closed port as open")
	}
}

func TestRemoteServiceGetServiceName(t *testing.T) {
	d := NewRemoteServiceDetector()
	tests := map[protocol.RemoteProtocol]string{
		protocol.RemoteProtocolSSH:    "SSH Server",
		protocol.RemoteProtocolRDP:    "RDP Server",
		protocol.RemoteProtocolVNC:    "VNC Server",
		protocol.RemoteProtocolTelnet: "Telnet Server",
		protocol.RemoteProtocolFTP:    "FTP Server",
		protocol.RemoteProtocol("x"):  "x",
	}
	for proto, want := range tests {
		if got := d.getServiceName(proto); got != want {
			t.Errorf("getServiceName(%q) = %q, want %q", proto, got, want)
		}
	}
}

func TestGetRemoteCapabilityFromDetected(t *testing.T) {
	cap := &protocol.Capability{
		Type: "remote-services",
		Metadata: map[string]interface{}{
			"ssh": map[string]interface{}{"running": true, "host": "127.0.0.1", "port": float64(22)},
			"rdp": map[string]interface{}{"running": false, "host": "127.0.0.1", "port": float64(3389)},
			"vnc": map[string]interface{}{"running": true, "host": "127.0.0.1", "port": float64(5900)},
		},
	}
	info := GetRemoteCapability(*cap)
	if info == nil {
		t.Fatal("GetRemoteCapability() should parse remote-services capability")
	}
	if info.SSH == nil || !info.SSH.Enabled || info.SSH.Port != 22 {
		t.Errorf("SSH info = %+v", info.SSH)
	}
	if info.RDP != nil {
		t.Errorf("non-running rdp should be skipped, got %+v", info.RDP)
	}
	if info.VNC == nil || info.VNC.Port != 5900 {
		t.Errorf("VNC info = %+v", info.VNC)
	}
}

func TestGetRemoteCapabilityWrongType(t *testing.T) {
	if info := GetRemoteCapability(protocol.Capability{Type: "docker-api"}); info != nil {
		t.Errorf("GetRemoteCapability() should return nil for other types, got %+v", info)
	}
}

// ============ Hardware detector helpers ============

func TestHardwareDetectorHelpers(t *testing.T) {
	d := &HardwareDetector{}
	_ = d.hasSmartctl()
	_ = d.hasTempSensors()
	_ = d.hasUPS()

	disks, err := GetDisks()
	if err != nil {
		t.Fatalf("GetDisks() error = %v", err)
	}
	// On Linux CI hosts /sys/block exists; entries must look like /dev/xxx
	for _, disk := range disks {
		if len(disk) == 0 || disk[0] != '/' {
			t.Errorf("disk entry %q does not look like a device path", disk)
		}
	}
}

// ============ OpenWrt detector ============

func TestOpenWrtGetSystemInfoWithoutUbus(t *testing.T) {
	if _, err := os.Stat("/bin/ubus"); err == nil {
		t.Skip("/bin/ubus exists; environment-specific behavior")
	}
	if _, err := GetSystemInfo(); err == nil {
		t.Error("GetSystemInfo() should fail when /bin/ubus is missing")
	}
}

// ============ Detector registry ============

func TestRegisteredDetectorsIncludeCoreOnes(t *testing.T) {
	want := map[string]bool{
		"docker-api": false, "pve-api": false, "openwrt": false,
		"overlay": false, "hardware-monitor": false,
	}
	for _, d := range All() {
		name := d.Name()
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for n, found := range want {
		if !found {
			t.Errorf("detector %q not registered", n)
		}
	}
}

// fakeBinDir creates a temp dir with executable stub scripts and prepends it
// to PATH so environment-dependent lookups succeed deterministically.
func fakeBinDir(t *testing.T, scripts map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range scripts {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0755); err != nil {
			t.Fatalf("write stub %s: %v", name, err)
		}
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}

func TestHardwareDetectWithStubTools(t *testing.T) {
	fakeBinDir(t, map[string]string{
		"smartctl": "exit 0",
		"sensors":  "exit 0",
		"upsc":     "exit 0",
	})

	d := &HardwareDetector{}
	if !d.hasSmartctl() {
		t.Error("hasSmartctl should be true with stubbed smartctl")
	}
	if !d.hasUPS() {
		t.Error("hasUPS should be true with stubbed upsc")
	}

	cap, err := d.Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if cap == nil {
		t.Fatal("Detect() should report hardware capability with stub tools")
	}
	meta := cap.Metadata
	if meta["smart"] != true {
		t.Errorf("smart feature = %v, want true", meta["smart"])
	}
	if meta["ups"] != true {
		t.Errorf("ups feature = %v, want true", meta["ups"])
	}
}

func TestHardwareSmartctlFailsToRun(t *testing.T) {
	fakeBinDir(t, map[string]string{"smartctl": "exit 1"})
	d := &HardwareDetector{}
	if d.hasSmartctl() {
		t.Error("hasSmartctl should be false when smartctl exits non-zero")
	}
}

func TestHardwareTempSensorsWithStub(t *testing.T) {
	fakeBinDir(t, map[string]string{"sensors": "exit 0"})
	d := &HardwareDetector{}
	if !d.hasTempSensors() {
		t.Error("hasTempSensors should be true with stubbed sensors")
	}
}

func TestHardwareTempSensorsFails(t *testing.T) {
	fakeBinDir(t, map[string]string{"sensors": "exit 1"})
	d := &HardwareDetector{}
	// /sys/class/thermal may still exist on the host, so only check it does not panic
	_ = d.hasTempSensors()
}

func TestHardwareDetectWithoutTools(t *testing.T) {
	// Empty PATH: no tool can be found
	t.Setenv("PATH", t.TempDir())
	d := &HardwareDetector{}
	cap, err := d.Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if cap != nil {
		t.Errorf("Detect() should return nil without tools and thermal dir, got %+v", cap)
	}
}

var _ = time.Second
