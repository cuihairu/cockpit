package storage

import (
	"strings"
	"testing"
)

// TestAcmeDeployTargetCRUD 部署目标表 CRUD 与替换语义（D-2026-10-12-3）：
// 整体替换（先删后插、重绑定状态清零）、单目标状态回写、删证书级联清目标
func TestAcmeDeployTargetCRUD(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	cert := &AcmeCert{Domains: []string{"a.com"}, CADirectory: "staging", Status: "pending", RenewBeforeDays: 30, LastStatus: "never"}
	if err := db.CreateAcmeCert(cert); err != nil {
		t.Fatalf("CreateAcmeCert: %v", err)
	}
	if err := db.ReplaceAcmeDeployTargets(cert.ID, []AcmeDeployTarget{
		{AgentID: "ag-1", CertPath: "/c/a.crt.pem", KeyPath: "/c/a.key.pem"},
		{AgentID: "ag-2", CertPath: "/c/b.crt.pem", KeyPath: "/c/b.key.pem"},
	}); err != nil {
		t.Fatalf("ReplaceAcmeDeployTargets: %v", err)
	}

	targets, err := db.ListAcmeDeployTargets(cert.ID)
	if err != nil {
		t.Fatalf("ListAcmeDeployTargets: %v", err)
	}
	if len(targets) != 2 || targets[0].AgentID != "ag-1" || targets[1].AgentID != "ag-2" {
		t.Fatalf("targets = %+v", targets)
	}
	for _, g := range targets {
		if g.CertID != cert.ID || g.CertPath == "" || g.KeyPath == "" {
			t.Errorf("target not bound: %+v", g)
		}
		if g.LastDeployAt != 0 || g.LastDeployError != "" {
			t.Errorf("fresh target should have clean status: %+v", g)
		}
	}

	// 单目标状态回写：成功（空 errMsg）与失败两形态
	if err := db.UpdateAcmeDeployTargetStatus(targets[0].ID, 1726500000, ""); err != nil {
		t.Fatalf("UpdateAcmeDeployTargetStatus ok: %v", err)
	}
	if err := db.UpdateAcmeDeployTargetStatus(targets[1].ID, 1726500001, "agent offline"); err != nil {
		t.Fatalf("UpdateAcmeDeployTargetStatus fail: %v", err)
	}
	targets, _ = db.ListAcmeDeployTargets(cert.ID)
	if targets[0].LastDeployAt != 1726500000 || targets[0].LastDeployError != "" {
		t.Errorf("ok status = %+v", targets[0])
	}
	if targets[1].LastDeployAt != 1726500001 || targets[1].LastDeployError != "agent offline" {
		t.Errorf("fail status = %+v", targets[1])
	}

	// 整体替换：换目标 + 状态清零（重绑定视为新目标）
	if err := db.ReplaceAcmeDeployTargets(cert.ID, []AcmeDeployTarget{
		{AgentID: "ag-9", CertPath: "/c/n.crt.pem", KeyPath: "/c/n.key.pem", LastDeployAt: 999, LastDeployError: "stale"},
	}); err != nil {
		t.Fatalf("rebind: %v", err)
	}
	targets, _ = db.ListAcmeDeployTargets(cert.ID)
	if len(targets) != 1 || targets[0].AgentID != "ag-9" || targets[0].LastDeployAt != 0 || targets[0].LastDeployError != "" {
		t.Fatalf("after rebind targets = %+v", targets)
	}

	// 空数组 = 全部解绑
	if err := db.ReplaceAcmeDeployTargets(cert.ID, nil); err != nil {
		t.Fatalf("unbind: %v", err)
	}
	if targets, _ = db.ListAcmeDeployTargets(cert.ID); len(targets) != 0 {
		t.Fatalf("after unbind targets = %+v", targets)
	}

	// 级联：重新绑定后删证书 → 目标行随删
	if err := db.ReplaceAcmeDeployTargets(cert.ID, []AcmeDeployTarget{
		{AgentID: "ag-1", CertPath: "/c/a.crt.pem", KeyPath: "/c/a.key.pem"},
	}); err != nil {
		t.Fatalf("rebind 2: %v", err)
	}
	if err := db.DeleteAcmeCert(cert.ID); err != nil {
		t.Fatalf("DeleteAcmeCert: %v", err)
	}
	if targets, _ = db.ListAcmeDeployTargets(cert.ID); len(targets) != 0 {
		t.Errorf("targets must cascade-delete with cert, got %+v", targets)
	}
}

