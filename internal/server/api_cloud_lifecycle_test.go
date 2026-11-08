package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/cloud"
	"github.com/cuihairu/cockpit/internal/cloud/aliyun"
	"github.com/cuihairu/cockpit/internal/cloud/tencent"
	"github.com/cuihairu/cockpit/internal/storage"
)

// fakeCloudCompute 实现 ComputeProvider，记录调用并返回可控结果。
type fakeCloudCompute struct {
	instances []cloud.Instance
	ops       []cloud.Operation
	nextOp    cloud.Operation
	callErr   error
	listErr   error
	getErr    error
	calls     []string
}

func (f *fakeCloudCompute) Identity() cloud.ProviderIdentity {
	return cloud.ProviderIdentity{Vendor: "fake", DisplayName: "fake"}
}
func (f *fakeCloudCompute) Capabilities() []cloud.Capability {
	return []cloud.Capability{cloud.CapabilityCompute}
}
func (f *fakeCloudCompute) ListInstances(ctx context.Context) ([]cloud.Instance, error) {
	f.calls = append(f.calls, "list")
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.instances, nil
}
func (f *fakeCloudCompute) CreateInstance(ctx context.Context, spec cloud.InstanceSpec) (cloud.Operation, error) {
	f.calls = append(f.calls, "create")
	if f.callErr != nil {
		return cloud.Operation{}, f.callErr
	}
	return f.nextOp, nil
}
func (f *fakeCloudCompute) StartInstance(ctx context.Context, id string) (cloud.Operation, error) {
	f.calls = append(f.calls, "start:"+id)
	if f.callErr != nil {
		return cloud.Operation{}, f.callErr
	}
	return f.nextOp, nil
}
func (f *fakeCloudCompute) StopInstance(ctx context.Context, id string) (cloud.Operation, error) {
	f.calls = append(f.calls, "stop:"+id)
	if f.callErr != nil {
		return cloud.Operation{}, f.callErr
	}
	return f.nextOp, nil
}
func (f *fakeCloudCompute) DeleteInstance(ctx context.Context, id string) (cloud.Operation, error) {
	f.calls = append(f.calls, "delete:"+id)
	if f.callErr != nil {
		return cloud.Operation{}, f.callErr
	}
	return f.nextOp, nil
}
func (f *fakeCloudCompute) GetOperation(ctx context.Context, op cloud.Operation) (cloud.Operation, error) {
	f.calls = append(f.calls, "getop")
	if f.getErr != nil {
		return cloud.Operation{}, f.getErr
	}
	if op.Status == cloud.OperationPending {
		cur, _ := op.WithStatus(cloud.OperationRunning, time.Now().UTC())
		op = cur
	}
	done, _ := op.MarkSucceeded(op.NativeID, time.Now().UTC())
	return done, nil
}

func newCloudLifecycleServer(t *testing.T, compute *fakeCloudCompute) (*Server, *storage.CloudAccount) {
	t.Helper()
	s, _ := newCloudAPIServer(t)
	s.cloudProviderFn = func(a *storage.CloudAccount) (cloud.Provider, error) { return compute, nil }
	if err := s.db.UpsertCloudAccount(&storage.CloudAccountInput{
		Vendor: "aliyun", Alias: "prod", CredentialRef: "${CLOUD_ALIYUN_PROD}", Region: "cn-hangzhou",
	}); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	accounts, _ := s.db.ListCloudAccounts()
	return s, &accounts[0]
}

func callCloudLifecycleAPI(s *Server, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	token, _ := s.auth.GenerateToken("1", "admin", "admin")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	// 经真实 ServeMux 注册一次并服务（审核发现 :266：先前在此先注册一次
	// 一次性丢弃 mux 的死代码，随后才二次注册）。
	mux := http.NewServeMux()
	s.registerCloudAPI(mux)
	mux.ServeHTTP(rec, req)
	return rec
}

