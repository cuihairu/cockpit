import axios, { AxiosInstance } from 'axios'
import type { UISettings } from '@/contexts/settingsTypes'
import type {
  Agent,
  ComputeInstance,
  Domain,
  Certificate,
  Service,
  Gateway,
  Storage,
  ContainerInfo,
  ImageInfo,
  PaginatedResponse,
  TOTPGenerateResponse,
  TOTPVerifyResponse,
  LoginResponse,
  UserInfo,
  StackView,
  StackDetail,
  StackCompose,
  StackComposeSaveResponse,
  StackTaskStartResponse,
  StackTask,
  StackInfo,
  StackDeployment,
  ProbeConfig,
  ProbeResult,
  LogsStatus,
  LogsSources,
  LogsQuery,
  LogsQueryResult,
  LogsSearchQuery,
  LogsSearchResult,
  DriftCheckResult,
  ConsistencyReport,
  DriftDiffResult,
  DriftScanConfig,
  RecordingConfig,
  TerminalRecording,
  ServerBackupFile,
  ServerBackupConfig,
  DNSStatus,
  DNSZone,
  DNSRecord,
  DNSRecordInput,
  DNSRecordsPage,
  NotificationStatus,
  NotificationSendResult,
  BackupConfig,
  BackupConfigInput,
  BackupRun,
  BackupFile,
  BackupTask,
  FileEntry,
  FileSearchResult,
  FileReadResult,
  ProxySite,
  ProxySiteDetail,
  ProxyStatus,
  CronJob,
  CronJobsResult,
  SystemdTimer,
  ServiceActionName,
  ServiceUnitFile,
  ServiceUnitFileSaveResult,
  ServiceListResult,
  ServiceStatus,
  OverlayStatus,
  OverlayCloudStatus,
  CronStatus,
  CronUserEntry,
  SmartScanConfig,
  SmartStatus,
  NasScanConfig,
  NasStatus,
  DDNSConfig,
  DDNSConfigInput,
  DDNSCheckResult,
  DDNSScanConfig,
  AcmeCertView,
  AcmeCertInput,
  AcmeAccount,
  AcmeScanConfig,
  DomainBinding,
  DomainBindingInput,
  DomainApplyStep,
  DomainDriftResponse,
} from '@/types'
import { logger } from '@/utils/logger'

type StatusResponse = {
  services: { running: number; down: number; unknown: number }
  domains: { valid: number; expiring: number }
  certificates: { valid: number; expiring: number }
  infrastructure: { total: number; online: number }
}

class ApiService {
  private client: AxiosInstance

  constructor() {
    this.client = axios.create({
      baseURL: '/api',
      timeout: 30000,
      headers: {
        'Content-Type': 'application/json',
      },
    })

    // 请求拦截器 - 添加 token
    this.client.interceptors.request.use(
      (config) => {
        const token = localStorage.getItem('token')
        if (token) {
          config.headers.Authorization = `Bearer ${token}`
        }
        return config
      },
      (error) => Promise.reject(error)
    )

    // 响应拦截器
    this.client.interceptors.response.use(
      (response) => response.data,
      (error) => {
        if (error.response?.status === 401) {
          // Token 过期，清除并跳转登录
          localStorage.removeItem('token')
          localStorage.removeItem('username')
          window.location.href = '/login'
        }
        logger.error('API Error:', error)
        return Promise.reject(error)
      }
    )
  }

  // ========== 认证 ==========
  async login(username: string, password: string): Promise<LoginResponse> {
    const data = await this.client.post<unknown, LoginResponse>('/auth/login', { username, password })
    return data
  }

  async logout() {
    localStorage.removeItem('token')
    localStorage.removeItem('username')
    window.location.href = '/login'
  }

  async refreshToken() {
    return this.client.post<{ token: string }>('/auth/refresh')
  }

  // 获取当前用户信息
  async getCurrentUser(): Promise<UserInfo> {
    return this.client.get<unknown, UserInfo>('/me')
  }

  // 更新当前用户信息
  async updateProfile(data: { email?: string; phone?: string; department?: string }): Promise<{
    message: string
    id: string
    username: string
    email?: string
    phone?: string
    department?: string
  }> {
    return this.client.put<
      unknown,
      {
        message: string
        id: string
        username: string
        email?: string
        phone?: string
        department?: string
      }
    >('/me/profile', data)
  }

  // 保存用户设置
  async saveSettings(data: Partial<UISettings>): Promise<{ message: string }> {
    return this.client.put<unknown, { message: string }>('/settings', data)
  }

  // 修改当前用户密码
  async changePassword(currentPassword: string, newPassword: string): Promise<{ message: string }> {
    return this.client.put<unknown, { message: string }>('/me/password', {
      currentPassword,
      newPassword,
    })
  }

  // ========== TOTP 二次验证 ==========

  // 忘记密码 - 发送重置邮件
  async forgotPassword(username: string): Promise<{ email: string; masked_email: string; message: string }> {
    return this.client.post<unknown, { email: string; masked_email: string; message: string }>('/auth/forgot-password', { username })
  }

  // 重置密码
  async resetPassword(token: string, code: string, newPassword: string): Promise<{ message: string }> {
    return this.client.post<unknown, { message: string }>('/auth/reset-password', {
      token,
      code,
      newPassword,
    })
  }

  // 验证重置验证码
  async verifyResetCode(token: string, code: string): Promise<{ valid: boolean; message?: string }> {
    return this.client.post<unknown, { valid: boolean; message?: string }>('/auth/verify-reset-code', { token, code })
  }

  // 生成 TOTP 密钥和 QR 码
  async generateTOTP(): Promise<TOTPGenerateResponse> {
    return this.client.post<unknown, TOTPGenerateResponse>('/auth/totp/generate')
  }

