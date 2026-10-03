import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { message as antdMessage } from 'antd'
import TerminalModal from './index'

// TerminalModal：xterm + WebSocket 远程终端——票据/消息分支/输入转发/清理

const terminalMock = vi.hoisted(() => ({
  loadAddon: vi.fn(),
  open: vi.fn(),
  writeln: vi.fn(),
  write: vi.fn(),
  onData: vi.fn(),
  reset: vi.fn(),
  dispose: vi.fn(),
  rows: 24,
  cols: 80,
}))
const fitMock = vi.hoisted(() => ({ fit: vi.fn() }))
// new Terminal() 须返回 mock 对象：普通 function 构造器显式 return
vi.mock('@xterm/xterm', () => ({ Terminal: vi.fn(function () { return terminalMock }) }))
vi.mock('@xterm/addon-fit', () => ({ FitAddon: vi.fn(function () { return fitMock }) }))
vi.mock('@xterm/addon-web-links', () => ({ WebLinksAddon: vi.fn(function () { return {} }) }))

const createRemoteTicket = vi.hoisted(() => vi.fn())
const vaultMocks = vi.hoisted(() => ({
  listVaultCredentials: vi.fn().mockResolvedValue([]),
  saveVaultCredential: vi.fn().mockResolvedValue(undefined),
  deleteVaultCredential: vi.fn().mockResolvedValue(undefined),
  verifyVault: vi.fn().mockResolvedValue({ token: 'vt-1', expiresAt: '' }),
  hasVaultToken: vi.fn(() => false),
}))
vi.mock('@/services/remote', () => ({ createRemoteTicket, ...vaultMocks }))

// 终端弹窗的 message 提示经 spy 断言：antd message 渲染有 3s 逗留，
// DOM 查询跨用例易串扰；clearAllMocks 会一并清掉调用记录
const msgSuccess = vi.spyOn(antdMessage, 'success')
const msgWarning = vi.spyOn(antdMessage, 'warning')
const msgError = vi.spyOn(antdMessage, 'error')

class FakeWebSocket {
  static CONNECTING = 0
  static OPEN = 1
  static instances: FakeWebSocket[] = []
  readyState = FakeWebSocket.CONNECTING
  sent: string[] = []
  onopen: (() => void) | null = null
  onmessage: ((e: { data: string }) => void) | null = null
  onerror: (() => void) | null = null
  onclose: (() => void) | null = null
  constructor(
    public url: string,
    public protocols?: string[] | string,
  ) {
    FakeWebSocket.instances.push(this)
  }
  send(data: string) {
    this.sent.push(data)
  }
  close = vi.fn(() => {
    this.readyState = 3
  })
}

const flush = () => act(async () => {
  await new Promise((r) => setTimeout(r, 0))
})

