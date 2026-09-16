package rpc

// 覆盖率补充测试：file_provider.go 错误分支与常量导出。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCovFileChunkLimitsAndType(t *testing.T) {
	if FileReadChunkLimit() != 1024*1024 || FileWriteChunkLimit() != 1024*1024 {
		t.Errorf("chunk limits = %d %d", FileReadChunkLimit(), FileWriteChunkLimit())
	}
	p := newFileTestProvider()
	if p.Type() != "file" {
		t.Errorf("type = %q", p.Type())
	}
	if _, err := callFile(t, p, "bogus", nil); err == nil || !strings.Contains(err.Error(), "unknown file action") {
		t.Errorf("unknown action err = %v", err)
	}
}

func TestCovFileListMissingDir(t *testing.T) {
	p := newFileTestProvider()
	if _, err := p.List(filepath.Join(t.TempDir(), "missing")); err == nil ||
		!strings.Contains(err.Error(), "read dir") {
		t.Errorf("list missing err = %v", err)
	}
}

func TestCovFileReadErrors(t *testing.T) {
	p := newFileTestProvider()
	// 不存在 → stat err
	if _, err := p.Read(map[string]interface{}{"path": filepath.Join(t.TempDir(), "nope")}); err == nil ||
		!strings.Contains(err.Error(), "stat:") {
		t.Errorf("read missing err = %v", err)
	}
	// 无读权限 → open err（非 root）
	if os.Geteuid() != 0 {
		f := filepath.Join(t.TempDir(), "secret")
		mustWrite(t, f, "x")
		os.Chmod(f, 0o000)
		t.Cleanup(func() { os.Chmod(f, 0o644) })
		if _, err := p.Read(map[string]interface{}{"path": f}); err == nil ||
			!strings.Contains(err.Error(), "open:") {
			t.Errorf("read unreadable err = %v", err)
		}
	}
}

func TestCovFileWriteErrors(t *testing.T) {
	p := newFileTestProvider()
	// 非法 base64
	if _, err := p.Write(map[string]interface{}{"path": "/tmp/cov-file.txt", "data": "!!!not-base64!!!"}); err == nil ||
		!strings.Contains(err.Error(), "data must be base64") {
		t.Errorf("bad base64 err = %v", err)
	}

	if os.Geteuid() != 0 {
		// 父目录不可穿透 → Lstat 报 stat err
		ro := t.TempDir()
		os.Chmod(ro, 0o000)
		t.Cleanup(func() { os.Chmod(ro, 0o700) })
		if _, err := p.Write(map[string]interface{}{
			"path": filepath.Join(ro, "new.txt"), "data": "eA==", "truncate": true,
		}); err == nil || !strings.Contains(err.Error(), "stat:") {
			t.Errorf("stat err = %v", err)
		}

		// 父目录只读 → OpenFile 失败
		wo := t.TempDir()
		os.Chmod(wo, 0o500)
		t.Cleanup(func() { os.Chmod(wo, 0o700) })
		if _, err := p.Write(map[string]interface{}{
			"path": filepath.Join(wo, "new.txt"), "data": "eA==", "truncate": true,
		}); err == nil || !strings.Contains(err.Error(), "open:") {
			t.Errorf("open err = %v", err)
		}
	}

	// 父路径中段是文件 → Lstat 报 ENOTDIR（非 IsNotExist）→ stat err
	middle := filepath.Join(t.TempDir(), "blocker")
	mustWrite(t, middle, "x")
	if _, err := p.Write(map[string]interface{}{
		"path": filepath.Join(middle, "sub", "new.txt"), "data": "eA==", "truncate": true,
	}); err == nil || !strings.Contains(err.Error(), "stat:") {
		t.Errorf("notdir stat err = %v", err)
	}

	if os.Geteuid() != 0 {
		// 只读目录下新建多级路径 → MkdirAll 失败（Lstat 得 ENOENT）
		ro2 := t.TempDir()
		os.Chmod(ro2, 0o500)
		t.Cleanup(func() { os.Chmod(ro2, 0o700) })
		if _, err := p.Write(map[string]interface{}{
			"path": filepath.Join(ro2, "sub", "new.txt"), "data": "eA==", "truncate": true,
		}); err == nil || !strings.Contains(err.Error(), "create parent dir") {
			t.Errorf("mkdir err = %v", err)
		}
	}
}

func TestCovFileMkdirDeleteRenameErrors(t *testing.T) {
	p := newFileTestProvider()
	// mkdir：路径中段是文件
	middle := filepath.Join(t.TempDir(), "blocker")
	mustWrite(t, middle, "x")
	if _, err := p.Mkdir(filepath.Join(middle, "sub")); err == nil ||
		!strings.Contains(err.Error(), "mkdir:") {
		t.Errorf("mkdir err = %v", err)
	}

	if os.Geteuid() != 0 {
		// delete：目录只读，其内容删不掉（非 root）
		ro := t.TempDir()
		mustWrite(t, filepath.Join(ro, "child"), "x")
		os.Chmod(ro, 0o500)
		t.Cleanup(func() { os.Chmod(ro, 0o700) })
		if _, err := p.Delete(ro); err == nil || !strings.Contains(err.Error(), "remove:") {
			t.Errorf("delete err = %v", err)
		}
	}

	// rename：源不存在
	dir := t.TempDir()
	if _, err := p.Rename(filepath.Join(dir, "ghost"), "newname"); err == nil ||
		!strings.Contains(err.Error(), "rename:") {
		t.Errorf("rename missing err = %v", err)
	}
	// rename：目标已存在
	src := filepath.Join(dir, "src")
	mustWrite(t, src, "x")
	mustWrite(t, filepath.Join(dir, "dst"), "y")
	if _, err := p.Rename(src, "dst"); err == nil ||
		!strings.Contains(err.Error(), "target already exists") {
		t.Errorf("rename exists err = %v", err)
	}
}
