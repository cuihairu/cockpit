//go:build windows

// Windows SCM 验收测试服务（cockpit-acc-scm / cockpit-acc-scm-hang 专用映像）。
//
// 一个映像两种行为，由 CreateService args 区分：
//
//	scm-acc-svc.exe cockpit-acc-scm       normal  <logpath>  — 正常服务：stop 立即 Stopped
//	scm-acc-svc.exe cockpit-acc-scm-hang  hang    <logpath>  — 挂死服务：收到 stop 报 StopPending
//	                                                            后永不退出 → 确定性构造
//	                                                            scmStopAndWait 的 30s
//	                                                            超时路径（checklist:253）
//
// 每次启动向 logpath 追加一行 START pid=...（探针据此证明 restart 换了进程）。
// 仅用于验收，不承载业务。
package main

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows/svc"
)

func main() {
	name := os.Args[1]
	mode := os.Args[2]
	logPath := os.Args[3]
	_ = svc.Run(name, &accHandler{name: name, hang: mode == "hang", logPath: logPath})
}

type accHandler struct {
	name    string
	hang    bool
	logPath string
}

func (h *accHandler) appendLog(format string, a ...interface{}) {
	f, err := os.OpenFile(h.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintln(f, time.Now().Format("2006-01-02T15:04:05.000")+" "+h.name+" "+msg)
}

func (h *accHandler) Execute(args []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown
	s <- svc.Status{State: svc.StartPending}
	pid := os.Getpid()
	h.appendLog("START pid=%d hang=%v", pid, h.hang)
	s <- svc.Status{State: svc.Running, Accepts: accepted}
	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			s <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			if h.hang {
				// 挂死路径：报 StopPending 后阻塞，SCM 停止流程永不收敛
				h.appendLog("STOP-REQ pid=%d (hang, ignore)", pid)
				s <- svc.Status{State: svc.StopPending, WaitHint: 120000}
				select {}
			}
			h.appendLog("STOP pid=%d", pid)
			s <- svc.Status{State: svc.StopPending, WaitHint: 3000}
			s <- svc.Status{State: svc.Stopped}
			return false, 0
		}
	}
	return false, 0
}
