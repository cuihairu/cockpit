package server

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/cloud"
	"github.com/cuihairu/cockpit/internal/storage"
	"github.com/cuihairu/cockpit/internal/vault"
)

// cloudVaultCreds 返回可控凭据的 vault 假件（provider 真实构造直测）。
type cloudVaultCreds struct {
	entry *vault.Entry
}

func (f *cloudVaultCreds) Name() string                         { return "creds" }
func (f *cloudVaultCreds) Unlock(string) error                  { return nil }
func (f *cloudVaultCreds) Lock()                                {}
func (f *cloudVaultCreds) Read(string) (*vault.Entry, error)    { return f.entry, nil }
func (f *cloudVaultCreds) Write(string, *vault.Entry) error     { return nil }
func (f *cloudVaultCreds) List(string) ([]vault.EntryMeta, error) { return nil, nil }
func (f *cloudVaultCreds) Delete(string) error                  { return nil }

// TestCovCloudProviderWithRegionRealPath 直测 cloudProviderWithRegion 的真实
// 构造路径（不注入 cloudProviderFn）。既有 lifecycle/provider 测试全走注入
// 点，2f4aa23f 引入的真实分支（disabled/未知厂商/aliyun/tencent 的 nil 与
// 成功）此前未覆盖。
func TestCovCloudProviderWithRegionRealPath(t *testing.T) {
	s, _ := newCloudAPIServer(t)
	s.cloudProviderFn = nil
	s.vaultProvider = &cloudVaultCreds{entry: &vault.Entry{Username: "AK", Value: "SK"}}

	if _, err := s.cloudProviderWithRegion(&storage.CloudAccount{Enabled: false, Vendor: "aliyun"}, "cn-hangzhou"); err != cloud.ErrNotSupported {
		t.Fatalf("disabled account = %v", err)
	}
	if _, err := s.cloudProviderWithRegion(&storage.CloudAccount{Enabled: true, Vendor: "aws"}, "cn-hangzhou"); err != cloud.ErrNotSupported {
		t.Fatalf("unknown vendor = %v", err)
	}
	if _, err := s.cloudProviderWithRegion(&storage.CloudAccount{Enabled: true, Vendor: "aliyun"}, "cn-hangzhou"); err != nil {
		t.Fatalf("aliyun ok = %v", err)
	}
	if _, err := s.cloudProviderWithRegion(&storage.CloudAccount{Enabled: true, Vendor: "aliyun"}, ""); err == nil {
		t.Fatal("aliyun empty region should be not configured")
	}
	if _, err := s.cloudProviderWithRegion(&storage.CloudAccount{Enabled: true, Vendor: "tencent"}, "cn-hangzhou"); err != nil {
		t.Fatalf("tencent ok = %v", err)
	}
	if _, err := s.cloudProviderWithRegion(&storage.CloudAccount{Enabled: true, Vendor: "tencent"}, ""); err == nil {
		t.Fatal("tencent empty region should be not configured")
	}
}

// TestCovCloudProviderForAccountGaps 直测 cloudProviderForAccount：成功路径
// 与账号查无错误分支。
func TestCovCloudProviderForAccountGaps(t *testing.T) {
	s, _ := newCloudAPIServer(t)
	s.cloudProviderFn = nil
	s.vaultProvider = &cloudVaultCreds{entry: &vault.Entry{Username: "AK", Value: "SK"}}
	if err := s.db.UpsertCloudAccount(&storage.CloudAccountInput{
		Vendor: "aliyun", Alias: "prod", CredentialRef: "${X}", Region: "cn-hangzhou",
	}); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	accounts, _ := s.db.ListCloudAccounts()
	if _, _, err := s.cloudProviderForAccount(accounts[0].ID); err != nil {
		t.Fatalf("ok = %v", err)
	}
	if _, _, err := s.cloudProviderForAccount("nope"); err == nil {
		t.Fatal("missing account should err")
	}
}

