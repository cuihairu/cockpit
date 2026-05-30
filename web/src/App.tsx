import { useState, lazy, Suspense } from 'react'
import { BrowserRouter, Routes, Route, Navigate, useLocation, useNavigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntdApp, Spin } from 'antd'
import ProLayout, { ProLayoutProps } from '@ant-design/pro-layout'
import { Button, Dropdown, Avatar, Space, Input, ConfigProvider } from 'antd'
import {
  DashboardOutlined,
  ApiOutlined,
  SettingOutlined,
  UserOutlined,
  LogoutOutlined,
  QuestionCircleOutlined,
  AppstoreOutlined,
} from '@ant-design/icons'
import Login from './pages/Login'
import NotificationDropdown from './components/Notifications'
import { UserProvider, useUser } from './contexts/UserContext'
import { logger } from '@/utils/logger'
import logo from '@/assets/logo.svg'
import './App.less'

// Route-level code splitting
const Dashboard = lazy(() => import('./pages/Dashboard'))
const Resources = lazy(() => import('./pages/Resources'))
const Workbench = lazy(() => import('./pages/Workbench'))
const Settings = lazy(() => import('./pages/Settings'))
const Profile = lazy(() => import('./pages/Profile'))
const AuditLogs = lazy(() => import('./pages/AuditLogs'))
const Monitor = lazy(() => import('./pages/Monitor'))

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
})

// Page loading fallback
const PageLoading = () => (
  <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '50vh' }}>
    <Spin size="large" />
  </div>
)

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
      path: '/workbench',
      name: '工作台',
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
  const [settings] = useState<{
    fixSiderbar: boolean
    layout: 'side' | 'top' | 'mix'
    theme: 'light' | 'dark'
    colorWeak: boolean
  }>({
    fixSiderbar: true,
    layout: 'mix',
    theme: 'light',
    colorWeak: false,
  })
  const { user, logout } = useUser()

  const handleLogout = () => {
    logout()
    window.location.href = '/login'
  }

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

  const HeaderContent = () => (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', flex: 1 }}>
      <Input.Search
        placeholder="搜索产品、文档、资源..."
        style={{ maxWidth: 500, width: '100%' }}
        size="middle"
        onSearch={(value) => logger.debug('Search:', value)}
      />
    </div>
  )

  return (
    <ProLayout
      {...settings}
      title="Cockpit"
      logo={logo}
      navTheme="light"
      contentWidth="Fluid"
      location={{ pathname: location.pathname }}
      route={routeConfig}
      fixedHeader
      siderWidth={208}
      headerContentRender={HeaderContent}
      rightContentRender={RightContent}
      headerTitleRender={(logo, title) => (
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          {logo}
          {title}
        </div>
      )}
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
      breadcrumbRender={(routers = []) => {
        return [
          {
            path: '/',
            breadcrumbName: '首页',
          },
          ...routers,
        ]
      }}
      itemRender={(route, _params, routes, _paths) => {
        const first = routes.indexOf(route) === 0
        return first ? (
          <a href="/" onClick={(e) => { e.preventDefault(); navigate('/') }}>
            {route.breadcrumbName}
          </a>
        ) : (
          <span>{route.breadcrumbName}</span>
        )
      }}
      menuHeaderRender={false}
    >
      <Suspense fallback={<PageLoading />}>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/resources" element={<Resources />} />
          <Route path="/resources/*" element={<Resources />} />
          <Route path="/workbench" element={<Workbench />} />
          <Route path="/agents" element={<Navigate to="/workbench" replace />} />
          <Route path="/monitor" element={<Monitor />} />
          <Route path="/settings" element={<Settings />} />
          <Route path="/settings/audit-logs" element={<AuditLogs />} />
          <Route path="/profile" element={<Profile />} />
        </Routes>
      </Suspense>
    </ProLayout>
  )
}

const App = () => {
  return (
    <ConfigProvider
      theme={{
        token: {
          colorPrimary: '#165DFF',
        },
      }}
    >
      <AntdApp>
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
      </AntdApp>
    </ConfigProvider>
  )
}

export default App
