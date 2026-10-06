import { Dropdown, Grid } from 'antd'
import {
  AppstoreOutlined,
  ContainerOutlined,
  DashboardOutlined,
  DesktopOutlined,
  SettingOutlined,
} from '@ant-design/icons'
import type { ReactNode } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'

import './index.less'

// 结构性子集：与 App.tsx 的 PermRouteItem（filterMenuRoutes 产物）同构，
// 组件不感知 perm 语义——传入前已裁剪。
export interface MobileNavRoute {
  path: string
  name?: string
  routes?: MobileNavRoute[]
}

interface TabDef {
  path: string
  label: string
  icon: ReactNode
}

// 固定四槽按移动场景频率挑选（总览看状态、主机/容器做关键操作、设置改连接），
// 其余目的地收敛进「更多」；可见性随传入路由树裁剪（无权限的槽自动隐藏），
// 与 mobile-design.md「手机看、桌面改」定位一致。
const FIXED_TABS: TabDef[] = [
  { path: '/', label: '总览', icon: <DashboardOutlined /> },
  { path: '/agents', label: '主机', icon: <DesktopOutlined /> },
  { path: '/docker', label: '容器', icon: <ContainerOutlined /> },
  { path: '/settings', label: '设置', icon: <SettingOutlined /> },
]

// 更多面板：树展开为叶子，父级名做前缀消歧（「域名」与「域名绑定」等同名面）
const flattenLeaves = (routes: MobileNavRoute[], parent?: string): { path: string; label: string }[] =>
  routes.flatMap((r) => {
    if (r.routes?.length) {
      return flattenLeaves(r.routes, r.name)
    }
    if (!r.name) return []
    return [{ path: r.path, label: parent ? `${parent} · ${r.name}` : r.name }]
  })

const routeInTree = (routes: MobileNavRoute[], path: string): boolean =>
  routes.some((r) => r.path === path || (r.routes?.length ? routeInTree(r.routes, path) : false))

// 底部导航（<768px）：fixed 吸底 + safe-area 适配；桌面（≥md）不渲染。
export const MobileTabBar = ({ routes }: { routes: MobileNavRoute[] }) => {
  const location = useLocation()
  const navigate = useNavigate()
  const screens = Grid.useBreakpoint()
  // 断点未确定（首次渲染 {}）时不渲染——桌面端避免首帧闪现，移动端一帧后再现
  if (screens.md !== false) {
    return null
  }

  const visibleTabs = FIXED_TABS.filter((tab) => routeInTree(routes, tab.path))
  const moreLeaves = flattenLeaves(routes).filter((leaf) => !FIXED_TABS.some((t) => t.path === leaf.path))

  const isActive = (path: string) => (path === '/' ? location.pathname === '/' : location.pathname.startsWith(path))

  const items = moreLeaves.map((leaf) => ({
    key: leaf.path,
    label: leaf.label,
    onClick: () => navigate(leaf.path),
  }))

  return (
    <nav className="mobile-tabbar" aria-label="移动端主导航">
      {visibleTabs.map((tab) => (
        <a
          key={tab.path}
          href={tab.path}
          className={`mobile-tabbar-item${isActive(tab.path) ? ' active' : ''}`}
          onClick={(e) => {
            e.preventDefault()
            navigate(tab.path)
          }}
        >
          {tab.icon}
          <span>{tab.label}</span>
        </a>
      ))}
      <Dropdown
        menu={{
          items,
          selectable: false,
          style: { maxHeight: '60vh', overflowY: 'auto' },
        }}
        placement="topRight"
        trigger={['click']}
      >
        <a href="#" className="mobile-tabbar-item" onClick={(e) => e.preventDefault()}>
          <AppstoreOutlined />
          <span>更多</span>
        </a>
      </Dropdown>
    </nav>
  )
}
