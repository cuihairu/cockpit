package rpc

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newFileTestProvider() *FileProvider { return NewFileProvider() }

func callFile(t *testing.T, p *FileProvider, action string, params map[string]interface{}) (interface{}, error) {
	t.Helper()
	return p.Call(action, params)
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFileList(t *testing.T) {
	p := newFileTestProvider()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "b.txt"), "x")
	mustWrite(t, filepath.Join(root, "a.txt"), "yy")
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.txt", filepath.Join(root, "z.link")); err != nil {
		t.Fatal(err)
	}

	res, err := callFile(t, p, "list", map[string]interface{}{"dir": root})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	m := res.(map[string]interface{})
	entries := m["entries"].([]map[string]interface{})
	if len(entries) != 4 {
		t.Fatalf("entries = %d, want 4", len(entries))
	}
	// 目录优先 + 名称排序：sub, a.txt, b.txt, z.link
	order := []string{}
	for _, e := range entries {
		order = append(order, e["name"].(string))
	}
	want := []string{"sub", "a.txt", "b.txt", "z.link"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
	// symlink 标注与 target
	var link map[string]interface{}
	for _, e := range entries {
		if e["name"] == "z.link" {
			link = e
		}
	}
	if link["isSymlink"] != true || link["target"] != "a.txt" {
		t.Fatalf("link entry = %v", link)
	}
	// symlink 的 Lstat size 是目标路径字符串长度（"a.txt" = 5）
	if link["isDir"] != false || link["size"] != int64(len("a.txt")) {
		t.Fatalf("link size/IsDir = %v", link)
	}
}

func TestFileReadChunked(t *testing.T) {
	p := newFileTestProvider()
	path := filepath.Join(t.TempDir(), "big.bin")
	want := make([]byte, fileReadChunkLimit+10)
	for i := range want {
		want[i] = byte(i % 249)
	}
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatal(err)
	}

	var got []byte
	offset := int64(0)
	for {
		res, err := callFile(t, p, "read", map[string]interface{}{
			"path": path, "offset": float64(offset), "length": float64(700 * 1024),
		})
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		m := res.(map[string]interface{})
		data, _ := base64.StdEncoding.DecodeString(m["data"].(string))
		got = append(got, data...)
		if m["eof"].(bool) {
			if size, _ := m["size"].(int64); size != int64(len(want)) {
				t.Fatalf("size = %v, want %d", m["size"], len(want))
			}
			break
		}
		offset += int64(len(data))
	}
	if string(got) != string(want) {
		t.Fatalf("chunked read mismatch: %d vs %d bytes", len(got), len(want))
	}

	// 越界 → 空数据 + eof
	res, _ := callFile(t, p, "read", map[string]interface{}{
		"path": path, "offset": float64(len(want)), "length": float64(100),
	})
	m := res.(map[string]interface{})
	if m["data"] != "" || m["eof"] != true {
		t.Fatalf("past-eof = %v", m)
	}
}