  // 启用 TOTP
  async enableTOTP(code: string): Promise<{ status: string; message: string }> {
    return this.client.post<unknown, { status: string; message: string }>('/auth/totp/enable', { code })
  }

  // 禁用 TOTP
  async disableTOTP(code: string): Promise<{ status: string; message: string }> {
    return this.client.post<unknown, { status: string; message: string }>('/auth/totp/disable', { code })
  }

  // 验证 TOTP 代码（登录时的二次验证）
  async verifyTOTP(code: string, tmpToken: string): Promise<TOTPVerifyResponse> {
    return this.client.post<unknown, TOTPVerifyResponse>('/auth/totp/verify', { code, tmp_token: tmpToken })
  }

  // ========== Agent ==========
  async getAgents(): Promise<Agent[]> {
    return this.client.get<unknown, Agent[]>('/agents')
  }

  async getAgent(id: string): Promise<Agent> {
    return this.client.get<unknown, Agent>(`/agents/${id}`)
  }

  // ========== Docker（通过 Agent RPC 代理） ==========
  async getContainers(agentId: string, all = true): Promise<ContainerInfo[]> {
    return this.client.get<unknown, ContainerInfo[]>(`/docker/agents/${agentId}/containers`, {
      params: { all },
    })
  }

  async getImages(agentId: string): Promise<ImageInfo[]> {
    return this.client.get<unknown, ImageInfo[]>(`/docker/agents/${agentId}/images`)
  }

  async startContainer(agentId: string, containerId: string): Promise<void> {
    await this.client.post(`/docker/agents/${agentId}/containers/${containerId}/start`)
  }

  async stopContainer(agentId: string, containerId: string, timeout?: number): Promise<void> {
    await this.client.post(`/docker/agents/${agentId}/containers/${containerId}/stop`, undefined, {
      params: timeout !== undefined ? { timeout } : undefined,
    })
  }

  async restartContainer(agentId: string, containerId: string, timeout?: number): Promise<void> {
    await this.client.post(`/docker/agents/${agentId}/containers/${containerId}/restart`, undefined, {
      params: timeout !== undefined ? { timeout } : undefined,
    })
  }

  async pauseContainer(agentId: string, containerId: string): Promise<void> {
    await this.client.post(`/docker/agents/${agentId}/containers/${containerId}/pause`)
  }

  async unpauseContainer(agentId: string, containerId: string): Promise<void> {
    await this.client.post(`/docker/agents/${agentId}/containers/${containerId}/unpause`)
  }

  async removeContainer(
    agentId: string,
    containerId: string,
    opts?: { force?: boolean; volumes?: boolean },
  ): Promise<void> {
    await this.client.delete(`/docker/agents/${agentId}/containers/${containerId}`, {
      params: opts,
    })
  }

  async getContainerLogs(
    agentId: string,
    containerId: string,
    opts?: { tail?: string; timestamps?: boolean },
  ): Promise<string> {
    return this.client.get<unknown, string>(
      `/docker/agents/${agentId}/containers/${containerId}/logs`,
      { params: { tail: opts?.tail ?? '100', timestamps: opts?.timestamps ?? false } },
    )
  }

  // ========== 应用部署（Compose Stack，通过 Agent RPC 代理） ==========
  // 聚合所有 agent 的 Stack 列表（agentInfo 为各 agent 的目录自检信息，M1.5）
  async getStacks(): Promise<{ stacks: StackView[]; agentInfo?: Record<string, StackInfo> }> {
    return this.client.get<unknown, { stacks: StackView[]; agentInfo?: Record<string, StackInfo> }>('/stacks')
  }

  // Stack 详情（服务列表与运行状态）
  async getStackDetail(agentId: string, name: string): Promise<StackDetail> {
    return this.client.get<unknown, StackDetail>(`/stacks/agents/${agentId}/${name}`)
  }

  // 读取 Stack 的 compose.yml 与 .env 内容
  async getStackCompose(agentId: string, name: string): Promise<StackCompose> {
    return this.client.get<unknown, StackCompose>(`/stacks/agents/${agentId}/${name}/compose`)
  }

  // 保存 compose.yml / .env；YAML 校验失败时返回 502，error 含 docker compose config 输出
  async saveStackCompose(
    agentId: string,
    name: string,
    data: { compose: string; env?: string },
  ): Promise<StackComposeSaveResponse> {
    return this.client.put<unknown, StackComposeSaveResponse>(
      `/stacks/agents/${agentId}/${name}/compose`,
      data,
    )
  }

  // 启动 / 停止 / 重启 / 拉取镜像 Stack（异步任务）
  async stackAction(
    agentId: string,
    name: string,
    action: 'up' | 'down' | 'restart' | 'pull',
  ): Promise<StackTaskStartResponse> {
    return this.client.post<unknown, StackTaskStartResponse>(
      `/stacks/agents/${agentId}/${name}/${action}`,
    )
  }

  // Stack 部署历史（server 侧记录，agent 离线时仍可查）
  async getStackHistory(agentId: string, name: string, limit = 50): Promise<{ deployments: StackDeployment[] }> {
    return this.client.get<unknown, { deployments: StackDeployment[] }>(
      `/stacks/agents/${agentId}/${name}/history`,
      { params: { limit } },
    )
  }

  // 查询 Stack 异步任务状态
  async getStackTask(agentId: string, taskId: string): Promise<StackTask> {
    return this.client.get<unknown, StackTask>(`/stacks/agents/${agentId}/tasks/${taskId}`)
  }

  // Stack 部署日志（可按服务过滤）
  async getStackLogs(
    agentId: string,
    name: string,
    opts?: { service?: string; tail?: number },
  ): Promise<{ logs: string }> {
    return this.client.get<unknown, { logs: string }>(`/stacks/agents/${agentId}/${name}/logs`, {
      params: { service: opts?.service, tail: opts?.tail ?? 200 },
    })
  }

