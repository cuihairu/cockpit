import { render, screen, fireEvent } from '@testing-library/react'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { describe, expect, it, vi, beforeEach } from 'vitest'

// Grid.useBreakpoint 由测试控制（同 Monitor/index.test.tsx 范式），
// 覆盖窄屏渲染 / 宽屏不渲染两分支；全局 setup 的 matchMedia 恒 false。
const breakpoint = vi.hoisted(() => ({ current: {} as Record<string, boolean> }))
vi.mock('antd', async (importOriginal) => {
  const antd = await importOriginal<typeof import('antd')>()
  return {
    ...antd,
    Grid: {
      ...antd.Grid,
      useBreakpoint: () => breakpoint.current,
    },
  }
})

import { MobileTabBar } from './index'

const FULL_ROUTES = [
  { path: '/', name: '总览' },
  {
    path: '/resources',
    name: '资源管理',
    routes: [{ path: '/resources/compute', name: '计算实例' }],
  },
  { path: '/agents', name: '主机' },
  { path: '/docker', name: '容器管理' },
  { path: '/settings', name: '设置' },
  { path: '/proxy', name: '反向代理' },
  { path: '/profile', name: '个人中心' },
]

const renderBar = (routes = FULL_ROUTES, pathname = '/agents') =>
  render(
    <MemoryRouter initialEntries={[pathname]}>
      <MobileTabBar routes={routes} />
    </MemoryRouter>
  )

describe('MobileTabBar', () => {
  beforeEach(() => {
    breakpoint.current = { md: false }
  })

  it('桌面（≥md）不渲染', () => {
    breakpoint.current = { md: true }
    const { container } = renderBar()
    expect(container.querySelector('.mobile-tabbar')).toBeNull()
  })

  it('窄屏渲染固定槽与「更多」', () => {
    renderBar()
    for (const label of ['总览', '主机', '容器', '设置', '更多']) {
      expect(screen.getByText(label)).toBeInTheDocument()
    }
  })

  it('无权限的槽随路由树裁剪（无 docker 权限时不渲染容器槽）', () => {
    renderBar(FULL_ROUTES.filter((r) => r.path !== '/docker'))
    expect(screen.queryByText('容器')).not.toBeInTheDocument()
    expect(screen.getByText('总览')).toBeInTheDocument()
  })

  it('当前路径对应槽带 active 态（/agents 前缀匹配）', () => {
    const { container } = renderBar(FULL_ROUTES, '/agents')
    const active = container.querySelector('.mobile-tabbar-item.active')
    expect(active).not.toBeNull()
    expect(active?.textContent).toContain('主机')
  })

  it('总览槽仅在根路径 active（「/」前缀不吞掉其余路由）', () => {
    const { container } = renderBar(FULL_ROUTES, '/agents')
    const overview = Array.from(container.querySelectorAll('.mobile-tabbar-item')).find((el) =>
      el.textContent?.includes('总览')
    )
    expect(overview?.className).not.toContain('active')
  })

  it('点击槽导航到对应路径', () => {
    const LocationProbe = () => {
      const loc = useLocation()
      return <div data-testid="loc">{loc.pathname}</div>
    }
    render(
      <MemoryRouter initialEntries={['/']}>
        <MobileTabBar routes={FULL_ROUTES} />
        <LocationProbe />
      </MemoryRouter>
    )
    fireEvent.click(screen.getByText('容器'))
    expect(screen.getByTestId('loc').textContent).toBe('/docker')
  })

  it('「更多」收敛其余目的地（固定四槽不在列、父级名做前缀）', async () => {
    renderBar()
    fireEvent.click(screen.getByText('更多'))
    // 资源管理子页带父级前缀消歧
    expect(await screen.findByText('资源管理 · 计算实例')).toBeInTheDocument()
    // 其余顶级目的地平铺出现
    expect(screen.queryByText('反向代理')).toBeInTheDocument()
    expect(screen.queryByText('个人中心')).toBeInTheDocument()
    for (const label of ['总览', '主机', '容器', '设置']) {
      expect(screen.queryAllByText(label).length).toBe(1)
    }
  })
})
