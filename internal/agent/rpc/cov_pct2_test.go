package rpc

// cov_pct2_test.go 覆盖率冲刺（file / nas / drift / service / nginx）：
// file 侧用权限位、sysfs 空读属性、FIFO、条目上限触发错误分支；service 侧用
// RLIMIT_FSIZE 触发临时文件写失败；nas 侧用 httptest 伪装 DSM/TrueNAS 与
// nasMdstatPath var 注入。所有环境依赖分支（root、sysfs、rlimit）先探测后
// Skip，不引入 flaky。

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// TestCovPctFileListLstatErr file.list 条目 lstat 失败分支（L113）：
// 目录去 x 位后 ReadDir 仍可列名，但 lstat(dir/entry) EACCES → continue 跳过。
// root 不理会权限位，跳过。
func TestCovPctFileListLstatErr(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root 不受目录 x 位限制，无法触发 lstat 失败")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o400); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o700) }()

	p := NewFileProvider()
	res, err := p.List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	m, _ := res.(map[string]interface{})
	entries, _ := m["entries"].([]map[string]interface{})
	if len(entries) != 0 {
		t.Fatalf("lstat 失败的条目应被跳过, got %d", len(entries))
	}
}

// TestCovPctFileReadEmptySysfs file.read 的 Read 错误且 n==0 分支（L180）：
// sysfs 属性 st_size 恒为 4096 但内容为空 → offset(0)<total 成立 → Read
// 返回 (0, EOF) → 报 read 错误。运行时探测候选文件，环境不具备则 Skip。
func TestCovPctFileReadEmptySysfs(t *testing.T) {
	candidates := []string{
		"/sys/power/pm_trace_dev_match",
		"/sys/kernel/kexec/crash_cma_ranges",
		"/sys/devices/software/uevent",
		"/sys/devices/pnp0/uevent",
		"/sys/devices/isa/uevent",
	}
	target := ""
	for _, c := range candidates {
		info, err := os.Stat(c)
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			continue
		}
		f, err := os.Open(c)
		if err != nil {
			continue
		}
		var probe [1]byte
		n, rerr := f.Read(probe[:])
		f.Close()
		if rerr != nil && n == 0 { // 空读属性：立即 EOF
			target = c
			break
		}
	}
	if target == "" {
		t.Skip("未找到 size>0 但空读的 sysfs 属性")
	}

	p := NewFileProvider()
	if _, err := p.Read(map[string]interface{}{"path": target, "offset": float64(0)}); err == nil {
		t.Fatalf("%s 空读应报 read 错误", target)
	}
}

// TestCovPctFileWriteChmodDevNull file.write 显式 0600 的 f.Chmod 失败分支
// （L256）：/dev/null 为 root 属主且全局可写——打开成功但 fchmod 因非属主
// EPERM。root 下 chmod 会真的改设备权限，跳过。
func TestCovPctFileWriteChmodDevNull(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root 对 /dev/null fchmod 会成功并改动设备权限")
	}
	info, err := os.Lstat("/dev/null")
	if err != nil || !info.Mode().IsRegular() == false && info.Mode()&os.ModeDevice == 0 {
		t.Skip("/dev/null 不可用")
	}
	p := NewFileProvider()
	_, err = p.Write(map[string]interface{}{
		"path": "/dev/null", "data": "eA==", "truncate": true, "mode": float64(0o600),
	})
	if err == nil || !strings.Contains(err.Error(), "chmod") {
		t.Fatalf("非属主 fchmod 应报 chmod 错误, got %v", err)
	}
}