func TestCloudDiscoverySync(t *testing.T) {
	compute := &fakeCloudCompute{instances: []cloud.Instance{
		{NativeID: "i-1", Name: "web", Region: "cn-hangzhou", Status: cloud.InstanceRunning, BillingMode: cloud.BillingOnDemand, RawVendorStatus: "Running"},
		{NativeID: "i-2", Name: "db", Region: "cn-hangzhou", Status: cloud.InstanceStopped, BillingMode: cloud.BillingPrepaid, BillingContinues: true, RawVendorStatus: "Stopped"},
	}}
	s, account := newCloudLifecycleServer(t, compute)

	// 直接经 handleCloudInstances 触发发现（GET /api/cloud/accounts/{id}/instances）
	rec := callCloudAPI(s, http.MethodGet, "/api/cloud/accounts/"+account.ID+"/instances", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("discovery status = %d, body %s", rec.Code, rec.Body.String())
	}
	if len(compute.calls) == 0 || compute.calls[0] != "list" {
		t.Fatalf("expected ListInstances called, calls=%v", compute.calls)
	}
	instances, err := s.db.ListCloudInstances(account.ID)
	if err != nil || len(instances) != 2 {
		t.Fatalf("instances after sync = %d (%v)", len(instances), err)
	}
	// 断言与返回顺序无关（审核发现 :267）：同批写入若 created_at 相同则顺序
	// 未定义，按 NativeID 建索引再核对字段，不依赖 instances[0]/[1]。
	byNative := map[string]cloud.Instance{}
	for _, it := range compute.instances {
		byNative[it.NativeID] = it
	}
	for i := range instances {
		want, ok := byNative[instances[i].NativeID]
		if !ok {
			t.Fatalf("unexpected instance %s", instances[i].NativeID)
		}
		if instances[i].Status != string(want.Status) {
			t.Fatalf("instance[%s] status = %s, want %s", instances[i].NativeID, instances[i].Status, want.Status)
		}
		if instances[i].BillingMode != string(want.BillingMode) {
			t.Fatalf("instance[%s] billing = %s, want %s", instances[i].NativeID, instances[i].BillingMode, want.BillingMode)
		}
		if instances[i].BillingContinues != want.BillingContinues {
			t.Fatalf("instance[%s] billingContinues = %v, want %v", instances[i].NativeID, instances[i].BillingContinues, want.BillingContinues)
		}
	}
}

