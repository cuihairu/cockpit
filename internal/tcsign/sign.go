// Package tcsign 腾讯云 API 3.0 TC3-HMAC-SHA256 签名（纯函数，无网络），
// 供所有腾讯云产品客户端复用：internal/cloud/tencent（CVM 适配器）为当前
// 唯一使用方，后续腾讯云 DNS 等接入直接引用。
// 流程（官方 TC3 文档）：POST + JSON body，canonical request 只签三个头
// content-type/host/x-tc-action，body 以 sha256hex 摘要参与签名；三级密钥
// 派生 SecretDate → SecretService → SecretSigning 后 HMAC-SHA256 stringToSign。
package tcsign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// HashSHA256Hex SHA256 摘要的 hex 编码（payload 摘要与 canonicalRequest 摘要共用）。
func HashSHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// hmacSHA256 单步 HMAC-SHA256（密钥派生链与最终签名共用）。
func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

// scope 生成 CredentialScope：date/service/tc3_request（date 为 UTC 日期）。
func scope(secretKey, service string, timestamp int64) string {
	date := time.Unix(timestamp, 0).UTC().Format("2006-01-02")
	return date + "/" + service + "/tc3_request"
}

// CanonicalRequest 组装 canonical request。契约：POST、路径 /、空 query，
// 三个参与签名的头按固定顺序，末尾挂 body 摘要。
func CanonicalRequest(host, action string, timestamp int64, payload []byte) string {
	headers := "content-type:application/json; charset=utf-8\n" +
		"host:" + host + "\n" +
		"x-tc-action:" + strings.ToLower(action) + "\n"
	signedHeaders := "content-type;host;x-tc-action"
	return "POST\n/\n\n" + headers + "\n" + signedHeaders + "\n" + HashSHA256Hex(payload)
}

// StringToSign 组装 TC3 string to sign。
func StringToSign(secretKey, service, host, action string, timestamp int64, payload []byte) string {
	canonical := CanonicalRequest(host, action, timestamp, payload)
	return "TC3-HMAC-SHA256\n" + strconv.FormatInt(timestamp, 10) + "\n" +
		scope(secretKey, service, timestamp) + "\n" + HashSHA256Hex([]byte(canonical))
}

// Sign 计算签名：三级密钥派生后 HMAC-SHA256 stringToSign，hex 编码。
func Sign(secretKey, service string, timestamp int64, stringToSign string) string {
	date := time.Unix(timestamp, 0).UTC().Format("2006-01-02")
	secretDate := hmacSHA256([]byte(secretKey), date)
	secretService := hmacSHA256(secretDate, service)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	return hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))
}

// Authorization 组装 Authorization 头值。host 必须是请求实际发送的 Host
// （默认端点即 cvm.tencentcloudapi.com；测试端点即 127.0.0.1:port）。
func Authorization(secretID, secretKey, service, host, action string, timestamp int64, payload []byte) string {
	signature := Sign(secretKey, service, timestamp,
		StringToSign(secretKey, service, host, action, timestamp, payload))
	return "TC3-HMAC-SHA256 Credential=" + secretID + "/" + scope(secretKey, service, timestamp) +
		", SignedHeaders=content-type;host;x-tc-action, Signature=" + signature
}
