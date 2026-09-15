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
  DriftCheckResult,
  DriftScanConfig,
  NotificationStatus,
  NotificationSendResult,
  BackupConfig,
  BackupConfigInput,
  BackupRun,
  BackupFile,
  BackupTask,
  FileEntry,
  FileReadResult,
  ProxySite,
  ProxySiteDetail,
  ProxyStatus,
  CronJob,
  CronJobsResult,
  CronStatus,
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

  // ========== 防漂移检测 ==========
  // 全量比对：面板写路径基线 vs 磁盘当前内容（nginx/cron/stack）
  async checkDrift(agentId: string): Promise<DriftCheckResult> {
    return this.client.post<unknown, DriftCheckResult>(
      `/agents/${encodeURIComponent(agentId)}/drift/check`,
    )
  }

  // 巡检配置：server 定时扫描全部主机并产生漂移告警（0 = 关闭）
  async getDriftConfig(): Promise<DriftScanConfig> {
    return this.client.get<unknown, DriftScanConfig>('/drift/config')
  }

  async putDriftConfig(scanIntervalSeconds: number): Promise<void> {
    await this.client.put('/drift/config', { scan_interval_seconds: scanIntervalSeconds })
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

  // 概览：运行用户 + 条目计数
  async getCronStatus(agentId: string): Promise<CronStatus> {
    return this.client.get<unknown, CronStatus>(`/agents/${encodeURIComponent(agentId)}/cron/status`)
  }

  // cockpit 名下任务列表 + 外部条目原文（只读）
  async getCronJobs(agentId: string): Promise<CronJobsResult> {
    return this.client.get<unknown, CronJobsResult>(`/agents/${encodeURIComponent(agentId)}/cron/jobs`)
  }

  // 应用任务（写回时 agent 保证外部条目逐行不变）
  async applyCronJob(agentId: string, job: CronJob): Promise<{ name: string }> {
    return this.client.put<unknown, { name: string }>(
      `/agents/${encodeURIComponent(agentId)}/cron/jobs/${encodeURIComponent(job.name)}`,
      job,
    )
  }

  // 删除任务
  async deleteCronJob(agentId: string, name: string): Promise<void> {
    await this.client.delete(`/agents/${encodeURIComponent(agentId)}/cron/jobs/${encodeURIComponent(name)}`)
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