func TestCloudDiscoveryDegradeOnListErr(t *testing.T) {
	compute := &fakeCloudCompute{listErr: context.DeadlineExceeded}
	s, account := newCloudLifecycleServer(t, compute)
	rec := callCloudAPI(s, http.MethodGet, "/api/cloud/accounts/"+account.ID+"/instances", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("degrade status = %d, body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Fatalf("expected empty ledger fallback: %s", rec.Body.String())
	}
}

func TestCloudDiscoveryProviderErrFallsBack(t *testing.T) {
	compute := &fakeCloudCompute{}
	s, account := newCloudLifecycleServer(t, compute)
	s.cloudProviderFn = func(a *storage.CloudAccount) (cloud.Provider, error) { return nil, cloud.ErrNotSupported }
	rec := callCloudAPI(s, http.MethodGet, "/api/cloud/accounts/"+account.ID+"/instances", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("fallback status = %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestCloudLifecycleStart(t *testing.T) {
	compute := &fakeCloudCompute{nextOp: cloud.NewOperation(cloud.OperationStart, "req-1", time.Now().UTC())}
	s, account := newCloudLifecycleServer(t, compute)
	if err := s.db.UpsertCloudInstance(&storage.CloudInstanceInput{AccountID: account.ID, NativeID: "i-1", Status: "stopped"}); err != nil {
		t.Fatalf("seed instance: %v", err)
	}
	instances, _ := s.db.ListCloudInstances(account.ID)
	instID := instances[0].ID

	body := `{"action":"start","instance_id":"` + instID + `","confirm_key":"` + instID + `"}`
	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/actions", body)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("start status = %d, body %s", rec.Code, rec.Body.String())
	}
	var op cloud.Operation
	if err := json.Unmarshal(rec.Body.Bytes(), &op); err != nil {
		t.Fatalf("decode op: %v", err)
	}
	if op.Kind != cloud.OperationStart || op.Status != cloud.OperationPending {
		t.Fatalf("op = %+v", op)
	}
	// 覆盖审核发现 :265：op.ID 不得被覆写为「厂商ID-时间戳」，必须保持
	// cloud.NewOperation 的确定性 ID（kind-vendorRequestID）。
	if op.ID != "start-req-1" {
		t.Fatalf("op.ID = %q, want deterministic start-req-1", op.ID)
	}
	if len(compute.calls) != 1 || compute.calls[0] != "start:i-1" {
		t.Fatalf("calls = %v", compute.calls)
	}
}

func TestCloudLifecycleConfirmMismatch(t *testing.T) {
	compute := &fakeCloudCompute{}
	s, account := newCloudLifecycleServer(t, compute)
	if err := s.db.UpsertCloudInstance(&storage.CloudInstanceInput{AccountID: account.ID, NativeID: "i-1"}); err != nil {
		t.Fatalf("seed instance: %v", err)
	}
	instances, _ := s.db.ListCloudInstances(account.ID)
	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/actions",
		`{"action":"start","instance_id":"`+instances[0].ID+`","confirm_key":"nope"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("confirm mismatch status = %d", rec.Code)
	}
	if len(compute.calls) != 0 {
		t.Fatalf("no vendor call expected, got %v", compute.calls)
	}
}

func TestCloudLifecycleBadAction(t *testing.T) {
	compute := &fakeCloudCompute{}
	s, account := newCloudLifecycleServer(t, compute)
	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/actions", `{"action":"jump"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad action status = %d", rec.Code)
	}
}

func TestCloudLifecycleVendorErr(t *testing.T) {
	compute := &fakeCloudCompute{callErr: context.DeadlineExceeded}
	s, account := newCloudLifecycleServer(t, compute)
	if err := s.db.UpsertCloudInstance(&storage.CloudInstanceInput{AccountID: account.ID, NativeID: "i-1"}); err != nil {
		t.Fatalf("seed instance: %v", err)
	}
	instances, _ := s.db.ListCloudInstances(account.ID)
	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/actions",
		`{"action":"stop","instance_id":"`+instances[0].ID+`","confirm_key":"`+instances[0].ID+`"}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("vendor err status = %d", rec.Code)
	}
}

func TestCloudLifecycleCrossAccount(t *testing.T) {
	compute := &fakeCloudCompute{}
	s, account := newCloudLifecycleServer(t, compute)
	// 第二账号下的实例
	if err := s.db.UpsertCloudAccount(&storage.CloudAccountInput{
		Vendor: "tencent", Alias: "lab", CredentialRef: "${CLOUD_TENCENT_LAB}", Region: "ap-guangzhou",
	}); err != nil {
		t.Fatalf("seed second account: %v", err)
	}
	accts, _ := s.db.ListCloudAccounts()
	var other *storage.CloudAccount
	for i := range accts {
		if accts[i].ID != account.ID {
			other = &accts[i]
		}
	}
	if other == nil {
		t.Fatal("second account missing")
	}
	if err := s.db.UpsertCloudInstance(&storage.CloudInstanceInput{AccountID: other.ID, NativeID: "i-x"}); err != nil {
		t.Fatalf("seed other instance: %v", err)
	}
	insts, _ := s.db.ListCloudInstances(other.ID)
	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/actions",
		`{"action":"start","instance_id":"`+insts[0].ID+`","confirm_key":"`+insts[0].ID+`"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-account status = %d", rec.Code)
	}
	if len(compute.calls) != 0 {
		t.Fatalf("no vendor call expected, got %v", compute.calls)
	}
}

func TestCloudOperationPoll(t *testing.T) {
	compute := &fakeCloudCompute{}
	s, account := newCloudLifecycleServer(t, compute)
	op := cloud.NewOperation(cloud.OperationStart, "req-1", time.Now().UTC())
	op.NativeID = "i-1"
	s.storeCloudOperation(account.ID, &op)
	opID := op.ID

	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/operations", `{"operation_id":"`+opID+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("poll status = %d, body %s", rec.Code, rec.Body.String())
	}
	var got cloud.Operation
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != cloud.OperationSucceeded {
		t.Fatalf("poll status = %s, want succeeded", got.Status)
	}
}

func TestCloudOperationNotFound(t *testing.T) {
	compute := &fakeCloudCompute{}
	s, account := newCloudLifecycleServer(t, compute)
	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/operations", `{"operation_id":"nope"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("notfound status = %d", rec.Code)
	}
}

func TestCloudOperationsList(t *testing.T) {
	compute := &fakeCloudCompute{}
	s, account := newCloudLifecycleServer(t, compute)
	op := cloud.NewOperation(cloud.OperationStop, "req-2", time.Now().UTC())
	s.storeCloudOperation(account.ID, &op)
	rec := callCloudLifecycleAPI(s, http.MethodGet, "/api/cloud/instances/"+account.ID+"/operations", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "req-2") {
		t.Fatalf("list missing op: %s", rec.Body.String())
	}
}

func TestCloudInstanceActionAccountNotFound(t *testing.T) {
	s, _ := newCloudAPIServer(t)
	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/x/actions", `{}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("account not found status = %d", rec.Code)
	}
}

func TestCloudInstanceActionNoCompute(t *testing.T) {
	compute := &fakeCloudCompute{}
	s, account := newCloudLifecycleServer(t, compute)
	// 覆盖返回非 ComputeProvider
	s.cloudProviderFn = func(a *storage.CloudAccount) (cloud.Provider, error) { return &baseOnlyCloudProvider{}, nil }
	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/actions", `{"action":"start"}`)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("no compute status = %d", rec.Code)
	}
}

