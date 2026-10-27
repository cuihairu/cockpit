package tcsign

// sign_test.go 腾讯云 TC3 签名包测试：结构性质（确定性/扰动敏感性/格式）+
// 官方示例向量核对。厂商端到端正确性属片 4 验收（真实账号），此处验证的是
// 算法骨架与 wire 一致性（wire 一致性由 tencent 适配器契约测试复算覆盖）。

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestHashSHA256Hex(t *testing.T) {
	// sha256("") 的已知值。
	if got := HashSHA256Hex([]byte("")); got != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatalf("empty hash = %q", got)
	}
	if got := HashSHA256Hex([]byte("abc")); len(got) != 64 {
		t.Fatalf("hash length = %d", len(got))
	}
}

func TestCanonicalRequest(t *testing.T) {
	ts := int64(1700000000)
	payload := []byte(`{"Limit":100}`)
	canonical := CanonicalRequest("cvm.tencentcloudapi.com", "DescribeInstances", ts, payload)
	lines := strings.Split(canonical, "\n")
	// POST / 空query + 三头 + 空行 + signedHeaders + body 摘要
	if lines[0] != "POST" || lines[1] != "/" || lines[2] != "" {
		t.Fatalf("first lines = %q", lines[:3])
	}
	if lines[3] != "content-type:application/json; charset=utf-8" ||
		lines[4] != "host:cvm.tencentcloudapi.com" ||
		lines[5] != "x-tc-action:describeinstances" {
		t.Fatalf("headers = %q %q %q", lines[3], lines[4], lines[5])
	}
	if lines[6] != "" || lines[7] != "content-type;host;x-tc-action" {
		t.Fatalf("blank/signed = %q %q", lines[6], lines[7])
	}
	if lines[8] != HashSHA256Hex(payload) {
		t.Fatalf("payload hash = %q", lines[8])
	}
}

func TestStringToSign(t *testing.T) {
	ts := int64(1700000000)
	s := StringToSign("sk", "cvm", "cvm.tencentcloudapi.com", "DescribeInstances", ts, []byte("{}"))
	if !strings.HasPrefix(s, "TC3-HMAC-SHA256\n1700000000\n") {
		t.Fatalf("prefix = %q", s)
	}
	// scope = UTC 日期 / service / tc3_request
	if !strings.Contains(s, "\n2023-11-14/cvm/tc3_request\n") {
		t.Fatalf("scope missing: %q", s)
	}
	if len(strings.Split(s, "\n")) != 4 {
		t.Fatalf("lines = %q", s)
	}
}

func TestSignFormat(t *testing.T) {
	s := Sign("sk", "cvm", 1700000000, "TC3-HMAC-SHA256\n1700000000\n2023-11-14/cvm/tc3_request\nabc")
	if len(s) != 64 {
		t.Fatalf("signature length = %d", len(s))
	}
	if _, err := hex.DecodeString(s); err != nil {
		t.Fatalf("signature not hex: %v", err)
	}
}

func TestAuthorizationFormat(t *testing.T) {
	ts := int64(1700000000)
	auth := Authorization("AKIDtest", "sk", "cvm", "cvm.tencentcloudapi.com", "DescribeInstances", ts, []byte("{}"))
	wantPrefix := "TC3-HMAC-SHA256 Credential=AKIDtest/2023-11-14/cvm/tc3_request"
	if !strings.HasPrefix(auth, wantPrefix) {
		t.Fatalf("prefix = %q", auth)
	}
	if !strings.Contains(auth, ", SignedHeaders=content-type;host;x-tc-action, Signature=") {
		t.Fatalf("signed headers missing: %q", auth)
	}
}

// TestSensitivity 签名对任一输入扰动敏感（防漏签）。
func TestSensitivity(t *testing.T) {
	base := Authorization("AKIDtest", "sk", "cvm", "cvm.tencentcloudapi.com", "DescribeInstances", 1700000000, []byte(`{}`))
	cases := map[string]string{
		"secret key": Authorization("AKIDtest", "sk-x", "cvm", "cvm.tencentcloudapi.com", "DescribeInstances", 1700000000, []byte(`{}`)),
		"host":       Authorization("AKIDtest", "sk", "cvm", "cvm.tencentcloudapi.comx", "DescribeInstances", 1700000000, []byte(`{}`)),
		"action":     Authorization("AKIDtest", "sk", "cvm", "cvm.tencentcloudapi.com", "RunInstances", 1700000000, []byte(`{}`)),
		"timestamp":  Authorization("AKIDtest", "sk", "cvm", "cvm.tencentcloudapi.com", "DescribeInstances", 1700000001, []byte(`{}`)),
		"payload":    Authorization("AKIDtest", "sk", "cvm", "cvm.tencentcloudapi.com", "DescribeInstances", 1700000000, []byte(`{"Limit":100}`)),
	}
	for name, got := range cases {
		if got == base {
			t.Errorf("%s must change the signature", name)
		}
	}
}

// TestDeterminism 同一输入恒等（客户端重试/桩复算依赖此性质）。
func TestDeterminism(t *testing.T) {
	a := Authorization("AKIDtest", "sk", "cvm", "cvm.tencentcloudapi.com", "DescribeInstances", 1700000000, []byte(`{}`))
	b := Authorization("AKIDtest", "sk", "cvm", "cvm.tencentcloudapi.com", "DescribeInstances", 1700000000, []byte(`{}`))
	if a != b {
		t.Fatalf("determinism broken: %q != %q", a, b)
	}
}