// TestCovPctFileChmodChownErr file.chown/chown 权限错误分支：
// chmod 对 root 属主的 /dev/null EPERM（L363）；非 root 对自有文件 chown
// 到其他 uid 恒 EPERM（L383）。root 下两条都会成功，跳过。
func TestCovPctFileChmodChownErr(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root 执行 chmod/chown 不会失败")
	}
	p := NewFileProvider()

	if _, err := p.Chmod(map[string]interface{}{"path": "/dev/null", "mode": float64(0o644)}); err == nil ||
		!strings.Contains(err.Error(), "chmod") {
		t.Fatalf("非属主 chmod 应报错, got %v", err)
	}

	own := filepath.Join(t.TempDir(), "own.txt")
	if err := os.WriteFile(own, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := p.Chown(map[string]interface{}{"path": own, "uid": float64(1), "gid": float64(1)})
	if err == nil || !strings.Contains(err.Error(), "chown") {
		t.Fatalf("非 root chown 到其他 uid 应报错, got %v", err)
	}
}

// TestCovPctFileSearchScannedLimit file.search 扫描文件数上限分支（L472）：
// 平铺 5010 个小文件，第 5001 个使 scanned 超限 → truncated 且 SkipAll
func TestCovPctFileSearchScannedLimit(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5010; i++ {
		name := filepath.Join(dir, fmt.Sprintf("f%04d.txt", i))
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p := NewFileProvider()
	res, err := p.Search(map[string]interface{}{"dir": dir, "query": "absent-needle"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	m, _ := res.(map[string]interface{})
	if m["truncated"] != true {
		t.Fatal("超上限应置 truncated")
	}
	if n, _ := m["scanned"].(int); n != 5001 {
		t.Fatalf("scanned 应停在 5001, got %v", m["scanned"])
	}
}

// TestCovPctSearchInFileFIFOSeekErr searchInFile 的 Seek 失败分支（L520）：
// FIFO 读端打开后写端立即关闭 → 首读 (0, EOF) 无 NUL → Seek(0,0) 对管道
// ESPIPE → 静默返回零值。FIFO 阻塞语义保证确定性，无睡眠时序。
func TestCovPctSearchInFileFIFOSeekErr(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skipf("mkfifo 不可用: %v", err)
	}
	go func() {
		if w, err := os.OpenFile(fifo, os.O_WRONLY, 0); err == nil {
			_ = w.Close() // 立即关闭：读端得到 EOF
		}
	}()
	matches, binarySkipped := searchInFile(fifo, "needle", false, 5)
	if matches != nil || binarySkipped {
		t.Fatalf("管道上 Seek 失败应返回零值, got %v %v", matches, binarySkipped)
	}
}

// TestCovPctDetectNasMdstatVar DetectNas 的 mdstat 存在分支（L118）：
// nasMdstatPath 为包级 var——指向临时文件后 Stat 成功即返回 true
func TestCovPctDetectNasMdstatVar(t *testing.T) {
	t.Setenv("COCKPIT_NAS_TARGETS", "")
	old := nasMdstatPath
	nasMdstatPath = filepath.Join(t.TempDir(), "mdstat")
	defer func() { nasMdstatPath = old }()
	if err := os.WriteFile(nasMdstatPath, []byte("Personalities : \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !DetectNas() {
		t.Fatal("mdstat 存在应判可用")
	}
}

// TestCovPctTruenasNonFatalGets truenasSnapshot 的 dataset/smb/nfs 拉取失败
// 不致命分支（L128/L132/L136）：pool 正常返回，其余端点 500 → 池保留，
// 挂载与共享为空，整体不出错
func TestCovPctTruenasNonFatalGets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2.0/pool" {
			_, _ = w.Write([]byte(`[{"name":"tank","status":"ONLINE","size":2000000000,"allocated":1000000}]`))
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	target := NasTarget{Name: "tn", Type: "truenas", Addr: srv.URL, Username: "u", Password: "p"}
	pools, mounts, shares, err := truenasSnapshot(context.Background(), target)
	if err != nil {
		t.Fatalf("dataset/共享失败不应致命: %v", err)
	}
	if len(pools) != 1 || pools[0].Name != "tank" || pools[0].State != "healthy" {
		t.Fatalf("池应正常解析, got %+v", pools)
	}
	if len(mounts) != 0 || len(shares) != 0 {
		t.Fatalf("失败的 dataset/共享应为空, got %d/%d", len(mounts), len(shares))
	}
}

// TestCovPctDSMEntryBadJSON dsmEntry 响应体 JSON 解码失败分支（L113）：
// httptest 返回 200 + 非 JSON 体
func TestCovPctDSMEntryBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	s := &nasDsmSession{target: NasTarget{Name: "dsm", Addr: srv.URL}, client: srv.Client()}
	if _, err := s.dsmEntry(context.Background(), url.Values{"api": {"X"}}); err == nil {
		t.Fatal("非 JSON 响应应报解码错误")
	}
}

// TestCovPctDriftTraefikReadErr drift.check 的 traefik 片段读取失败分支
// （L267）：目录内放指向不存在目标的符号链接——Glob 按名命中、ReadFile
// 跟随失败 ENOENT（root 下同样成立），该类产出 error 条目
func TestCovPctDriftTraefikReadErr(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(root, "gone-target"), filepath.Join(root, "cockpit-site-x.yml")); err != nil {
		t.Fatal(err)
	}
	p := NewDriftProvider(NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json")), DriftConfig{
		ConfDir:    t.TempDir(),
		DynamicDir: root,
		StacksDir:  t.TempDir(),
	})
	res, err := p.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	m, _ := res.(map[string]interface{})
	items, _ := m["items"].([]driftItem)
	for _, it := range items {
		if it.Kind == "traefik" && it.Status == "error" {
			return
		}
	}
	t.Fatalf("应产出 traefik error 条目, got %+v", items)
}

