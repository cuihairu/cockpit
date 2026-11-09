// 侧信道：容器/本机命令（自 main.go 按职责拆分）；失败返回 ""（错误另行
// 记证据，不混进断言文本——错误串里若含标记词会造成假阳性）。
package main

import (
	"os"
	"strings"
)

// dockerExec 容器侧信道读文件/执行命令；失败返回 ""（错误另行记证据，
// 不混进断言文本——错误串里若含标记词会造成假阳性）
func dockerExec(args ...string) string {
	out, err := dockerExecCmd(args...)
	if err != nil {
		ev("[sidechannel] docker exec 失败: %v: %s", err, strings.TrimSpace(string(out)))
		return ""
	}
	return strings.TrimSpace(string(out))
}

// hostReadFile 本机侧信道（S4 本机 sshd 场景）；失败同上返回 ""
func hostReadFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		ev("[sidechannel] host read 失败: %v", err)
		return ""
	}
	return strings.TrimSpace(string(b))
}

func fileReadCmd(p string) string { return "cat " + p }
