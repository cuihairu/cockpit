package rpc

// 覆盖率补充测试：pve_provider.go 参数校验分支（vmid 非法 / snapshot name 缺失）。

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCovPVEProviderInvalidVMID(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(pveTestHandler))
	defer ts.Close()
	p := NewPVEProvider(ts.URL, "t", "s")
	if p.Type() != "pve" {
		t.Fatalf("type = %q", p.Type())
	}

	// 所有依赖 vmid 的 action：非数字字符串 → GetVMID 失败
	actions := []string{
		"vms.get", "vms.start", "vms.stop", "vms.restart", "vms.suspend", "vms.resume",
		"containers.get", "containers.start", "containers.stop", "containers.restart",
		"snapshots.list", "snapshots.create", "snapshots.delete",
	}
	for _, action := range actions {
		_, err := p.Call(action, map[string]interface{}{"node": "pve1", "vmid": "abc", "name": "snap1"})
		if err == nil {
			t.Errorf("Call(%q) with non-numeric vmid should fail", action)
		}
	}
	// vmid 类型不支持（数组）
	if _, err := p.GetVM(map[string]interface{}{"vmid": []interface{}{1}}); err == nil {
		t.Error("array vmid should fail")
	}
	// vmid 缺失（nil）
	if _, err := p.StartVM(map[string]interface{}{}); err == nil {
		t.Error("missing vmid should fail")
	}
}

func TestCovPVEProviderStringVMIDAndSnapshotName(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(pveTestHandler))
	defer ts.Close()
	p := NewPVEProvider(ts.URL, "t", "s")

	// 字符串数字 vmid 可解析
	if _, err := p.GetVM(map[string]interface{}{"node": "pve1", "vmid": "100"}); err != nil {
		t.Errorf("string vmid should parse: %v", err)
	}
	// float64 vmid（JSON 反序列化形状）
	if _, err := p.ListSnapshots(map[string]interface{}{"node": "pve1", "vmid": float64(100)}); err != nil {
		t.Errorf("float64 vmid should parse: %v", err)
	}

	// DeleteSnapshot：name 缺失 / 非字符串
	if _, err := p.DeleteSnapshot(map[string]interface{}{"node": "pve1", "vmid": 100}); err == nil {
		t.Error("missing snapshot name should fail")
	}
	if _, err := p.DeleteSnapshot(map[string]interface{}{"node": "pve1", "vmid": 100, "name": ""}); err == nil {
		t.Error("empty snapshot name should fail")
	}
	// CreateSnapshot：name/description 非字符串时按空串处理（不失败）
	res, err := p.CreateSnapshot(map[string]interface{}{"node": "pve1", "vmid": 100, "name": 42})
	if err != nil {
		t.Fatalf("create with non-string name: %v", err)
	}
	if res.(map[string]interface{})["name"] != "" {
		t.Errorf("non-string name should coerce empty, got %v", res)
	}
}
