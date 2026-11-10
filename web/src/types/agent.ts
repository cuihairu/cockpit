// Agent 与能力探测域类型
// 位置信息：后端一律顶层序列化 region/zone（storage.Agent/ComputeInstance），
// 空串/缺省=未上报，展示侧自行兜底

// Agent 上报的本机开放服务面（SSH/RDP/VNC/telnet…）
export interface AgentService {
  protocol: string
  host?: string
  port: number
  name?: string
  running: boolean
  authMethods?: string[]
  detectedAt?: number
}

// 服务器标签（服务端持久化）
export interface AgentTagRef {
  id: string
  name: string
  color: string
}

// Agent 状态
export interface Agent {
  id: string
  region?: string
  zone?: string
  capabilities: Capability[]
  hostname: string
  ip: string
  status: 'online' | 'offline'
  lastSeen: number // Unix 秒（后端 LastSeen.Unix()）
  // 虚拟化信息
  virtType?: string  // kvm, vmware, qemu, docker, none
  virtRole?: string  // guest (虚拟机), host (物理机)
  // 标签（支持复杂类型）
  labels?: Record<string, unknown>
  // 元信息列（2026-10-02 新增）
  version?: string
  startedAt?: number // Unix 秒，agent 进程启动时刻
  osName?: string
  osVersion?: string
  arch?: string
  services?: AgentService[]
  tags?: AgentTagRef[]
}

// 一键清理离线 Agent 的结果（POST /agents/cleanup）
export interface AgentCleanupResult {
  status: string
  removed: string[]
  count: number
}

// 能力定义
export interface Capability {
  type: string
  endpoint?: string
  version?: string
  metadata?: Record<string, unknown>
}

// Docker 容器信息（字段对齐后端 internal/docker/client.go ContainerInfo）
// 注意：Go json 序列化保持大写缩写词命名，Created 为 Unix 秒级时间戳
export interface ContainerInfo {
  ID: string
  Name: string
  Image: string
  ImageID: string
  State: string // running / paused / exited / created 等
  Status: string // 可读状态串，如 "Up 2 hours"
  Labels: Record<string, string>
  Created: number // Unix 秒级时间戳
}

// Docker 镜像信息（字段对齐后端 internal/docker/client.go ImageInfo）
export interface ImageInfo {
  ID: string
  RepoTags: string[]
  Size: number // 字节
  Created: number // Unix 秒级时间戳
}
