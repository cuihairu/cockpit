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

// 统一管理 Terminal/Guacamole 两类 modal 的开关状态与目标信息
// （TerminalModal：telnet + SSH 兜底入口；GuacamoleModal：rdp/vnc/ssh）
export const useRemoteModals = () => {
  const [terminalVisible, setTerminalVisible] = useState(false)
  const [terminalConfig, setTerminalConfig] = useState<RemoteTarget | null>(null)
  // RDP/VNC/SSH 走 Guacamole 网关（guacd + guacamole-common-js；
  // docs/remote-access-integration-design.md D3——telnet 维持 agent 通道）
  const [guacVisible, setGuacVisible] = useState(false)
  const [guacConfig, setGuacConfig] = useState<
    (RemoteTarget & { protocol: 'rdp' | 'vnc' | 'ssh' }) | null
  >(null)

  // 根据协议分流打开对应的 modal
  const open = (protocol: RemoteProtocol, agentId: string, host: string, port: number) => {
    if (protocol === 'rdp' || protocol === 'vnc' || protocol === 'ssh') {
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