  // 删除 Stack（异步任务：先 down 再删目录）
  async deleteStack(agentId: string, name: string): Promise<StackTaskStartResponse> {
    return this.client.delete<unknown, StackTaskStartResponse>(`/stacks/agents/${agentId}/${name}`)
  }

  // ========== 计算实例 ==========
  async getComputeInstances(params?: {
    region?: string
    zone?: string
    type?: string
  }): Promise<PaginatedResponse<ComputeInstance>> {
    return this.client.get<unknown, PaginatedResponse<ComputeInstance>>('/resources/compute-instances', {
      params,
    })
  }

  async getComputeInstance(id: string): Promise<ComputeInstance> {
    return this.client.get<unknown, ComputeInstance>(`/resources/compute-instances/${id}`)
  }

  // ========== 域名 ==========
  async getDomains(): Promise<PaginatedResponse<Domain>> {
    return this.client.get<unknown, PaginatedResponse<Domain>>('/resources/domains')
  }

  async getDomain(id: string): Promise<Domain> {
    return this.client.get<unknown, Domain>(`/resources/domains/${id}`)
  }

  // ========== 证书 ==========
  async getCertificates(): Promise<PaginatedResponse<Certificate>> {
    return this.client.get<unknown, PaginatedResponse<Certificate>>('/resources/certificates')
  }

  async getCertificate(id: string): Promise<Certificate> {
    return this.client.get<unknown, Certificate>(`/resources/certificates/${id}`)
  }

  // ========== 服务 ==========
  async getServices(): Promise<PaginatedResponse<Service>> {
    return this.client.get<unknown, PaginatedResponse<Service>>('/resources/services')
  }

  async getService(id: string): Promise<Service> {
    return this.client.get<unknown, Service>(`/resources/services/${id}`)
  }

  // ========== Gateway ==========
  async getGateways(): Promise<PaginatedResponse<Gateway>> {
    return this.client.get<unknown, PaginatedResponse<Gateway>>('/resources/gateways')
  }

  async getGateway(id: string): Promise<Gateway> {
    return this.client.get<unknown, Gateway>(`/resources/gateways/${id}`)
  }

  // ========== 存储 ==========
  async getStorages(): Promise<PaginatedResponse<Storage>> {
    return this.client.get<unknown, PaginatedResponse<Storage>>('/resources/storages')
  }

  async getStorage(id: string): Promise<Storage> {
    return this.client.get<unknown, Storage>(`/resources/storages/${id}`)
  }

  // ========== 系统状态 ==========
  async getStatus(): Promise<StatusResponse> {
    return this.client.get<unknown, StatusResponse>('/status')
  }

  // ========== 警告/通知 ==========
  async getAlerts(): Promise<{ data: Alert[] }> {
    return this.client.get<unknown, { data: Alert[] }>('/alerts')
  }

  async markAlertRead(id: string): Promise<void> {
    return this.client.put(`/alerts/${id}/read`)
  }

  async markAllAlertsRead(): Promise<void> {
    return this.client.put('/alerts/read-all')
  }

  // ========== 拨测与通知（拨测增强） ==========
  // 当前探测配置（间隔 + 失败阈值 + 告警阈值及其合法范围）
  async getProbeConfig(): Promise<ProbeConfig> {
    return this.client.get<unknown, ProbeConfig>('/probe/config')
  }

  // 全量保存拨测与告警阈值配置（M2/D14），立即生效并持久化
  async saveProbeConfig(config: ProbeConfig): Promise<ProbeConfig> {
    return this.client.put<unknown, ProbeConfig>('/probe/config', {
      interval_seconds: config.interval_seconds,
      fail_threshold: config.fail_threshold,
      disk_percent: config.disk_percent,
      memory_percent: config.memory_percent,
      cert_warn_days: config.cert_warn_days,
      cert_info_days: config.cert_info_days,
    })
  }

  // 拨测历史：按目标取最近 N 条（心跳条数据源）
  async getProbeHistory(
    resourceType: string,
    resourceId: string,
    limit = 50,
  ): Promise<{ results: ProbeResult[] }> {
    return this.client.get<unknown, { results: ProbeResult[] }>(
      `/probe/history?resource_type=${encodeURIComponent(resourceType)}&resource_id=${encodeURIComponent(resourceId)}&limit=${limit}`,
    )
  }

  // 通知服务状态（渠道摘要 + 事件开关，不含凭据）
  async getNotificationStatus(): Promise<NotificationStatus> {
    return this.client.get<unknown, NotificationStatus>('/notification/status')
  }

  // 向全部启用渠道发送测试通知，返回逐渠道结果
  async testNotification(): Promise<{ results: NotificationSendResult[] }> {
    return this.client.post<unknown, { results: NotificationSendResult[] }>('/notification/test')
  }

  // ========== 远程日志查询（日志查看） ==========
  // 两类日志源可用性（journalctl / docker）
  async getLogsStatus(agentId: string): Promise<LogsStatus> {
    return this.client.get<unknown, LogsStatus>(
      `/agents/${encodeURIComponent(agentId)}/logs/status`,
    )
  }

  // 运行中的 systemd 服务与 docker 容器
  async getLogsSources(agentId: string): Promise<LogsSources> {
    return this.client.get<unknown, LogsSources>(
      `/agents/${encodeURIComponent(agentId)}/logs/sources`,
    )
  }

  // 查询日志（tail / since_minutes / grep 过滤在 agent 侧完成）
  async queryLogs(agentId: string, query: LogsQuery): Promise<LogsQueryResult> {
    return this.client.post<unknown, LogsQueryResult>(
      `/agents/${encodeURIComponent(agentId)}/logs/query`,
      query,
    )
  }

