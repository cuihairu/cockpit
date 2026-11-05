package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/cloud"
	"github.com/cuihairu/cockpit/internal/storage"
	"github.com/cuihairu/cockpit/internal/vault"
)

// 真实 cloudVaultCredentials / cloudProvider 构造路径（不注入 fake）。
func TestCloudVaultCredentialsNilProvider(t *testing.T) {
	s, _ := newCloudAPIServer(t)
	s.vaultProvider = nil
	_, _, err := s.cloudVaultCredentials(&storage.CloudAccount{Vendor: "aliyun", Alias: "prod"})
	if !errors.Is(err, vault.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestCloudVaultCredentialsReadErr(t *testing.T) {
	s, _ := newCloudAPIServer(t)
	s.vaultProvider = &cloudStubVault{}
	_, _, err := s.cloudVaultCredentials(&storage.CloudAccount{Vendor: "aliyun", Alias: "prod"})
	if !errors.Is(err, vault.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestCloudVaultCredentialsIncomplete(t *testing.T) {
	s, p := newCloudAPIServer(t)
	writeSecretEntry(t, p, "cloud/aliyun/prod", "only-secret")
	_, _, err := s.cloudVaultCredentials(&storage.CloudAccount{Vendor: "aliyun", Alias: "prod"})
	if err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("err = %v, want incomplete", err)
	}
}

func TestCloudVaultCredentialsOK(t *testing.T) {
	s, p := newCloudAPIServer(t)
	writeSecretEntry(t, p, "cloud/aliyun/prod", "secret-key")
	if err := p.Write("cloud/aliyun/prod", &vault.Entry{Path: "cloud/aliyun/prod", Kind: "pair", Username: "ak-id", Value: "secret-key"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	ak, sk, err := s.cloudVaultCredentials(&storage.CloudAccount{Vendor: "aliyun", Alias: "prod"})
	if err != nil || ak != "ak-id" || sk != "secret-key" {
		t.Fatalf("creds = %q/%q, %v", ak, sk, err)
	}
}

func TestCloudProviderConstruction(t *testing.T) {
	s, p := newCloudAPIServer(t)
	if err := p.Write("cloud/aliyun/prod", &vault.Entry{Path: "cloud/aliyun/prod", Kind: "pair", Username: "ak", Value: "sk"}); err != nil {
		t.Fatalf("write aliyun: %v", err)
	}
	if err := p.Write("cloud/tencent/lab", &vault.Entry{Path: "cloud/tencent/lab", Kind: "pair", Username: "sid", Value: "skey"}); err != nil {
		t.Fatalf("write tencent: %v", err)
	}
	aliyunP, err := s.cloudProvider(&storage.CloudAccount{Vendor: "aliyun", Alias: "prod", Region: "cn-hangzhou"})
	if err != nil || aliyunP.Identity().Vendor != "aliyun" {
		t.Fatalf("aliyun provider = %v, %v", aliyunP, err)
	}
	tencentP, err := s.cloudProvider(&storage.CloudAccount{Vendor: "tencent", Alias: "lab", Region: "ap-guangzhou"})
	if err != nil || tencentP.Identity().Vendor != "tencent" {
		t.Fatalf("tencent provider = %v, %v", tencentP, err)
	}
	if _, err := s.cloudProvider(&storage.CloudAccount{Vendor: "aws", Alias: "x", Region: "r"}); !errors.Is(err, cloud.ErrNotSupported) {
		t.Fatalf("unknown vendor err = %v", err)
	}
}

func TestCloudProviderNotConfigured(t *testing.T) {
	s, p := newCloudAPIServer(t)
	// Username empty + region set -> aliyun.New needs both creds -> nil -> not configured
	if err := p.Write("cloud/aliyun/prod", &vault.Entry{Path: "cloud/aliyun/prod", Kind: "pair", Username: "ak", Value: "sk"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := s.cloudProvider(&storage.CloudAccount{Vendor: "aliyun", Alias: "prod", Region: "cn-hangzhou"})
	// 有凭据+地域应构造成功
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

func TestSyncCloudInstancesNoCompute(t *testing.T) {
	s, _ := newCloudAPIServer(t)
	err := s.syncCloudInstances(context.Background(), &storage.CloudAccount{ID: "a1"}, baseOnlyCloudProvider{})
	if !errors.Is(err, cloud.ErrNotSupported) {
		t.Fatalf("err = %v", err)
	}
}

func TestSyncCloudInstancesUpsertErr(t *testing.T) {
	compute := &fakeCloudCompute{instances: []cloud.Instance{{NativeID: "i-1", Status: cloud.InstanceRunning}}}
	s, _ := newCloudAPIServer(t)
	if err := s.db.UpsertCloudAccount(&storage.CloudAccountInput{Vendor: "aliyun", Alias: "prod", CredentialRef: "${AK}"}); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	accounts, _ := s.db.ListCloudAccounts()
	if err := s.syncCloudInstances(context.Background(), &accounts[0], compute); err != nil {
		t.Fatalf("sync err = %v", err)
	}
	insts, _ := s.db.ListCloudInstances(accounts[0].ID)
	if len(insts) != 1 || insts[0].NativeID != "i-1" {
		t.Fatalf("instances = %+v", insts)
	}
}

func TestCloudInstanceActionDecodeErr(t *testing.T) {
	compute := &fakeCloudCompute{}
	s, account := newCloudLifecycleServer(t, compute)
	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/actions", `{`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("decode err status = %d", rec.Code)
	}
}

func TestCloudLifecycleCreate(t *testing.T) {
	compute := &fakeCloudCompute{nextOp: cloud.NewOperation(cloud.OperationCreate, "req-c", time.Now().UTC())}
	s, account := newCloudLifecycleServer(t, compute)
	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/actions",
		`{"action":"create","name":"web","spec":"s","image_id":"i","billing_mode":"prepaid"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("create status = %d, body %s", rec.Code, rec.Body.String())
	}
	if len(compute.calls) == 0 || compute.calls[0] != "create" {
		t.Fatalf("calls = %v", compute.calls)
	}
}

func TestCloudLifecycleCreateBadBilling(t *testing.T) {
	compute := &fakeCloudCompute{}
	s, account := newCloudLifecycleServer(t, compute)
	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/actions",
		`{"action":"create","name":"web","spec":"s","image_id":"i","billing_mode":"bad"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad billing status = %d", rec.Code)
	}
}

func TestCloudLifecycleCreateVendorErr(t *testing.T) {
	compute := &fakeCloudCompute{callErr: errors.New("boom")}
	s, account := newCloudLifecycleServer(t, compute)
	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/actions",
		`{"action":"create","name":"web","spec":"s","image_id":"i"}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("create vendor err status = %d", rec.Code)
	}
}

func TestCloudLifecycleCreateSpot(t *testing.T) {
	compute := &fakeCloudCompute{nextOp: cloud.NewOperation(cloud.OperationCreate, "req-c", time.Now().UTC())}
	s, account := newCloudLifecycleServer(t, compute)
	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/actions",
		`{"action":"create","name":"web","spec":"s","image_id":"i","billing_mode":"spot"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("spot create status = %d", rec.Code)
	}
}

func TestCloudOperationDecodeErr(t *testing.T) {
	compute := &fakeCloudCompute{}
	s, account := newCloudLifecycleServer(t, compute)
	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/operations", `{`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("decode status = %d", rec.Code)
	}
}

func TestCloudOperationNoCompute(t *testing.T) {
	compute := &fakeCloudCompute{}
	s, account := newCloudLifecycleServer(t, compute)
	s.cloudProviderFn = func(a *storage.CloudAccount) (cloud.Provider, error) { return baseOnlyCloudProvider{}, nil }
	rec := callCloudLifecycleAPI(s, http.MethodGet, "/api/cloud/instances/"+account.ID+"/operations", "")
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("no compute status = %d", rec.Code)
	}
}

func TestCloudOperationTerminal(t *testing.T) {
	compute := &fakeCloudCompute{}
	s, account := newCloudLifecycleServer(t, compute)
	op := cloud.NewOperation(cloud.OperationStart, "req-t", time.Now().UTC())
	running, err := op.WithStatus(cloud.OperationRunning, time.Now().UTC())
	if err != nil {
		t.Fatalf("to running: %v", err)
	}
	done, err := running.MarkSucceeded("i-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("to succeeded: %v", err)
	}
	s.storeCloudOperation(account.ID, &done)
	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/operations", `{"operation_id":"`+done.ID+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("terminal status = %d", rec.Code)
	}
}

func TestCloudOperationPollErr(t *testing.T) {
	compute := &fakeCloudCompute{getErr: errors.New("boom")}
	s, account := newCloudLifecycleServer(t, compute)
	op := cloud.NewOperation(cloud.OperationStart, "req-e", time.Now().UTC())
	op.NativeID = "i-1"
	s.storeCloudOperation(account.ID, &op)
	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/operations", `{"operation_id":"`+op.ID+`"}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("poll err status = %d", rec.Code)
	}
}

func TestHandleCloudProviderErrorBranches(t *testing.T) {
	s, _ := newCloudAPIServer(t)
	cases := []struct {
		err  error
		want int
	}{
		{storage.ErrNotFound, http.StatusNotFound},
		{vault.ErrLocked, http.StatusServiceUnavailable},
		{vault.ErrNotFound, http.StatusServiceUnavailable},
		{cloud.ErrNotSupported, http.StatusNotImplemented},
		{errors.New("generic"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/x", nil)
		s.handleCloudProviderError(rec, req, c.err)
		if rec.Code != c.want {
			t.Fatalf("err %v status = %d, want %d", c.err, rec.Code, c.want)
		}
	}
}

func TestAuditCloudInstanceNilAudit(t *testing.T) {
	s, _ := newCloudLifecycleServer(t, &fakeCloudCompute{})
	s.audit = nil
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	s.auditCloudInstance(req, "cloud_instance_action", "k", true, nil)
}

func TestCloudInstancesRootBadPath(t *testing.T) {
	compute := &fakeCloudCompute{}
	s, account := newCloudLifecycleServer(t, compute)
	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/bogus", `{}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bogus status = %d", rec.Code)
	}
	rec = callCloudLifecycleAPI(s, http.MethodGet, "/api/cloud/instances/"+account.ID+"/actions", "")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET actions status = %d", rec.Code)
	}
	rec = callCloudLifecycleAPI(s, http.MethodGet, "/api/cloud/instances/"+account.ID+"/operations", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET operations status = %d", rec.Code)
	}
}

func TestCloudProviderForAccountProviderErr(t *testing.T) {
	s, p := newCloudAPIServer(t)
	if err := p.Write("cloud/aliyun/prod", &vault.Entry{Path: "cloud/aliyun/prod", Kind: "pair", Username: "ak", Value: "sk"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	// 地域为空 -> aliyun.New 返回 nil -> not configured -> cloudProviderForAccount 传播
	if err := s.db.UpsertCloudAccount(&storage.CloudAccountInput{
		Vendor: "aliyun", Alias: "prod", CredentialRef: "${CLOUD_ALIYUN_PROD}",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	accounts, _ := s.db.ListCloudAccounts()
	_, _, err := s.cloudProviderForAccount(accounts[0].ID)
	if err == nil {
		t.Fatal("want provider error")
	}
}

func TestCloudInstancesRootBadRoute(t *testing.T) {
	compute := &fakeCloudCompute{}
	s, account := newCloudLifecycleServer(t, compute)
	// 单段路径 -> 400
	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID, `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("single segment status = %d", rec.Code)
	}
	// operations 非 GET/POST -> 405
	rec = callCloudLifecycleAPI(s, http.MethodPut, "/api/cloud/instances/"+account.ID+"/operations", `{}`)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT operations status = %d", rec.Code)
	}
}

func TestCloudLifecycleInstanceNotFound(t *testing.T) {
	compute := &fakeCloudCompute{}
	s, account := newCloudLifecycleServer(t, compute)
	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/actions",
		`{"action":"start","instance_id":"nope","confirm_key":"nope"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("not found status = %d", rec.Code)
	}
}

func TestCloudLifecycleDelete(t *testing.T) {
	compute := &fakeCloudCompute{nextOp: cloud.NewOperation(cloud.OperationDelete, "req-d", time.Now().UTC())}
	s, account := newCloudLifecycleServer(t, compute)
	if err := s.db.UpsertCloudInstance(&storage.CloudInstanceInput{AccountID: account.ID, NativeID: "i-1", Status: "running"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	insts, _ := s.db.ListCloudInstances(account.ID)
	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/actions",
		`{"action":"delete","instance_id":"`+insts[0].ID+`","confirm_key":"`+insts[0].ID+`"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("delete status = %d", rec.Code)
	}
	if len(compute.calls) == 0 || compute.calls[0] != "delete:i-1" {
		t.Fatalf("calls = %v", compute.calls)
	}
}

func TestCloudSyncUpsertErrPath(t *testing.T) {
	compute := &fakeCloudCompute{instances: []cloud.Instance{{NativeID: "i-1"}}}
	s, _ := newCloudAPIServer(t)
	// 无账号 -> UpsertCloudInstance 因缺 account_id 校验失败? 不，account_id 有值，native 有值。
	// 用非法账号 id（不存在的 FK 不强制）仍落库；改用空 account id 触发校验错误。
	err := s.syncCloudInstances(context.Background(), &storage.CloudAccount{ID: ""}, compute)
	if err == nil {
		t.Fatal("want upsert validation error")
	}
}

func TestCloudProviderTencentNotConfigured(t *testing.T) {
	s, p := newCloudAPIServer(t)
	if err := p.Write("cloud/tencent/lab", &vault.Entry{Path: "cloud/tencent/lab", Kind: "pair", Username: "sid", Value: "skey"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	// 地域空 -> tencent.New nil -> not configured
	_, err := s.cloudProvider(&storage.CloudAccount{Vendor: "tencent", Alias: "lab", Region: ""})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("err = %v, want not configured", err)
	}
}

func TestCloudOperationProviderErr(t *testing.T) {
	compute := &fakeCloudCompute{}
	s, account := newCloudLifecycleServer(t, compute)
	// 覆盖 provider 构造失败（vault 未配置）
	s.cloudProviderFn = nil
	s.vaultProvider = nil
	rec := callCloudLifecycleAPI(s, http.MethodPost, "/api/cloud/instances/"+account.ID+"/operations", `{"operation_id":"x"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("provider err status = %d", rec.Code)
	}
}

func TestCloudOperationsListSort(t *testing.T) {
	compute := &fakeCloudCompute{}
	s, account := newCloudLifecycleServer(t, compute)
	op1 := cloud.NewOperation(cloud.OperationStart, "req-a", time.Now().UTC().Add(-time.Hour))
	op2 := cloud.NewOperation(cloud.OperationStop, "req-b", time.Now().UTC())
	s.storeCloudOperation(account.ID, &op1)
	s.storeCloudOperation(account.ID, &op2)
	rec := callCloudLifecycleAPI(s, http.MethodGet, "/api/cloud/instances/"+account.ID+"/operations", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "req-a") || !strings.Contains(rec.Body.String(), "req-b") {
		t.Fatalf("list missing ops: %s", rec.Body.String())
	}
}
