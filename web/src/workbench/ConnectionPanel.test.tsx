import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ConnectionPanel from './ConnectionPanel'
import OverviewPanel from './OverviewPanel'
import { UserProvider } from '@/contexts/UserContext'
import type { RemoteService } from './types'
import type { Agent } from '@/types'

// ConnectionPanel：协议连接面板（PermGuard 裁剪）；OverviewPanel：概览描述

const apiMock = vi.hoisted(() => ({
  login: vi.fn(),
  getCurrentUser: vi.fn(),
  logout: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

const grant = (permissions: string[]) => {
  localStorage.setItem('token', 'tk')
  localStorage.setItem('username', 'admin')
  apiMock.getCurrentUser.mockResolvedValue({
    id: 'u1', username: 'admin', role: 'admin', permissions, totp_enabled: false,
  })
}

describe('ConnectionPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
  })

  const service: RemoteService = {
    protocol: 'ssh', host: '10.0.0.1', port: 22, name: 'sshd', running: true,
  }

  it('无 service 提示未检测到', () => {
    render(<ConnectionPanel protocol="ssh" onConnect={vi.fn()} />)
    expect(screen.getByText('当前服务器未检测到 SSH 服务')).toBeInTheDocument()
  })

  it('有 service 渲染主机/端口/服务名；有 terminal:write 显示连接按钮', async () => {
    grant(['terminal:write'])
    render(
      <UserProvider>
        <ConnectionPanel protocol="rdp" service={service} onConnect={vi.fn()} />
      </UserProvider>,
    )
    expect(screen.getByText('RDP 连接')).toBeInTheDocument()
    expect(screen.getByText('10.0.0.1')).toBeInTheDocument()
    expect(screen.getByText('22')).toBeInTheDocument()
    expect(screen.getByText('sshd')).toBeInTheDocument()
    expect(await screen.findByText('打开 RDP')).toBeInTheDocument()
  })

  it('无 terminal:write 不渲染连接按钮', async () => {
    grant([])
    render(
      <UserProvider>
        <ConnectionPanel protocol="vnc" service={service} onConnect={vi.fn()} />
      </UserProvider>,
    )
    await waitForUser()
    expect(screen.queryByText('打开 VNC')).toBeNull()
  })

  it('传 onFallback 渲染兜底按钮并回调；缺省不渲染', async () => {
    const onFallback = vi.fn()
    grant(['terminal:write'])
    const { rerender } = render(
      <UserProvider>
        <ConnectionPanel
          protocol="ssh"
          service={service}
          onConnect={vi.fn()}
          onFallback={onFallback}
          fallbackLabel="内置终端（经 Agent）"
        />
      </UserProvider>,
    )
    const btn = await screen.findByText('内置终端（经 Agent）')
    btn.click()
    expect(onFallback).toHaveBeenCalledOnce()
    // 缺省 onFallback：不渲染兜底按钮（RDP/VNC 面板形态不变）
    rerender(
      <UserProvider>
        <ConnectionPanel protocol="ssh" service={service} onConnect={vi.fn()} />
      </UserProvider>,
    )
    expect(screen.queryByText('内置终端（经 Agent）')).toBeNull()
    expect(screen.queryByText('备选入口')).toBeNull()

    // 有 onFallback 但缺省 fallbackLabel：落兜底文案
    rerender(
      <UserProvider>
        <ConnectionPanel protocol="ssh" service={service} onConnect={vi.fn()} onFallback={onFallback} />
      </UserProvider>,
    )
    expect(screen.getByText('备选入口')).toBeInTheDocument()
  })
})

async function waitForUser() {
  await new Promise((r) => setTimeout(r, 0))
}

describe('OverviewPanel', () => {
  it('无 agent 提示选择服务器', () => {
    render(<OverviewPanel agent={null} />)
    expect(screen.getByText('请选择左侧服务器')).toBeInTheDocument()
  })

  it('渲染 agent 概览与能力 tags', () => {
    const agent: Agent = {
      id: 'ag1', hostname: 'web-01', ip: '10.0.0.1', status: 'online', lastSeen: '',
      region: 'cn', zone: 'z',
      capabilities: [{ type: 'shell' }, { type: 'metrics' }] as Agent['capabilities'],
    }
    render(<OverviewPanel agent={agent} />)
    expect(screen.getByText('web-01')).toBeInTheDocument()
    expect(screen.getByText('ag1', { selector: 'code' })).toBeInTheDocument()
    expect(screen.getByText('在线')).toBeInTheDocument()
    expect(screen.getByText('shell')).toBeInTheDocument()
    expect(screen.getByText('metrics')).toBeInTheDocument()
  })

  it('空能力不渲染 tags；离线显示离线', () => {
    const agent: Agent = {
      id: 'ag2', hostname: '', ip: '', status: 'offline', lastSeen: '',
      region: '', zone: '', capabilities: [],
    }
    render(<OverviewPanel agent={agent} />)
    expect(screen.getByText('离线')).toBeInTheDocument()
    expect(document.querySelectorAll('.ant-tag').length).toBe(1) // 仅状态 tag
  })

  it('capabilities 字段缺失（undefined）走 || [] 兜底：不崩且无能力 tag', () => {
    const agent = {
      id: 'ag3', hostname: 'bare', ip: '10.0.0.3', status: 'online', lastSeen: '',
      region: 'cn', zone: 'z',
    } as Agent
    render(<OverviewPanel agent={agent} />)
    expect(screen.getByText('bare')).toBeInTheDocument()
    expect(screen.getByText('在线')).toBeInTheDocument()
    // 仅状态一枚 tag，能力区空
    expect(document.querySelectorAll('.ant-tag').length).toBe(1)
  })
})
