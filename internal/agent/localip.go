package agent

import (
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// publicIP 返回本机公网 IPv4 地址（通过外部服务探测）。
// 失败时返回空字符串，注册仍继续。
func publicIP() string {
	client := &http.Client{Timeout: 3 * time.Second}

	for _, endpoint := range []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
		"https://icanhazip.com",
	} {
		resp, err := client.Get(endpoint)
		if err != nil {
			log.Printf("publicIP: %s: %v", endpoint, err)
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}
		ip := strings.TrimSpace(string(body))
		if ip != "" {
			return ip
		}
	}

	log.Printf("publicIP: all endpoints failed")
	return ""
}