// TestCovPctPlistSkipErr parsePlistDaemon 的 dec.Skip() 失败分支（L130）：
// 值位置出现 array 后流被截断 → Skip 消耗到 EOF 报错
func TestCovPctPlistSkipErr(t *testing.T) {
	bad := []byte(`<plist><dict><key>K</key><array><string>unterminated`)
	if _, err := parsePlistDaemon(bad); err == nil {
		t.Fatal("截断的复杂值应报 Skip 错误")
	}
}

// TestCovPctAtomicWriteFileSizeLimit atomicWriteFile 的 tmp.Write 失败分支
// （L305）：把 RLIMIT_FSIZE 压到 1KB 后写入 4KB 数据 → EFBIG → 原子写失败
// 且目标不落盘；恢复 rlimit 后窗口外零影响。无法设置则 Skip。
func TestCovPctAtomicWriteFileSizeLimit(t *testing.T) {
	var old syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &old); err != nil {
		t.Skipf("getrlimit 不可用: %v", err)
	}
	if old.Max < 4096 {
		t.Skip("硬上限过小，无法安全收窄软限制")
	}
	lim := old
	lim.Cur = 1024
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &lim); err != nil {
		t.Skipf("setrlimit 不可用: %v", err)
	}
	defer func() { _ = syscall.Setrlimit(syscall.RLIMIT_FSIZE, &old) }()

	path := filepath.Join(t.TempDir(), "unit.conf")
	err := atomicWriteFile(path, make([]byte, 4096), 0o600)
	if err == nil || !strings.Contains(err.Error(), "file too large") {
		t.Fatalf("超 FSIZE 写入应报 file too large, got %v", err)
	}
	if _, serr := os.Lstat(path); serr == nil {
		t.Fatal("写失败后目标不应落盘")
	}
}

// TestCovPctDetectNginxENOEXEC DetectNginx 的执行失败且 stderr 为空分支
// （L175）：PATH 注入带执行位的非可执行格式文件——LookPath 通过、
// exec 报 ENOEXEC、stderr 缓冲为空（nil）→ 返回未安装
func TestCovPctDetectNginxENOEXEC(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "nginx")
	if err := os.WriteFile(bin, []byte("definitely not an executable\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	version, ok := DetectNginx()
	if ok || version != "" {
		t.Fatalf("ENOEXEC 应返回未安装, got %q %v", version, ok)
	}
}
