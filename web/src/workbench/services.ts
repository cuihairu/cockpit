import type { Agent } from '@/types'
import type { RemoteService } from './types'
import type { RemoteProtocol } from '@/services/remote'

export const getRemoteServices = (agent: Agent | null): RemoteService[] => {
  if (!agent) return []
  // 优先服务端持久化的服务面（agent 能力探测 + 心跳刷新口径一致）。
  // 老 agent/旧构建没有 services 字段时回退能力元数据（同一探测，
  // 只是承载格式不同——见 remote.go Detect/DetectServices 注释）。
  if (agent.services && agent.services.length > 0) {
    return agent.services
      .filter((svc) => svc.running && svc.port && ['ssh', 'rdp', 'vnc', 'telnet'].includes(svc.protocol))
      .map((svc) => ({
        protocol: svc.protocol as RemoteProtocol,
        host: svc.host || '127.0.0.1',
        port: svc.port,
        name: svc.name || `${svc.protocol.toUpperCase()} Server`,
        running: true,
        authMethods: svc.authMethods,
      }))
  }
  if (!agent.capabilities) return []
  const remoteCap = agent.capabilities.find((cap) => cap.type === 'remote-services')
  if (!remoteCap || !remoteCap.metadata) return []

  return Object.entries(remoteCap.metadata)
    .map(([key, value]): RemoteService | null => {
      if (typeof value !== 'object' || value === null || !['ssh', 'rdp', 'vnc', 'telnet'].includes(key)) {
        return null
      }
      const service = value as { host?: string; port?: number; name?: string; running?: boolean; authMethods?: string[] }
      if (!service.running || !service.port) return null
      return {
        protocol: key as RemoteProtocol,
        host: service.host || '127.0.0.1',
        port: service.port,
        name: service.name || `${key.toUpperCase()} Server`,
        running: true,
        authMethods: service.authMethods,
      }
    })
    .filter((service): service is RemoteService => Boolean(service))
}