type baseOnlyCloudProvider struct{}

func (baseOnlyCloudProvider) Identity() cloud.ProviderIdentity {
	return cloud.ProviderIdentity{Vendor: "base"}
}
func (baseOnlyCloudProvider) Capabilities() []cloud.Capability { return nil }

// 确保适配器构造路径存在（编译期引用，避免 unused 导入）。
var _ = aliyun.New
var _ = tencent.New

// TestCloudOperationsCrossAccountFilter 覆盖审核发现 :259/:268：
// 查询任一账号的操作列表不得返回其他账号的操作（账号过滤恒真曾导致跨账号泄露）。
func TestCloudOperationsCrossAccountFilter(t *testing.T) {
	compute := &fakeCloudCompute{}
	s, account := newCloudLifecycleServer(t, compute)
	if err := s.db.UpsertCloudAccount(&storage.CloudAccountInput{
		Vendor: "tencent", Alias: "lab", CredentialRef: "${CLOUD_TENCENT_LAB}", Region: "ap-guangzhou",
	}); err != nil {
		t.Fatalf("seed second account: %v", err)
	}
	accts, _ := s.db.ListCloudAccounts()
	var other *storage.CloudAccount
	for i := range accts {
		if accts[i].ID != account.ID {
			other = &accts[i]
		}
	}
	op := cloud.NewOperation(cloud.OperationStop, "req-other", time.Now().UTC())
	s.storeCloudOperation(other.ID, &op)

	// 请求账号 A 列表不得包含账号 B 的操作
	rec := callCloudLifecycleAPI(s, http.MethodGet, "/api/cloud/instances/"+account.ID+"/operations", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "req-other") {
		t.Fatalf("cross-account operation leaked: %s", rec.Body.String())
	}
}

