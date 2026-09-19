package server

// cov_round4_test.go 第四轮覆盖率：四个定时巡检循环的 tick 循环体
// （ddns/nas/smart/acme，与 driftScanLoop 同构，drift 已有先例）。
// 注入短节奏包级变量 + cancel 退出，恢复纪律同 cov_loops_test.go
// （清理前恢复默认值，防泄漏 goroutine 读注入变量构成数据竞争）。
// 注意：cancel 会终结 server 的 ctx，每个循环必须用独立的
// covLoopServer（否则首个 cancel 后其余循环在 select 立即退出）。

import (
	"testing"
	"time"
)

// TestCovScanLoopsTicks 四个巡检循环：短节奏下各跑数轮 tick（空库 scanOnce
// 直接返回），cancel 干净退出。默认间隔（1800s/3600s）远大于 tick 节奏，
// 首扫后 `lastScan 未到间隔 → continue` 分支也被覆盖。
func TestCovScanLoopsTicks(t *testing.T) {
	run := func(t *testing.T, name string, fn func(), cancel func(), tick, wait *time.Duration) {
		t.Helper()
		ov, ow := *tick, *wait
		*tick, *wait = 40*time.Millisecond, time.Millisecond
		t.Cleanup(func() { *tick, *wait = ov, ow })
		exited := covSpawnLoop(fn)
		time.Sleep(150 * time.Millisecond)
		cancel()
		covWaitExit(t, name, exited)
	}

	for _, tc := range []struct {
		name string
		fn   func(s *Server) func()
		tick *time.Duration
		wait *time.Duration
	}{
		{"ddnsScanLoop", func(s *Server) func() { return s.ddnsScanLoop }, &ddnsScanTick, &ddnsScanStartWait},
		{"nasScanLoop", func(s *Server) func() { return s.nasScanLoop }, &nasScanTick, &nasScanStartWait},
		{"smartScanLoop", func(s *Server) func() { return s.smartScanLoop }, &smartScanTick, &smartScanStartWait},
		{"acmeScanLoop", func(s *Server) func() { return s.acmeScanLoop }, &acmeScanTick, &acmeScanStartWait},
	} {
		s := covLoopServer(t) // 每循环独立 server：cancel 不影响后续循环
		run(t, tc.name, tc.fn(s), s.cancel, tc.tick, tc.wait)
	}
}

// TestCovScanLoopsDisabledInterval 间隔为 0（关闭）时循环醒来直接 continue：
// 四个循环各自在独立 server 上预置间隔 0，跑数轮 tick 后退出
// （覆盖 interval<=0 continue 分支）
func TestCovScanLoopsDisabledInterval(t *testing.T) {
	saved := []struct {
		tick, wait *time.Duration
		ov, ow     time.Duration
	}{
		{&ddnsScanTick, &ddnsScanStartWait, ddnsScanTick, ddnsScanStartWait},
		{&nasScanTick, &nasScanStartWait, nasScanTick, nasScanStartWait},
		{&smartScanTick, &smartScanStartWait, smartScanTick, smartScanStartWait},
		{&acmeScanTick, &acmeScanStartWait, acmeScanTick, acmeScanStartWait},
	}
	for _, sv := range saved {
		*sv.tick, *sv.wait = 30*time.Millisecond, time.Millisecond
	}
	t.Cleanup(func() {
		for _, sv := range saved {
			*sv.tick, *sv.wait = sv.ov, sv.ow
		}
	})

	spawn := func(t *testing.T, name string, fn func(s *Server) func(), setInterval func(*Server) error) {
		t.Helper()
		s := covLoopServer(t)
		if err := setInterval(s); err != nil {
			t.Fatalf("set interval: %v", err)
		}
		exited := covSpawnLoop(fn(s))
		time.Sleep(120 * time.Millisecond)
		s.cancel()
		covWaitExit(t, name, exited)
	}

	spawn(t, "ddnsScanLoop disabled", func(s *Server) func() { return s.ddnsScanLoop }, func(s *Server) error { return s.SetDDNSScanInterval(0) })
	spawn(t, "nasScanLoop disabled", func(s *Server) func() { return s.nasScanLoop }, func(s *Server) error { return s.SetNASScanInterval(0) })
	spawn(t, "smartScanLoop disabled", func(s *Server) func() { return s.smartScanLoop }, func(s *Server) error { return s.SetSmartScanInterval(0) })
	spawn(t, "acmeScanLoop disabled", func(s *Server) func() { return s.acmeScanLoop }, func(s *Server) error { return s.SetAcmeScanInterval(0) })
}