  // 跨机日志联邦检索：server 并行扇出 logs.query 按主机分组返回
  // （logs-design.md M3/D11-D16；tail 上限 500 由 server 校验）
  async searchLogs(payload: LogsSearchQuery): Promise<LogsSearchResult> {
    return this.client.post<unknown, LogsSearchResult>('/logs/search', payload)
  }

  // 实时尾随：NDJSON 流式响应（{"data":...} 数据帧 / {"eof":true,"reason":...} 终止帧）。
  // EventSource 不能带 Authorization 头，用 fetch streaming（signal 供 abort 断流，
  // 断开后 server 补发 follow.stop）。见 docs/guide/logs-design.md F4/F7
  async followLogs(agentId: string, query: LogsQuery, signal: AbortSignal): Promise<Response> {
    const token = localStorage.getItem('token')
    return fetch(`/api/agents/${encodeURIComponent(agentId)}/logs/follow`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify(query),
      signal,
    })
  }

  // ========== 防漂移检测 ==========
  // 全量比对：面板写路径基线 vs 磁盘当前内容（nginx/cron/stack）
  async checkDrift(agentId: string): Promise<DriftCheckResult> {
    return this.client.post<unknown, DriftCheckResult>(
      `/agents/${encodeURIComponent(agentId)}/drift/check`,
    )
  }

  // 漂移 diff：单对象两侧全文（基线原文 vs 磁盘当前；M3）
  async driftDiff(agentId: string, kind: string, name: string): Promise<DriftDiffResult> {
    return this.client.post<unknown, DriftDiffResult>(
      `/agents/${encodeURIComponent(agentId)}/drift/diff`,
      { kind, name },
    )
  }

  // 手动登记基线「以当前为准」（M4）：no_baseline 纳入检测 / drifted 确认手改
  async driftRecord(agentId: string, kind: string, name: string): Promise<void> {
    await this.client.post(`/agents/${encodeURIComponent(agentId)}/drift/record`, {
      kind,
      name,
    })
  }

  // 巡检配置：server 定时扫描全部主机并产生漂移告警（0 = 关闭）
  async getDriftConfig(): Promise<DriftScanConfig> {
    return this.client.get<unknown, DriftScanConfig>('/drift/config')
  }

  async putDriftConfig(scanIntervalSeconds: number): Promise<void> {
    await this.client.put('/drift/config', { scan_interval_seconds: scanIntervalSeconds })
  }

  // CMDB 一致性（M6）：inventory 声明 vs agent 实报按需比对（server 未启用 inventory 时 503）
  async getInventoryConsistency(): Promise<ConsistencyReport> {
    return this.client.get<unknown, ConsistencyReport>('/inventory/consistency')
  }

  // ========== 会话录制 ==========
  // 录制列表（倒序；进行中的也在列）
  async getRecordings(): Promise<TerminalRecording[]> {
    const resp = await this.client.get<unknown, { data: TerminalRecording[] }>('/recordings')
    return resp.data ?? []
  }

  // 取 .cast 内容（回放数据源，同下载）
  async getRecordingCast(sessionId: string): Promise<string> {
    const resp = await this.client.get(`/recordings/${encodeURIComponent(sessionId)}/cast`, {
      responseType: 'text',
      transformResponse: [(data: string) => data],
    })
    return resp as unknown as string
  }

  // 下载录制文件（blob，带 JWT）
  async downloadRecording(sessionId: string): Promise<Blob> {
    const resp = await this.client.get(`/recordings/${encodeURIComponent(sessionId)}/cast`, {
      responseType: 'blob',
    })
    return resp as unknown as Blob
  }

  // 删除录制（文件+元数据）
  async deleteRecording(sessionId: string): Promise<void> {
    await this.client.delete(`/recordings/${encodeURIComponent(sessionId)}`)
  }

  // 录制配置：开关/保留天数/异地归档目标（recording M2 D19）
  async getRecordingsConfig(): Promise<RecordingConfig> {
    return this.client.get<unknown, RecordingConfig>('/recordings/config')
  }

  async putRecordingsConfig(input: {
    enabled?: boolean
    retention_days?: number
    remote_dest?: string
  }): Promise<void> {
    await this.client.put('/recordings/config', input)
  }

  // 手动补推指定录制归档到异地（recording M2 D18）
  async syncRecordingRemote(sessionId: string): Promise<{ status: string }> {
    return this.client.post<unknown, { status: string }>(
      `/recordings/${encodeURIComponent(sessionId)}/sync-remote`,
    )
  }

  // ========== Server 自身数据库备份 ==========
  // 备份文件列表（目录扫描）
  async getServerBackups(): Promise<ServerBackupFile[]> {
    const resp = await this.client.get<unknown, { data: ServerBackupFile[] }>('/server-backups')
    return resp.data ?? []
  }

  async getServerBackupConfig(): Promise<ServerBackupConfig> {
    return this.client.get<unknown, ServerBackupConfig>('/server-backups/config')
  }

  async putServerBackupConfig(input: {
    interval_hours?: number
    retention_days?: number
    remote_dest?: string
  }): Promise<void> {
    await this.client.put('/server-backups/config', input)
  }

  // 立即备份，返回产物文件名
  async runServerBackup(): Promise<{ name: string }> {
    return this.client.post<unknown, { name: string }>('/server-backups/run')
  }

  // 手动补推指定备份到已配置的异地目标（server-backup M2 D17）
  async syncServerBackupRemote(name: string): Promise<{ status: string }> {
    return this.client.post<unknown, { status: string }>(
      `/server-backups/${encodeURIComponent(name)}/sync-remote`,
    )
  }

  async downloadServerBackup(name: string): Promise<Blob> {
    const resp = await this.client.get(
      `/server-backups/${encodeURIComponent(name)}/download`,
      { responseType: 'blob' },
    )
    return resp as unknown as Blob
  }

  async deleteServerBackup(name: string): Promise<void> {
    await this.client.delete(`/server-backups/${encodeURIComponent(name)}`)
  }

  // ========== DNS 管理（Cloudflare） ==========
  // 配置探测（未配置 token 时业务端点统一 503）
  async getDNSStatus(): Promise<DNSStatus> {
    return this.client.get<unknown, DNSStatus>('/dns/status')
  }

  async getDNSZones(): Promise<DNSZone[]> {
    const resp = await this.client.get<unknown, { data: DNSZone[] }>('/dns/zones')
    return resp.data ?? []
  }

  async getDNSRecords(zoneID: string, type?: string, page?: number): Promise<DNSRecordsPage> {
    const params: Record<string, string> = {}
    if (type) params.type = type
    if (page) params.page = String(page)
    const resp = await this.client.get<unknown, { data: DNSRecordsPage }>(
      `/dns/zones/${encodeURIComponent(zoneID)}/records`,
      { params },
    )
    return resp.data
  }

  async createDNSRecord(zoneID: string, input: DNSRecordInput): Promise<DNSRecord> {
    return this.client.post<unknown, DNSRecord>(
      `/dns/zones/${encodeURIComponent(zoneID)}/records`,
      input,
    )
  }

  async updateDNSRecord(zoneID: string, recordID: string, input: DNSRecordInput): Promise<DNSRecord> {
    return this.client.put<unknown, DNSRecord>(
      `/dns/zones/${encodeURIComponent(zoneID)}/records/${encodeURIComponent(recordID)}`,
      input,
    )
  }

  async deleteDNSRecord(zoneID: string, recordID: string): Promise<void> {
    await this.client.delete(
      `/dns/zones/${encodeURIComponent(zoneID)}/records/${encodeURIComponent(recordID)}`,
    )
  }

  // ========== 备份管理 ==========
  // 备份配置列表
  async getBackupConfigs(): Promise<{ configs: BackupConfig[] }> {
    return this.client.get<unknown, { configs: BackupConfig[] }>('/backups/configs')
  }

  // 创建备份配置
  async createBackupConfig(input: BackupConfigInput): Promise<BackupConfig> {
    return this.client.post<unknown, BackupConfig>('/backups/configs', input)
  }

  // 更新备份配置
  async updateBackupConfig(id: number, input: BackupConfigInput): Promise<BackupConfig> {
    return this.client.put<unknown, BackupConfig>(`/backups/configs/${id}`, input)
  }

  // 删除备份配置（级联删除运行历史）
  async deleteBackupConfig(id: number): Promise<void> {
    await this.client.delete(`/backups/configs/${id}`)
  }

  // 立即运行一次备份
  async runBackup(id: number): Promise<{ status: string }> {
    return this.client.post<unknown, { status: string }>(`/backups/configs/${id}/run`)
  }

  // 全部运行历史
  async getBackupRuns(limit = 50): Promise<{ runs: BackupRun[] }> {
    return this.client.get<unknown, { runs: BackupRun[] }>(`/backups/runs?limit=${limit}`)
  }

  // 某配置的运行历史
  async getBackupConfigRuns(id: number, limit = 50): Promise<{ runs: BackupRun[] }> {
    return this.client.get<unknown, { runs: BackupRun[] }>(`/backups/configs/${id}/runs?limit=${limit}`)
  }

  // 浏览 Agent 备份目录产物
  async getBackupFiles(id: number): Promise<{ files: BackupFile[] }> {
    return this.client.get<unknown, { files: BackupFile[] }>(`/backups/configs/${id}/files`)
  }

  // 删除 Agent 上的单个备份文件
  async deleteBackupFile(id: number, name: string): Promise<void> {
    await this.client.post(`/backups/configs/${id}/files/delete`, { name })
  }

  // 手动补传单个备份文件到 rclone 远端（M2 D24；rclone 幂等可重复调用）
  async syncBackupFileRemote(id: number, name: string): Promise<{ status: string }> {
    return this.client.post<unknown, { status: string }>(`/backups/configs/${id}/files/sync-remote`, { name })
  }

  // 恢复备份到独立目录（双确认：confirm_name 必须与 file 一致）
  async restoreBackup(
    id: number,
    input: { file: string; dest_dir: string; confirm_name: string }
  ): Promise<{ taskId: string; status: string }> {
    return this.client.post<unknown, { taskId: string; status: string }>(`/backups/configs/${id}/restore`, input)
  }

  // 轮询 Agent 侧任务状态（restore 等，不落库）
  async getBackupTask(id: number, taskId: string): Promise<BackupTask> {
    return this.client.get<unknown, BackupTask>(`/backups/configs/${id}/tasks/${encodeURIComponent(taskId)}`)
  }

  // 下载备份文件（blob，带 JWT；调用方负责 createObjectURL 触发保存）
  async downloadBackupFile(id: number, name: string): Promise<Blob> {
    const resp = await this.client.get(`/backups/configs/${id}/files/download`, {
      params: { name },
      responseType: 'blob',
    })
    return resp as unknown as Blob
  }

  // ============ 远程文件管理（Workbench 文件 Tab） ============

  // 列目录直属条目
  async listFiles(agentId: string, dir: string): Promise<{ dir: string; entries: FileEntry[] }> {
    return this.client.post<unknown, { dir: string; entries: FileEntry[] }>(
      `/agents/${encodeURIComponent(agentId)}/files/list`,
      { dir },
    )
  }

  // 读一个分块（编辑器与下载分块拉取共用）
  async readFileChunk(agentId: string, path: string, offset: number, length: number): Promise<FileReadResult> {
    return this.client.post<unknown, FileReadResult>(
      `/agents/${encodeURIComponent(agentId)}/files/read`,
      { path, offset, length },
    )
  }

  // 写文件：truncate=true 覆盖/新建（编辑保存、上传），false 追加
  async writeFile(
    agentId: string,
    path: string,
    data: string, // base64
    truncate = true,
  ): Promise<{ size: number }> {
    return this.client.post<unknown, { size: number }>(
      `/agents/${encodeURIComponent(agentId)}/files/write`,
      { path, data, truncate },
    )
  }

  async createRemoteDir(agentId: string, path: string): Promise<void> {
    await this.client.post(`/agents/${encodeURIComponent(agentId)}/files/mkdir`, { path })
  }

  async deleteRemoteFile(agentId: string, path: string): Promise<void> {
    await this.client.post(`/agents/${encodeURIComponent(agentId)}/files/delete`, { path })
  }

  async renameRemoteFile(agentId: string, path: string, name: string): Promise<void> {
    await this.client.post(`/agents/${encodeURIComponent(agentId)}/files/rename`, { path, name })
  }

  // 改权限位（0-0o777，file-manager-design M4/D22）
  async chmodFile(agentId: string, path: string, mode: number): Promise<void> {
    await this.client.post(`/agents/${encodeURIComponent(agentId)}/files/chmod`, { path, mode })
  }

  // 改属主 uid/gid（数字，不做用户名解析）
  async chownFile(agentId: string, path: string, uid: number, gid: number): Promise<void> {
    await this.client.post(`/agents/${encodeURIComponent(agentId)}/files/chown`, { path, uid, gid })
  }

  // 目录内递归文本搜索（file-manager-design.md D10-D12：纯文本 contains，
  // 默认大小写不敏感；agent 侧有深度/文件数/命中数/超时多重上限）
  async searchFiles(
    agentId: string,
    dir: string,
    query: string,
    caseSensitive = false,
  ): Promise<FileSearchResult> {
    return this.client.post<unknown, FileSearchResult>(
      `/agents/${encodeURIComponent(agentId)}/files/search`,
      { dir, query, caseSensitive },
    )
  }

  // 下载远程文件（流式 blob；大小不限，server 分块拉取不落盘）
  async downloadRemoteFile(agentId: string, path: string): Promise<Blob> {
    const resp = await this.client.get(`/agents/${encodeURIComponent(agentId)}/files/download`, {
      params: { path },
      responseType: 'blob',
    })
    return resp as unknown as Blob
  }

  // ============ 反向代理管理（Nginx 站点下发） ============

  // nginx 安装状态概览
  async getProxyStatus(agentId: string): Promise<ProxyStatus> {
    return this.client.get<unknown, ProxyStatus>(`/agents/${encodeURIComponent(agentId)}/proxy/status`)
  }

  // 列 cockpit 名下站点
  async getProxySites(agentId: string): Promise<{ sites: ProxySite[] }> {
    return this.client.get<unknown, { sites: ProxySite[] }>(
      `/agents/${encodeURIComponent(agentId)}/proxy/sites`,
    )
  }

  // 查看站点与渲染后的配置全文
  async getProxySite(agentId: string, name: string): Promise<ProxySiteDetail> {
    return this.client.get<unknown, ProxySiteDetail>(
      `/agents/${encodeURIComponent(agentId)}/proxy/sites/${encodeURIComponent(name)}`,
    )
  }

  // 应用站点（nginx -t → 写 → reload；失败时错误信息带 nginx -t stderr 摘要）
  async applyProxySite(agentId: string, site: ProxySite): Promise<{ name: string; file: string }> {
    return this.client.put<unknown, { name: string; file: string }>(
      `/agents/${encodeURIComponent(agentId)}/proxy/sites/${encodeURIComponent(site.name)}`,
      site,
    )
  }

  // 删除站点（reload 失败时 agent 自动恢复文件）
  async deleteProxySite(agentId: string, name: string): Promise<void> {
    await this.client.delete(`/agents/${encodeURIComponent(agentId)}/proxy/sites/${encodeURIComponent(name)}`)
  }

  // ============ 定时任务管理（Crontab） ============

  // user 可选查询参数（M4 D21：空/缺省 = agent 当前运行用户）
  private cronUserQuery(user?: string): string {
    return user ? `?user=${encodeURIComponent(user)}` : ''
  }

  // 概览：运行用户 + 条目计数（user 指定时为该用户视角）
  async getCronStatus(agentId: string, user?: string): Promise<CronStatus> {
    return this.client.get<unknown, CronStatus>(
      `/agents/${encodeURIComponent(agentId)}/cron/status${this.cronUserQuery(user)}`,
    )
  }

  // cockpit 名下任务列表 + 外部条目原文（只读）
  async getCronJobs(agentId: string, user?: string): Promise<CronJobsResult> {
    return this.client.get<unknown, CronJobsResult>(
      `/agents/${encodeURIComponent(agentId)}/cron/jobs${this.cronUserQuery(user)}`,
    )
  }

  // 系统用户枚举（M4 D22/D23：getent 优先 /etc/passwd 兜底，全量不过滤）
  async getCronUsers(agentId: string): Promise<{ users: CronUserEntry[] }> {
    return this.client.get<unknown, { users: CronUserEntry[] }>(
      `/agents/${encodeURIComponent(agentId)}/cron/users`,
    )
  }

  // systemd timer 只读列表（cron-design.md M3；systemd 全局无 user 维度）
  async getCronTimers(agentId: string): Promise<{ timers: SystemdTimer[] }> {
    return this.client.get<unknown, { timers: SystemdTimer[] }>(
      `/agents/${encodeURIComponent(agentId)}/cron/timers`,
    )
  }

  // 应用任务（写回时 agent 保证外部条目逐行不变；user 指定目标用户 crontab）
  async applyCronJob(agentId: string, job: CronJob, user?: string): Promise<{ name: string }> {
    return this.client.put<unknown, { name: string }>(
      `/agents/${encodeURIComponent(agentId)}/cron/jobs/${encodeURIComponent(job.name)}${this.cronUserQuery(user)}`,
      job,
    )
  }

  // 删除任务
  async deleteCronJob(agentId: string, name: string, user?: string): Promise<void> {
    await this.client.delete(
      `/agents/${encodeURIComponent(agentId)}/cron/jobs/${encodeURIComponent(name)}${this.cronUserQuery(user)}`,
    )
  }

  // ============ systemd 服务管理（见 docs/guide/service-design.md） ============

  // 概览：systemd 整体状态 + 服务计数
  async getServiceStatus(agentId: string): Promise<ServiceStatus> {
    return this.client.get<unknown, ServiceStatus>(`/agents/${encodeURIComponent(agentId)}/services/status`)
  }

  // 服务列表（运行态 + 自启态合并）
  async getAgentServices(agentId: string): Promise<ServiceListResult> {
    return this.client.get<unknown, ServiceListResult>(`/agents/${encodeURIComponent(agentId)}/services`)
  }

  // 执行 systemctl 动作（start/stop/restart/reload/enable/disable）
  async serviceAction(agentId: string, unit: string, action: ServiceActionName): Promise<{ name: string; action: string }> {
    return this.client.post<unknown, { name: string; action: string }>(
      `/agents/${encodeURIComponent(agentId)}/services/${encodeURIComponent(unit)}/${action}`,
    )
  }

  // systemctl daemon-reload：刷新 systemd manager 配置（D12，仅 systemd 后端）
  async serviceDaemonReload(agentId: string): Promise<{ reloaded: boolean }> {
    return this.client.post<unknown, { reloaded: boolean }>(
      `/agents/${encodeURIComponent(agentId)}/services/daemon-reload`,
    )
  }

  // unit 文件有效视图（D13，systemctl cat 全文）
  async getServiceUnitFile(agentId: string, unit: string): Promise<ServiceUnitFile> {
    return this.client.get<unknown, ServiceUnitFile>(
      `/agents/${encodeURIComponent(agentId)}/services/${encodeURIComponent(unit)}/file`,
    )
  }

  // 保存 unit 文件（D13）：包管文件自动复制到 /etc 覆盖位 + daemon-reload
  async saveServiceUnitFile(agentId: string, unit: string, content: string): Promise<ServiceUnitFileSaveResult> {
    return this.client.put<unknown, ServiceUnitFileSaveResult>(
      `/agents/${encodeURIComponent(agentId)}/services/${encodeURIComponent(unit)}/file`,
      { content },
    )
  }

  // ============ Overlay 组网观测 ============

  // 组网工具快照（ZeroTier/Tailscale/WireGuard/frp，只读纯转发）
  async getOverlayStatus(agentId: string): Promise<OverlayStatus> {
    return this.client.get<unknown, OverlayStatus>(`/agents/${encodeURIComponent(agentId)}/overlay/status`)
  }

  // ============ Overlay 云管理面（M2，server 直连云控制面） ============

  // 云端成员/设备列表 + managed 对照（浏览不审计）
  async getOverlayCloud(): Promise<OverlayCloudStatus> {
    return this.client.get<unknown, OverlayCloudStatus>('/overlay/cloud')
  }

  // ZeroTier 成员授权/取消授权（审计 overlay_authz）
  async setOverlayZTMemberAuthorized(networkId: string, memberId: string, authorized: boolean): Promise<void> {
    await this.client.post(
      `/overlay/cloud/zerotier/networks/${encodeURIComponent(networkId)}/members/${encodeURIComponent(memberId)}`,
      { authorized },
    )
  }

  // ZeroTier 成员除名（审计 overlay_remove；重新 join 可再授权）
  async removeOverlayZTMember(networkId: string, memberId: string): Promise<void> {
    await this.client.delete(
      `/overlay/cloud/zerotier/networks/${encodeURIComponent(networkId)}/members/${encodeURIComponent(memberId)}`,
    )
  }

  // Tailscale 设备授权（审计 overlay_authz）
  async authorizeOverlayTSDevice(deviceId: string): Promise<void> {
    await this.client.post(`/overlay/cloud/tailscale/devices/${encodeURIComponent(deviceId)}/authorize`)
  }

  // Tailscale 设备删除（审计 overlay_remove；破坏性高于 ZT，UI 侧输入名称确认）
  async removeOverlayTSDevice(deviceId: string): Promise<void> {
    await this.client.delete(`/overlay/cloud/tailscale/devices/${encodeURIComponent(deviceId)}`)
  }

  // ========== 磁盘健康（SMART，见 disk-health-design.md） ==========

  // 单机磁盘健康快照（纯转发，浏览类不记审计）
  async getSmartStatus(agentId: string): Promise<SmartStatus> {
    return this.client.get<unknown, SmartStatus>(`/agents/${encodeURIComponent(agentId)}/smart/status`)
  }

  // 巡检配置：server 定时扫描全部主机并产生磁盘告警（0 = 关闭）
  async getSmartConfig(): Promise<SmartScanConfig> {
    return this.client.get<unknown, SmartScanConfig>('/smart/config')
  }

  async putSmartConfig(scanIntervalSeconds: number): Promise<void> {
    await this.client.put('/smart/config', { scan_interval_seconds: scanIntervalSeconds })
  }

  // ========== NAS 存储观测（见 nas-design.md） ==========

  async getNASStatus(agentId: string): Promise<NasStatus> {
    return this.client.get<unknown, NasStatus>(`/agents/${encodeURIComponent(agentId)}/nas/status`)
  }

  async getNASConfig(): Promise<NasScanConfig> {
    return this.client.get<unknown, NasScanConfig>('/nas/config')
  }

  async putNASConfig(scanIntervalSeconds: number, usageWarnPercent?: number): Promise<void> {
    await this.client.put('/nas/config', {
      scan_interval_seconds: scanIntervalSeconds,
      usage_warn_percent: usageWarnPercent,
    })
  }

  // ========== DDNS（动态域名解析，见 ddns-design.md） ==========

  async getDDNSConfigs(): Promise<DDNSConfig[]> {
    return this.client.get<unknown, DDNSConfig[]>('/ddns')
  }

  async createDDNSConfig(input: DDNSConfigInput): Promise<DDNSConfig> {
    return this.client.post<unknown, DDNSConfig>('/ddns', input)
  }

  async updateDDNSConfig(id: number, input: DDNSConfigInput): Promise<DDNSConfig> {
    return this.client.put<unknown, DDNSConfig>(`/ddns/${id}`, input)
  }

  async deleteDDNSConfig(id: number): Promise<void> {
    await this.client.delete(`/ddns/${id}`)
  }

  // 立即检查单条（不等巡检周期）
  async checkDDNSConfig(id: number): Promise<DDNSCheckResult> {
    return this.client.post<unknown, DDNSCheckResult>(`/ddns/${id}/check`)
  }

  async getDDNSScanConfig(): Promise<DDNSScanConfig> {
    return this.client.get<unknown, DDNSScanConfig>('/ddns/config')
  }

  async putDDNSScanConfig(scanIntervalSeconds: number): Promise<void> {
    await this.client.put('/ddns/config', { scan_interval_seconds: scanIntervalSeconds })
  }

  // ========== ACME（证书自动签发，见 acme-design.md） ==========

  async getAcmeCerts(): Promise<AcmeCertView[]> {
    return this.client.get<unknown, AcmeCertView[]>('/acme/certs')
  }

  async createAcmeCert(input: AcmeCertInput): Promise<AcmeCertView> {
    return this.client.post<unknown, AcmeCertView>('/acme/certs', input)
  }

  async updateAcmeCert(id: number, input: AcmeCertInput): Promise<AcmeCertView> {
    return this.client.put<unknown, AcmeCertView>(`/acme/certs/${id}`, input)
  }

  async deleteAcmeCert(id: number): Promise<void> {
    await this.client.delete(`/acme/certs/${id}`)
  }

  // 立即签发/重签（同步；DNS-01 传播等待通常几秒~几十秒）
  async issueAcmeCert(id: number): Promise<AcmeCertView> {
    return this.client.post<unknown, AcmeCertView>(`/acme/certs/${id}/issue`)
  }

  // 下载 PEM（part=key 记审计）；返回原文供前端触发保存
  async downloadAcmeCert(id: number, part: 'cert' | 'issuer' | 'key'): Promise<string> {
    const resp = await this.client.get<unknown, string>(`/acme/certs/${id}/download`, { params: { part }, responseType: 'text' })
    return resp
  }

  async getAcmeAccount(): Promise<AcmeAccount> {
    return this.client.get<unknown, AcmeAccount>('/acme/account')
  }

  // 立即部署到绑定 agent（D14）
  async deployAcmeCert(id: number): Promise<AcmeCertView> {
    return this.client.post<unknown, AcmeCertView>(`/acme/certs/${id}/deploy`)
  }

  async putAcmeAccount(email: string): Promise<void> {
    await this.client.put('/acme/account', { email })
  }

  async getAcmeScanConfig(): Promise<AcmeScanConfig> {
    return this.client.get<unknown, AcmeScanConfig>('/acme/config')
  }

  async putAcmeScanConfig(scanIntervalSeconds: number): Promise<void> {
    await this.client.put('/acme/config', { scan_interval_seconds: scanIntervalSeconds })
  }

  // ========== 服务域名绑定（domain-binding-design.md） ==========

  async getDomainBindings(agentId?: string): Promise<DomainBinding[]> {
    const resp = await this.client.get<unknown, { data: DomainBinding[] }>('/domains', {
      params: agentId ? { agent: agentId } : undefined,
    })
    return resp.data ?? []
  }

  async saveDomainBinding(input: DomainBindingInput): Promise<DomainBinding> {
    return this.client.post<unknown, DomainBinding>('/domains', input)
  }

  async deleteDomainBinding(domain: string): Promise<void> {
    await this.client.delete(`/domains/${encodeURIComponent(domain)}`)
  }

  // 执行三联动，返回各步骤结果
  async applyDomainBinding(
    domain: string,
  ): Promise<{ domain: string; status: string; steps: DomainApplyStep[] }> {
    return this.client.post<unknown, { domain: string; status: string; steps: DomainApplyStep[] }>(
      `/domains/${encodeURIComponent(domain)}/apply`,
    )
  }

  // D6 漂移检查（实时逐条；按需触发，不内嵌列表）
  async getDomainDrift(agentId?: string): Promise<DomainDriftResponse> {
    return this.client.get<unknown, DomainDriftResponse>('/domains/drift', {
      params: agentId ? { agent: agentId } : undefined,
    })
  }

  // D7 该 agent 的域名清单
  async getAgentDomains(agentId: string): Promise<DomainBinding[]> {
    const resp = await this.client.get<unknown, { data: DomainBinding[] }>(
      `/agents/${encodeURIComponent(agentId)}/domains`,
    )
    return resp.data ?? []
  }

  // D7 配置片段（text/plain：env 行 + nginx server_name 行）
  async getAgentDomainsSnippet(agentId: string): Promise<string> {
    const resp = await this.client.get<unknown, string>(
      `/agents/${encodeURIComponent(agentId)}/domains/snippet`,
      { responseType: 'text' },
    )
    return resp
  }
}

// 警告类型
export interface Alert {
  id: string
  type: 'info' | 'warning' | 'error' | 'success'
  title: string
  message: string
  resource_id?: string
  resource_type?: string
  created_at: string
  read: boolean
}

// 导出单例
export const api = new ApiService()
