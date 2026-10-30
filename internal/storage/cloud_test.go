package storage

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateCloudCredentialRef(t *testing.T) {
	cases := []struct {
		ref   string
		valid bool
	}{
		{"${ALIYUN_AK}", true},
		{"${_PRIVATE_KEY}", true},
		{"${A1_B2}", true},
		{"", false},                   // 空：无凭据不得静默通过
		{"ak-local-plaintext", false}, // 明文引用（红线：不落明文）
		{"${VAR", false},              // 未闭合
		{"$VAR", false},               // 缺花括号
		{"${1VAR}", false},            // 数字开头
		{"${VAR}extra", false},        // 引用外拼接（防夹带明文）
		{"prefix${VAR}", false},       // 同上
		{"${VAR}${VAR2}", false},      // 多段拼接
		{"${}", false},                // 空变量名
	}
	for _, c := range cases {
		err := ValidateCloudCredentialRef(c.ref)
		if c.valid && err != nil {
			t.Errorf("ValidateCloudCredentialRef(%q) = %v, want nil", c.ref, err)
		}
		if !c.valid && err == nil {
			t.Errorf("ValidateCloudCredentialRef(%q) = nil, want error", c.ref)
		}
	}
}

func TestCloudAccountCRUD(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	in := &CloudAccountInput{
		Vendor:        "aliyun",
		Alias:         "prod",
		CredentialRef: "${ALIYUN_MAIN_AK}",
		Scopes:        []string{"ecs:read", "ecs:write"},
	}
	if err := db.UpsertCloudAccount(in); err != nil {
		t.Fatalf("UpsertCloudAccount() error = %v", err)
	}

	accounts, err := db.ListCloudAccounts()
	if err != nil {
		t.Fatalf("ListCloudAccounts() error = %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("ListCloudAccounts() len = %d, want 1", len(accounts))
	}
	acc := accounts[0]
	if acc.Vendor != "aliyun" || acc.Alias != "prod" || acc.CredentialRef != "${ALIYUN_MAIN_AK}" {
		t.Errorf("account fields = %+v", acc)
	}
	if !acc.Enabled {
		t.Error("new account should default to enabled")
	}
	if len(acc.Scopes) != 2 {
		t.Errorf("Scopes = %v, want 2 entries", acc.Scopes)
	}
	if acc.CredentialRef != "${ALIYUN_MAIN_AK}" && strings.Contains(acc.CredentialRef, "secret") {
		t.Error("credential ref must stay a reference, never plaintext")
	}

	// 同 vendor+alias 视为更新：改 ref 不新增行
	in2 := &CloudAccountInput{
		Vendor:        "aliyun",
		Alias:         "prod",
		CredentialRef: "${ALIYUN_BACKUP_AK}",
		Enabled:       true,
	}
	if err := db.UpsertCloudAccount(in2); err != nil {
		t.Fatalf("UpsertCloudAccount(update) error = %v", err)
	}
	accounts, _ = db.ListCloudAccounts()
	if len(accounts) != 1 {
		t.Fatalf("after update len = %d, want 1", len(accounts))
	}
	if accounts[0].CredentialRef != "${ALIYUN_BACKUP_AK}" {
		t.Errorf("ref not updated: %q", accounts[0].CredentialRef)
	}

	// 凭据引用非法必须拒绝落库
	if err := db.UpsertCloudAccount(&CloudAccountInput{Vendor: "tencent", Alias: "bad", CredentialRef: "AKIDplaintext"}); err == nil {
		t.Error("plaintext credential ref should be rejected")
	}

	if err := db.DeleteCloudAccount(acc.ID); err != nil {
		t.Fatalf("DeleteCloudAccount() error = %v", err)
	}
	if err := db.DeleteCloudAccount(acc.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("double delete = %v, want ErrNotFound", err)
	}
}

func TestCloudInstanceCRUD(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	acc := &CloudAccountInput{Vendor: "aliyun", Alias: "prod", CredentialRef: "${AK}"}
	if err := db.UpsertCloudAccount(acc); err != nil {
		t.Fatalf("UpsertCloudAccount() error = %v", err)
	}
	accounts, _ := db.ListCloudAccounts()

	in := &CloudInstanceInput{
		AccountID:        accounts[0].ID,
		NativeID:         "i-bp1abc",
		Name:             "web-1",
		Region:           "cn-hangzhou",
		Zone:             "cn-hangzhou-a",
		Spec:             "ecs.c7.large",
		Status:           "running",
		BillingMode:      "prepaid",
		BillingContinues: true, // 包年包月停机仍计费：适配器按事实填充
		RawVendorStatus:  "Running",
	}
	if err := db.UpsertCloudInstance(in); err != nil {
		t.Fatalf("UpsertCloudInstance() error = %v", err)
	}

	list, err := db.ListCloudInstances(accounts[0].ID)
	if err != nil {
		t.Fatalf("ListCloudInstances() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListCloudInstances() len = %d, want 1", len(list))
	}
	got := list[0]
	if got.NativeID != "i-bp1abc" || !got.BillingContinues || got.BillingMode != "prepaid" {
		t.Errorf("instance fields = %+v", got)
	}

	// 同账号+原生 ID 视为更新：状态翻转 + BillingContinues 显式归 false（释放后不再计费）
	in.Status = "stopped"
	in.BillingContinues = false
	if err := db.UpsertCloudInstance(in); err != nil {
		t.Fatalf("UpsertCloudInstance(update) error = %v", err)
	}
	list, _ = db.ListCloudInstances(accounts[0].ID)
	if len(list) != 1 {
		t.Fatalf("after update len = %d, want 1", len(list))
	}
	if list[0].Status != "stopped" || list[0].BillingContinues {
		t.Errorf("update lost: %+v", list[0])
	}

	// 按账号过滤：别的账号看不到
	if err := db.UpsertCloudAccount(&CloudAccountInput{Vendor: "tencent", Alias: "lab", CredentialRef: "${TC_AK}"}); err != nil {
		t.Fatalf("UpsertCloudAccount(tencent) error = %v", err)
	}
	all, _ := db.ListCloudAccounts()
	var tencentID string
	for _, a := range all {
		if a.Vendor == "tencent" {
			tencentID = a.ID
		}
	}
	other, _ := db.ListCloudInstances(tencentID)
	if len(other) != 0 {
		t.Errorf("cross-account leak: %d rows", len(other))
	}

	if err := db.DeleteCloudInstance(got.ID); err != nil {
		t.Fatalf("DeleteCloudInstance() error = %v", err)
	}
	if err := db.DeleteCloudInstance(got.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("double delete = %v, want ErrNotFound", err)
	}
}

// TestDeleteCloudAccountCascadesInstances 验证删除账号会级联删除其实例，
// 且在同一事务内：不留孤儿实例行。
func TestDeleteCloudAccountCascadesInstances(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	if err := db.UpsertCloudAccount(&CloudAccountInput{Vendor: "aliyun", Alias: "prod", CredentialRef: "${AK}"}); err != nil {
		t.Fatalf("UpsertCloudAccount() error = %v", err)
	}
	accounts, _ := db.ListCloudAccounts()
	accID := accounts[0].ID
	if err := db.UpsertCloudInstance(&CloudInstanceInput{
		AccountID: accID, NativeID: "i-1", Name: "web-1", Region: "cn-hangzhou",
		Status: "running", BillingMode: "prepaid",
	}); err != nil {
		t.Fatalf("UpsertCloudInstance() error = %v", err)
	}

	if err := db.DeleteCloudAccount(accID); err != nil {
		t.Fatalf("DeleteCloudAccount() error = %v", err)
	}
	if insts, _ := db.ListCloudInstances(accID); len(insts) != 0 {
		t.Fatalf("orphan instances remain: %+v", insts)
	}
	if accts, _ := db.ListCloudAccounts(); len(accts) != 0 {
		t.Fatalf("account not deleted: %+v", accts)
	}
}

func TestResolveCloudCredentialRef(t *testing.T) {
	t.Setenv("CLOUD_TEST_AK", "resolved-value")

	v, err := ResolveCloudCredentialRef("${CLOUD_TEST_AK}")
	if err != nil || v != "resolved-value" {
		t.Errorf("ResolveCloudCredentialRef() = %q, %v", v, err)
	}

	// 未设置：fail-closed 报错，不返回空串静默放行
	if _, err := ResolveCloudCredentialRef("${CLOUD_TEST_MISSING}"); err == nil {
		t.Error("missing env var should fail closed")
	}

	// 非引用形态：直接拒绝，不走 env
	if _, err := ResolveCloudCredentialRef("AKIDplaintext"); err == nil {
		t.Error("non-reference should be rejected")
	}
}
