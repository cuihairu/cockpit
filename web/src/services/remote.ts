import axios from 'axios'
import { logger } from '@/utils/logger'

export type RemoteProtocol = 'ssh' | 'rdp' | 'vnc' | 'telnet'

export interface RemoteTicketRequest {
  agentId: string
  host: string
  port: number
  protocol: RemoteProtocol
  username?: string
  password?: string
  /** SSH PEM 私钥（优先于 password） */
  privateKey?: string
  domain?: string
  width?: number
  height?: number
  /** 用保险箱已存凭据连接（服务端注入缺失项，浏览器零凭据） */
  useSaved?: boolean
}

export interface RemoteTicket {
  ticket: string
  expiresAt: string
}

export interface RemoteSession {
  id: string
  agentId: string
  userId: string
  username: string
  protocol: RemoteProtocol
  host: string
  port: number
  status: 'pending' | 'connected' | 'closed' | 'failed'
  error?: string
  createdAt: string
  updatedAt: string
  closedAt?: string
}

export interface CreateRemoteSessionRequest {
  agentId: string
  protocol: RemoteProtocol
  host: string
  port: number
}

const remoteClient = axios.create({
  baseURL: '/api/remote',
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json',
  },
})

remoteClient.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => Promise.reject(error)
)

remoteClient.interceptors.response.use(
  (response) => response.data,
  (error) => {
    // /vault/ 的 401 = 二次验证缺失/过期（重验即可），不是登录会话过期，
    // 不能重定向登录页
    const isVaultPath = typeof error.config?.url === 'string' && error.config.url.includes('/vault/')
    if (error.response?.status === 401 && !isVaultPath) {
      localStorage.removeItem('token')
      localStorage.removeItem('username')
      window.location.href = '/login'
    }
    logger.error('Remote API Error:', error)
    return Promise.reject(error)
  }
)

export async function createRemoteTicket(params: RemoteTicketRequest): Promise<RemoteTicket> {
  const response = await remoteClient.post<unknown, { ticket: string; expires_at: string }>('/tickets', {
    agent_id: params.agentId,
    host: params.host,
    port: params.port,
    protocol: params.protocol,
    username: params.username,
    password: params.password,
    private_key: params.privateKey,
    domain: params.domain,
    width: params.width,
    height: params.height,
    use_saved: params.useSaved,
  })

  return {
    ticket: response.ticket,
    expiresAt: response.expires_at,
  }
}

export async function getRemoteSessions(): Promise<RemoteSession[]> {
  const response = await remoteClient.get<unknown, { data: RemoteSession[] }>('/sessions')
  return response.data
}

export async function createRemoteSession(data: CreateRemoteSessionRequest): Promise<RemoteSession> {
  return remoteClient.post<unknown, RemoteSession>('/sessions', data)
}

export async function getRemoteSession(id: string): Promise<RemoteSession> {
  return remoteClient.get<unknown, RemoteSession>(`/sessions/${id}`)
}

export async function deleteRemoteSession(id: string): Promise<void> {
  return remoteClient.delete(`/sessions/${id}`)
}

// ============ 凭据保险箱（阿里云 Workbench 密码箱模式） ============
// 明文密码/私钥只在保存请求里上行一次，之后连接走 use_saved 由服务端注入；
// 列表只回元数据。管理操作（保存/删除）需二次验证签发的 vault token。

export interface VaultCredentialMeta {
  id: string
  agentId: string
  host: string
  port: number
  protocol: RemoteProtocol
  username: string
  domain?: string
  hasPassword: boolean
  hasPrivateKey: boolean
  updatedAt: string
}

export interface SaveVaultCredentialParams {
  agentId: string
  host: string
  port: number
  protocol: RemoteProtocol
  username?: string
  password?: string
  privateKey?: string
  domain?: string
}

// vault token 只存模块内存：刷新/关页即失效，二次验证随之重来
let vaultTokenValue = ''

export function hasVaultToken(): boolean {
  return vaultTokenValue !== ''
}

function vaultHeaders(): Record<string, string> {
  return vaultTokenValue ? { 'X-Vault-Token': vaultTokenValue } : {}
}

/** 二次验证（登录密码或 TOTP 动态码），通过后自动记录 vault token */
export async function verifyVault(params: {
  password?: string
  totpCode?: string
}): Promise<{ token: string; expiresAt: string }> {
  const resp = await remoteClient.post<unknown, { token: string; expires_at: string }>(
    '/vault/verify',
    { password: params.password, totp_code: params.totpCode },
  )
  vaultTokenValue = resp.token
  return { token: resp.token, expiresAt: resp.expires_at }
}

/** 已存凭据元数据列表（无明文；弹窗打开时探测「一键连接」可用性） */
export async function listVaultCredentials(): Promise<VaultCredentialMeta[]> {
  const resp = await remoteClient.get<unknown, { data: VaultCredentialMeta[] }>('/vault/credentials')
  return resp.data ?? []
}

/** 保存/更新凭据（需先 verifyVault；同目标覆盖） */
export async function saveVaultCredential(params: SaveVaultCredentialParams): Promise<void> {
  await remoteClient.put(
    '/vault/credentials',
    {
      agent_id: params.agentId,
      host: params.host,
      port: params.port,
      protocol: params.protocol,
      username: params.username,
      password: params.password,
      private_key: params.privateKey,
      domain: params.domain,
    },
    { headers: vaultHeaders() },
  )
}

/** 删除一条已存凭据（需先 verifyVault） */
export async function deleteVaultCredential(id: string): Promise<void> {
  await remoteClient.delete(`/vault/credentials/${id}`, { headers: vaultHeaders() })
}
