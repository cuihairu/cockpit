package rpc

import "testing"

// vault NAS 目标下发路径（vault-design.md V6）：NewNasProviderWithTargets
// 构造、set_targets RPC 热更新（合法/缺字段/不可序列化）与导出解析包装。
func TestNasProviderWithTargetsAndSetTargetsRPC(t *testing.T) {
	p := NewNasProviderWithTargets(nil, []NasTarget{
		{Name: "box1", Type: "dsm", Addr: "https://h:5001", Username: "u", Password: "pw"},
	})
	if len(p.targets) != 1 || p.targets[0].Name != "box1" {
		t.Fatalf("constructor targets = %+v", p.targets)
	}

	// 合法热更新（整组替换）
	err := p.setTargetsFromParams(map[string]interface{}{
		"targets": []map[string]interface{}{
			{"name": "box2", "type": "omv", "addr": "https://h2", "username": "u2", "password": "pw2"},
		},
	})
	if err != nil {
		t.Fatalf("setTargetsFromParams: %v", err)
	}
	if len(p.targets) != 1 || p.targets[0].Name != "box2" {
		t.Fatalf("after set = %+v", p.targets)
	}

	// Call 分发 set_targets
	if _, err := p.Call("set_targets", map[string]interface{}{
		"targets": []map[string]interface{}{
			{"name": "box3", "type": "dsm", "addr": "https://h3", "username": "u3", "password": "pw3"},
		},
	}); err != nil {
		t.Fatalf("Call set_targets: %v", err)
	}
	if len(p.targets) != 1 || p.targets[0].Name != "box3" {
		t.Fatalf("after Call = %+v", p.targets)
	}

	// 缺 targets 键
	if err := p.setTargetsFromParams(map[string]interface{}{}); err == nil || err.Error() != "targets required" {
		t.Fatalf("missing targets err = %v", err)
	}

	// 不可序列化载荷（函数值）
	if err := p.setTargetsFromParams(map[string]interface{}{"targets": func() {}}); err == nil {
		t.Fatal("marshal failure must error")
	}
}

// ParseNasTargetsJSON 导出包装与 env 解析同形：合法条目与整体非法。
func TestParseNasTargetsJSONExported(t *testing.T) {
	ts := ParseNasTargetsJSON(`[{"name":"n1","type":"dsm","addr":"https://a:5001","username":"u","password":"p"}]`)
	if len(ts) != 1 || ts[0].Name != "n1" || ts[0].Password != "p" {
		t.Fatalf("ParseNasTargetsJSON = %+v", ts)
	}
	if ParseNasTargetsJSON("not json") != nil {
		t.Fatal("invalid json must be nil")
	}
}

// Handler.Provider 按类型取 provider（注册响应引导定位既有 provider）。
func TestHandlerProviderGetter(t *testing.T) {
	h := NewHandler()
	if h.Provider("nas") != nil {
		t.Fatal("empty handler must return nil")
	}
	p := NewNasProviderWithTargets(nil, nil)
	h.RegisterProvider(p)
	if h.Provider("nas") != p {
		t.Fatal("Provider getter must return registered provider")
	}
	if h.Provider("missing") != nil {
		t.Fatal("unknown type must return nil")
	}
}