// TestCovApplyTerminalCloudOperationGaps 直测 applyTerminalCloudOperation：
// 非终态早退、Delete/Start/Stop 台账落账、闭库下 ListCloudInstances 错误。
func TestCovApplyTerminalCloudOperationGaps(t *testing.T) {
	s, account := newCloudLifecycleServer(t, &fakeCloudCompute{})

	if err := s.applyTerminalCloudOperation(account.ID, &cloud.Operation{Status: cloud.OperationPending}); err != nil {
		t.Fatalf("pending op should early-return nil, got %v", err)
	}

	for _, c := range []struct {
		nativeID, status string
	}{
		{"i-d", "running"},
		{"i-s", "stopped"},
		{"i-t", "running"},
	} {
		if err := s.db.UpsertCloudInstance(&storage.CloudInstanceInput{
			AccountID: account.ID, NativeID: c.nativeID, Region: "cn-hangzhou", Status: c.status,
		}); err != nil {
			t.Fatalf("seed instance %s: %v", c.nativeID, err)
		}
	}

	// Delete 成功：匹配 NativeID → 删台账。
	if err := s.applyTerminalCloudOperation(account.ID, &cloud.Operation{
		Status: cloud.OperationSucceeded, Kind: cloud.OperationDelete, NativeID: "i-d",
	}); err != nil {
		t.Fatalf("delete = %v", err)
	}
	// Start 成功：status → running。
	if err := s.applyTerminalCloudOperation(account.ID, &cloud.Operation{
		Status: cloud.OperationSucceeded, Kind: cloud.OperationStart, NativeID: "i-s",
	}); err != nil {
		t.Fatalf("start = %v", err)
	}
	// Stop 成功：status → stopped。
	if err := s.applyTerminalCloudOperation(account.ID, &cloud.Operation{
		Status: cloud.OperationSucceeded, Kind: cloud.OperationStop, NativeID: "i-t",
	}); err != nil {
		t.Fatalf("stop = %v", err)
	}

	// 闭库 → ListCloudInstances 错误（Delete 与 Start 两条错误分支）。
	sd, acct := newCloudLifecycleServer(t, &fakeCloudCompute{})
	covCloseDB(t, sd)
	if err := sd.applyTerminalCloudOperation(acct.ID, &cloud.Operation{
		Status: cloud.OperationSucceeded, Kind: cloud.OperationDelete, NativeID: "x",
	}); err == nil {
		t.Fatal("delete on closed db should err")
	}
	if err := sd.applyTerminalCloudOperation(acct.ID, &cloud.Operation{
		Status: cloud.OperationSucceeded, Kind: cloud.OperationStart, NativeID: "x",
	}); err == nil {
		t.Fatal("start on closed db should err")
	}
}

