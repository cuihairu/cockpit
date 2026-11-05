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
	s.registerCloudAPI(http.NewServeMux())
	// 直接调 handler（与现有 callCloudAPI 同法）
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
	if instances[0].NativeID != "i-1" || instances[0].Status != string(cloud.InstanceRunning) {
		t.Fatalf("instance[0] = %+v", instances[0])
	}
	if instances[1].BillingContinues != true || instances[1].BillingMode != string(cloud.BillingPrepaid) {
		t.Fatalf("instance[1] billing = %+v", instances[1])
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
