import type { RemoteProtocol } from '@/services/remote'

export type WorkbenchTab = 'overview' | 'ssh' | 'rdp' | 'vnc' | 'files' | 'logs'

export type RemoteService = {
  protocol: RemoteProtocol
  host: string
  port: number
  name: string
  running: boolean
  /** SSH 服务器支持的认证方式（如 ["publickey","password"]；缺省视为都支持） */
  authMethods?: string[]
}

export type SessionConfig = {
  agentId: string
  host: string
  port: number
  protocol: RemoteProtocol
  title: string
  authMethods?: string[]
}
