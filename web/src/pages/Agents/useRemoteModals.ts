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
  const [desktopVisible, setDesktopVisible] = useState(false)
  const [desktopConfig, setDesktopConfig] = useState<RemoteTarget | null>(null)
  const [vncVisible, setVncVisible] = useState(false)
  const [vncConfig, setVncConfig] = useState<RemoteTarget | null>(null)

  // 根据协议分流打开对应的 modal
  const open = (protocol: RemoteProtocol, agentId: string, host: string, port: number) => {
    if (protocol === 'rdp') {
      setDesktopConfig({ agentId, host, port, title: `RDP - ${host}:${port}` })
      setDesktopVisible(true)
    } else if (protocol === 'vnc') {
      setVncConfig({ agentId, host, port, title: `VNC - ${host}:${port}` })
      setVncVisible(true)
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
    desktopVisible,
    desktopConfig,
    vncVisible,
    vncConfig,
    setTerminalVisible,
    setDesktopVisible,
    setVncVisible,
    open,
  }
}
