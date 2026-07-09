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
  domain?: string
  width?: number
  height?: number
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
    if (error.response?.status === 401) {
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
    domain: params.domain,
    width: params.width,
    height: params.height,
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
