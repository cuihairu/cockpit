import type { AuditLogsPage, AuditLogStats } from '@/types'
import type { UISettings } from '@/contexts/settingsTypes'
import type { ApiCtx } from './core'

// audit 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface AuditMethods {
    getAuditLogs(filters: Record<string, string | undefined>, page: number, pageSize: number): Promise<AuditLogsPage>
    getAuditStats(): Promise<AuditLogStats>
    exportAuditLogs(filters: Record<string, string | undefined>): Promise<Blob>
    updateProfile(data: { email?: string; phone?: string; department?: string }): Promise<{
    message: string
    id: string
    username: string
    email?: string
    phone?: string
    department?: string
  }>
    saveSettings(data: Partial<UISettings>): Promise<{ message: string }>
    changePassword(currentPassword: string, newPassword: string): Promise<{ message: string }>
}

export const auditMethods: AuditMethods = {
  async getAuditLogs(this: ApiCtx, filters: Record<string, string | undefined>, page: number, pageSize: number): Promise<AuditLogsPage> {

    const params: Record<string, string> = { page: String(page), page_size: String(pageSize) }
    Object.entries(filters).forEach(([k, v]) => {
      if (v) params[k] = v
    })
    return this.client.get<unknown, AuditLogsPage>('/admin/audit/logs', { params })
  
  },

  async getAuditStats(this: ApiCtx): Promise<AuditLogStats> {

    return this.client.get<unknown, AuditLogStats>('/admin/audit/stats')
  
  },

  async exportAuditLogs(this: ApiCtx, filters: Record<string, string | undefined>): Promise<Blob> {

    const params: Record<string, string> = {}
    Object.entries(filters).forEach(([k, v]) => {
      if (v) params[k] = v
    })
    return this.client.get<unknown, Blob>('/admin/audit/export', {
      params,
      responseType: 'blob',
    })
  
  },

  async updateProfile(this: ApiCtx, data: { email?: string; phone?: string; department?: string }): Promise<{
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
  
  },

  async saveSettings(this: ApiCtx, data: Partial<UISettings>): Promise<{ message: string }> {

    return this.client.put<unknown, { message: string }>('/settings', data)
  
  },

  async changePassword(this: ApiCtx, currentPassword: string, newPassword: string): Promise<{ message: string }> {

    return this.client.put<unknown, { message: string }>('/me/password', {
      currentPassword,
      newPassword,
    })
  
  }
}
