import { useState } from 'react'
import type { RemoteProtocol } from '@/services/remote'

// 远程连接目标信息
export interface RemoteTarget {
  agentId: string
  host: string
  port: number
  title: string
  protocol?: RemoteProtocol
}

// 统一管理 Terminal/Desktop/VNC 三个 modal 的开关状态与目标信息
export const useRemoteModals = () => {
  const [terminalVisible, setTerminalVisible] = useState(false)
  const [terminalConfig, setTerminalConfig] = useState<RemoteTarget | null>(null)
  // RDP/VNC 走 Guacamole 网关（guacd + guacamole-common-js）
  const [guacVisible, setGuacVisible] = useState(false)
  const [guacConfig, setGuacConfig] = useState<
    (RemoteTarget & { protocol: 'rdp' | 'vnc' }) | null
  >(null)

  // 根据协议分流打开对应的 modal
  const open = (protocol: RemoteProtocol, agentId: string, host: string, port: number) => {
    if (protocol === 'rdp' || protocol === 'vnc') {
      setGuacConfig({
        agentId,
        host,
        port,
        protocol,
        title: `${protocol.toUpperCase()} - ${host}:${port}`,
      })
      setGuacVisible(true)
    } else {
      setTerminalConfig({
        agentId,
        host,
        port,
        protocol,
        title: `${protocol.toUpperCase()} - ${host}:${port}`,
      })
      setTerminalVisible(true)
    }
  }

  return {
    terminalVisible,
    terminalConfig,
    guacVisible,
    guacConfig,
    setTerminalVisible,
    setGuacVisible,
    open,
  }
}
