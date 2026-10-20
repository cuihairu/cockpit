package alisign

// sign_test.go 签名纯函数的表驱动与对拍测试（原 internal/dns/sign.go 的
// 单元测试，随签名器抽成中立包 internal/alisign 迁移；断言一字未删）。

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"net/url"
	"strconv"
	"testing"
)

// TestPercentEncode 阿里云规则：unreserved 不转义、空格 %20、
// 特殊字符 !'()* 全编码且大写 HEX
func TestPercentEncode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"abc-_.~123", "abc-_.~123"},
		{"a b", "a%20b"},
		{"!", "%21"},
		{"'", "%27"},
		{"(", "%28"},
		{")", "%29"},
		{"*", "%2A"},
		{"2015-01-09T15:04:05Z", "2015-01-09T15%3A04%3A05Z"},
		{"a+b", "a%2Bb"},
	}
	for _, c := range cases {
		if got := PercentEncode(c.in); got != c.want {
			t.Errorf("PercentEncode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestCanonicalQuery 参数按名称排序后逐个编码
func TestCanonicalQuery(t *testing.T) {
	q := CanonicalQuery(url.Values{
		"Action":   {"DescribeDomains"},
		"PageSize": {"50"},
		"Format":   {"json"},
	})
	want := "Action=DescribeDomains&Format=json&PageSize=50"
	if q != want {
		t.Errorf("canonical = %q, want %q", q, want)
	}
	if got := CanonicalQuery(nil); got != "" {
		t.Errorf("nil params = %q", got)
	}
}

// TestSign 与独立实现的 HMAC-SHA1(secret+"&") 对拍
func TestSign(t *testing.T) {
	got := Sign("GET&%2F&abc", "testsecret")
	mac := hmac.New(sha1.New, []byte("testsecret&"))
	mac.Write([]byte("GET&%2F&abc"))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if got != want {
		t.Errorf("Sign = %q, want %q", got, want)
	}
}

// TestSignQuery 一步式与分步拼装对拍：canonical + "&Signature=" + 编码后签名
func TestSignQuery(t *testing.T) {
	params := url.Values{"Action": {"DescribeInstances"}, "Format": {"json"}}
	got := SignQuery("testsecret", params)
	canonical := CanonicalQuery(params)
	want := canonical + "&Signature=" + PercentEncode(Sign("GET&%2F&"+PercentEncode(canonical), "testsecret"))
	if got != want {
		t.Errorf("SignQuery = %q, want %q", got, want)
	}
}

// TestNonce 唯一数非空且两次生成不同
func TestNonce(t *testing.T) {
	a, b := Nonce(), Nonce()
	if a == "" || a == b {
		t.Errorf("nonce should be non-empty and unique, got %q %q", a, b)
	}
}

// TestNonceRandFail 注入随机数失败，覆盖签名唯一数的
// 纳秒时间戳后备分支。
func TestNonceRandFail(t *testing.T) {
	orig := RandRead
	t.Cleanup(func() { RandRead = orig })
	RandRead = func([]byte) (int, error) { return 0, errors.New("boom rand") }

	nonce := Nonce()
	if _, err := strconv.Atoi(nonce); err != nil {
		t.Errorf("fallback nonce = %q, want numeric timestamp", nonce)
	}
}
