package rpc

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// ============ dispatch 层覆盖补测（第三轮覆盖率） ============

// TestServiceProviderCallDispatch Call 分发走通全部 6 个 action + 未知 action
// （既有测试直接调 Status/List 等方法，分发层未覆盖）
func TestServiceProviderCallDispatch(t *testing.T) {
	tmp := t.TempDir()
	oldEtc := etcSystemdDir
	etcSystemdDir = tmp
	t.Cleanup(func() { etcSystemdDir = oldEtc })

	m := &mockSystemctl{
		unitsOut:     sampleUnitsOut,
		filesOut:     sampleFilesOut,
		isRunningOut: "running",
		fragmentPath: tmp + "/a.service", // etcSystemdDir 之下 → unitsave 直写分支
		catOut:       "[Unit]\nDescription=A\n",
	}
	p := NewServiceProvider(m.run)

	if _, err := p.Call("status", nil); err != nil {
		t.Errorf("status: %v", err)
	}
	if _, err := p.Call("list", nil); err != nil {
		t.Errorf("list: %v", err)
	}
	res, err := p.Call("action", map[string]interface{}{"name": "nginx.service", "action": "restart"})
	if err != nil {
		t.Errorf("action: %v", err)
	}
	if got := res.(map[string]interface{}); got["name"] != "nginx.service" || got["action"] != "restart" {
		t.Errorf("action resp = %+v", got)
	}
	if _, err := p.Call("daemon-reload", nil); err != nil {
		t.Errorf("daemon-reload: %v", err)
	}
	if _, err := p.Call("unitfile", map[string]interface{}{"name": "a.service"}); err != nil {
		t.Errorf("unitfile: %v", err)
	}
	res, err = p.Call("unitsave", map[string]interface{}{"name": "a.service", "content": "[Unit]\nDescription=A2\n"})
	if err != nil {
		t.Errorf("unitsave: %v", err)
	}
	if got := res.(map[string]interface{}); got["path"] != tmp+"/a.service" || got["reloaded"] != true {
		t.Errorf("unitsave resp = %+v", got)
	}
	if _, err := os.Stat(tmp + "/a.service"); err != nil {
		t.Errorf("unit file not written: %v", err)
	}
	if _, err := p.Call("nope", nil); err == nil {
		t.Error("unknown action should fail")
	}
}

// TestNewServiceProviderNilRun 构造器 nil 回退 defaultCommander
func TestNewServiceProviderNilRun(t *testing.T) {
	if p := NewServiceProvider(nil); p.run == nil {
		t.Error("nil run should default to defaultCommander")
	}
}

// TestAtomicWriteFileCreateTempError 目标目录不存在 → CreateTemp 失败
func TestAtomicWriteFileCreateTempError(t *testing.T) {
	if err := atomicWriteFile(filepath.Join(t.TempDir(), "gone", "u.file"), []byte("x"), 0o644); err == nil {
		t.Error("missing dir should fail")
	}
}

// TestFileStatOwnerMissing 路径不存在 → (-1, -1)
func TestFileStatOwnerMissing(t *testing.T) {
	uid, gid := fileStatOwner(filepath.Join(t.TempDir(), "absent"))
	if uid != -1 || gid != -1 {
		t.Errorf("missing path owner = %d:%d, want -1:-1", uid, gid)
	}
}

// TestDriftCallDispatch drift Call 分发：check/diff/record/未知
func TestDriftCallDispatch(t *testing.T) {
	dir := t.TempDir()
	p := NewDriftProvider(NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json")), DriftConfig{ConfDir: dir})

	if _, err := p.Call("check", nil); err != nil {
		t.Errorf("check: %v", err)
	}
	// 未登记条目 diff 报错即走到分发（Diff 校验在方法内）
	if _, err := p.Call("diff", map[string]interface{}{"kind": "nginx", "name": "x"}); err == nil {
		t.Error("diff of unrecorded entry should fail")
	}
	if _, err := p.Call("record", map[string]interface{}{"kind": "bogus", "name": "x"}); err == nil {
		t.Error("record with bad kind should fail")
	}
	if _, err := p.Call("wat", nil); err == nil {
		t.Error("unknown action should fail")
	}
}

// TestNasCallDispatch nas Call 分发：status（降级快照）+ 未知
func TestNasCallDispatch(t *testing.T) {
	p := NewNasProvider(nil)
	if _, err := p.Call("status", nil); err != nil {
		t.Errorf("status: %v", err)
	}
	if _, err := p.Call("other", nil); err == nil {
		t.Error("unknown action should fail")
	}
}

// TestClipPeers peers 列表截断两分支
func TestClipPeers(t *testing.T) {
	in := []map[string]interface{}{{"a": 1}, {"b": 2}}
	if got := clipPeers(in, 1); len(got) != 1 {
		t.Errorf("over max: len = %d", len(got))
	}
	if got := clipPeers(in, 5); len(got) != 2 {
		t.Errorf("under max: len = %d", len(got))
	}
}

// TestOmvEntryProtoErrors omvEntry 错误分支：params 不可序列化、addr 非法
func TestOmvEntryProtoErrors(t *testing.T) {
	if _, err := omvEntry(context.Background(), nil, "http://127.0.0.1:1", "", "s", "m", make(chan int)); err == nil {
		t.Error("unmarshalable params should fail")
	}
	// URL 含空格 → NewRequestWithContext 构造失败
	if _, err := omvEntry(context.Background(), nil, "http://exa mple.com/rpc.php", "", "s", "m", nil); err == nil {
		t.Error("invalid addr should fail")
	}
}
