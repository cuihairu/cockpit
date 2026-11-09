// ========== 应用部署（Compose Stack） ==========
// 单条已发布端口映射（hostPort=0 的未发布端口不会出现在列表里）
export interface StackPort {
  hostPort: number
  containerPort: number
  proto: string // tcp / udp / sctp
}

// Stack 内单个服务的信息
export interface StackService {
  name: string
  image: string
  state: string // running / exited 等（docker 状态）
  status: string // 可读状态串，如 "Up 2 hours"
  containerId: string
  ports?: StackPort[] // 已发布端口（inventory 联动会收编成 CMDB 台账行）
}

// Stack 列表视图（聚合所有 agent；离线 agent 条目 online=false 且无 services）
export interface StackView {
  agentId: string
  agentName: string
  name: string
  running: number
  total: number
  lastAction: string // 最近部署动作，如 up / down
  lastStatus: string // 最近部署结果，如 success / failed
  lastDeployedAt: number // Unix 秒级时间戳，0 = 无部署记录
  online: boolean
  services?: StackService[]
}

// Stack 详情（单个 agent 上的单个 Stack）
export interface StackDetail {
  name: string
  running: number
  total: number
  services: StackService[]
}

// Stack 的 compose.yml 与 .env 内容
export interface StackCompose {
  name: string
  compose: string
  env: string
  composeFile: string // 服务端实际文件路径
  modifiedAt: number // Unix 秒级时间戳
}

// 保存 compose 文件的响应
export interface StackComposeSaveResponse {
  status: string // "saved"
  created: boolean // 是否为新创建的 Stack
}

// Stack 异步任务启动响应（up / down / 删除）
export interface StackTaskStartResponse {
  taskId: string
  status: string // "started"
}

// Stack 异步任务状态
export interface StackTask {
  id: string
  stack: string
  action: string // up / down / delete
  status: 'running' | 'success' | 'failed'
  log: string
  startedAt: number // Unix 秒级时间戳
  finishedAt: number // Unix 秒级时间戳，0 = 未结束
}

// Agent 侧 stacks 目录自检信息（M1.5）
export interface StackInfo {
  dir: string
  dirWritable: boolean
  dirError?: string
  composeVersion?: string
}

// Stack 部署历史（server 侧记录；agent 离线时仍可查）
export interface StackDeployment {
  id: number
  agentId: string
  stackName: string
  action: string // up / down / restart / pull / remove
  status: 'running' | 'success' | 'failed'
  taskId: string
  startedAt: number // Unix 秒级时间戳
  finishedAt: number // Unix 秒级时间戳，0 = 未结束
}

// Stacks 目录的 git 接入状态（P2 ① Git-to-deploy）
// enabled=false 时 reason: git-unavailable / not-a-repo（未接入是常态非错误）
export interface StackGitChange {
  code: string // porcelain XY 码：M / ?? / A 等
  path: string
}

export interface StackGitStatus {
  enabled: boolean
  reason?: string
  remote?: string
  branch?: string
  head?: string // 本地 HEAD 短 SHA；空 = 无提交
  remoteHead?: string // origin/<branch> 短 SHA（上次 fetch 时点）
  ahead: number
  behind: number
  changes?: StackGitChange[] // drift：本地改动 vs 仓库（.cockpit/ 已过滤）
}

// git pull（首次即接入）响应：同 StackGitStatus 的接入后快照
export interface StackGitPullResponse {
  enabled: boolean
  remote?: string
  branch?: string
  head?: string
  remoteHead?: string
  ahead: number
  behind: number
  changes?: StackGitChange[]
}
