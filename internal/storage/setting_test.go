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
