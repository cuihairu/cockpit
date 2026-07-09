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
  PaginatedResponse,
  TOTPGenerateResponse,
  TOTPVerifyResponse,
  LoginResponse,
  UserInfo,
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
