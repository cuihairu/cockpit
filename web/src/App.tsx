import { useState, useEffect } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import ProLayout from '@ant-design/pro-layout'
import { Button, Dropdown, Avatar, Space, Divider } from 'antd'
import {
  DashboardOutlined,
  CloudServerOutlined,
  ApiOutlined,
  SettingOutlined,
  UserOutlined,
  LogoutOutlined,
  QuestionCircleOutlined,
  GithubOutlined,
} from '@ant-design/icons'
import Dashboard from './pages/Dashboard'
import Resources from './pages/Resources'
import Agents from './pages/Agents'
import Settings from './pages/Settings'
import Login from './pages/Login'
import NotificationDropdown from './components/Notifications'
import { UserProvider, useUser } from './contexts/UserContext'
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
  const { token } = useUser()
  if (!token) {
    return <Navigate to="/login" replace />
  }
  return <>{children}</>
}

// 主布局组件
const MainLayout = () => {
  const [pathname, setPathname] = useState(window.location.pathname)
  const { user, logout } = useUser()

  const handleLogout = () => {
    logout()
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
      // 右侧操作区域
      actionsRender={() => [
        <Button
          key="docs"
          type="text"
          icon={<QuestionCircleOutlined />}
          onClick={() => window.open('https://github.com/cuihairu/cockpit', '_blank')}
        />,
        <Button
          key="github"
          type="text"
          icon={<GithubOutlined />}
          onClick={() => window.open('https://github.com/cuihairu/cockpit', '_blank')}
        />,
        <NotificationDropdown key="notifications" />,
      ]}
      // 用户头像
      avatarProps={{
        src: undefined,
        icon: <UserOutlined />,
        title: user?.username || 'Admin',
        size: 'small',
        render: (_, dom) => (
          <Dropdown menu={{ items: userMenuItems }} placement="bottomRight">
            <Space style={{ cursor: 'pointer' }}>
              <Avatar size="small" icon={<UserOutlined />} />
              <span>{user?.username || 'Admin'}</span>
            </Space>
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
