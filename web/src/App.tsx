import { useState, useEffect } from 'react'
import { BrowserRouter, Routes, Route, Navigate, useLocation, useNavigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import ProLayout, { PageHeader, ProLayoutProps } from '@ant-design/pro-layout'
import { Button, Dropdown, Avatar, Space, Input, Badge, Breadcrumb } from 'antd'
import {
  DashboardOutlined,
  CloudServerOutlined,
  ApiOutlined,
  SettingOutlined,
  UserOutlined,
  LogoutOutlined,
  QuestionCircleOutlined,
  AppstoreOutlined,
  SearchOutlined,
  BellOutlined,
  HomeOutlined,
} from '@ant-design/icons'
import Dashboard from './pages/Dashboard'
import Resources from './pages/Resources'
import Agents from './pages/Agents'
import Settings from './pages/Settings'
import Profile from './pages/Profile'
import AuditLogs from './pages/AuditLogs'
import Monitor from './pages/Monitor'
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

// 路由配置
const routeConfig: ProLayoutProps['route'] = {
  path: '/',
  routes: [
    {
      path: '/',
      name: '总览',
      icon: <DashboardOutlined />,
    },
    {
      path: '/resources',
      name: '资源管理',
      icon: <AppstoreOutlined />,
      routes: [
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
      path: '/monitor',
      name: '系统监控',
      icon: <DashboardOutlined />,
    },
    {
      path: '/settings',
      name: '设置',
      icon: <SettingOutlined />,
    },
  ],
}

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
  const location = useLocation()
  const navigate = useNavigate()
  const [pathname, setPathname] = useState(location.pathname)
  const [settings, setSetting] = useState<{
    fixSiderbar: boolean
    layout: 'side' | 'top' | 'mix'
    theme: 'light' | 'dark'
    colorWeak: boolean
  }>({
    fixSiderbar: true,
    layout: 'mix', // 阿里云风格：mix 布局
    theme: 'light',
    colorWeak: false,
  })
  const { user, logout } = useUser()

  // 监听路由变化
  useEffect(() => {
    setPathname(location.pathname)
  }, [location.pathname])

  const handleLogout = () => {
    logout()
    window.location.href = '/login'
  }

  // 用户菜单
  const userMenuItems = [
    {
      key: 'profile',
      icon: <UserOutlined />,
      label: '个人中心',
      onClick: () => {
        navigate('/profile')
      },
    },
    {
      key: 'settings',
      icon: <SettingOutlined />,
      label: '设置',
      onClick: () => {
        navigate('/settings')
      },
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

  // 右侧内容区域
  const RightContent = () => (
    <Space size="middle">
      <Button
        type="text"
        icon={<QuestionCircleOutlined />}
        href="https://cuihairu.github.io/cockpit/"
        target="_blank"
        style={{ color: '#4E5969' }}
      >
        文档
      </Button>
      <NotificationDropdown />
      <Dropdown menu={{ items: userMenuItems }} placement="bottomRight">
        <Space style={{ cursor: 'pointer' }}>
          <Avatar size="small" icon={<UserOutlined />} />
          <span style={{ color: '#1D2129', fontSize: 14 }}>{user?.username || 'Admin'}</span>
        </Space>
      </Dropdown>
    </Space>
  )

  // 头部内容渲染（搜索框）
  const HeaderContent = () => (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', flex: 1 }}>
      <Input.Search
        placeholder="搜索产品、文档、资源..."
        style={{ maxWidth: 500, width: '100%' }}
        size="middle"
        onSearch={(value) => console.log('Search:', value)}
      />
    </div>
  )

  return (
    <ProLayout
      {...settings}
      title="Cockpit"
      logo={logo}
      navTheme="light"
      headerTheme="light"
      contentWidth="Fluid"
      location={{ pathname }}
      route={routeConfig}
      // 阿里云风格配置
      fixedHeader
      siderWidth={208}
      headerContentRender={HeaderContent}
      rightContentRender={RightContent}
      // 顶部主菜单配置
      headerTitleRender={(logo, title) => (
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          {logo}
          {title}
        </div>
      )}
      // 菜单点击处理
      menuItemRender={(menuItemProps, defaultDom) => {
        return (
          <a
            href={menuItemProps.path}
            onClick={(e) => {
              e.preventDefault()
              navigate(menuItemProps.path || '/')
            }}
          >
            {defaultDom}
          </a>
        )
      }}
      // 面包屑渲染
      breadcrumbRender={(routers = []) => {
        return [
          {
            path: '/',
            breadcrumbName: '首页',
          },
          ...routers,
        ]
      }}
      // 面包屑项渲染
      itemRender={(route, params, routes, paths) => {
        const first = routes.indexOf(route) === 0
        return first ? (
          <a href="/" onClick={(e) => { e.preventDefault(); navigate('/') }}>
            {route.breadcrumbName}
          </a>
        ) : (
          <span>{route.breadcrumbName}</span>
        )
      }}
      // 隐藏菜单头部的 logo 区域（因为顶部已经有了）
      menuHeaderRender={false}
    >
      <Routes>
        <Route path="/" element={<Dashboard />} />
        <Route path="/resources" element={<Resources />} />
        <Route path="/resources/*" element={<Resources />} />
        <Route path="/agents" element={<Agents />} />
        <Route path="/monitor" element={<Monitor />} />
        <Route path="/settings" element={<Settings />} />
        <Route path="/settings/audit-logs" element={<AuditLogs />} />
        <Route path="/profile" element={<Profile />} />
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
