import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import AgentSidebar from './AgentSidebar'
import type { Agent } from '@/types'

// AgentSidebar：服务器列表——搜索/刷新/选中/在线离线标识

const agent = (overrides: Partial<Agent> = {}): Agent => ({
  id: 'ag1', hostname: 'web-01', ip: '10.0.0.1', status: 'online', lastSeen: '',
  region: 'cn-east', zone: 'a', capabilities: [],
  ...overrides,
})

describe('AgentSidebar', () => {
  it('渲染主机名/在线标识/IP/地域', () => {
    render(
      <AgentSidebar
        agents={[agent()]}
        loading={false}
        query=""
        selectedAgentId=""
        onQueryChange={vi.fn()}
        onRefresh={vi.fn()}
        onSelect={vi.fn()}
      />,
    )
    expect(screen.getByText('web-01')).toBeInTheDocument()
    expect(screen.getByText('在线')).toBeInTheDocument()
    expect(screen.getByText('10.0.0.1')).toBeInTheDocument()
    expect(screen.getByText('cn-east / a')).toBeInTheDocument()
  })

  it('离线主机显示离线；hostname 缺省用 id；ip 缺省 -', () => {
    render(
      <AgentSidebar
        agents={[agent({ hostname: '', ip: '', status: 'offline', region: '', zone: '' })]}
        loading={false}
        query=""
        selectedAgentId=""
        onQueryChange={vi.fn()}
        onRefresh={vi.fn()}
        onSelect={vi.fn()}
      />,
    )
    expect(screen.getByText('ag1')).toBeInTheDocument()
    expect(screen.getByText('离线')).toBeInTheDocument()
    expect(screen.getAllByText('-').length).toBe(1) // ip
    expect(screen.getByText('- / -')).toBeInTheDocument() // region/zone 合并文本
  })

  it('输入搜索词回调 onQueryChange；点刷新回调 onRefresh', () => {
    const onQueryChange = vi.fn()
    const onRefresh = vi.fn()
    render(
      <AgentSidebar
        agents={[]}
        loading={false}
        query=""
        selectedAgentId=""
        onQueryChange={onQueryChange}
        onRefresh={onRefresh}
        onSelect={vi.fn()}
      />,
    )
    fireEvent.change(screen.getByPlaceholderText('搜索主机名、IP、Agent ID'), {
      target: { value: 'web' },
    })
    expect(onQueryChange).toHaveBeenCalledWith('web')
    fireEvent.click(containerRefresh())
    expect(onRefresh).toHaveBeenCalledTimes(1)

    function containerRefresh() {
      return document.querySelector('.ant-card-extra')!.querySelector('[role="img"]')!
    }
  })

  it('点列表项回调 onSelect(id)；选中项高亮', () => {
    const onSelect = vi.fn()
    const { container } = render(
      <AgentSidebar
        agents={[agent(), agent({ id: 'ag2', hostname: 'db-01' })]}
        loading={false}
        query=""
        selectedAgentId="ag2"
        onQueryChange={vi.fn()}
        onRefresh={vi.fn()}
        onSelect={onSelect}
      />,
    )
    const items = Array.from(container.querySelectorAll('.ant-list-item')) as HTMLElement[]
    expect(items[1].style.background).not.toBe('transparent') // ag2 选中高亮
    expect(items[0].style.background).toBe('transparent')
    fireEvent.click(items[0])
    expect(onSelect).toHaveBeenCalledWith('ag1')
  })
})
