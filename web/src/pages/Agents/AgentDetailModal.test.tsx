import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AgentDetailModal } from './AgentDetailModal'
import type { Agent } from '@/types'

// AgentDetailModal：Descriptions 详情 + LabelsDisplay 分支 + 远程服务卡片透传
// （RemoteServicesCard 有独立测试，mock 成透传桩）

const onConnect = vi.fn()

vi.mock('@/components/RemoteServices', () => ({
  default: ({
    services,
    onConnect: connect,
  }: {
    services: Array<{ protocol: string; host: string; port: number }>
    onConnect: (p: string, h: string, port: number) => void
  }) => (
    <div data-testid="remote-card">
      {services.map((s) => (
        <button key={s.protocol} onClick={() => connect(s.protocol, s.host, s.port)}>
          conn-{s.protocol}
        </button>
      ))}
    </div>
  ),
}))

const agent = {
  id: 'ag-1',
  hostname: 'web-01',
  ip: '10.0.0.1',
  region: 'cn-bj', zone: 'z1',
  status: 'online',
  lastSeen: '1758000000',
  virtType: 'kvm',
  virtRole: 'guest',
  capabilities: [
    { type: 'remote-services', metadata: { ssh: { host: '10.0.0.1', port: 22, name: 'SSH', running: true } } },
    { type: 'files' },
  ],
  labels: { env: 'prod', gpu: true, roles: ['web', 'cache'] },
} as unknown as Agent

const renderModal = (over: Partial<Parameters<typeof AgentDetailModal>[0]> = {}) =>
  render(
    <AgentDetailModal
      open
      agent={agent}
      loading={false}
      onClose={vi.fn()}
      onConnect={onConnect}
      {...over}
    />,
  )

describe('AgentDetailModal', () => {
  it('agent 为 null 直接不渲染', () => {
    const { container } = renderModal({ agent: null })
    expect(container).toBeEmptyDOMElement()
  })

  it('渲染基础信息与能力 Tag', () => {
    renderModal()
    expect(screen.getByText('Agent 详情')).toBeInTheDocument()
    expect(screen.getByText('web-01')).toBeInTheDocument()
    expect(screen.getByText('10.0.0.1')).toBeInTheDocument()
    expect(screen.getByText('cn-bj')).toBeInTheDocument()
    expect(screen.getByText('z1')).toBeInTheDocument()
    expect(screen.getByText('在线')).toBeInTheDocument()
    expect(screen.getByText('KVM (虚拟机)')).toBeInTheDocument()
    expect(screen.getByText('remote-services')).toBeInTheDocument()
    expect(screen.getByText('files')).toBeInTheDocument()
  })

  it('标签三分支：字符串/布尔/数组', () => {
    renderModal()
    // Tag 内 strong 与值文本分节点，用包含匹配锚定 Tag 元素
    const tagText = (name: string) =>
      screen.getByText((_, el) => !!el?.closest('.ant-tag') && el.textContent === name)
    expect(tagText('env: prod')).toBeInTheDocument()
    expect(tagText('gpu: 是')).toBeInTheDocument()
    expect(tagText('roles: web, cache')).toBeInTheDocument()
  })

  it('远程服务透传 getRemoteServices 结果并转发 onConnect', () => {
    renderModal()
    fireEvent.click(screen.getByText('conn-ssh'))
    expect(onConnect).toHaveBeenCalledWith('ssh', '10.0.0.1', 22)
  })

  it('空标签显示占位符', () => {
    renderModal({ agent: { ...agent, labels: {} } as Agent })
    // 标签 Descriptions 行内的占位「-」（LabelsDisplay 空态）
    expect(screen.getAllByText('-').length).toBeGreaterThanOrEqual(1)
  })

  it('字段缺省侧：空 hostname/ip/region/lastSeen、宿主机角色、离线态、能力缺省', () => {
    renderModal({
      agent: {
        id: 'ag-min',
        status: 'offline',
        virtRole: 'host',
        lastSeen: '',
        capabilities: undefined,
        labels: undefined,
      } as unknown as Agent,
    })
    // hostname/ip/region/zone/lastSeen/labels 六处占位 '-'
    expect(screen.getAllByText('-')).toHaveLength(6)
    expect(screen.getByText('物理机 (宿主机)')).toBeInTheDocument()
    expect(screen.getByText('离线')).toBeInTheDocument()
    expect(screen.queryByText('remote-services')).not.toBeInTheDocument()
  })

  it('布尔标签两侧：true 显「是」success 色，false 显「否」default 色', () => {
    renderModal({ agent: { ...agent, labels: { backup: false, gpu: true } } as Agent })
    const tagText = (name: string) =>
      screen.getByText((_, el) => !!el?.closest('.ant-tag') && el.textContent === name)
    expect(tagText('backup: 否')).toBeInTheDocument()
    expect(tagText('backup: 否')).toHaveClass('ant-tag-default')
    expect(tagText('gpu: 是')).toBeInTheDocument()
    expect(tagText('gpu: 是')).toHaveClass('ant-tag-success')
  })
})
