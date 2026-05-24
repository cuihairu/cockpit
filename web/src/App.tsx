import { useState, useEffect } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import ProLayout from '@ant-design/pro-layout'
import { Button, Dropdown, Avatar, Space, Input, Badge, Card, Row, Col, Statistic } from 'antd'
import {
  DashboardOutlined,
  CloudServerOutlined,
  ApiOutlined,
  SettingOutlined,
  UserOutlined,
  LogoutOutlined,
  QuestionCircleOutlined,
  GithubOutlined,
  SearchOutlined,
  BellOutlined,
  AppstoreOutlined,
} from '@ant-design/icons'
import Dashboard from './pages/Dashboard'
import Resources from './pages/Resources'
import Agents from './pages/Agents'
import Settings from './pages/Settings'
import Login from './pages/Login'
import NotificationDropdown from './components/Notifications'
import { UserProvider, useUser } from './contexts/UserContext'
import logo from '@/assets/logo.svg'
import './App.less'

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
  const { token } = useUser()
  if (!token) {
    return <Navigate to="/login" replace />
  }
  return <>{children}</>
}

// 主布局组件
const MainLayout = () => {
  const [pathname, setPathname] = useState(window.location.pathname)
  const [settings, setSetting] = useState<{
    fixSiderbar: boolean
    layout: 'side' | 'top' | 'mix'
    theme: 'light' | 'dark'
    colorWeak: boolean
  }>({
    fixSiderbar: true,
    layout: 'side',
    theme: 'light',
    colorWeak: false,
  })
  const { user, logout } = useUser()

  const handleLogout = () => {
    logout()
    window.location.href = '/login'
  }

  const menuData = [
    {
      path: '/',
      name: '总览',
      icon: <DashboardOutlined />,
    },
    {
      path: '/resources',
      name: '资源管理',
      icon: <AppstoreOutlined />,
      children: [
        {
          path: '/resources/compute',
          name: '计算实例',
        },
        {
          path: '/resources/domains',
          name: '域名',
        },
        {
          path: '/resources/certificates',
          name: '证书',
        },
        {
          path: '/resources/services',
          name: '服务',
        },
        {
          path: '/resources/gateways',
          name: '网关',
        },
        {
          path: '/resources/storages',
          name: '存储',
        },
      ],
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

  // 用户菜单
  const userMenuItems = [
    {
      key: 'profile',
      icon: <UserOutlined />,
      label: '个人中心',
    },
    {
      key: 'settings',
      icon: <SettingOutlined />,
      label: '设置',
    },
    {
      type: 'divider' as const,
    },
    {
      key: 'logout',
      icon: <LogoutOutlined />,
      label: '退出登录',
      onClick: handleLogout,
    },
  ]

  return (
    <ProLayout
      {...settings}
      title=""
      logo={null}
      navTheme="light"
      headerTheme="light"
      contentWidth="Fluid"
      location={{ pathname }}
      route={{
        path: '/',
        routes: menuData,
      }}
      // 顶部 Header 内容
      headerContentRender={() => (
        <div style={{ display: 'flex', alignItems: 'center', gap: 24 }}>
          {/* Logo */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <img src={logo} alt="Cockpit" style={{ width: 32, height: 32 }} />
            <span style={{ fontSize: 18, fontWeight: 600, color: '#1D2129' }}>Cockpit</span>
          </div>
          {/* 搜索框 */}
          <Input.Search
            placeholder="搜索资源、文档..."
            style={{ width: 400 }}
            size="middle"
            bordered={false}
            onSearch={(value) => console.log('Search:', value)}
          />
        </div>
      )}
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
      // 右侧操作区域 - 用户头像和通知
      actionsRender={() => [
        <Button
          key="docs"
          type="text"
          icon={<QuestionCircleOutlined />}
          style={{ color: '#86909C' }}
          onClick={() => window.open('https://github.com/cuihairu/cockpit', '_blank')}
        >
          文档
        </Button>,
        <NotificationDropdown key="notifications" />,
        // 用户头像
        <Dropdown key="user" menu={{ items: userMenuItems }} placement="bottomRight">
          <Space style={{ cursor: 'pointer', padding: '4px 12px', borderRadius: '8px', transition: 'all 0.3s' }}
            className="user-dropdown">
            <Avatar size="small" icon={<UserOutlined />} />
            <span style={{ color: '#1D2129' }}>{user?.username || 'Admin'}</span>
          </Space>
        </Dropdown>,
      ]}
    >
      <Routes>
        <Route path="/" element={<Dashboard />} />
        <Route path="/resources" element={<Resources />} />
        <Route path="/resources/*" element={<Resources />} />
        <Route path="/agents" element={<Agents />} />
        <Route path="/settings" element={<Settings />} />
      </Routes>
    </ProLayout>
  )
}

const App = () => {
  return (
    <QueryClientProvider client={queryClient}>
      <UserProvider>
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
      </UserProvider>
    </QueryClientProvider>
  )
}

export default App