func TestFileReadWriteModes(t *testing.T) {
	p := newFileTestProvider()
	path := filepath.Join(t.TempDir(), "f.txt")

	// truncate=true 新建
	_, err := callFile(t, p, "write", map[string]interface{}{
		"path": path, "data": base64.StdEncoding.EncodeToString([]byte("hello")), "truncate": true,
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	// truncate=false 追加
	res, err := callFile(t, p, "write", map[string]interface{}{
		"path": path, "data": base64.StdEncoding.EncodeToString([]byte(" world")), "truncate": false,
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if size := res.(map[string]interface{})["size"].(int64); size != 11 {
		t.Fatalf("size = %v, want 11", size)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "hello world" {
		t.Fatalf("content = %q", got)
	}
	// truncate=true 覆盖
	_, err = callFile(t, p, "write", map[string]interface{}{
		"path": path, "data": base64.StdEncoding.EncodeToString([]byte("hi")), "truncate": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(path)
	if string(got) != "hi" {
		t.Fatalf("after truncate = %q", got)
	}
	// 追加到不存在的文件拒绝
	_, err = callFile(t, p, "write", map[string]interface{}{
		"path": filepath.Join(t.TempDir(), "missing.txt"), "data": "", "truncate": false,
	})
	if err == nil {
		t.Fatal("append to missing file should fail")
	}
	// 单块超限拒绝
	_, err = callFile(t, p, "write", map[string]interface{}{
		"path": path, "data": base64.StdEncoding.EncodeToString(make([]byte, fileWriteChunkLimit+1)), "truncate": true,
	})
	if err == nil {
		t.Fatal("oversized chunk should fail")
	}
	// 写入自动建父目录
	nested := filepath.Join(t.TempDir(), "a", "b", "c.txt")
	_, err = callFile(t, p, "write", map[string]interface{}{
		"path": nested, "data": base64.StdEncoding.EncodeToString([]byte("x")), "truncate": true,
	})
	if err != nil {
		t.Fatalf("nested write: %v", err)
	}
	if _, err := os.Stat(nested); err != nil {
		t.Fatal(err)
	}
}

func TestFileMkdirAndDelete(t *testing.T) {
	p := newFileTestProvider()
	root := t.TempDir()

	// 多级新建
	nested := filepath.Join(root, "a", "b", "c")
	if _, err := callFile(t, p, "mkdir", map[string]interface{}{"path": nested}); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if info, err := os.Stat(nested); err != nil || !info.IsDir() {
		t.Fatalf("nested dir not created: %v", err)
	}
	// 已存在拒绝
	if _, err := callFile(t, p, "mkdir", map[string]interface{}{"path": nested}); err == nil {
		t.Fatal("mkdir existing should fail")
	}

	// 递归删除目录
	deep := filepath.Join(nested, "f.txt")
	mustWrite(t, deep, "x")
	if _, err := callFile(t, p, "delete", map[string]interface{}{"path": filepath.Join(root, "a")}); err != nil {
		t.Fatalf("delete dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "a")); !os.IsNotExist(err) {
		t.Fatal("dir should be gone")
	}

	// 删 symlink 只删链接本身
	real := filepath.Join(root, "real.txt")
	mustWrite(t, real, "keep")
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := callFile(t, p, "delete", map[string]interface{}{"path": link}); err != nil {
		t.Fatalf("delete symlink: %v", err)
	}
	if _, err := os.Stat(link); !os.IsNotExist(err) {
		t.Fatal("symlink should be gone")
	}
	if _, err := os.Stat(real); err != nil {
		t.Fatalf("symlink target must survive: %v", err)
	}

	// 删根目录拒绝
	if _, err := callFile(t, p, "delete", map[string]interface{}{"path": "/"}); err == nil {
		t.Fatal("delete / should be rejected")
	}
}

func TestFileRename(t *testing.T) {
	p := newFileTestProvider()
	root := t.TempDir()
	old := filepath.Join(root, "old.txt")
	mustWrite(t, old, "data")

	// 正常改名
	res, err := callFile(t, p, "rename", map[string]interface{}{"path": old, "name": "new.txt"})
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	np := res.(map[string]interface{})["path"].(string)
	if _, err := os.Stat(np); err != nil {
		t.Fatalf("renamed file missing: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("old file should be gone")
	}

	// 目标已存在拒绝
	other := filepath.Join(root, "other.txt")
	mustWrite(t, other, "x")
	if _, err := callFile(t, p, "rename", map[string]interface{}{"path": np, "name": "other.txt"}); err == nil {
		t.Fatal("rename onto existing should fail")
	}

	// 非法名字
	for _, name := range []string{"a/b", "..", ".", ""} {
		if _, err := callFile(t, p, "rename", map[string]interface{}{"path": np, "name": name}); err == nil {
			t.Errorf("name %q should be rejected", name)
		}
	}
}

func TestFileRejectsUnsafePaths(t *testing.T) {
	p := newFileTestProvider()
	root := t.TempDir()
	dir := filepath.Join(root, "dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		desc, action string
		params       map[string]interface{}
	}{
		{"relative list", "list", map[string]interface{}{"dir": "rel/path"}},
		{"root list", "list", map[string]interface{}{"dir": "/"}},
		{"read dir", "read", map[string]interface{}{"path": dir, "offset": float64(0), "length": float64(100)}},
		{"read symlink", "read", map[string]interface{}{"path": link, "offset": float64(0), "length": float64(100)}},
		{"read root", "read", map[string]interface{}{"path": "/", "offset": float64(0), "length": float64(100)}},
		{"relative read", "read", map[string]interface{}{"path": "etc/passwd", "offset": float64(0), "length": float64(100)}},
		{"write symlink", "write", map[string]interface{}{"path": link, "data": base64.StdEncoding.EncodeToString([]byte("x")), "truncate": true}},
		{"write into dir path", "write", map[string]interface{}{"path": dir, "data": base64.StdEncoding.EncodeToString([]byte("x")), "truncate": true}},
		{"relative delete", "delete", map[string]interface{}{"path": "x/y"}},
		{"mkdir relative", "mkdir", map[string]interface{}{"path": "x"}},
	}
	for _, c := range cases {
		if _, err := callFile(t, p, c.action, c.params); err == nil {
			t.Errorf("%s: should be rejected", c.desc)
		}
	}

	// `..` 穿越形态：Clean 后变绝对可解析路径，但仍须拒绝「根」与相对
	// 注："/a/../b" Clean 后为 "/b" 合法（无越界），这里验证相对穿越被拒
	if _, err := callFile(t, p, "read", map[string]interface{}{
		"path": "../secret", "offset": float64(0), "length": float64(10),
	}); err == nil {
		t.Error("relative traversal should be rejected")
	}
	if _, err := callFile(t, p, "delete", map[string]interface{}{"path": strings.Repeat("a/", 50) + ".."}); err == nil {
		t.Error("relative deep traversal should be rejected")
	}
}

// TestFileWriteMode mode 白名单（ACME 部署写私钥 0600，D14）：
// 缺省/0 = 0644 既有行为，0600/0644 显式放行，其他值拒绝
func TestFileWriteMode(t *testing.T) {
	p := newFileTestProvider()
	dir := t.TempDir()

	// 缺省 = 0644（既有行为不变）
	def := filepath.Join(dir, "d.txt")
	if _, err := callFile(t, p, "write", map[string]interface{}{
		"path": def, "data": base64.StdEncoding.EncodeToString([]byte("x")), "truncate": true,
	}); err != nil {
		t.Fatalf("default write: %v", err)
	}
	if fi, err := os.Stat(def); err != nil || fi.Mode().Perm() != 0o644 {
		t.Fatalf("default perm = %v, want 0644", fi.Mode().Perm())
	}

	// 0600 与 0644 显式放行
	for _, mode := range []float64{0o600, 0o644} {
		path := filepath.Join(dir, fmt.Sprintf("m%o.txt", int(mode)))
		if _, err := callFile(t, p, "write", map[string]interface{}{
			"path": path, "data": base64.StdEncoding.EncodeToString([]byte("x")), "truncate": true, "mode": mode,
		}); err != nil {
			t.Fatalf("mode %o: %v", int(mode), err)
		}
		if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != os.FileMode(mode) {
			t.Fatalf("mode %o perm = %v", int(mode), fi.Mode().Perm())
		}
	}

	// 覆盖写也落准权限（先 0644 再以 0600 覆盖）
	over := filepath.Join(dir, "over.txt")
	for _, mode := range []float64{0o644, 0o600} {
		if _, err := callFile(t, p, "write", map[string]interface{}{
			"path": over, "data": base64.StdEncoding.EncodeToString([]byte("x")), "truncate": true, "mode": mode,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if fi, err := os.Stat(over); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("after overwrite perm = %v, want 0600", fi.Mode().Perm())
	}

	// 白名单外拒绝
	for _, bad := range []interface{}{0o777, 0o400, "600"} {
		_, err := callFile(t, p, "write", map[string]interface{}{
			"path": filepath.Join(dir, "bad.txt"), "data": base64.StdEncoding.EncodeToString([]byte("x")), "truncate": true, "mode": bad,
		})
		if err == nil {
			t.Fatalf("mode %v should be rejected", bad)
		}
	}

	// mode=0 与缺省等价
	zero := filepath.Join(dir, "z.txt")
	if _, err := callFile(t, p, "write", map[string]interface{}{
		"path": zero, "data": base64.StdEncoding.EncodeToString([]byte("x")), "truncate": true, "mode": float64(0),
	}); err != nil {
		t.Fatalf("mode 0: %v", err)
	}
	if fi, err := os.Stat(zero); err != nil || fi.Mode().Perm() != 0o644 {
		t.Fatalf("mode 0 perm = %v, want 0644", fi.Mode().Perm())
	}
}
