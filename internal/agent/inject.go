package agent

import "sync"

// injectMu 守卫本包的测试注入点 var（写方=同包测试的注入与 t.Cleanup/defer
// 恢复，读方=register/心跳/服务面刷新等后台 goroutine）。
//
// 为什么必须加锁：后台循环的生命周期不受 Stop 约束——Stop 只 cancel +
// 关连接，循环异步退出；测试还会直接 `go a.reconnect()` 自起。因此测试
// goroutine 的恢复写与在途 register 的读可并发（CI race 实测：cov_gaps2_test
// 的 Cleanup 写 publicIPEndpoints vs localip.go publicIP 读，main race 腿
// 2026-11-08 红过一次；复现依赖时序，本地 -count=10 未必能炸）。逐测试
// "起 goroutine 前写、等退出后再恢复"的纪律救不了跨测试泄漏的在途
// goroutine，故在结构上收口：
//
//   - 写方 injectSwap：原子替换并返回旧值，注入与恢复共用；
//   - 读方 injectRead：返回值拷贝，**调用与 I/O 一律在锁外**（publicIP
//     有 3s HTTP 超时、readSysFile/os.Stat 走文件系统，严禁持锁做）。
//
// 写法（测试）：
//
//	old := injectSwap(&target, v)
//	t.Cleanup(func() { injectSwap(&target, old) })
//
// 不经锁的例外（经评估无并发写，改动前先读 inject.go 顶部）：
//   - 包初始化赋值（先于一切 goroutine）；
//   - TestMain 在 m.Run() 前写 reconnectDelay/reconnectRetryDelay，此后
//     进程内只读（见 cov_agent_test.go TestMain 注释）。
//
// 除上述例外，裸读写本包注入点 var 即为违例。
var injectMu sync.RWMutex

// injectSwap 原子替换注入点当前值并返回旧值（写方唯一入口）。
func injectSwap[T any](dst *T, v T) (old T) {
	injectMu.Lock()
	defer injectMu.Unlock()
	old, *dst = *dst, v
	return old
}

// injectRead 读注入点当前值的快照（读方唯一入口）。返回的是拷贝：func 型
// 注入点在锁外调用（injectRead(&f)()），值型直接使用。
func injectRead[T any](dst *T) T {
	injectMu.RLock()
	defer injectMu.RUnlock()
	return *dst
}
