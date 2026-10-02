//go:build !linux && !darwin && !windows

package agent

// machineID 不支持的平台返回空字符串，调用方 fallback 到 hostname + random。
func machineID() string {
	return ""
}