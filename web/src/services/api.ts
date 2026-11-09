import axios, { type AxiosInstance } from 'axios'
import { logger } from '@/utils/logger'
import type { ApiCtx } from './api/core'
import { acmeMethods, type AcmeMethods } from './api/acme'
import { agentMethods, type AgentMethods } from './api/agent'
import { auditMethods, type AuditMethods } from './api/audit'
import { authMethods, type AuthMethods } from './api/auth'
import { backupMethods, type BackupMethods } from './api/backup'
import { cloudMethods, type CloudMethods } from './api/cloud'
import { cronMethods, type CronMethods } from './api/cron'
import { ddnsMethods, type DdnsMethods } from './api/ddns'
import { diskMethods, type DiskMethods } from './api/disk'
import { dnsMethods, type DnsMethods } from './api/dns'
import { domainBindingMethods, type DomainBindingMethods } from './api/domain-binding'
import { driftMethods, type DriftMethods } from './api/drift'
import { fileMethods, type FileMethods } from './api/file'
import { firewallMethods, type FirewallMethods } from './api/firewall'
import { healthMethods, type HealthMethods } from './api/health'
import { logsMethods, type LogsMethods } from './api/logs'
import { nasMethods, type NasMethods } from './api/nas'
import { notificationMethods, type NotificationMethods } from './api/notification'
import { oobMethods, type OobMethods } from './api/oob'
import { overlayMethods, type OverlayMethods } from './api/overlay'
import { probeMethods, type ProbeMethods } from './api/probe'
import { proxyMethods, type ProxyMethods } from './api/proxy'
import { recordingMethods, type RecordingMethods } from './api/recording'
import { resourceMethods, type ResourceMethods } from './api/resource'
import { serviceMethods, type ServiceMethods } from './api/service'
import { stackMethods, type StackMethods } from './api/stack'
import { statusMethods, type StatusMethods } from './api/status'
import { usersMethods, type UsersMethods } from './api/users'

// ApiService：核心持有共享 axios 实例（拦截器/构造），域方法按资源边界拆分在
// ./api/<domain>.ts，经 Object.assign 挂回原型；接口合并让消费端类型完整。
export class ApiService implements ApiCtx {
  client: AxiosInstance

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
}

export interface ApiService extends AcmeMethods, AgentMethods, AuditMethods, AuthMethods, BackupMethods, CloudMethods, CronMethods, DdnsMethods, DiskMethods, DnsMethods, DomainBindingMethods, DriftMethods, FileMethods, FirewallMethods, HealthMethods, LogsMethods, NasMethods, NotificationMethods, OobMethods, OverlayMethods, ProbeMethods, ProxyMethods, RecordingMethods, ResourceMethods, ServiceMethods, StackMethods, StatusMethods, UsersMethods {}

Object.assign(ApiService.prototype, acmeMethods, agentMethods, auditMethods, authMethods, backupMethods, cloudMethods, cronMethods, ddnsMethods, diskMethods, dnsMethods, domainBindingMethods, driftMethods, fileMethods, firewallMethods, healthMethods, logsMethods, nasMethods, notificationMethods, oobMethods, overlayMethods, probeMethods, proxyMethods, recordingMethods, resourceMethods, serviceMethods, stackMethods, statusMethods, usersMethods)

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
