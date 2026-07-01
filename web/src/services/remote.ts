import axios from 'axios'
import { logger } from '@/utils/logger'

export type RemoteProtocol = 'ssh' | 'rdp' | 'vnc' | 'telnet'

export interface RemoteTicketRequest {
  agent_id: string
  host: string
  port: number
  protocol: RemoteProtocol
  username?: string
  password?: string
  domain?: string
  width?: number
  height?: number
}

export interface RemoteTicketResponse {
  ticket: string
  expires_at: string
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

export async function createRemoteTicket(data: RemoteTicketRequest): Promise<RemoteTicketResponse> {
  return remoteClient.post<unknown, RemoteTicketResponse>('/tickets', data)
}
