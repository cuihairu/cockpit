import { useState, useEffect } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import ProLayout from '@ant-design/pro-layout'
import { Button, Dropdown } from 'antd'
import {
  DashboardOutlined,
  CloudServerOutlined,
  ApiOutlined,
  SettingOutlined,
  UserOutlined,
  LogoutOutlined,
} from '@ant-design/icons'
import Dashboard from './pages/Dashboard'
import Resources from './pages/Resources'
import Agents from './pages/Agents'
import Settings from './pages/Settings'
import Login from './pages/Login'
import logo from '@/assets/logo.svg'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
})

// 受保护的路由组件
const ProtectedRoute = ({ children }: { children: React.ReactNode }) => {
  const token = localStorage.getItem('token')
  if (!token) {
    return <Navigate to="/login" replace />
  }
  return <>{children}</>
}

// 主布局组件
const MainLayout = () => {
  const [pathname, setPathname] = useState(window.location.pathname)

  const handleLogout = () => {
    localStorage.removeItem('token')
    localStorage.removeItem('username')
    window.location.href = '/login'
  }

  const menuItems = [
    {
      path: '/',
      name: '仪表盘',
      icon: <DashboardOutlined />,
    },
    {
      path: '/resources',
      name: '资源管理',
      icon: <CloudServerOutlined />,
    },
    {
      path: '/agents',
      name: 'Agent 管理',
      icon: <ApiOutlined />,
    },
    {
      path: '/settings',
      name: '设置',
      icon: <SettingOutlined />,
    },
  ]

  return (
    <ProLayout
      title="Cockpit"
      logo={logo}
      layout="mix"
      splitMenus={true}
      navTheme="light"
      headerTheme="light"
      contentWidth="Fluid"
      location={{ pathname }}
      route={{
        path: '/',
        routes: menuItems,
      }}
      menuItemRender={(menuItemProps, defaultDom) => {
        return (
          <a
            href={menuItemProps.path}
            onClick={(e) => {
              e.preventDefault()
              setPathname(menuItemProps.path || '/')
              window.history.pushState({}, '', menuItemProps.path)
            }}
          >
            {defaultDom}
          </a>
        )
      }}
      avatarProps={{
        icon: <UserOutlined />,
        title: localStorage.getItem('username') || 'Admin',
        render: (props, dom) => (
          <Dropdown
            menu={{
              items: [
                {
                  key: 'logout',
                  icon: <LogoutOutlined />,
                  label: '退出登录',
                  onClick: handleLogout,
                },
              ],
            }}
          >
            <div style={{ cursor: 'pointer' }}>{dom}</div>
          </Dropdown>
        ),
      }}
    >
      <Routes>
        <Route path="/" element={<Dashboard />} />
        <Route path="/resources" element={<Resources />} />
        <Route path="/agents" element={<Agents />} />
        <Route path="/settings" element={<Settings />} />
      </Routes>
    </ProLayout>
  )
}

const App = () => {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route
            path="/*"
            element={
              <ProtectedRoute>
                <MainLayout />
              </ProtectedRoute>
            }
          />
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  )
}

export default App
