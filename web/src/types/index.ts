// 资源类型定义
export type ResourceType =
  | 'compute-instance'
  | 'container-service'
  | 'domain'
  | 'certificate'
  | 'service'
  | 'gateway'
  | 'ci-service'
  | 'storage'

// 位置信息
export interface Location {
  region: string
  zone: string
}

// Agent 状态
export interface Agent {
  id: string
  location: Location
  capabilities: Capability[]
  hostname: string
  ip: string
  status: 'online' | 'offline'
  lastSeen: string
  // 虚拟化信息
  virtType?: string  // kvm, vmware, qemu, docker, none
  virtRole?: string  // guest (虚拟机), host (物理机)
  // 标签（支持复杂类型）
  labels?: Record<string, unknown>
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

// 计算实例
export interface ComputeInstance {
  id: string
  name: string
  displayName: string
  location: Location
  type: 'bare-metal' | 'vm' | 'container' | 'vps'
  region?: string
  zone?: string
  cpuCores?: number
  memoryMb?: number
  diskGb?: number
  agentId?: string
  ipv4?: string
  platform?: string
  platformUrl?: string
  hardware?: {
    cpu?: {
      model?: string
      cores?: number
      threads?: number
    }
    memory?: {
      capacity: string
    }
    disk?: Array<{
      device: string
      capacity: string
      type: string
    }>
  }
  access?: {
    web?: {
      url: string
    }
    ssh?: {
      host: string
      port: number
      user: string
    }
  }
  monitoring?: {
    enabled: boolean
    nezhaAgentId?: number
  }
  status?: 'running' | 'stopped' | 'unknown'
  tags?: Record<string, string>
}

// 域名
export interface Domain {
  id: string
  name: string
  displayName: string
  registrar: string
  registeredDate: string
  expiryDate: string
  autoRenew: boolean
  registrarConsoleUrl: string
  dnsProvider: string
  dnsConsoleUrl: string
  certificates: string[]
  subdomains: string[]
  status?: 'valid' | 'expiring' | 'expired'
  tags?: Record<string, string>
}

// 证书
export interface Certificate {
  id: string
  name: string
  displayName: string
  type: 'letsencrypt' | 'commercial' | 'self-signed' | 'cloudflare-origin'
  commonName: string
  sans: string[]
  issuedDate: string
  expiryDate: string
  autoRenew: boolean
  acmeProvider?: string
  deployedOn: Array<{
    computeRef: string
    path: string
  }>
  monitoring?: {
    enabled: boolean
    checkUrl?: string
  }
  daysRemaining?: number
  status?: 'valid' | 'expiring' | 'expired'
  tags?: Record<string, string>
}

// 服务
export interface Service {
  id: string
  name: string
  displayName: string
  type: 'web' | 'api' | 'database' | 'cache' | 'message-queue' | 'other'
  description?: string
  urls: string[]
  domainRef?: string
  certificateRef?: string
  computeRef?: string
  containerRef?: string
  healthCheck?: {
    enabled: boolean
    method: string
    url: string
    interval: string
    expectedStatus: number
  }
  repository?: {
    type: string
    url: string
    branch: string
  }
  dependsOn?: Array<{
    serviceRef: string
  }>
  status?: 'running' | 'stopped' | 'unhealthy' | 'unknown'
  health?: 'healthy' | 'unhealthy' | 'unknown'
  tags?: Record<string, string>
}

// Gateway / OpenWrt
export interface Gateway {
  id: string
  name: string
  displayName: string
  location: Location
  type: 'openwrt'
  agentId?: string
  ipv4?: string
  ipv6?: string
  upstream?: string
  model?: string
  firmware?: string
  access?: {
    web?: {
      url: string
    }
    ssh?: {
      host: string
      port: number
      user: string
    }
  }
  role: 'main-router' | 'ap' | 'bypass-gateway' | 'client-gateway'
  tunnels?: Tunnel[]
  monitoring?: {
    enabled: boolean
    nezhaAgentId?: number
    publicIp: boolean
  }
  status?: 'online' | 'offline'
  tags?: Record<string, string>
}

// 隧道
export interface Tunnel {
  type: 'wireguard' | 'cloudflare-tunnel' | 'vxlan' | 'gre'
  name: string
  peerEndpoint?: string
  subnet?: string
  allowedIPs?: string[]
  keepalive?: number
  tunnelId?: string
  tunnelUrl?: string
  services?: Array<{
    serviceRef: string
    publicUrl: string
  }>
}

// 存储设备
export interface Storage {
  id: string
  name: string
  displayName: string
  location: Location
  type: 'nas' | 'san' | 'object-storage' | 'local' | 'nfs' | 'iscsi' | 'ceph'
  agentId?: string
  path?: string
  totalGb?: number
  usedGb?: number
  availableGb?: number
  provider?: string
  access?: {
    web?: {
      url: string
    }
    ssh?: {
      host: string
      port: number
      user: string
    }
    smb?: string[]
    nfs?: string[]
  }
  capacity?: {
    total: string
    used: string
    available: string
  }
  usage?: Array<{
    type: string
    description: string
  }>
  monitoring?: {
    enabled: boolean
    diskHealth: boolean
    nezhaAgentId?: number
  }
  status?: 'online' | 'offline'
  tags?: Record<string, string>
}

// API 响应
export interface ApiResponse<T> {
  data?: T
  error?: string
  message?: string
}

// 分页参数
export interface PaginationParams {
  current: number
  pageSize: number
}

// 分页响应（与后端 api.go 返回格式一致）
export interface PaginatedResponse<T> {
  data: T[]
  total: number
  page: number
  pageSize: number
  totalPages: number
}

// ========== TOTP 二次验证相关类型 ==========

// TOTP 生成响应
export interface TOTPGenerateResponse {
  secret: string
  qr_code: string
  backup_codes: string[]
}

// TOTP 验证请求
export interface TOTPVerifyRequest {
  code: string
  tmp_token?: string
}

// TOTP 验证响应
export interface TOTPVerifyResponse {
  token: string
  expires_at: number
  user_id: string
  username: string
  role: string
}

// TOTP 启用/禁用请求
export interface TOTPCodeRequest {
  code: string
}

// 登录响应（扩展支持 TOTP）
export interface LoginResponse {
  token?: string
  expires_at?: number
  user_id: string
  username: string
  role?: string
  requires_totp: boolean
  tmp_token?: string
}

// 用户信息（包含 TOTP 状态）
export interface UserInfo {
  id: string
  username: string
  email?: string
  phone?: string
  department?: string
  role: string
  totp_enabled: boolean
  totp_setup_at?: string
}

// ========== 应用部署（Compose Stack） ==========
// Stack 内单个服务的信息
export interface StackService {
  name: string
  image: string
  state: string // running / exited 等（docker 状态）
  status: string // 可读状态串，如 "Up 2 hours"
  containerId: string
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

// 拨测配置（探测间隔可配置）
export interface ProbeConfig {
  interval_seconds: number
  min_interval_seconds: number
  max_interval_seconds: number
}

// 通知渠道摘要（不含 token/secret 凭据）
export interface NotificationChannelSummary {
  channel: string // herald / ntfy / webhook / telegram
  target: string
}

// 通知事件开关（config.yaml notification.events 透传）
export interface NotificationEventRow {
  type: string // service.down / service.up / certificate.expired ...
  enabled: boolean
}

// 通知服务状态
export interface NotificationStatus {
  enabled: boolean
  channels: NotificationChannelSummary[]
  events: NotificationEventRow[]
}

// 单渠道投递结果
export interface NotificationSendResult {
  channel: string
  target?: string
  ok: boolean
  error?: string
}

// ========== 备份管理 ==========

// 备份任务配置（备份在 Agent 侧本地执行，见 docs/guide/backup-design.md）
export interface BackupConfig {
  id: number
  agent_id: string
  name: string // 备份文件名前缀
  sources: string[] // 源路径列表
  dest_dir: string
  schedule: string // manual / daily@HH:mm / every:Nh
  retention: number // 保留份数，0 = 不清理
  enabled: boolean
  last_run_at: number
  next_run_at: number // manual 恒为 0
  last_status: string // "" / running / success / failed
  created_at: number
}

// 备份配置创建/更新请求
export interface BackupConfigInput {
  agent_id: string
  name: string
  sources: string[]
  dest_dir: string
  schedule: string
  retention: number
  enabled: boolean
}

// 备份单次运行记录
export interface BackupRun {
  id: number
  configId: number
  taskId: string
  status: 'running' | 'success' | 'failed' | 'timeout'
  file: string
  size: number
  error?: string
  startedAt: number
  finishedAt: number
}

// Agent 备份目录下的产物文件
export interface BackupFile {
  name: string
  size: number
  mtime: number
}
