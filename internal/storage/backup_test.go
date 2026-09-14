package storage

import "testing"

func TestBackupConfigCRUD(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	cfg := &BackupConfig{
		AgentID:   "agent-1",
		Name:      "etc-backup",
		Sources:   `["/etc/nginx","/var/lib/foo"]`,
		DestDir:   "/mnt/backup",
		Schedule:  "daily@03:00",
		Retention: 7,
		Enabled:   true,
	}
	if err := db.CreateBackupConfig(cfg); err != nil {
		t.Fatalf("CreateBackupConfig: %v", err)
	}
	if cfg.ID == 0 {
		t.Fatal("CreateBackupConfig should set ID")
	}

	got, err := db.GetBackupConfig(cfg.ID)
	if err != nil {
		t.Fatalf("GetBackupConfig: %v", err)
	}
	if got.Name != "etc-backup" || got.Retention != 7 || !got.Enabled {
		t.Errorf("GetBackupConfig = %+v", got)
	}

	got.Schedule = "every:6h"
	got.Enabled = false
	if err := db.UpdateBackupConfig(got); err != nil {
		t.Fatalf("UpdateBackupConfig: %v", err)
	}
	again, _ := db.GetBackupConfig(cfg.ID)
	if again.Schedule != "every:6h" || again.Enabled {
		t.Errorf("update not persisted: %+v", again)
	}

	list, err := db.ListBackupConfigs()
	if err != nil || len(list) != 1 {
		t.Fatalf("ListBackupConfigs = %v, %v", list, err)
	}
}

func TestDueBackupConfigs(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mk := func(name string, enabled bool, next int64, status string) *BackupConfig {
		return &BackupConfig{AgentID: "a", Name: name, DestDir: "/b", Enabled: enabled,
			NextRunAt: next, LastStatus: status}
	}
	due := mk("due", true, 100, "")
	later := mk("later", true, 999, "")
	manual := mk("manual", true, 0, "")
	disabled := mk("disabled", false, 100, "")
	running := mk("running", true, 100, "running")
	for _, c := range []*BackupConfig{due, later, manual, disabled, running} {
		if err := db.CreateBackupConfig(c); err != nil {
			t.Fatalf("CreateBackupConfig: %v", err)
		}
	}

	list, err := db.DueBackupConfigs(500)
	if err != nil {
		t.Fatalf("DueBackupConfigs: %v", err)
	}
	if len(list) != 1 || list[0].Name != "due" {
		t.Errorf("DueBackupConfigs(500) = %+v; want only [due]", list)
	}
}

func TestBackupRunLifecycleAndCascade(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	cfg := &BackupConfig{AgentID: "a", Name: "db", DestDir: "/b", Enabled: true}
	if err := db.CreateBackupConfig(cfg); err != nil {
		t.Fatalf("CreateBackupConfig: %v", err)
	}

	run := &BackupRun{ConfigID: cfg.ID, TaskID: "t-1", Status: "running", StartedAt: 100}
	if err := db.CreateBackupRun(run); err != nil {
		t.Fatalf("CreateBackupRun: %v", err)
	}

	run.Status = "success"
	run.File = "db-20260914-030000.tar.gz"
	run.Size = 12345
	run.FinishedAt = 200
	if err := db.UpdateBackupRun(run); err != nil {
		t.Fatalf("UpdateBackupRun: %v", err)
	}

	// 另一条 running 记录用于 RunningBackupRuns
	stuck := &BackupRun{ConfigID: cfg.ID, TaskID: "t-2", Status: "running", StartedAt: 300}
	if err := db.CreateBackupRun(stuck); err != nil {
		t.Fatalf("CreateBackupRun: %v", err)
	}

	runs, err := db.ListBackupRuns(cfg.ID, 0)
	if err != nil || len(runs) != 2 {
		t.Fatalf("ListBackupRuns = %d runs, %v", len(runs), err)
	}
	if runs[0].TaskID != "t-2" {
		t.Errorf("ListBackupRuns should be id DESC, first = %s", runs[0].TaskID)
	}
	if got, _ := db.GetBackupRun(run.ID); got.File != "db-20260914-030000.tar.gz" || got.Size != 12345 {
		t.Errorf("GetBackupRun = %+v", got)
	}

	stuckRuns, err := db.RunningBackupRuns()
	if err != nil || len(stuckRuns) != 1 || stuckRuns[0].TaskID != "t-2" {
		t.Errorf("RunningBackupRuns = %+v, %v", stuckRuns, err)
	}

	// 级联删除
	if err := db.DeleteBackupConfig(cfg.ID); err != nil {
		t.Fatalf("DeleteBackupConfig: %v", err)
	}
	if _, err := db.GetBackupConfig(cfg.ID); err == nil {
		t.Error("config should be gone")
	}
	orphans, _ := db.ListBackupRuns(cfg.ID, 0)
	if len(orphans) != 0 {
		t.Errorf("runs should be cascaded, got %d", len(orphans))
	}
}
