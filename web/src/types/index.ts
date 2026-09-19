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
  fail_threshold: number
  disk_percent: number
  memory_percent: number
  cert_warn_days: number
  cert_info_days: number
  min_interval_seconds: number
  max_interval_seconds: number
  min_fail_threshold: number
  max_fail_threshold: number
  min_percent: number
  max_percent: number
}

// 拨测历史记录（心跳条数据源，见 docs/guide/probe-enhance-design.md M2）
export interface ProbeResult {
  id: string
  resourceType: string
  resourceId: string
  name: string
  status: string // up / down / degraded / active / valid / expiring / expired / error
  latencyMs: number
  message: string
  checkedAt: string
}

// 远程日志查询（见 docs/guide/logs-design.md）
export interface LogsStatus {
  journalctl: boolean
  docker: boolean
}

export interface LogsSources {
  systemd: string[]
  docker: string[]
}

export interface LogsQuery {
  type: 'systemd' | 'docker'
  source: string
  tail: number
  since_minutes: number
  grep: string
}

export interface LogsQueryResult {
  lines: string
  truncated: boolean
}

// 跨机日志联邦检索（见 docs/guide/logs-design.md M3，D11-D16）
export interface LogsSearchQuery {
  type: 'systemd' | 'docker'
  source: string
  tail?: number
  since_minutes?: number
  grep?: string
  agents?: string[]
}

export interface LogsSearchItem {
  agentId: string
  hostname: string
  ok: boolean
  truncated?: boolean
  lines?: string
  error?: string
}

export interface LogsSearchSkipped {
  agentId: string
  reason: 'offline' | 'no-logs'
}

export interface LogsSearchResult {
  results: LogsSearchItem[]
  skipped: LogsSearchSkipped[]
}

// 防漂移检测（见 docs/guide/drift-design.md）
export interface DriftCheckItem {
  kind: 'nginx' | 'traefik' | 'cron' | 'stack'
  name: string
  status: 'ok' | 'drifted' | 'missing' | 'no_baseline' | 'error' | 'none'
  baseline_sha: string
  current_sha: string
}

export interface DriftCheckResult {
  items: DriftCheckItem[]
  checked_at: number
}

// 漂移 diff：单对象两侧全文（cron 侧为美化后 JSON；见 drift-design.md M3）
export interface DriftDiffResult {
  expected: string
  current: string
  baseline_updated_at: number
}

// 巡检配置（server 定时 drift.check + 漂移告警；0 = 关闭）
export interface DriftScanConfig {
  scan_interval_seconds: number
  min: number
  max: number
  default: number
}

// 会话录制元数据（内容在 server 侧 .cast 文件，见 recording-design.md）
export interface TerminalRecording {
  id: number
  sessionId: string
  username: string
  agentId: string
  host: string
  port: number
  protocol: string
  startedAt: string
  durationMs: number // 0 = 进行中
  bytes: number
}

// Server 自身数据库备份（VACUUM INTO 产物，目录即事实源，见 server-backup-design.md）
export interface ServerBackupFile {
  name: string
  size: number
  modTime: string
}

export interface ServerBackupConfig {
  interval_hours: number
  retention_days: number
  max_interval_hours: number
  max_retention_days: number
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
  remote_dest?: string // rclone 远端目标 remote:path，空 = 不启用异地（M2 D20）
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
  remote_dest?: string // 空 = 不启用异地
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
  remoteStatus?: '' | 'ok' | 'failed' // 异地推送结果，空 = 未启用（M2 D20）
  remoteError?: string
  startedAt: number
  finishedAt: number
}

// Agent 备份目录下的产物文件
export interface BackupFile {
  name: string
  size: number
  mtime: number
}

// Agent 侧任务状态（restore 等，server 转发不落库）
export interface BackupTask {
  taskId: string
  action: 'backup' | 'restore'
  status: 'running' | 'success' | 'failed'
  error?: string
  file?: string
  size?: number
  startedAt?: number
  finishedAt?: number
  log?: string
}

// Agent 远程文件条目（目录直属，不递归）
export interface FileEntry {
  name: string
  size: number
  mode: string // 八进制权限，如 "0644"
  uid?: number // 属主 uid（unix；windows/旧 agent 无为 -1 或缺省）
  gid?: number
  mtime: number
  isDir: boolean
  isSymlink: boolean
  target: string // symlink 目标（仅 isSymlink 时有值）
}

// file.read 分块结果
export interface FileReadResult {
  data: string // base64
  size: number
  eof: boolean
}

// file.search 文本搜索结果（见 file-manager-design.md D10-D11）
export interface FileSearchMatch {
  path: string // 相对搜索根目录
  line: number
  text: string // 命中行（agent 侧已截断 200 字符）
}

export interface FileSearchResult {
  matches: FileSearchMatch[]
  truncated: boolean
  scanned: number
  skipped: number
}

// ============ 反向代理（Nginx 站点管理） ============

