import React, { useCallback, useEffect, useRef, useState } from 'react'
import { Modal, Form, Input, message, Alert, Button, Space } from 'antd'
import RemoteToolbar, { type ConnectionState } from '../RemoteToolbar'
import Guacamole from 'guacamole-common-js'
import { useConnectionTimeout } from '@/hooks/useConnectionTimeout'
import { createRemoteTicket, type RemoteProtocol } from '@/services/remote'
import { getRecentDesktopConfig, saveDesktopConfig } from '@/services/desktop'

// Guacamole 桌面 Modal（docs/remote-desktop-guacamole-design.md 阶段1 第 3 块）
// guacamole-common-js 承担渲染/输入/剪贴板/保活（ping），本组件只负责
// 「申请票据 → 建隧道 → 生命周期」。凭据由服务端放进 connect 指令（ticket
// 携带），浏览器侧零凭据；隧道内部控制指令由 Go 网关终结（见 D3）。

const CONNECTION_TIMEOUT = 30000 // 30 秒未连上报错

type GuacState = 'disconnected' | 'connecting' | 'connected'

interface GuacamoleModalProps {
  visible: boolean
  onClose: () => void
  agentId: string
  host: string
  port: number
  protocol: 'rdp' | 'vnc'
  title?: string
}

const GuacamoleModal: React.FC<GuacamoleModalProps> = ({
  visible,
  onClose,
  agentId,
  host,
  port,
  protocol,
  title,
}) => {
  const [showCredentials, setShowCredentials] = useState(true)
  const [state, setState] = useState<GuacState>('disconnected')
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [resolution, setResolution] = useState('1280x800')
  const [error, setError] = useState<string | null>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const displayRef = useRef<HTMLDivElement>(null)
  const clientRef = useRef<InstanceType<typeof Guacamole.Client> | null>(null)
  const [form] = Form.useForm()

  const cleanup = useCallback(() => {
    if (clientRef.current) {
      clientRef.current.disconnect()
      clientRef.current = null
    }
    // common-js 的 Display 会挂 canvas，安全清空容器
    if (displayRef.current) {
      while (displayRef.current.firstChild) {
        displayRef.current.removeChild(displayRef.current.firstChild)
      }
    }
  }, [])

  const { start: startTimeout, clear: clearTimeout } = useConnectionTimeout({
    timeout: CONNECTION_TIMEOUT,
    onTimeout: () => {
      cleanup()
      setState('disconnected')
      setShowCredentials(true)
      message.error('连接超时，请检查网络或重试')
    },
    enabled: state === 'connecting',
  })

  // 打开时自动填充上次使用的凭据（不含密码，同 DesktopModal）
  useEffect(() => {
    if (visible) {
      const saved = getRecentDesktopConfig(agentId, host, port)
      if (saved) {
        form.setFieldsValue({
          username: saved.username,
          domain: saved.domain,
        })
      }
    }
  }, [visible, agentId, host, port, form])

  const connect = useCallback(
    async (values: { username?: string; password?: string; domain?: string }) => {
      setError(null)
      setState('connecting')
      setShowCredentials(false)
      startTimeout()
      try {
        // 凭据只进 ticket（服务端随后放进 guacd 的 connect 指令），不进 URL
        const { ticket } = await createRemoteTicket({
          agentId,
          host,
          port,
          protocol: protocol as RemoteProtocol,
          username: values.username,
          password: values.password,
          domain: values.domain,
          width: Number(resolution.split('x')[0]) || 1280,
          height: Number(resolution.split('x')[1]) || 800,
        })
        saveDesktopConfig({
          name: `${host}:${port}`,
          agentId,
          host,
          port,
          username: values.username || '',
          domain: values.domain || '',
          width: Number(resolution.split('x')[0]) || 1280,
          height: Number(resolution.split('x')[1]) || 800,
        })

        const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
        // 票据走 URL query：common-js 的 WebSocketTunnel 硬编码 subprotocol
        // "guacamole" 且 connect(data) 拼进 query（见 D3「票据双通道」）
        const tunnel = new Guacamole.WebSocketTunnel(
          `${wsProtocol}//${window.location.host}/api/remote/guacamole`,
        )
        const client = new Guacamole.Client(tunnel)
        clientRef.current = client

        client.onerror = (status) => {
          clearTimeout()
          setState('disconnected')
          setShowCredentials(true)
          setError(status?.message || '远程桌面连接失败')
          message.error(status?.message || '远程桌面连接失败')
        }
        client.onclipboard = (streams, mimetype) => {
          if (mimetype !== 'text/plain') return
          const reader = new Guacamole.StringReader(streams[0])
          let text = ''
          reader.ondata = (chunk) => {
            text += chunk
          }
          reader.onend = () => {
            navigator.clipboard.writeText(text).catch(() => {})
          }
        }

        const display = client.getDisplay()
        displayRef.current?.appendChild(display.getElement())

        // 触控 → 鼠标：Guacamole.Mouse 已处理 touch 事件（iPad Safari 验收项）
        const mouse = new Guacamole.Mouse(display.getElement())
        mouse.onmousedown = mouse.onmouseup = mouse.onmousemove = (s) => {
          client.sendMouseState(s)
        }
        const keyboard = new Guacamole.Keyboard(document)
        keyboard.onkeydown = (keysym) => client.sendKeyEvent(1, keysym)
        keyboard.onkeyup = (keysym) => client.sendKeyEvent(0, keysym)

        // connect(data) 的 data 落到 WS URL query；网关凭票据换出真凭据
        client.connect(`ticket=${encodeURIComponent(ticket)}`)
        clearTimeout()
        setState('connected')
      } catch (err) {
        clearTimeout()
        setState('disconnected')
        setShowCredentials(true)
        const msg = err instanceof Error ? err.message : '连接失败'
        setError(msg)
        message.error(msg)
      }
    },
    [agentId, host, port, protocol, resolution, startTimeout, clearTimeout],
  )

  const handleDisconnect = useCallback(() => {
    cleanup()
    setState('disconnected')
    setShowCredentials(true)
    if (isFullscreen) {
      document.exitFullscreen?.()
      setIsFullscreen(false)
    }
  }, [cleanup, isFullscreen])

  const handleClose = useCallback(() => {
    cleanup()
    setState('disconnected')
    setShowCredentials(true)
    setIsFullscreen(false)
    onClose()
  }, [cleanup, onClose])

  const handleToggleFullscreen = useCallback(() => {
    if (isFullscreen) {
      document.exitFullscreen?.()
      setIsFullscreen(false)
    } else {
      containerRef.current?.requestFullscreen?.()
      setIsFullscreen(true)
    }
  }, [isFullscreen])

  const handleResolutionChange = useCallback((width: number, height: number) => {
    clientRef.current?.sendSize(width, height)
    setResolution(`${width}x${height}`)
  }, [])

  useEffect(() => {
    const handler = () => setIsFullscreen(!!document.fullscreenElement)
    document.addEventListener('fullscreenchange', handler)
    return () => document.removeEventListener('fullscreenchange', handler)
  }, [])

  useEffect(() => () => cleanup(), [cleanup])

  if (showCredentials && state === 'disconnected') {
    return (
      <Modal
        title={title || `${protocol.toUpperCase()} 远程桌面 - ${host}:${port}`}
        open={visible}
        onCancel={handleClose}
        width={420}
        footer={null}
      >
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
          message="经 Guacamole 网关连接"
          description="口令仅经加密通道转发到 guacd 的 connect 指令，不落盘、不进日志。"
        />
        <Form
          form={form}
          layout="vertical"
          onFinish={(values) => void connect(values)}
        >
          {protocol === 'rdp' ? (
            <>
              <Form.Item
                label="用户名"
                name="username"
                rules={[{ required: true, message: '请输入用户名' }]}
              >
                <Input placeholder="administrator" autoFocus />
              </Form.Item>
              <Form.Item label="密码" name="password">
                <Input.Password placeholder="password" />
              </Form.Item>
              <Form.Item label="域" name="domain">
                <Input placeholder="(可选)" />
              </Form.Item>
            </>
          ) : (
            <Form.Item label="VNC 密码" name="password">
              <Input.Password placeholder="(可选) VNC 密码" autoFocus />
            </Form.Item>
          )}
          {error && (
            <Alert type="error" showIcon message={error} style={{ marginBottom: 16 }} />
          )}
          <Space style={{ display: 'flex', justifyContent: 'flex-end' }}>
            <Button onClick={handleClose}>取消</Button>
            <Button type="primary" htmlType="submit">
              连接
            </Button>
          </Space>
        </Form>
      </Modal>
    )
  }

  return (
    <Modal
      title={null}
      open={visible}
      onCancel={handleClose}
      footer={null}
      width={isFullscreen ? '100vw' : 1320}
      style={isFullscreen ? { top: 0, maxWidth: '100vw', paddingBottom: 0 } : undefined}
      styles={{
        body: { padding: 0, background: '#1e1e1e', height: isFullscreen ? '100vh' : 800 },
      }}
      closable={!isFullscreen}
      destroyOnClose
    >
      <div
        ref={containerRef}
        style={{ display: 'flex', flexDirection: 'column', height: '100%', background: '#000' }}
      >
        <RemoteToolbar
          state={state as ConnectionState}
          resolution={resolution}
          isFullscreen={isFullscreen}
          onToggleFullscreen={handleToggleFullscreen}
          onDisconnect={handleDisconnect}
          onResolutionChange={handleResolutionChange}
        >
          {error && <span style={{ color: '#ff4d4f' }}>{error}</span>}
        </RemoteToolbar>
        <div
          style={{
            flex: 1,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            overflow: 'hidden',
          }}
        >
          {state === 'connecting' && (
            <div style={{ color: '#888', fontSize: 16 }}>
              正在连接到 {host}:{port}...
            </div>
          )}
          <div
            ref={displayRef}
            style={{
              width: '100%',
              height: '100%',
              display: state === 'connected' ? 'block' : 'none',
            }}
          />
          {state === 'disconnected' && !showCredentials && (
            <div style={{ color: '#888', fontSize: 16 }}>连接已断开</div>
          )}
        </div>
      </div>
    </Modal>
  )
}

export default GuacamoleModal
