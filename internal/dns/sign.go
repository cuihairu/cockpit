package dns

// sign.go 阿里云 RPC V1 签名（HMAC-SHA1）纯函数，alidns.go 使用，M2 D13。
// 流程：参数按名称排序 percentEncode 拼成 canonical query →
// StringToSign = "GET&%2F&" + percentEncode(canonical) →
// HMAC-SHA1(key = secret + "&") 后 base64。

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// aliPercentEncode 阿里云规则：RFC3986 unreserved（字母数字与 -_.~）不转义，
// 其余转 %大写HEX；空格编 %20 不编 +
func aliPercentEncode(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// aliCanonicalQuery 参数按名称排序后逐个 percentEncode 拼接
func aliCanonicalQuery(params url.Values) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, aliPercentEncode(k)+"="+aliPercentEncode(params.Get(k)))
	}
	return strings.Join(parts, "&")
}

// aliSign 对 StringToSign 做 HMAC-SHA1（key = secret + "&"）后 base64
func aliSign(stringToSign, secret string) string {
	mac := hmac.New(sha1.New, []byte(secret+"&"))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// aliSignQuery 一步式：生成签名后的完整 query（canonical + Signature）
func aliSignQuery(secret string, params url.Values) string {
	canonical := aliCanonicalQuery(params)
	stringToSign := "GET&%2F&" + aliPercentEncode(canonical)
	return canonical + "&Signature=" + aliPercentEncode(aliSign(stringToSign, secret))
}

// aliRandRead 注入点：go1.26 crypto/rand.Read 不会失败，错误分支仅供测试覆盖。
var aliRandRead = rand.Read

// aliNonce 签名唯一数：crypto/rand 16 字节 hex；失败退化纳秒时间戳
func aliNonce() string {
	var b [16]byte
	if _, err := aliRandRead(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(b[:])
}