// proxy.status：nginx 安装状态概览
export interface ProxyStatus {
  backend?: 'nginx' | 'traefik' // M2 起区分后端；旧 agent 无此字段视为 nginx
  installed: boolean
  version: string
  confDir: string
  siteCount: number
  reloadMode: 'systemctl' | 'signal' | 'hot'
}

// 一个反代站点的声明（与 agent 侧 ProxySite 同构）
export interface ProxySite {
  name: string
  serverNames: string[]
  upstream: string
  scheme: 'http' | 'https'
  tlsCert?: string
  tlsKey?: string
  websocket?: boolean
  extra?: string
}

// proxy.site.get：站点元数据 + 渲染后的配置全文
export interface ProxySiteDetail {
  name: string
  site: ProxySite
  content: string
}

// ============ 定时任务（Crontab 管理） ============

// cron.status：概览
export interface CronStatus {
  user: string
  cockpitCount: number
  externalCount: number
}

// cockpit 名下的一个定时任务（next_run 为下次触发预览 unix 秒，
// disabled/@reboot/解析失败为 0；见 docs/guide/cron-design.md M2）
export interface CronJob {
  name: string
  schedule: string
  command: string
  enabled: boolean
  next_run?: number
}

// cron.jobs 返回：cockpit 任务列表 + 外部条目原文（只读）
export interface CronJobsResult {
  jobs: CronJob[]
  external: string
}

// 一个 systemd timer unit 的只读快照（cron-design.md M3：日程为
// OnCalendar/OnBootUSec 原文不做语义解析；时间为 unix 秒，缺失/不可解析为 0；
// 模板单元等 show 失败的仅 unit 名，其余字段空）
export interface SystemdTimer {
  unit: string
  description: string
  schedule: string
  state: string
  unitFileState: string
  last_trigger: number
  next_run: number
}

// ============ systemd 服务管理（见 docs/guide/service-design.md） ============

// 一个 *.service unit 的观测快照
export interface ServiceUnit {
  name: string
  description: string
  loadState: string
  activeState: string
  subState: string
  unitFileState: string // enabled/disabled/static/... （自启态）
  preset: string
}

// service.list 返回
export interface ServiceListResult {
  services: ServiceUnit[]
}

// service.status：概览
export interface ServiceStatus {
  systemState: string
  total: number
  active: number
  failed: number
  enabled: number
}

// 服务管理动作（白名单，双端同规则；mask/unmask 仅 systemd 后端）
export type ServiceActionName = 'start' | 'stop' | 'restart' | 'reload' | 'enable' | 'disable' | 'mask' | 'unmask'

// service.unitfile 返回（D13）：systemctl cat 有效视图
export interface ServiceUnitFile {
  name: string
  fragmentPath: string
  content: string
}

// service.unitsave 返回：包管文件保存时 path 为 /etc 覆盖位
export interface ServiceUnitFileSaveResult {
  name: string
  path: string
  reloaded: boolean
}

// ========== Overlay 组网观测（见 overlay-design.md） ==========

// 组网对端节点（工具间字段取可用子集）
export interface OverlayPeer {
  id: string
  name?: string
  virtualIps?: string[]
  version?: string
  latencyMs?: number
  online: boolean
  endpoint?: string
  relay?: string
  role?: string
  lastHandshake?: string
}

// 本机加入的网络（ZeroTier/Tailscale）
export interface OverlayNetwork {
  id: string
  name?: string
  status?: string
  type?: string
  dev?: string
  online?: boolean
  ips?: string[]
}

// WireGuard 接口
export interface OverlayWGInterface {
  name: string
  listenPort?: string
  peerCount: number
  peers: OverlayPeer[]
}

// 单个组网工具的观测快照
export interface OverlayTool {
  tool: 'zerotier' | 'tailscale' | 'wireguard' | 'frp'
  status: 'ok' | 'degraded' | 'error' | 'unavailable'
  version?: string
  error?: string
  networks?: OverlayNetwork[]
  peers?: OverlayPeer[]
  interfaces?: OverlayWGInterface[]
  extra?: Record<string, unknown>
}

// overlay.status 返回
export interface OverlayStatus {
  tools: OverlayTool[]
}

// ========== 磁盘健康（SMART，见 disk-health-design.md） ==========

// smart.status devices[] 元素（白名单字段，取不到的省略）
export interface SmartDevice {
  name: string
  model?: string
  serial?: string
  sizeBytes?: number
  health: 'passed' | 'failed' | 'unknown'
  temperatureC?: number
  powerOnHours?: number
  reallocatedSectors?: number
  pendingSectors?: number
  mediaErrors?: number
  percentUsed?: number
  error?: string
}

// smart.status 返回
export interface SmartStatus {
  available: boolean
  devices: SmartDevice[]
}

// 巡检配置（server 定时 smart.status + 磁盘告警；0 = 关闭）
export interface SmartScanConfig {
  scan_interval_seconds: number
  min: number
  max: number
  default: number
}

// ========== DNS 管理（Cloudflare，见 dns-design.md） ==========

// DNS 配置探测（只返回布尔，不含 token）
export interface DNSStatus {
  configured: boolean
  provider: string
}

