package healthprobe

// Semantics 探针语义（对标 k8s probe 术语，croupier agent-core §6.1 同款
// 三语义）：
//   - Liveness（存活）：自己活没活——阈值状态机判定，故障后果（自愈重启）
//     归调用方；
//   - Readiness（依赖就绪）：依赖就绪没就绪——判定与 Liveness 共用同一份
//     状态机，故障后果（只告警不动手）归调用方；
//   - Heartbeat（周期心跳）：只打点不定性——无状态迁移无窗口，回答「最近
//     活体时刻」（Snapshot.LastOK），故障判定由调用方按 LastOK 新鲜度自做。
//
// Liveness 与 Readiness 在 core 内判定行为完全一致（后果都在调用方），区别
// 只随窗口记录留痕——回查「什么时间段服务不可用」时能答出是哪路语义判的。
type Semantics string

const (
	SemLiveness  Semantics = "liveness"
	SemReadiness Semantics = "readiness"
	SemHeartbeat Semantics = "heartbeat"
)

// judging 心跳语义不判定（无迁移无窗口），其余两语义共用阈值状态机。
func (s Semantics) judging() bool { return s != SemHeartbeat }