// TestCovCloudGetOperationDBErr POST /operations 时 GetCloudOperation 返回
// 非 not-found 错误 → 500（审核发现 :271 后的加载错误面）。ErrNotFound 走
// 404（既有 TestCloudOperationNotFound），非 not-found 需闭库直测触发。
func TestCovCloudGetOperationDBErr(t *testing.T) {
	compute := &fakeCloudCompute{}
	s, account := newCloudLifecycleServer(t, compute)
	covCloseDB(t, s)
	rec := callCloudLifecycleAPI(s, http.MethodPost,
		"/api/cloud/instances/"+account.ID+"/operations", `{"operation_id":"op-1"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("get op db err status = %d, body %s", rec.Code, rec.Body.String())
	}
}

// TestCovCloudAccountValidateSecretGaps 补 validate() 的 secret-required
// 分支（region 校验通过但凭据缺失）。
func TestCovCloudAccountValidateSecretGaps(t *testing.T) {
	req := &cloudAccountRequest{Vendor: "aliyun", Alias: "prod", Region: "cn-hangzhou"}
	if msg, ok := req.validate(); ok || msg != "secret required" {
		t.Fatalf("validate() = %q, %v; want secret required", msg, ok)
	}
}

// TestCovCloudProviderRealPathGaps 直测 cloudProvider（非 WithRegion）的
// 未知厂商 default 与 aliyun 缺 region 构造失败分支。
func TestCovCloudProviderRealPathGaps(t *testing.T) {
	s, _ := newCloudAPIServer(t)
	s.cloudProviderFn = nil
	s.vaultProvider = &cloudVaultCreds{entry: &vault.Entry{Username: "AK", Value: "SK"}}
	if _, err := s.cloudProvider(&storage.CloudAccount{Enabled: true, Vendor: "aws"}); err != cloud.ErrNotSupported {
		t.Fatalf("unknown vendor = %v", err)
	}
	_, err := s.cloudProvider(&storage.CloudAccount{Enabled: true, Vendor: "aliyun"})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("aliyun empty region err = %v, want not configured", err)
	}
}

// TestCovCloudOperationListDBErr 列表路径闭库：listCloudOperations 吞错返回
// 空列表（列表页不因台账故障 5xx）。
func TestCovCloudOperationListDBErr(t *testing.T) {
	s, account := newCloudLifecycleServer(t, &fakeCloudCompute{})
	covCloseDB(t, s)
	rec := callCloudLifecycleAPI(s, http.MethodGet,
		"/api/cloud/instances/"+account.ID+"/operations", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list closed db status = %d, body %s", rec.Code, rec.Body.String())
	}
}

// TestCovCloudPollRegionFromLedger 轮询路径 region 取实例台账（跨地域实例
// 轮询用实例所在 region，而非账号默认地域）。
func TestCovCloudPollRegionFromLedger(t *testing.T) {
	compute := &fakeCloudCompute{}
	s, account := newCloudLifecycleServer(t, compute)
	if err := s.db.UpsertCloudInstance(&storage.CloudInstanceInput{
		AccountID: account.ID, NativeID: "i-r2", Region: "ap-beijing", Status: "running",
	}); err != nil {
		t.Fatalf("seed instance: %v", err)
	}
	op := cloud.NewOperation(cloud.OperationStart, "req-lr", time.Now().UTC())
	op.NativeID = "i-r2"
	s.storeCloudOperation(account.ID, &op)
	var gotRegion string
	s.cloudProviderFn = func(a *storage.CloudAccount) (cloud.Provider, error) {
		gotRegion = a.Region
		return compute, nil
	}
	rec := callCloudLifecycleAPI(s, http.MethodPost,
		"/api/cloud/instances/"+account.ID+"/operations", `{"operation_id":"`+op.ID+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("poll status = %d, body %s", rec.Code, rec.Body.String())
	}
	if gotRegion != "ap-beijing" {
		t.Fatalf("poll region = %q, want ap-beijing (ledger override)", gotRegion)
	}
}

// TestCovApplyTerminalCreateKind create 终态无需台账回写（native ID 已在
// 受理时落库），早退分支。
func TestCovApplyTerminalCreateKind(t *testing.T) {
	s, account := newCloudLifecycleServer(t, &fakeCloudCompute{})
	if err := s.applyTerminalCloudOperation(account.ID, &cloud.Operation{
		Status: cloud.OperationSucceeded, Kind: cloud.OperationCreate, NativeID: "i-new",
	}); err != nil {
		t.Fatalf("create terminal = %v", err)
	}
}

// TestCovCloudPollPartialDBFailure 部分表故障：轮询链路里 GetCloudOperation
// 成功但 GetCloudAccount / ListCloudInstances 失败——闭库会让第一步就 500，
// 无法注入这种部分故障，经 Gorm() drop 单表直测错误分支。
func TestCovCloudPollPartialDBFailure(t *testing.T) {
	// 非终态轮询 + cloud_accounts 缺失 → GetCloudAccount 失败 → 500。
	s, account := newCloudLifecycleServer(t, &fakeCloudCompute{})
	op := cloud.NewOperation(cloud.OperationStart, "req-pf1", time.Now().UTC())
	op.NativeID = "i-x"
	s.storeCloudOperation(account.ID, &op)
	if err := s.db.Gorm().Exec("DROP TABLE cloud_accounts").Error; err != nil {
		t.Fatalf("drop cloud_accounts: %v", err)
	}
	rec := callCloudLifecycleAPI(s, http.MethodPost,
		"/api/cloud/instances/"+account.ID+"/operations", `{"operation_id":"`+op.ID+`"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("account failure status = %d, body %s", rec.Code, rec.Body.String())
	}

	// 已终态 op + cloud_instances 缺失 → apply 台账失败仅记日志，仍 200。
	s2, account2 := newCloudLifecycleServer(t, &fakeCloudCompute{})
	op2 := cloud.NewOperation(cloud.OperationDelete, "req-pf2", time.Now().UTC())
	op2.NativeID = "i-y"
	running2, err := op2.WithStatus(cloud.OperationRunning, time.Now().UTC())
	if err != nil {
		t.Fatalf("to running: %v", err)
	}
	succ2, err := running2.MarkSucceeded("i-y", time.Now().UTC())
	if err != nil {
		t.Fatalf("to succeeded: %v", err)
	}
	s2.storeCloudOperation(account2.ID, &succ2)
	if err := s2.db.Gorm().Exec("DROP TABLE cloud_instances").Error; err != nil {
		t.Fatalf("drop cloud_instances: %v", err)
	}
	rec = callCloudLifecycleAPI(s2, http.MethodPost,
		"/api/cloud/instances/"+account2.ID+"/operations", `{"operation_id":"`+succ2.ID+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("terminal apply failure status = %d, body %s", rec.Code, rec.Body.String())
	}

	// 轮询推进到终态 + cloud_instances 缺失 → advanced apply 失败仅记日志。
	s3, account3 := newCloudLifecycleServer(t, &fakeCloudCompute{})
	op3 := cloud.NewOperation(cloud.OperationStop, "req-pf3", time.Now().UTC())
	op3.NativeID = "i-z"
	s3.storeCloudOperation(account3.ID, &op3)
	if err := s3.db.Gorm().Exec("DROP TABLE cloud_instances").Error; err != nil {
		t.Fatalf("drop cloud_instances: %v", err)
	}
	rec = callCloudLifecycleAPI(s3, http.MethodPost,
		"/api/cloud/instances/"+account3.ID+"/operations", `{"operation_id":"`+op3.ID+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("advanced apply failure status = %d, body %s", rec.Code, rec.Body.String())
	}
}

// TestCovCloudActionRegionOverride 覆盖 req.Region（审核发现 :270）与
// provider 构造错误两个边界分支。
func TestCovCloudActionRegionOverride(t *testing.T) {
	compute := &fakeCloudCompute{nextOp: cloud.NewOperation(cloud.OperationStart, "req-r", time.Now().UTC())}
	s, account := newCloudLifecycleServer(t, compute)
	if err := s.db.UpsertCloudInstance(&storage.CloudInstanceInput{
		AccountID: account.ID, NativeID: "i-r", Region: "ap-beijing", Status: "stopped",
	}); err != nil {
		t.Fatalf("seed instance: %v", err)
	}
	insts, _ := s.db.ListCloudInstances(account.ID)
	var gotRegion string
	s.cloudProviderFn = func(a *storage.CloudAccount) (cloud.Provider, error) {
		gotRegion = a.Region
		return compute, nil
	}
	// req.Region 覆盖实例 region（us-east-1 优先于 ap-beijing）。
	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/actions",
		`{"action":"start","instance_id":"`+insts[0].ID+`","region":"us-east-1","confirm_key":"`+insts[0].ID+`"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("region override status = %d, body %s", rec.Code, rec.Body.String())
	}
	if gotRegion != "us-east-1" {
		t.Fatalf("provider region = %q, want us-east-1 (req override)", gotRegion)
	}

	// provider 构造错误 → handleCloudProviderError 分支。
	s2, account2 := newCloudLifecycleServer(t, &fakeCloudCompute{})
	s2.cloudProviderFn = func(a *storage.CloudAccount) (cloud.Provider, error) {
		return nil, errors.New("provider boom")
	}
	rec2 := callCloudLifecycleAPI(s2, http.MethodPost, "/api/cloud/instances/"+account2.ID+"/actions",
		`{"action":"start","instance_id":"","confirm_key":"`+account2.ID+`"}`)
	if rec2.Code == http.StatusAccepted {
		t.Fatalf("provider err should not be accepted, body %s", rec2.Body.String())
	}
}
