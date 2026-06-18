import {
  CheckCircleOutlined,
  CloseCircleOutlined,
  CloudServerOutlined,
  DesktopOutlined,
  ExclamationCircleOutlined,
} from '@ant-design/icons'
import type React from 'react'
import type { Agent } from '@/types'

// 虚拟化类型显示配置
export const virtTypeConfig: Record<
  string,
  { label: string; icon: React.ReactNode; color: string }
> = {
  kvm: { label: 'KVM', icon: <CloudServerOutlined />, color: 'blue' },
  vmware: { label: 'VMware', icon: <CloudServerOutlined />, color: 'green' },
  qemu: { label: 'QEMU', icon: <CloudServerOutlined />, color: 'cyan' },
  xen: { label: 'Xen', icon: <CloudServerOutlined />, color: 'purple' },
  virtualbox: { label: 'VirtualBox', icon: <CloudServerOutlined />, color: 'orange' },
  hyperv: { label: 'Hyper-V', icon: <CloudServerOutlined />, color: 'blue' },
  docker: { label: 'Docker', icon: <CloudServerOutlined />, color: 'blue' },
  lxc: { label: 'LXC', icon: <CloudServerOutlined />, color: 'geekblue' },
  container: { label: '容器', icon: <CloudServerOutlined />, color: 'magenta' },
  none: { label: '物理机', icon: <DesktopOutlined />, color: 'default' },
}

// 获取虚拟化显示信息
export const getVirtDisplay = (agent: Agent) => {
  if (agent.virtRole === 'host' || agent.virtType === 'none') {
    return virtTypeConfig.none
  }
  if (agent.virtType && virtTypeConfig[agent.virtType]) {
    return virtTypeConfig[agent.virtType]
  }
  return { label: '未知', icon: <ExclamationCircleOutlined />, color: 'default' }
}

export interface RemoteService {
  protocol: 'ssh' | 'rdp' | 'vnc' | 'telnet' | 'ftp'
  host: string
  port: number
  name: string
  running: boolean
}

// 从 Agent 的 capabilities 中提取远程服务
export const getRemoteServices = (agent: Agent | null): RemoteService[] => {
  if (!agent || !agent.capabilities) return []
  const remoteCap = agent.capabilities.find((cap) => cap.type === 'remote-services')
  if (!remoteCap || !remoteCap.metadata) return []

  const services: RemoteService[] = []
  for (const [key, value] of Object.entries(remoteCap.metadata)) {
    if (typeof value === 'object' && value !== null && 'running' in value) {
      const service = value as { host: string; port: number; name: string; running: boolean }
      if (service.running && ['ssh', 'rdp', 'vnc', 'telnet', 'ftp'].includes(key)) {
        services.push({
          protocol: key as RemoteService['protocol'],
          host: service.host || '127.0.0.1',
          port: service.port,
          name: service.name || `${key.toUpperCase()} Server`,
          running: true,
        })
      }
    }
  }
  return services
}

// 状态显示配置
export const statusConfig: Record<
  string,
  { icon: React.ReactNode; color: string; text: string }
> = {
  online: { icon: <CheckCircleOutlined />, color: 'success', text: '在线' },
  offline: { icon: <CloseCircleOutlined />, color: 'default', text: '离线' },
  error: { icon: <ExclamationCircleOutlined />, color: 'error', text: '异常' },
}
