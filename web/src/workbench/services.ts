import type { Agent } from '@/types'
import type { RemoteService } from './types'
import type { RemoteProtocol } from '@/services/remote'

export const getRemoteServices = (agent: Agent | null): RemoteService[] => {
  if (!agent || !agent.capabilities) return []
  const remoteCap = agent.capabilities.find((cap) => cap.type === 'remote-services')
  if (!remoteCap || !remoteCap.metadata) return []

  return Object.entries(remoteCap.metadata)
    .map(([key, value]) => {
      if (typeof value !== 'object' || value === null || !['ssh', 'rdp', 'vnc', 'telnet'].includes(key)) {
        return null
      }
      const service = value as { host?: string; port?: number; name?: string; running?: boolean }
      if (!service.running || !service.port) return null
      return {
        protocol: key as RemoteProtocol,
        host: service.host || '127.0.0.1',
        port: service.port,
        name: service.name || `${key.toUpperCase()} Server`,
        running: true,
      }
    })
    .filter((service): service is RemoteService => Boolean(service))
}
