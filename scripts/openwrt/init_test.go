package openwrt

// init_test.go OpenWrt 片D Go 门禁：锁住自举节点的工件契约，防止夜线产物退化。
// 设计 docs/design/oob-openwrt-bootstrap.md §5/§6 Slice D —— procd init 脚本须
// 显式 respawn 看门狗参数（threshold/retries/cooldown），且 ipk 工件必须含
// init/conf/agent 二进制并 postinst 自启用。真机断电恢复验收仍待真机（设计
// 分「Go gate + on-machine acceptance」，本测试只管 Go 侧可验的契约）。

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// readArtifact 读 CWD（=scripts/openwrt/）下的工件文件。
func readArtifact(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

// TestInitScriptRespawnExplicit procd init 脚本：procd 服务、自启级别、显式
// respawn 三参数（设计 §5 明示 threshold/retries/cooldown，不得回退默认）。
func TestInitScriptRespawnExplicit(t *testing.T) {
	init := readArtifact(t, "cockpit-agent.init")
	if !strings.Contains(init, "USE_PROCD=1") {
		t.Fatal("init must declare USE_PROCD=1 (procd service)")
	}
	if !strings.Contains(init, "START=99") || !strings.Contains(init, "STOP=15") {
		t.Fatal("init must declare boot priority START=99 / STOP=15")
	}
	if !strings.Contains(init, "procd_open_instance") || !strings.Contains(init, "procd_close_instance") {
		t.Fatal("init must open/close a procd instance")
	}
	re := regexp.MustCompile(`procd_set_param respawn\s+(\d+)\s+(\d+)\s+(\d+)`)
	m := re.FindStringSubmatch(init)
	if m == nil {
		t.Fatal("respawn must be explicit with three numeric params: procd_set_param respawn <threshold> <timeout> <retry>")
	}
	threshold, _ := strconv.Atoi(m[1])
	timeout, _ := strconv.Atoi(m[2])
	retry, _ := strconv.Atoi(m[3])
	if threshold < 60 {
		t.Fatalf("respawn threshold must be >=60s (a crash loop must not reset counters quickly), got %d", threshold)
	}
	if timeout < 1 || timeout > 300 {
		t.Fatalf("respawn cooldown timeout must be 1..300s, got %d", timeout)
	}
	if retry < 1 {
		t.Fatalf("respawn retry must be >=1 (bounded crash-loop protection), got %d", retry)
	}
	if !strings.Contains(init, "/usr/bin/cockpit-agent") {
		t.Fatal("init must exec the agent from /usr/bin/cockpit-agent")
	}
}

// TestInitScriptUCIMapping init 读取的 UCI 字段与 conf 声明的 option 对齐，
// 防止脚本引用了配置里不存在的键（静默空值 = 该参丢失）。
func TestInitScriptUCIMapping(t *testing.T) {
	init := readArtifact(t, "cockpit-agent.init")
	conf := readArtifact(t, "cockpit-agent.conf")
	for _, field := range []string{"server", "secret", "id", "region", "zone", "labels"} {
		// config_get <var> main <option>：option 名是第三 token（变量名可不同）。
		re := regexp.MustCompile(`config_get\s+\w+\s+main\s+` + regexp.QuoteMeta(field) + `\b`)
		if !re.MatchString(init) {
			t.Errorf("init must read uci option %q via config_get <var> main %q", field, field)
		}
		if !strings.Contains(conf, "option "+field) {
			t.Errorf("conf must declare uci option %q", field)
		}
	}
	if !strings.Contains(init, "config_load cockpit-agent") {
		t.Fatal("init must config_load the cockpit-agent uci package")
	}
}

// TestPackageIPKWiring package-ipk.sh 工件接线：init→/etc/init.d/、
// conf→/etc/config/、agent→/usr/bin/，postinst 自启用，conffiles 保配置。
func TestPackageIPKWiring(t *testing.T) {
	pkg := readArtifact(t, "../package-ipk.sh")
	for _, want := range []string{
		"data/usr/bin/cockpit-agent",
		"data/etc/init.d/cockpit-agent",
		"data/etc/config/cockpit-agent",
		"/etc/init.d/cockpit-agent enable",
		"/etc/config/cockpit-agent", // conffiles 升级保留
	} {
		if !strings.Contains(pkg, want) {
			t.Errorf("package-ipk.sh must wire %q into the artifact", want)
		}
	}
	if !strings.Contains(pkg, "ar rc") || !strings.Contains(pkg, "debian-binary") {
		t.Fatal("package-ipk.sh must build a proper ar ipk (debian-binary first member)")
	}
}

// TestIPKArtifactEndToEnd 端到端（linux-only，CI=ubuntu）：真实跑
// package-ipk.sh，验证 ipk ar 结构与 tar.gz 载荷——产物回归在夜线断供前被拦截。
func TestIPKArtifactEndToEnd(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("ipk packaging requires linux toolchain (ar/tar/install)")
	}
	if _, err := exec.LookPath("ar"); err != nil {
		t.Skip("ar not installed")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "agent")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	cmd := exec.Command("bash", filepath.Join("..", "package-ipk.sh"), binary, "mipsel", "0.0.0-test", out)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("package-ipk.sh: %v\n%s", err, output)
	}
	ipk := filepath.Join(out, "cockpit-agent_0.0.0-test_mipsel.ipk")
	tgz := filepath.Join(out, "cockpit-agent-openwrt-mipsel.tar.gz")

	assertTarGzPayload(t, tgz)

	f, err := os.Open(ipk)
	if err != nil {
		t.Fatalf("open ipk: %v", err)
	}
	defer f.Close()
	magic := make([]byte, 8)
	if _, err := io.ReadFull(f, magic); err != nil {
		t.Fatalf("read ipk magic: %v", err)
	}
	if string(magic) != "!<arch>\n" {
		t.Fatalf("ipk is not an ar archive, magic=%q", magic)
	}
	var names []string
	var payloads = map[string][]byte{}
	for {
		hdr := make([]byte, 60)
		if _, err := io.ReadFull(f, hdr); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("read ar header: %v", err)
		}
		name := strings.TrimRight(strings.TrimSpace(string(hdr[0:16])), "/")
		size, err := strconv.Atoi(strings.TrimSpace(string(hdr[48:58])))
		if err != nil {
			t.Fatalf("ar member %q bad size field: %v", name, err)
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(f, payload); err != nil {
			t.Fatalf("read ar member %q: %v", name, err)
		}
		names = append(names, name)
		payloads[name] = payload
		if size%2 != 0 { // ar 成员按偶数字节对齐
			io.CopyN(io.Discard, f, 1)
		}
	}
	want := []string{"debian-binary", "control.tar.gz", "data.tar.gz"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("ipk members = %v, want %v", names, want)
	}
	if string(payloads["debian-binary"]) != "2.0\n" {
		t.Fatalf("debian-binary = %q, want 2.0", payloads["debian-binary"])
	}
}

