import React, { useCallback, useEffect, useRef, useState } from 'react'
import { Modal, Form, Input, message, Alert, Button, Space } from 'antd'
import { SoundOutlined, AudioMutedOutlined } from '@ant-design/icons'
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

// SSH 尺寸同步的防抖（见下方「SSH 终端尺寸同步」effect）：拖窗口/切全屏会
// 连着一串 resize，去抖后只发最后一条 size，避免 guacd 反复换算列/行
const RESIZE_DEBOUNCE_MS = 150

type GuacState = 'disconnected' | 'connecting' | 'connected'

interface GuacamoleModalProps {
  visible: boolean
  onClose: () => void
  agentId: string
  host: string
  port: number
  protocol: 'rdp' | 'vnc' | 'ssh'
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
  // 静音（M4 D2）：AudioContext 单例 suspend/resume 全局静音音频流
  const [muted, setMuted] = useState(false)
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

  // 注意：解构出的 clear 不能叫 clearTimeout——那会遮蔽全局 clearTimeout，
  // 本组件后面还有真实定时器（SSH 尺寸同步去抖）要用
  const { start: startTimeout, clear: clearConnTimeout } = useConnectionTimeout({
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
    async (values: {
      username?: string
      password?: string
      domain?: string
      privateKey?: string
    }) => {
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
          privateKey: values.privateKey,
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
          clearConnTimeout()
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
        clearConnTimeout()
        setState('connected')
      } catch (err) {
        clearConnTimeout()
        setState('disconnected')
        setShowCredentials(true)
        const msg = err instanceof Error ? err.message : '连接失败'
        setError(msg)
        message.error(msg)
      }
    },
    [agentId, host, port, protocol, resolution, startTimeout, clearConnTimeout],
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

  // 剪贴板反向（浏览器 → 远端）：guacd 的 ssh 插件收 clipboard 流指令，
  // 浏览器侧对应 common-js 的 createClipboardStream + StringWriter（官方写法；
  // Go 网关是字节管道，原样透传）。至此 SSH 会话剪贴板双向：
  //   - 正向：client.onclipboard（远端 → navigator.clipboard），三协议通用
  //   - 反向：本函数（浏览器 → 远端），仅 SSH 接线（见下方 effect）
  // 纯文本单层（设计「不做」：剪贴板富格式 RTF/HTML）
  const sendClipboardToRemote = useCallback((text: string) => {
    const client = clientRef.current
    if (!client) return
    if (!text) return
    const writer = new Guacamole.StringWriter(client.createClipboardStream('text/plain'))
    writer.send(text)
  }, [])

  // 工具栏「粘贴到远程」按钮（仅 SSH 渲染）：读浏览器剪贴板推给远端。
  // Clipboard API 的 readText 需要用户手势 + 授权，失败时兜底提示走终端内
  // Ctrl+V（不静默吞掉）
  const handlePasteToRemote = useCallback(() => {
    navigator.clipboard
      .readText()
      .then((text) => sendClipboardToRemote(text))
      .catch(() => message.warning('读取浏览器剪贴板失败，可在终端内按 Ctrl+V 粘贴'))
  }, [sendClipboardToRemote])

  // 静音：AudioContext 单例 suspend/resume（RawAudioPlayer 内部直连
  // context.destination，从外部插 GainNode 必须照抄其播放逻辑，属
  // 「自由发挥」禁区——见 todo.md M4 D2）
  const toggleMute = useCallback(() => {
    const ctx = Guacamole.AudioContextFactory?.getAudioContext()
    if (!ctx) return
    if (muted) {
      void ctx.resume()
      setMuted(false)
    } else {
      void ctx.suspend()
      setMuted(true)
    }
  }, [muted])

  useEffect(() => {
    const handler = () => setIsFullscreen(!!document.fullscreenElement)
    document.addEventListener('fullscreenchange', handler)
    return () => document.removeEventListener('fullscreenchange', handler)
  }, [])

  // SSH 专属的两条通道（docs/remote-access-integration-design.md 真机验收项
  // 「终端渲染正常（vim/top 全屏程序），窗口 size 变更后列/行随之变化」
  // 与「剪贴板双向」）。RDP/VNC 一律不走：那里的 size 语义是「切远端分辨率」
  // （工具栏下拉驱动，跟窗口变会变成「窗口多大桌面就多大」），剪贴板则维持
  // 既有的仅正向（client.onclipboard）——桌面路径零行为变更。
  useEffect(() => {
    if (protocol !== 'ssh' || state !== 'connected') return
    const el = displayRef.current
    if (!el) return

    // 尺寸同步：guacd 的 ssh 插件按 `size` 指令给的像素尺寸 + 默认字体度量
    // 换算终端列/行，不同步的话建连后窗口一变形，vim/top 就停在旧列行数、
    // 右侧留黑边。拖窗口/切全屏会连着一串 resize，去抖后只发最后一条。
    let timer: ReturnType<typeof setTimeout> | null = null
    const pushSize = (width: number, height: number) => {
      // 容器未布局（display:none、jsdom 无排版）时尺寸为 0，发出去会把列/行
      // 算成 0/1 反而弄坏终端——直接跳过
      if (width <= 0 || height <= 0) return
      if (timer) clearTimeout(timer)
      timer = setTimeout(() => {
        clientRef.current?.sendSize(Math.round(width), Math.round(height))
      }, RESIZE_DEBOUNCE_MS)
    }
    // 建连后先按当前窗口补一次（ResizeObserver 首次回调同值，去抖后合并成一条）
    pushSize(el.clientWidth, el.clientHeight)
    const ro = new ResizeObserver((entries) => {
      const box = entries[0].contentRect
      pushSize(box.width, box.height)
    })
    ro.observe(el)

    // 剪贴板反向：终端区域 Ctrl+V（paste 事件自带数据，不受 Clipboard API
    // 授权限制）。绑在 display 容器上而非 document，免得在凭据表单里误粘。
    const onPaste = (ev: ClipboardEvent) => {
      const text = ev.clipboardData?.getData('text/plain')
      if (text) sendClipboardToRemote(text)
    }
    el.addEventListener('paste', onPaste)

    return () => {
      if (timer) clearTimeout(timer)
      ro.disconnect()
      el.removeEventListener('paste', onPaste)
    }
  }, [protocol, state, sendClipboardToRemote])

  useEffect(() => () => cleanup(), [cleanup])

  if (showCredentials && state === 'disconnected') {
    return (
      <Modal
        title={
          title ||
          `${protocol.toUpperCase()} ${protocol === 'ssh' ? '终端' : '远程桌面'} - ${host}:${port}`
        }
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
          ) : protocol === 'ssh' ? (
            <>
              <Form.Item
                label="用户名"
                name="username"
                rules={[{ required: true, message: '请输入用户名' }]}
              >
                <Input placeholder="root" autoFocus />
              </Form.Item>
              <Form.Item label="密码" name="password">
                <Input.Password placeholder="(可选) 口令认证" />
              </Form.Item>
              <Form.Item
                label="私钥"
                name="privateKey"
                extra="PEM 私钥（优先于口令），仅经票据转发到 guacd，不落盘"
              >
                <Input.TextArea
                  rows={4}
                  placeholder="(可选) -----BEGIN OPENSSH PRIVATE KEY-----"
                  style={{ fontFamily: 'monospace' }}
                />
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
          // SSH 尺寸跟窗口走（见上方 effect），固定分辨率下拉与「粘贴到远程」
          // 按钮只对桌面协议有意义/可用
          showResolution={protocol !== 'ssh'}
          onToggleFullscreen={handleToggleFullscreen}
          onDisconnect={handleDisconnect}
          onClipboardPaste={protocol === 'ssh' ? handlePasteToRemote : undefined}
          onResolutionChange={handleResolutionChange}
          extraActions={[
            {
              key: 'mute',
              label: muted ? '取消静音' : '静音',
              icon: muted ? <AudioMutedOutlined /> : <SoundOutlined />,
              onClick: toggleMute,
            },
          ]}
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
