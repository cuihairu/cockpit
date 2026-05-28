import type { RemoteProtocol } from '@/services/remote'

export type WorkbenchTab = 'overview' | 'ssh' | 'rdp' | 'vnc'

export type RemoteService = {
  protocol: RemoteProtocol
  host: string
  port: number
  name: string
  running: boolean
}

export type SessionConfig = {
  agentId: string
  host: string
  port: number
  protocol: RemoteProtocol
  title: string
}