// assertTarGzPayload tar.gz 手动分发：含 agent 二进制（可执行）、init（显式
// respawn）、conf；缺一即产物断供被拦。
func assertTarGzPayload(t *testing.T, tgz string) {
	t.Helper()
	f, err := os.Open(tgz)
	if err != nil {
		t.Fatalf("open tar.gz: %v", err)
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	tr := tar.NewReader(gr)
	found := map[string]string{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar: %v", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read %s: %v", hdr.Name, err)
		}
		// tar -C ... . 生成 ./ 前缀条目，归一后再比对。
		found[strings.TrimPrefix(hdr.Name, "./")] = string(body)
	}
	if _, ok := found["usr/bin/cockpit-agent"]; !ok {
		t.Fatal("tar.gz missing usr/bin/cockpit-agent")
	}
	init, ok := found["etc/init.d/cockpit-agent"]
	if !ok {
		t.Fatal("tar.gz missing etc/init.d/cockpit-agent")
	}
	if !strings.Contains(init, "procd_set_param respawn 3600 5 5") {
		t.Fatalf("packaged init must carry explicit respawn, got:\n%s", init)
	}
	if _, ok := found["etc/config/cockpit-agent"]; !ok {
		t.Fatal("tar.gz missing etc/config/cockpit-agent")
	}
}
