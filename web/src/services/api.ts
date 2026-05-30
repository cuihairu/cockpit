import axios, { AxiosInstance } from 'axios'
import type {
  Agent,
  ComputeInstance,
  Domain,
  Certificate,
  Service,
  Gateway,
  Storage,
  PaginatedResponse,
  TOTPGenerateResponse,
  TOTPVerifyResponse,
  LoginResponse,
  UserInfo,
} from '@/types'
import { logger } from '@/utils/logger'

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
    const data = await this.client.post<any, LoginResponse>('/auth/login', { username, password })
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
    return this.client.get<any, UserInfo>('/me')
  }

  // 更新当前用户信息
  async updateProfile(data: { email?: string; phone?: string; department?: string }): Promise<{ message: string }> {
    return this.client.put<any, { message: string }>('/me/profile', data)
  }

  // 保存用户设置
  async saveSettings(data: {
    siteName?: string
    refreshInterval?: number
    enableNotifications?: boolean
    theme?: string
    compactMode?: boolean
    showResourceCount?: boolean
  }): Promise<{ message: string }> {
    return this.client.put<any, { message: string }>('/settings', data)
  }

  // 修改当前用户密码
  async changePassword(currentPassword: string, newPassword: string): Promise<{ message: string }> {
    return this.client.put<any, { message: string }>('/me/password', {
      currentPassword,
      newPassword,
    })
  }

  // ========== TOTP 二次验证 ==========

  // 忘记密码 - 发送重置邮件
  async forgotPassword(username: string): Promise<{ email: string; masked_email: string; message: string }> {
    return this.client.post<any, any>('/auth/forgot-password', { username })
  }

  // 重置密码
  async resetPassword(token: string, code: string, newPassword: string): Promise<{ message: string }> {
    return this.client.post<any, { message: string }>('/auth/reset-password', {
      token,
      code,
      newPassword,
    })
  }

  // 验证重置验证码
  async verifyResetCode(token: string, code: string): Promise<{ valid: boolean; message?: string }> {
    return this.client.post<any, any>('/auth/verify-reset-code', { token, code })
  }

  // 生成 TOTP 密钥和 QR 码
  async generateTOTP(): Promise<TOTPGenerateResponse> {
    return this.client.post<any, TOTPGenerateResponse>('/auth/totp/generate')
  }

  // 启用 TOTP
  async enableTOTP(code: string): Promise<{ status: string; message: string }> {
    return this.client.post<any, { status: string; message: string }>('/auth/totp/enable', { code })
  }

  // 禁用 TOTP
  async disableTOTP(code: string): Promise<{ status: string; message: string }> {
    return this.client.post<any, { status: string; message: string }>('/auth/totp/disable', { code })
  }

  // 验证 TOTP 代码（登录时的二次验证）
  async verifyTOTP(code: string, tmpToken: string): Promise<TOTPVerifyResponse> {
    return this.client.post<any, TOTPVerifyResponse>('/auth/totp/verify', { code, tmp_token: tmpToken })
  }

  // ========== Agent ==========
  async getAgents(): Promise<Agent[]> {
    return this.client.get<any, Agent[]>('/agents')
  }

  async getAgent(id: string): Promise<Agent> {
    return this.client.get<any, Agent>(`/agents/${id}`)
  }

  // ========== 计算实例 ==========
  async getComputeInstances(params?: {
    region?: string
    zone?: string
    type?: string
  }): Promise<PaginatedResponse<ComputeInstance>> {
    return this.client.get<any, PaginatedResponse<ComputeInstance>>('/resources/compute-instances', {
      params,
    })
  }

  async getComputeInstance(id: string): Promise<ComputeInstance> {
    return this.client.get<any, ComputeInstance>(`/resources/compute-instances/${id}`)
  }

  // ========== 域名 ==========
  async getDomains(): Promise<PaginatedResponse<Domain>> {
    return this.client.get<any, PaginatedResponse<Domain>>('/resources/domains')
  }

  async getDomain(id: string): Promise<Domain> {
    return this.client.get<any, Domain>(`/resources/domains/${id}`)
  }

  // ========== 证书 ==========
  async getCertificates(): Promise<PaginatedResponse<Certificate>> {
    return this.client.get<any, PaginatedResponse<Certificate>>('/resources/certificates')
  }

  async getCertificate(id: string): Promise<Certificate> {
    return this.client.get<any, Certificate>(`/resources/certificates/${id}`)
  }

  // ========== 服务 ==========
  async getServices(): Promise<PaginatedResponse<Service>> {
    return this.client.get<any, PaginatedResponse<Service>>('/resources/services')
  }

  async getService(id: string): Promise<Service> {
    return this.client.get<any, Service>(`/resources/services/${id}`)
  }

  // ========== Gateway ==========
  async getGateways(): Promise<PaginatedResponse<Gateway>> {
    return this.client.get<any, PaginatedResponse<Gateway>>('/resources/gateways')
  }

  async getGateway(id: string): Promise<Gateway> {
    return this.client.get<any, Gateway>(`/resources/gateways/${id}`)
  }

  // ========== 存储 ==========
  async getStorages(): Promise<PaginatedResponse<Storage>> {
    return this.client.get<any, PaginatedResponse<Storage>>('/resources/storages')
  }

  async getStorage(id: string): Promise<Storage> {
    return this.client.get<any, Storage>(`/resources/storages/${id}`)
  }

  // ========== 系统状态 ==========
  async getStatus(): Promise<{
    services: { running: number; down: number; unknown: number }
    domains: { valid: number; expiring: number }
    certificates: { valid: number; expiring: number }
    infrastructure: { total: number; online: number }
  }> {
    return this.client.get<any, any>('/status')
  }

  // ========== 警告/通知 ==========
  async getAlerts(): Promise<{ data: Alert[] }> {
    return this.client.get<any, any>('/alerts')
  }

  async markAlertRead(id: string): Promise<void> {
    return this.client.put(`/alerts/${id}/read`)
  }

  async markAllAlertsRead(): Promise<void> {
    return this.client.put('/alerts/read-all')
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