// TestAcmeDeployTargetBackfill 单目标旧列 → 部署目标表一次性迁移：
// 迁行并清旧列、幂等（二次零命中）、已有目标行的证书不重复迁但旧列仍归零
func TestAcmeDeployTargetBackfill(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	legacy := &AcmeCert{Domains: []string{"legacy.com"}, CADirectory: "staging", Status: "issued",
		RenewBeforeDays: 30, LastStatus: "ok",
		DeployAgentID: "ag-old", DeployCertPath: "/old.crt.pem", DeployKeyPath: "/old.key.pem",
		LastDeployAt: 1000, LastDeployError: "agent offline"}
	if err := db.CreateAcmeCert(legacy); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}
	// 已有目标行的证书（重复启动形态：旧列未清但目标行已存在）
	dup := &AcmeCert{Domains: []string{"dup.com"}, CADirectory: "staging", Status: "issued",
		RenewBeforeDays: 30, LastStatus: "ok",
		DeployAgentID: "ag-dup", DeployCertPath: "/dup.crt.pem", DeployKeyPath: "/dup.key.pem"}
	if err := db.CreateAcmeCert(dup); err != nil {
		t.Fatalf("seed dup: %v", err)
	}
	if err := db.ReplaceAcmeDeployTargets(dup.ID, []AcmeDeployTarget{
		{AgentID: "ag-dup", CertPath: "/dup.crt.pem", KeyPath: "/dup.key.pem"},
	}); err != nil {
		t.Fatalf("pre-bind dup: %v", err)
	}
	// 无绑定证书不参与迁移
	fresh := &AcmeCert{Domains: []string{"fresh.com"}, CADirectory: "staging", Status: "pending",
		RenewBeforeDays: 30, LastStatus: "never"}
	if err := db.CreateAcmeCert(fresh); err != nil {
		t.Fatalf("seed fresh: %v", err)
	}

	if err := backfillAcmeDeployTargets(db); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	got, err := db.ListAcmeDeployTargets(legacy.ID)
	if err != nil || len(got) != 1 {
		t.Fatalf("legacy backfilled = %v, %v", got, err)
	}
	if got[0].AgentID != "ag-old" || got[0].CertPath != "/old.crt.pem" || got[0].KeyPath != "/old.key.pem" ||
		got[0].LastDeployAt != 1000 || got[0].LastDeployError != "agent offline" {
		t.Errorf("backfilled row = %+v", got[0])
	}
	row, err := db.GetAcmeCert(legacy.ID)
	if err != nil {
		t.Fatalf("get legacy: %v", err)
	}
	if row.DeployAgentID != "" || row.DeployCertPath != "" || row.LastDeployAt != 0 {
		t.Errorf("legacy columns must be cleared, got %+v", row)
	}
	// dup 证书：不重复迁行
	if dups, _ := db.ListAcmeDeployTargets(dup.ID); len(dups) != 1 {
		t.Errorf("dup cert should keep exactly one target, got %+v", dups)
	}
	// 幂等：二次 backfill 零新增
	if err := backfillAcmeDeployTargets(db); err != nil {
		t.Fatalf("backfill 2: %v", err)
	}
	if again, _ := db.ListAcmeDeployTargets(legacy.ID); len(again) != 1 {
		t.Errorf("second backfill must be a no-op, got %+v", again)
	}
	if freshRows, _ := db.ListAcmeDeployTargets(fresh.ID); len(freshRows) != 0 {
		t.Errorf("unbound cert must stay unbound, got %+v", freshRows)
	}
}