// DNS 托管区（in_cmdb = 是否已登记在「资源 → 域名」）
export interface DNSZone {
  id: string
  name: string
  status: string
  name_servers: string[]
  in_cmdb: boolean
}

// DNS 记录
export interface DNSRecord {
  id: string
  type: string
  name: string
  content: string
  ttl: number
  proxied: boolean
  locked: boolean
}

// 创建/更新入参
export interface DNSRecordInput {
  type: string
  name: string
  content: string
  ttl: number
  proxied: boolean
}

// 记录分页结果
export interface DNSRecordsPage {
  records: DNSRecord[]
  page: number
  total_pages: number
}

// ========== DDNS（动态域名解析，见 ddns-design.md） ==========

// 一条 DDNS 配置（server 落库；巡检时让绑定 agent 探测公网 IP 并写 Cloudflare）
export interface DDNSConfig {
  id: number
  agentId: string
  zoneId: string
  zoneName: string
  recordName: string // 完整记录名，如 home.example.com
  type: 'A' | 'AAAA'
  enabled: boolean
  lastIP: string
  lastStatus: 'never' | 'ok' | 'failed'
  lastError: string
  checkedAt: number // Unix 秒，0 = 从未
  createdAt: string
  updatedAt: string
}

// DDNS 配置创建/更新入参
export interface DDNSConfigInput {
  agentId: string
  zoneId: string
  zoneName: string
  recordName: string
  type: 'A' | 'AAAA'
  enabled: boolean
}

// 立即检查结果
export interface DDNSCheckResult {
  ip: string
  changed: boolean
  status: string
  error: string
}

// 巡检配置（server 定时同步；0 = 关闭）
export interface DDNSScanConfig {
  scan_interval_seconds: number
  min: number
  max: number
  default: number
}

// ========== ACME 证书自动签发（见 acme-design.md） ==========

// 一条证书签发配置（列表/详情响应视图：PEM 字段永不出响应，
// hasPrivateKey 标记私钥是否已入库）
export interface AcmeCertView {
  id: number
  domains: string[]
  primaryDomain: string
  caDirectory: 'staging' | 'production'
  status: 'pending' | 'issued' | 'failed'
  hasPrivateKey: boolean
  expiresAt: string
  renewBeforeDays: number
  autoRenew: boolean
  lastRenewAt: number
  lastStatus: 'never' | 'ok' | 'failed'
  lastError: string
  checkedAt: number
  // 部署目标与状态（D14）
  deployAgentId: string
  deployCertPath: string
  deployKeyPath: string
  lastDeployAt: number
  lastDeployError: string
  createdAt: string
  updatedAt: string
}

// 签发配置创建/更新入参
export interface AcmeCertInput {
  domains: string[]
  caDirectory: 'staging' | 'production'
  autoRenew: boolean
  renewBeforeDays: number
  deployAgentId?: string
  deployCertPath?: string
  deployKeyPath?: string
}

// ACME 账户信息
export interface AcmeAccount {
  registered: boolean
  email?: string
  caDirectory?: string
  registrationURI?: string
}

// ACME 视角的 DNS provider 状态（D13：与 DNS 管理页的 Cloudflare 专属判定语义分叉）
export interface AcmeDnsStatus {
  provider: 'cloudflare' | 'dnspod' | 'alidns'
  configured: boolean
}

// 续期巡检配置（server 定时重签；0 = 关闭）
export interface AcmeScanConfig {
  scan_interval_seconds: number
  min: number
  max: number
  default: number
  dns?: AcmeDnsStatus
}

// ========== NAS 存储观测（见 nas-design.md） ==========

// 存储池：mdadm（/proc/mdstat）、ZFS（zpool）、LVM（vgs）、DSM/TrueNAS 厂商池
export interface NasPool {
  name: string
  kind: 'mdadm' | 'zfs' | 'lvm' | 'dsm' | 'truenas' | 'omv'
  state: 'healthy' | 'degraded' | 'resync' | 'failed' | 'unknown'
  totalGB?: number
  usedGB?: number
  devices?: string[]
  detail?: string
  host?: string // 来源设备：网络 NAS（M2）填 target 名，本地观测为空
}

// 挂载点容量（df 白名单：真实块设备且 ≥1GB；DSM 卷来自 load_info）
export interface NasMount {
  device: string
  mountPath: string
  fsType: string
  totalGB?: number
  usedGB?: number
  host?: string
}

// 网络共享（SMB testparm / NFS exportfs / DSM 共享文件夹标 smb）
export interface NasShare {
  protocol: 'smb' | 'nfs'
  name: string
  path: string
  comment?: string
  hosts?: string
  host?: string
}

// nas.status 返回（agent 本机只读观测，M1 源 = linux）
export interface NasStatus {
  available: boolean
  source: string
  pools: NasPool[]
  mounts: NasMount[]
  shares: NasShare[]
}

// 巡检配置（server 定时 nas.status + 池/容量告警；间隔 0 = 关闭）
export interface NasScanConfig {
  scan_interval_seconds: number
  min: number
  max: number
  default: number
  usage_warn_percent: number
  usageMin: number
  usageMax: number
}
