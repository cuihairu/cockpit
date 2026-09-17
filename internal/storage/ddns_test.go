package storage

import "testing"

func TestDDNSConfigCRUD(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	cfg := &DDNSConfig{
		AgentID:    "agent-1",
		ZoneID:     "zone-abc",
		ZoneName:   "example.com",
		RecordName: "home.example.com",
		Type:       "A",
		Enabled:    true,
		LastStatus: "never",
	}
	if err := db.CreateDDNSConfig(cfg); err != nil {
		t.Fatalf("CreateDDNSConfig: %v", err)
	}
	if cfg.ID == 0 {
		t.Fatal("CreateDDNSConfig should set ID")
	}

	got, err := db.GetDDNSConfig(cfg.ID)
	if err != nil {
		t.Fatalf("GetDDNSConfig: %v", err)
	}
	if got.RecordName != "home.example.com" || got.Type != "A" || got.LastStatus != "never" {
		t.Errorf("GetDDNSConfig = %+v", got)
	}

	// 更新：巡检回写状态
	got.LastIP = "203.0.113.7"
	got.LastStatus = "ok"
	got.CheckedAt = 1726500000
	if err := db.UpdateDDNSConfig(got); err != nil {
		t.Fatalf("UpdateDDNSConfig: %v", err)
	}
	re, _ := db.GetDDNSConfig(cfg.ID)
	if re.LastIP != "203.0.113.7" || re.LastStatus != "ok" || re.CheckedAt != 1726500000 {
		t.Errorf("after update = %+v", re)
	}

	// 列表
	list, err := db.ListDDNSConfigs()
	if err != nil {
		t.Fatalf("ListDDNSConfigs: %v", err)
	}
	if len(list) != 1 || list[0].ID != cfg.ID {
		t.Errorf("list = %+v", list)
	}

	// 删除
	if err := db.DeleteDDNSConfig(cfg.ID); err != nil {
		t.Fatalf("DeleteDDNSConfig: %v", err)
	}
	list, _ = db.ListDDNSConfigs()
	if len(list) != 0 {
		t.Errorf("after delete list = %+v", list)
	}
}