// TestAcmeDeployTargetErrorBranches 错误面收口（100% 覆盖纪律）：closed db
// 覆盖查询/整替/状态回写/清理/迁移查询失败；RAISE(ABORT) 触发器对齐
// agent_expiry_test 先例，覆盖 Replace 插入失败与 backfill 插入/清列失败
func TestAcmeDeployTargetErrorBranches(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	cert := &AcmeCert{Domains: []string{"e.com"}, CADirectory: "staging", Status: "pending", RenewBeforeDays: 30, LastStatus: "never"}
	if err := db.CreateAcmeCert(cert); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.ReplaceAcmeDeployTargets(cert.ID, []AcmeDeployTarget{
		{AgentID: "ag-e", CertPath: "/e.crt.pem", KeyPath: "/e.key.pem"},
	}); err != nil {
		t.Fatalf("bind: %v", err)
	}

	// Replace 插入失败：BEFORE INSERT 触发器阻断（先删后插，删成功插被阻）
	if err := db.db.Exec(`CREATE TRIGGER acme_target_boom BEFORE INSERT ON acme_deploy_targets
		BEGIN SELECT RAISE(ABORT, 'target insert boom'); END`).Error; err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	err := db.ReplaceAcmeDeployTargets(cert.ID, []AcmeDeployTarget{
		{AgentID: "ag-x", CertPath: "/x.crt.pem", KeyPath: "/x.key.pem"},
	})
	if err == nil || !strings.Contains(err.Error(), "target insert boom") {
		t.Errorf("blocked insert = %v, want boom", err)
	}

	// backfill 插入失败：目标行清零（否则迁移走 n>0 跳过插入）+ 旧列非空 + 插入被阻
	if err := db.DeleteAcmeDeployTargets(cert.ID); err != nil {
		t.Fatalf("clear targets: %v", err)
	}
	if err := db.db.Exec(`UPDATE acme_certs SET deploy_agent_id = 'ag-old' WHERE id = ?`, cert.ID).Error; err != nil {
		t.Fatalf("set legacy col: %v", err)
	}
	if err := backfillAcmeDeployTargets(db); err == nil || !strings.Contains(err.Error(), "target insert boom") {
		t.Errorf("backfill insert blocked = %v, want boom", err)
	}

	// backfill 清列失败：插入放行（触发器换打 UPDATE acme_certs）。
	// 顺序：先（无触发器时）补目标行与旧列，再挂 UPDATE 触发器
	if err := db.db.Exec(`DROP TRIGGER acme_target_boom`).Error; err != nil {
		t.Fatalf("drop trigger: %v", err)
	}
	if err := db.ReplaceAcmeDeployTargets(cert.ID, []AcmeDeployTarget{
		{AgentID: "ag-e", CertPath: "/e.crt.pem", KeyPath: "/e.key.pem"},
	}); err != nil {
		t.Fatalf("rebind: %v", err)
	}
	if err := db.db.Exec(`UPDATE acme_certs SET deploy_agent_id = 'ag-old2' WHERE id = ?`, cert.ID).Error; err != nil {
		t.Fatalf("set legacy col 2: %v", err)
	}
	if err := db.db.Exec(`CREATE TRIGGER acme_cert_boom BEFORE UPDATE ON acme_certs
		BEGIN SELECT RAISE(ABORT, 'cert update boom'); END`).Error; err != nil {
		t.Fatalf("create update trigger: %v", err)
	}
	// 目标行已存在 → 迁移跳过插入、直接清列 → 撞 UPDATE 触发器
	if err := backfillAcmeDeployTargets(db); err == nil || !strings.Contains(err.Error(), "cert update boom") {
		t.Errorf("backfill clear blocked = %v, want boom", err)
	}
	if err := db.db.Exec(`DROP TRIGGER acme_cert_boom`).Error; err != nil {
		t.Fatalf("drop update trigger: %v", err)
	}

	// 目标删除失败（Replace 先删 / DeleteAcmeCert 随删）：BEFORE DELETE 触发器
	// 阻断——closed db 时 Transaction 在 Begin 即失败、闭包不执行，盖不到这里
	if err := db.db.Exec(`CREATE TRIGGER acme_target_del_boom BEFORE DELETE ON acme_deploy_targets
		BEGIN SELECT RAISE(ABORT, 'target delete boom'); END`).Error; err != nil {
		t.Fatalf("create delete trigger: %v", err)
	}
	err = db.DeleteAcmeCert(cert.ID)
	if err == nil || !strings.Contains(err.Error(), "target delete boom") {
		t.Errorf("blocked target delete on DeleteAcmeCert = %v, want boom", err)
	}
	err = db.ReplaceAcmeDeployTargets(cert.ID, nil)
	if err == nil || !strings.Contains(err.Error(), "target delete boom") {
		t.Errorf("blocked target delete on Replace = %v, want boom", err)
	}
	if err := db.db.Exec(`DROP TRIGGER acme_target_del_boom`).Error; err != nil {
		t.Fatalf("drop delete trigger: %v", err)
	}

	// closed db：查询/整替/状态回写/清理/迁移全部失败
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := db.ListAcmeDeployTargets(cert.ID); err == nil {
		t.Error("closed db List should fail")
	}
	if err := db.ReplaceAcmeDeployTargets(cert.ID, nil); err == nil {
		t.Error("closed db Replace should fail")
	}
	if err := db.UpdateAcmeDeployTargetStatus(1, 0, ""); err == nil {
		t.Error("closed db UpdateStatus should fail")
	}
	if err := db.DeleteAcmeDeployTargets(cert.ID); err == nil {
		t.Error("closed db DeleteTargets should fail")
	}
	if err := db.DeleteAcmeCert(cert.ID); err == nil {
		t.Error("closed db DeleteAcmeCert should fail")
	}
	if err := backfillAcmeDeployTargets(db); err == nil {
		t.Error("closed db backfill should fail")
	}
}