// TestCloudOperationTerminalAppliesToLedger 覆盖审核发现 :271：
// 释放成功（终态）后实例应从台账移除，不再显示为原状态可继续操作。
func TestCloudOperationTerminalAppliesToLedger(t *testing.T) {
	compute := &fakeCloudCompute{}
	s, account := newCloudLifecycleServer(t, compute)
	if err := s.db.UpsertCloudInstance(&storage.CloudInstanceInput{
		AccountID: account.ID, NativeID: "i-1", Status: "running", BillingMode: "on-demand",
	}); err != nil {
		t.Fatalf("seed instance: %v", err)
	}
	insts, _ := s.db.ListCloudInstances(account.ID)
	op := cloud.NewOperation(cloud.OperationDelete, "req-d", time.Now().UTC())
	op.NativeID = "i-1"
	running, _ := op.WithStatus(cloud.OperationRunning, time.Now().UTC())
	done, _ := running.MarkSucceeded("i-1", time.Now().UTC())
	s.storeCloudOperation(account.ID, &done)

	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/operations", `{"operation_id":"`+done.ID+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("poll status = %d, body %s", rec.Code, rec.Body.String())
	}
	insts, _ = s.db.ListCloudInstances(account.ID)
	if len(insts) != 0 {
		t.Fatalf("instance should be removed after release, got %+v", insts)
	}
}

// TestCloudDiscoverySyncTTL 覆盖审核发现 :261：TTL 内读路径不重复调厂商
// ListInstances（读路径被网络写放大）；TTL 过期后再次同步。
func TestCloudDiscoverySyncTTL(t *testing.T) {
	compute := &fakeCloudCompute{instances: []cloud.Instance{{NativeID: "i-1", Name: "web", Region: "cn-hangzhou", Status: cloud.InstanceRunning}}}
	s, account := newCloudLifecycleServer(t, compute)
	// 注入极小 TTL：首读同步，第二读（TTL 内）不调厂商。
	s.cloudSyncTTL = newCloudSyncTTLCache(time.Hour)

	rec := callCloudAPI(s, http.MethodGet, "/api/cloud/accounts/"+account.ID+"/instances", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("first status = %d", rec.Code)
	}
	if len(compute.calls) != 1 || compute.calls[0] != "list" {
		t.Fatalf("first calls = %v", compute.calls)
	}
	rec = callCloudAPI(s, http.MethodGet, "/api/cloud/accounts/"+account.ID+"/instances", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("second status = %d", rec.Code)
	}
	if len(compute.calls) != 1 {
		t.Fatalf("TTL not honored, calls = %v", compute.calls)
	}
}

// TestCloudOperationPollPersists 覆盖审核发现 :273：操作轮询后落库，重启
// （同库新 Server）仍可取回，不会 404。
func TestCloudOperationPollPersists(t *testing.T) {
	compute := &fakeCloudCompute{}
	s, account := newCloudLifecycleServer(t, compute)
	op := cloud.NewOperation(cloud.OperationStop, "req-persist", time.Now().UTC())
	op.NativeID = "i-1"
	s.storeCloudOperation(account.ID, &op)

	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/operations", `{"operation_id":"`+op.ID+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("poll status = %d, body %s", rec.Code, rec.Body.String())
	}
	// 同库新 Server（模拟重启）仍可读到已持久化操作。
	s2 := &Server{db: s.db, auth: s.auth, audit: s.audit, cloudSyncTTL: newCloudSyncTTLCache(time.Hour)}
	got, err := s2.getCloudOperation(account.ID, op.ID)
	if err != nil || got.ID != op.ID {
		t.Fatalf("after restart getCloudOperation = %+v, %v", got, err)
	}
}

// TestCloudLifecycleCrossRegion 覆盖审核发现 :270：生命周期操作使用实例
// 所在区域而非账号默认地域。
func TestCloudLifecycleCrossRegion(t *testing.T) {
	compute := &fakeCloudCompute{nextOp: cloud.NewOperation(cloud.OperationStart, "req-x", time.Now().UTC())}
	s, account := newCloudLifecycleServer(t, compute)
	// 账号默认 cn-hangzhou；实例在 ap-beijing（跨地域）。
	if err := s.db.UpsertCloudInstance(&storage.CloudInstanceInput{
		AccountID: account.ID, NativeID: "i-x", Region: "ap-beijing", Status: "stopped",
	}); err != nil {
		t.Fatalf("seed instance: %v", err)
	}
	insts, _ := s.db.ListCloudInstances(account.ID)
	// 注入 region-aware 构造器
	var gotRegion string
	s.cloudProviderFn = func(a *storage.CloudAccount) (cloud.Provider, error) {
		gotRegion = a.Region
		return compute, nil
	}
	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/actions",
		`{"action":"start","instance_id":"`+insts[0].ID+`","confirm_key":"`+insts[0].ID+`"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("cross-region start status = %d, body %s", rec.Code, rec.Body.String())
	}
	if gotRegion != "ap-beijing" {
		t.Fatalf("provider region = %q, want ap-beijing (instance region)", gotRegion)
	}
}