describe('TerminalModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    FakeWebSocket.instances = []
    createRemoteTicket.mockReset()
    createRemoteTicket.mockResolvedValue({ ticket: 'tk1', expiresAt: '' })
    vi.stubGlobal('WebSocket', FakeWebSocket)
    vi.stubGlobal('ResizeObserver', class {
      observe() {}
      unobserve() {}
      disconnect() {}
    })
  })

  const props = {
    visible: true,
    onClose: vi.fn(),
    agentId: 'ag1',
    host: '10.0.0.1',
    port: 22,
    protocol: 'ssh' as const,
    // SSH 需要认证凭据；传入 username 跳过凭据表单直接连接
    username: 'testuser',
    password: 'testpass',
  }

  // ==== 凭据保险箱 ====

  const savedCred = {
    id: 'c1', agentId: 'ag1', host: '10.0.0.1', port: 22, protocol: 'ssh' as const,
    username: 'ops', hasPassword: true, hasPrivateKey: false, updatedAt: '',
  }

  it('保险箱有已存凭据：出「使用已保存凭据连接」面板，点击走 use_saved 零凭据', async () => {
    vaultMocks.listVaultCredentials.mockResolvedValueOnce([savedCred])
    // 不传 username → 走凭据表单/保险箱分支
    const { username: _u, password: _p, ...noCredProps } = props
    render(<TerminalModal {...noCredProps} />)
    const btn = await screen.findByRole('button', { name: /使用已保存凭据连接（ops）/ })
    fireEvent.click(btn)
    await waitFor(() =>
      expect(createRemoteTicket).toHaveBeenCalledWith(
        expect.objectContaining({ agentId: 'ag1', host: '10.0.0.1', port: 22, protocol: 'ssh', useSaved: true }),
      ),
    )
  })

  it('已存凭据态可切「手动输入」，勾选保存提交时先落保险箱再连接', async () => {
    vaultMocks.listVaultCredentials.mockResolvedValueOnce([savedCred])
    vaultMocks.hasVaultToken.mockReturnValue(true)
    const { username: _u2, password: _p2, ...noCredProps2 } = props
    render(<TerminalModal {...noCredProps2} />)
    await screen.findByRole('button', { name: /使用已保存凭据连接/ })
    fireEvent.click(screen.getByRole('button', { name: '手动输入' }))
    fireEvent.change(screen.getByPlaceholderText('root'), { target: { value: 'ops' } })
    fireEvent.change(screen.getByPlaceholderText('登录口令'), { target: { value: 'pw' } })
    fireEvent.click(screen.getByText(/保存凭据到服务器/))
    fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
    await waitFor(() => expect(vaultMocks.saveVaultCredential).toHaveBeenCalled())
    expect(vaultMocks.saveVaultCredential).toHaveBeenCalledWith(
      expect.objectContaining({ agentId: 'ag1', host: '10.0.0.1', port: 22, username: 'ops', password: 'pw' }),
    )
  })

  it('打开即建终端、输出欢迎语并凭票据连 WS', async () => {
    render(<TerminalModal {...props} />)
    await flush()
    expect(terminalMock.open).toHaveBeenCalled()
    expect(terminalMock.writeln).toHaveBeenCalledWith(expect.stringContaining('Cockpit 远程终端'))
    expect(createRemoteTicket).toHaveBeenCalledWith({
      agentId: 'ag1', host: '10.0.0.1', port: 22, protocol: 'ssh',
      username: 'testuser', password: 'testpass',
    })
    const ws = FakeWebSocket.instances[0]
    expect(ws.url).toBe('ws://localhost:3000/api/remote/terminal')
    expect(ws.protocols).toEqual(['tk1'])
  })

  it('票据失败输出错误并保持未连接', async () => {
    createRemoteTicket.mockRejectedValue(new Error('no'))
    render(<TerminalModal {...props} />)
    await flush()
    expect(terminalMock.writeln).toHaveBeenCalledWith(expect.stringContaining('创建连接票据失败'))
    expect(FakeWebSocket.instances).toHaveLength(0)
    expect(screen.getByText(/重\s*连/)).toBeInTheDocument()
  })

  it('onopen 置已连接；data 消息写入终端', async () => {
    render(<TerminalModal {...props} />)
    await flush()
    const ws = FakeWebSocket.instances[0]
    await act(async () => {
      ws.readyState = FakeWebSocket.OPEN
      ws.onopen!()
    })
    expect(terminalMock.writeln).toHaveBeenCalledWith(expect.stringContaining('连接成功'))
    expect(screen.getByText(/已\s*连接/).closest('button')!.disabled).toBe(true)

    terminalMock.write.mockClear()
    await act(async () => {
      ws.onmessage!({ data: JSON.stringify({ type: 'data', data: 'hello' }) })
      ws.onmessage!({ data: JSON.stringify({ type: 'resize' }) })
      ws.onmessage!({ data: JSON.stringify({ type: 'error', message: 'boom' }) })
      ws.onmessage!({ data: '{broken' })
    })
    expect(terminalMock.write).toHaveBeenCalledWith('hello')
    expect(fitMock.fit).toHaveBeenCalled()
    expect(terminalMock.writeln).toHaveBeenCalledWith(expect.stringContaining('错误: boom'))
    expect(terminalMock.writeln).toHaveBeenCalledWith(expect.stringContaining('消息解析错误'))
  })

  it('close 消息置断开；终端输入在 OPEN 时经 WS 发送', async () => {
    render(<TerminalModal {...props} />)
    await flush()
    const ws = FakeWebSocket.instances[0]
    await act(async () => {
      ws.readyState = FakeWebSocket.OPEN
      ws.onopen!()
      ws.onmessage!({ data: JSON.stringify({ type: 'close' }) })
    })
    expect(screen.getByText(/重\s*连/)).toBeInTheDocument()

    const onDataCb = terminalMock.onData.mock.calls[0][0] as (d: string) => void
    await act(async () => {
      onDataCb('ls\n')
    })
    expect(ws.sent).toContain(JSON.stringify({ type: 'input', data: 'ls\n' }))
  })

  it('卸载清理：关 WS、dispose 终端', async () => {
    const { unmount } = render(<TerminalModal {...props} />)
    await flush()
    const ws = FakeWebSocket.instances[0]
    unmount()
    expect(ws.close).toHaveBeenCalled()
    expect(terminalMock.dispose).toHaveBeenCalled()
  })

  it('「重连」重置终端并再次取票据', async () => {
    createRemoteTicket.mockRejectedValueOnce(new Error('first fails'))
    render(<TerminalModal {...props} />)
    await flush()
    fireEvent.click(screen.getByText(/重\s*连/))
    await flush()
    expect(terminalMock.reset).toHaveBeenCalled()
    expect(terminalMock.writeln).toHaveBeenCalledWith(expect.stringContaining('正在重连'))
    expect(createRemoteTicket).toHaveBeenCalledTimes(2)
  })

  it('ws.onerror 置断开并输出连接错误', async () => {
    render(<TerminalModal {...props} />)
    await flush()
    const ws = FakeWebSocket.instances[0]
    await act(async () => {
      ws.readyState = FakeWebSocket.OPEN
      ws.onopen!()
      ws.onerror!()
    })
    expect(terminalMock.writeln).toHaveBeenCalledWith(expect.stringContaining('连接错误'))
    expect(screen.getByText(/重\s*连/)).toBeInTheDocument()
  })

  it('ws.onclose 置断开（不覆盖文案）', async () => {
    render(<TerminalModal {...props} />)
    await flush()
    const ws = FakeWebSocket.instances[0]
    await act(async () => {
      ws.readyState = FakeWebSocket.OPEN
      ws.onopen!()
      ws.onclose!()
    })
    expect(screen.getByText(/重\s*连/)).toBeInTheDocument()
  })

  it('连接超时：CONNECTING 状态下 30s 后关 WS 并提示', async () => {
    vi.useFakeTimers()
    try {
      render(<TerminalModal {...props} />)
      await act(async () => {
        await vi.advanceTimersByTimeAsync(30_000)
      })
      const ws = FakeWebSocket.instances[0]
      expect(ws.close).toHaveBeenCalled()
      expect(terminalMock.writeln).toHaveBeenCalledWith(expect.stringContaining('连接超时'))
    } finally {
      vi.useRealTimers()
    }
  })

  it('visible=false 时清理 WS（effect cleanup）', async () => {
    const { rerender } = render(<TerminalModal {...props} />)
    await flush()
    const ws = FakeWebSocket.instances[0]
    await act(async () => {
      ws.readyState = FakeWebSocket.OPEN
      ws.onopen!()
    })
    expect(screen.getByText(/已\s*连接/)).toBeInTheDocument()
    rerender(<TerminalModal {...props} visible={false} />)
    // 第一个 useEffect 的 cleanup 负责关 WS + dispose 终端
    expect(ws.close).toHaveBeenCalled()
    expect(terminalMock.dispose).toHaveBeenCalled()
  })

  it('终端输入在非 OPEN 状态不发送', async () => {
    render(<TerminalModal {...props} />)
    await flush()
    const ws = FakeWebSocket.instances[0]
    const onDataCb = terminalMock.onData.mock.calls[0][0] as (d: string) => void
    await act(async () => {
      onDataCb('ls\n') // CONNECTING 状态
    })
    expect(ws.sent).toHaveLength(0)
  })

  // ============ SSH 凭据表单 ============

  it('SSH 无 username：先出凭据表单，提交后才建终端', async () => {
    render(<TerminalModal {...props} username={undefined} password={undefined} />)
    // 表单阶段不建终端、不取票据
    expect(terminalMock.open).not.toHaveBeenCalled()
    expect(createRemoteTicket).not.toHaveBeenCalled()
    expect(screen.getByText('用户名')).toBeInTheDocument()
    expect(screen.getByText('口令')).toBeInTheDocument()

    fireEvent.change(screen.getByPlaceholderText('root'), { target: { value: 'alice' } })
    fireEvent.change(screen.getByPlaceholderText('登录口令'), { target: { value: 'pw' } })
    fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
    // antd Form onFinish 异步 → effect 建终端，等重渲染
    await waitFor(() => expect(terminalMock.open).toHaveBeenCalled())
    expect(createRemoteTicket).toHaveBeenCalledWith({
      agentId: 'ag1', host: '10.0.0.1', port: 22, protocol: 'ssh',
      username: 'alice', password: 'pw',
    })
  })

  it('凭据表单空用户名拦截，不建终端', async () => {
    render(<TerminalModal {...props} username={undefined} password={undefined} />)
    fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
    expect(await screen.findByText('请输入用户名')).toBeInTheDocument()
    expect(terminalMock.open).not.toHaveBeenCalled()
    expect(createRemoteTicket).not.toHaveBeenCalled()
  })

  it('telnet 免认证：无 username 也直接建终端（不进凭据表单）', async () => {
    render(<TerminalModal {...props} protocol="telnet" username={undefined} password={undefined} />)
    await flush()
    expect(screen.queryByText('用户名')).not.toBeInTheDocument()
    expect(terminalMock.open).toHaveBeenCalled()
    expect(createRemoteTicket).toHaveBeenCalledWith({
      agentId: 'ag1', host: '10.0.0.1', port: 22, protocol: 'telnet',
    })
  })

  // ============ PTY 尺寸上报 ============

  it('窗口变化发 resize（rows/cols）到服务端', async () => {
    // ResizeObserver 手工驱动，验证 fit + resize 上报
    let roCb: ResizeObserverCallback | null = null
    vi.stubGlobal('ResizeObserver', class {
      constructor(cb: ResizeObserverCallback) { roCb = cb }
      observe() {}
      unobserve() {}
      disconnect() {}
    })

    render(<TerminalModal {...props} />)
    await flush()
    const ws = FakeWebSocket.instances[0]
    await act(async () => {
      ws.readyState = FakeWebSocket.OPEN
      ws.onopen!()
    })
    ws.sent.length = 0

    await act(async () => {
      roCb?.([], {} as ResizeObserver)
    })
    expect(fitMock.fit).toHaveBeenCalled()
    expect(ws.sent).toContain(JSON.stringify({ type: 'resize', rows: 24, cols: 80 }))
  })

  it('https 页面下终端 WS 用 wss', async () => {
    vi.stubGlobal('location', { protocol: 'https:', host: 'localhost:3000' })
    try {
      render(<TerminalModal {...props} />)
      await flush()
      expect(FakeWebSocket.instances[0].url.startsWith('wss://')).toBe(true)
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('WS 停在 CONNECTING 30s：超时关闭并写提示（fake timers 推进）', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      render(<TerminalModal {...props} />)
      await flush()
      // FakeWebSocket 默认 readyState=CONNECTING：不触发 onopen，等超时到点
      await act(async () => {
        await vi.advanceTimersByTimeAsync(30000)
      })
      expect(FakeWebSocket.instances[0].close).toHaveBeenCalled()
      expect(terminalMock.writeln).toHaveBeenCalledWith(
        expect.stringContaining('连接超时，请检查网络或重试'))
    } finally {
      vi.useRealTimers()
    }
  })

  it('WS 未建立时窗口变化只本地 fit 不上报', async () => {
    let roCb: ResizeObserverCallback | null = null
    vi.stubGlobal('ResizeObserver', class {
      constructor(cb: ResizeObserverCallback) { roCb = cb }
      observe() {}
      unobserve() {}
      disconnect() {}
    })

    render(<TerminalModal {...props} />)
    // ticket 未决窗口：wsRef 尚空，ResizeObserver 已可触发
    await act(async () => {
      roCb?.([], {} as ResizeObserver)
    })
    expect(fitMock.fit).toHaveBeenCalled()
    expect(FakeWebSocket.instances[0]?.sent ?? []).toHaveLength(0)
  })

  // ==== SSH 认证方式形态（authMethods 三态驱动面板三分支）====

  it('仅密钥认证（publickey only）：密钥面板出用户名表单，按钮与表单提交双通道建终端', async () => {
    const { username: _u, password: _p, ...noCred } = props
    const first = render(<TerminalModal {...noCred} authMethods={['publickey']} />)
    expect(screen.getByText('SSH 密钥认证')).toBeInTheDocument()
    expect(
      screen.getByText('该服务器仅支持密钥认证，将使用 Agent 默认密钥连接。请输入用户名。'),
    ).toBeInTheDocument()
    // 通道一：连接按钮——用户名并入票据（paramsRef 随 credentials 同步）
    fireEvent.change(screen.getByPlaceholderText('root'), { target: { value: 'bob' } })
    fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
    await waitFor(() => expect(terminalMock.open).toHaveBeenCalled())
    expect(createRemoteTicket).toHaveBeenCalledWith(
      expect.objectContaining({ agentId: 'ag1', host: '10.0.0.1', port: 22, protocol: 'ssh', username: 'bob' }),
    )
    first.unmount()

    // 通道二：表单 onFinish（回车提交）——空用户名走 root 兜底
    terminalMock.open.mockClear()
    createRemoteTicket.mockClear()
    render(<TerminalModal {...noCred} authMethods={['publickey']} />)
    fireEvent.submit(screen.getByPlaceholderText('root').closest('form')!)
    await waitFor(() => expect(terminalMock.open).toHaveBeenCalled())
    expect(createRemoteTicket).toHaveBeenCalledWith(
      expect.objectContaining({ protocol: 'ssh', username: 'root' }),
    )

    // 通道三：连接按钮空用户名——按钮内联 onClick 的 || 'root' 兜底侧
    terminalMock.open.mockClear()
    createRemoteTicket.mockClear()
    const third = render(<TerminalModal {...noCred} authMethods={['publickey']} />)
    fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
    await waitFor(() => expect(terminalMock.open).toHaveBeenCalled())
    expect(createRemoteTicket).toHaveBeenCalledWith(
      expect.objectContaining({ protocol: 'ssh', username: 'root' }),
    )
    third.unmount()
  })

  it('受限认证方式（无密码无密钥）：警示面板与关闭按钮', () => {
    const { username: _u, password: _p, ...noCred } = props
    render(<TerminalModal {...noCred} authMethods={['keyboard-interactive']} />)
    expect(screen.getByText('SSH 服务器认证方式受限')).toBeInTheDocument()
    expect(screen.getByText(/该 SSH 服务器支持的认证方式：keyboard-interactive/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /关\s*闭/ }))
    expect(props.onClose).toHaveBeenCalled()
  })

  it('勾选保存但无 vault token：先二次验证，通过后保存凭据并建终端', async () => {
    vaultMocks.hasVaultToken.mockReturnValue(false)
    const { username: _u, password: _p, ...noCred } = props
    render(<TerminalModal {...noCred} />)
    fireEvent.change(screen.getByPlaceholderText('root'), { target: { value: 'ops' } })
    fireEvent.change(screen.getByPlaceholderText('登录口令'), { target: { value: 'pw' } })
    fireEvent.click(screen.getByText(/保存凭据到服务器/))
    fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
    // 无 token：暂存动作并弹验证框，不保存不建终端
    expect(await screen.findByText('验证以保存凭据')).toBeInTheDocument()
    expect(vaultMocks.saveVaultCredential).not.toHaveBeenCalled()
    fireEvent.change(screen.getByPlaceholderText('登录密码'), { target: { value: 'admin-pw' } })
    fireEvent.click(screen.getByRole('button', { name: /验\s*证/ }))
    // 验证通过 → handleVaultVerified 续做 save-connect → proceedWithAuth
    await waitFor(() =>
      expect(vaultMocks.saveVaultCredential).toHaveBeenCalledWith(
        expect.objectContaining({ agentId: 'ag1', host: '10.0.0.1', port: 22, username: 'ops', password: 'pw' }),
      ),
    )
    await waitFor(() => expect(terminalMock.open).toHaveBeenCalled())
  })

  it('保存保险箱失败：仅警告不阻断连接', async () => {
    vaultMocks.hasVaultToken.mockReturnValue(true)
    vaultMocks.saveVaultCredential.mockRejectedValueOnce(new Error('down'))
    const { username: _u, password: _p, ...noCred } = props
    render(<TerminalModal {...noCred} />)
    fireEvent.change(screen.getByPlaceholderText('root'), { target: { value: 'ops' } })
    fireEvent.change(screen.getByPlaceholderText('登录口令'), { target: { value: 'pw' } })
    fireEvent.click(screen.getByText(/保存凭据到服务器/))
    fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
    await waitFor(() => expect(msgWarning).toHaveBeenCalledWith('凭据保存失败（不影响本次连接）'))
    await waitFor(() => expect(terminalMock.open).toHaveBeenCalled())
  })

  it('删除已存凭据：先验证（可取消再重开），通过后删除并回手输表单', async () => {
    vaultMocks.hasVaultToken.mockReturnValue(false)
    vaultMocks.listVaultCredentials.mockResolvedValueOnce([savedCred])
    const { username: _u, password: _p, ...noCred } = props
    render(<TerminalModal {...noCred} />)
    fireEvent.click(await screen.findByRole('button', { name: '删除已存' }))
    expect(await screen.findByText('验证以删除已存凭据')).toBeInTheDocument()
    // 已存面板态无凭据表单，「取消」按钮唯一。antd Modal 离场动画在 jsdom
    // 不推进（transitionend 缺失），消失断言不可达——改以行为断言：取消不
    // 验证、不删除
    fireEvent.click(screen.getByRole('button', { name: /取\s*消/ }))
    expect(vaultMocks.verifyVault).not.toHaveBeenCalled()
    expect(vaultMocks.deleteVaultCredential).not.toHaveBeenCalled()
    // 重开验证并提交通过 → doDeleteSaved 成功 → 切回手输表单
    fireEvent.click(screen.getByRole('button', { name: '删除已存' }))
    await screen.findByText('验证以删除已存凭据')
    fireEvent.change(screen.getByPlaceholderText('登录密码'), { target: { value: 'admin-pw' } })
    fireEvent.click(screen.getByRole('button', { name: /验\s*证/ }))
    await waitFor(() => expect(vaultMocks.deleteVaultCredential).toHaveBeenCalledWith('c1'))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已删除保存的凭据'))
    await waitFor(() => expect(screen.getByPlaceholderText('root')).toBeInTheDocument())
    expect(screen.queryByRole('button', { name: '删除已存' })).not.toBeInTheDocument()
  })

  it('已有 vault token：删除直连不弹验证；失败仅提示且面板保留', async () => {
    vaultMocks.hasVaultToken.mockReturnValue(true)
    vaultMocks.listVaultCredentials.mockResolvedValueOnce([savedCred])
    vaultMocks.deleteVaultCredential.mockRejectedValueOnce(new Error('down'))
    const { username: _u, password: _p, ...noCred } = props
    render(<TerminalModal {...noCred} />)
    fireEvent.click(await screen.findByRole('button', { name: '删除已存' }))
    await waitFor(() => expect(vaultMocks.deleteVaultCredential).toHaveBeenCalledWith('c1'))
    expect(screen.queryByText('验证以删除已存凭据')).not.toBeInTheDocument()
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('删除失败，请重试'))
    expect(await screen.findByRole('button', { name: '删除已存' })).toBeInTheDocument()
  })

  it('凭据表单经 onFinish 提交（与按钮 onClick 双通道）', async () => {
    const { username: _u, password: _p, ...noCred } = props
    render(<TerminalModal {...noCred} />)
    fireEvent.change(screen.getByPlaceholderText('root'), { target: { value: 'alice' } })
    fireEvent.change(screen.getByPlaceholderText('登录口令'), { target: { value: 'pw' } })
    fireEvent.submit(screen.getByPlaceholderText('root').closest('form')!)
    await waitFor(() => expect(terminalMock.open).toHaveBeenCalled())
  })

  it('超时回调触发时连接已非 CONNECTING（关而未开）：不重复关 WS 不误报', async () => {
    vi.useFakeTimers()
    try {
      render(<TerminalModal {...props} />)
      // 推进 0ms：票据 promise 落定、WS 建立（不触发 onopen，超时定时器在册）
      await act(async () => {
        await vi.advanceTimersByTimeAsync(0)
      })
      const ws = FakeWebSocket.instances[0]
      ws.close() // 服务端提前断开：readyState 置 CLOSED，定时器未清除
      terminalMock.writeln.mockClear()
      await act(async () => {
        await vi.advanceTimersByTimeAsync(30_000)
      })
      expect(ws.close).toHaveBeenCalledTimes(1) // 只有手动那次，超时分支未追加
      expect(terminalMock.writeln).not.toHaveBeenCalledWith(expect.stringContaining('连接超时'))
    } finally {
      vi.useRealTimers()
    }
  })
})
