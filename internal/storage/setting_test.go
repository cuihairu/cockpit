package storage

import "testing"

func TestSettingLifecycle(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	// 读不存在的 key
	if _, err := db.GetSetting("probe.interval_seconds"); err == nil {
		t.Error("GetSetting on missing key should return error")
	}

	// 写入
	if err := db.SetSetting("probe.interval_seconds", "60"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	v, err := db.GetSetting("probe.interval_seconds")
	if err != nil || v != "60" {
		t.Fatalf("GetSetting = %q, %v; want 60", v, err)
	}

	// upsert 覆盖
	if err := db.SetSetting("probe.interval_seconds", "120"); err != nil {
		t.Fatalf("SetSetting(upsert): %v", err)
	}
	v, _ = db.GetSetting("probe.interval_seconds")
	if v != "120" {
		t.Errorf("after upsert GetSetting = %q, want 120", v)
	}

	// 删除幂等
	if err := db.DeleteSetting("probe.interval_seconds"); err != nil {
		t.Fatalf("DeleteSetting: %v", err)
	}
	if err := db.DeleteSetting("probe.interval_seconds"); err != nil {
		t.Errorf("DeleteSetting should be idempotent, got %v", err)
	}
	if _, err := db.GetSetting("probe.interval_seconds"); err == nil {
		t.Error("GetSetting after delete should return error")
	}
}

func TestListSettingKeys(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	// 空前缀匹配
	keys, err := db.ListSettingKeys("service_health.config.")
	if err != nil || len(keys) != 0 {
		t.Fatalf("empty: %v %v", keys, err)
	}

	// 前缀命中且有序；不跨前缀误捞（service_health.state. 不属于 config.）
	for _, k := range []string{
		"service_health.config.b", "service_health.config.a", "service_health.state.a",
	} {
		if err := db.SetSetting(k, "x"); err != nil {
			t.Fatalf("SetSetting %s: %v", k, err)
		}
	}
	keys, err = db.ListSettingKeys("service_health.config.")
	if err != nil {
		t.Fatalf("ListSettingKeys: %v", err)
	}
	if len(keys) != 2 || keys[0] != "service_health.config.a" || keys[1] != "service_health.config.b" {
		t.Fatalf("keys = %v, want sorted config prefix only", keys)
	}

	// 库关闭后查询报错（错误分支）
	_ = db.Close()
	if _, err := db.ListSettingKeys("x."); err == nil {
		t.Error("ListSettingKeys on closed db should error")
	}
}
