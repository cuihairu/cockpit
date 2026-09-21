import { useEffect, lazy, Suspense, useMemo } from 'react'
import { BrowserRouter, Routes, Route, Navigate, matchRoutes, useLocation, useNavigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntdApp, Grid, Result, Spin, theme as antdTheme } from 'antd'
import ProLayout, { ProLayoutProps } from '@ant-design/pro-layout'
import { Button, Dropdown, Avatar, Space, Input, ConfigProvider } from 'antd'
import {
  DashboardOutlined,
  ApiOutlined,
  ClusterOutlined,
  HddOutlined,
  DatabaseOutlined,
  SettingOutlined,
  UserOutlined,
  LogoutOutlined,
  QuestionCircleOutlined,
  AppstoreOutlined,
  ContainerOutlined,
  RocketOutlined,
  CloudUploadOutlined,
  DeploymentUnitOutlined,
  ClockCircleOutlined,
  ThunderboltOutlined,
  SafetyCertificateOutlined,
  GlobalOutlined,
  LinkOutlined,
  VideoCameraOutlined,
  FileSearchOutlined,
  TeamOutlined,
} from '@ant-design/icons'
import Login from './pages/Login'
import NotificationDropdown from './components/Notifications'
import ErrorBoundary from './components/ErrorBoundary'
import { SettingsProvider } from './contexts/SettingsContext'
import { UserProvider } from './contexts/UserContext'
import { useSettingsContext } from './contexts/useSettingsContext'
import { useUser } from './contexts/useUser'
import { permAll } from '@/utils/perm'
import { logger } from '@/utils/logger'
import logo from '@/assets/logo.svg'
import './App.less'

// Route-level code splitting
const Dashboard = lazy(() => import('./pages/Dashboard'))
const Resources = lazy(() => import('./pages/Resources'))
const Workbench = lazy(() => import('./pages/Workbench'))
const LogSearch = lazy(() => import('./pages/LogSearch'))
const Docker = lazy(() => import('./pages/Docker'))
const Stacks = lazy(() => import('./pages/Stacks'))
const Backups = lazy(() => import('./pages/Backups'))
const Proxy = lazy(() => import('./pages/Proxy'))
const Cron = lazy(() => import('./pages/Cron'))
const Services = lazy(() => import('./pages/Services'))
const Network = lazy(() => import('./pages/Network'))
const Disk = lazy(() => import('./pages/Disk'))
const Nas = lazy(() => import('./pages/Nas'))
const Drift = lazy(() => import('./pages/Drift'))
const DNS = lazy(() => import('./pages/DNS'))
const Domains = lazy(() => import('./pages/Domains'))
const Acme = lazy(() => import('./pages/Acme'))
const Recordings = lazy(() => import('./pages/Recordings'))
const Settings = lazy(() => import('./pages/Settings'))
const Profile = lazy(() => import('./pages/Profile'))
const AuditLogs = lazy(() => import('./pages/AuditLogs'))
const Users = lazy(() => import('./pages/Users'))
const Roles = lazy(() => import('./pages/Roles'))
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

// 路由配置。perm：进入所需权限点（read 级，与后端 requiredPerms 的 GET
// 语义对齐）；缺省 = 登录即可（总览/设置——设置页含个人安全 Tab，模块级
// 裁剪在页面内做）。数组为 AND 语义。
interface PermRouteItem {
  path: string
  name?: string
  icon?: React.ReactNode
  perm?: string | string[]
  routes?: PermRouteItem[]
}

const routeConfig: PermRouteItem = {
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
          perm: 'inventory:read',
        },
        {
          path: '/resources/domains',
          name: '域名',
          perm: 'inventory:read',
        },
        {
          path: '/resources/certificates',
          name: '证书',
          perm: 'inventory:read',
        },
        {
          path: '/acme',
          name: '证书签发',
          perm: 'acme:read',
        },
        {
          path: '/resources/services',
          name: '服务',
          perm: 'services:read',
        },
        {
          path: '/resources/gateways',
          name: '网关',
          perm: 'proxy:read',
        },
        {
          path: '/resources/storages',
          name: '存储',
          perm: 'nas:read',
        },
      ],
    },
    {
      path: '/workbench',
      name: '工作台',
      icon: <ApiOutlined />,
      perm: 'inventory:read',
    },
    {
      path: '/logsearch',
      name: '日志检索',
      icon: <FileSearchOutlined />,
      perm: 'logs:read',
    },
    {
      path: '/docker',
      name: '容器管理',
      icon: <ContainerOutlined />,
      perm: 'docker:read',
    },
    {
      path: '/stacks',
      name: '应用部署',
      icon: <RocketOutlined />,
      perm: 'stack:read',
    },
    {
      path: '/backups',
      name: '备份管理',
      icon: <CloudUploadOutlined />,
      perm: 'backup:read',
    },
    {
      path: '/proxy',
      name: '反向代理',
      icon: <DeploymentUnitOutlined />,
      perm: 'proxy:read',
    },
    {
      path: '/cron',
      name: '定时任务',
      icon: <ClockCircleOutlined />,
      perm: 'cron:read',
    },
    {
      path: '/services',
      name: '服务管理',
      icon: <ThunderboltOutlined />,
      perm: 'services:read',
    },
    {
      path: '/network',
      name: '组网观测',
      icon: <ClusterOutlined />,
      perm: 'overlay:read',
    },
    {
      path: '/disk',
      name: '磁盘健康',
      icon: <HddOutlined />,
      perm: 'smart:read',
    },
    {
      path: '/nas',
      name: '存储池',
      icon: <DatabaseOutlined />,
      perm: 'nas:read',
    },
    {
      path: '/drift',
      name: '漂移检测',
      icon: <SafetyCertificateOutlined />,
      perm: 'drift:read',
    },
    {
      path: '/dns',
      name: 'DNS 管理',
      icon: <GlobalOutlined />,
      perm: 'dns:read',
    },
    {
      path: '/domains',
      name: '域名绑定',
      icon: <LinkOutlined />,
      perm: 'dns:read',
    },
    {
      path: '/monitor',
      name: '系统监控',
      icon: <DashboardOutlined />,
      perm: 'inventory:read',
    },
    {
      path: '/recordings',
      name: '会话录制',
      icon: <VideoCameraOutlined />,
      perm: 'recordings:read',
    },
    {
      path: '/settings',
      name: '设置',
      icon: <SettingOutlined />,
    },
    {
      path: '/access',
      name: '访问控制',
      icon: <TeamOutlined />,
      routes: [
        {
          path: '/access/users',
          name: '用户管理',
          perm: 'users:admin',
        },
        {
          path: '/access/roles',
          name: '角色管理',
          perm: 'roles:admin',
        },
      ],
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

// 菜单裁剪（RBAC P1 笔 6）：无权限项不渲染；分组的子项全裁则分组也藏。
// permissions 未加载（undefined）时全裁——等 /api/me 就绪再出菜单
const filterMenuRoutes = (routes: PermRouteItem[], granted: string[] | undefined): PermRouteItem[] => {
  if (!granted) return []
  return routes
    .map((r) => ({ ...r, routes: r.routes ? filterMenuRoutes(r.routes, granted) : undefined }))
    .filter((r) => (!r.perm || permAll(granted, r.perm)) && (!r.routes || r.routes.length > 0))
}

// 扁平化 (path → perm) 供路由守卫 matchRoutes 匹配
const flattenPermRoutes = (routes: PermRouteItem[]): { path: string; perm?: string | string[] }[] =>
  routes.flatMap((r) => [{ path: r.path, perm: r.perm }, ...flattenPermRoutes(r.routes ?? [])])

const allPermRoutes = flattenPermRoutes(routeConfig.routes ?? [])

// 403 页（路由守卫拒绝时；后端 RBAC 仍是权威，守卫纯 UX——直接敲 URL
// 到无权限页面在这里拦下，页面内的 API 调用由后端 403）
const ForbiddenPage = () => (
  <Result
    status="403"
    title="403"
    subTitle="抱歉，您没有访问此页面的权限。"
  />
)

// 主布局组件
const MainLayout = () => {
  const location = useLocation()
  const navigate = useNavigate()
  const { settings, resolvedTheme } = useSettingsContext()
  const { user, logout, permissionsReady } = useUser()
  // 窄屏（< 768px）：mix 的顶部菜单会溢出，强制切 side；
  // side 布局下 ProLayout 自带窄屏 Drawer 抽屉菜单（见 mobile-design.md D3）
  const screens = Grid.useBreakpoint()
  const isMobile = !screens.md

  // RBAC P1 笔 6：菜单按权限裁剪；路由守卫对当前 path 判权限
  const granted = user?.permissions
  const visibleRoutes = useMemo(
    () => filterMenuRoutes(routeConfig.routes ?? [], granted),
    [granted]
  )
  const permDenied = useMemo(() => {
    const matches = matchRoutes(allPermRoutes, { pathname: location.pathname })
    const perm = matches?.[matches.length - 1]?.route.perm
    return !!perm && !(granted && permAll(granted, perm))
  }, [location.pathname, granted])

  useEffect(() => {
    document.title = settings.siteName
  }, [settings.siteName])

  // /api/me 未回来前不出菜单与路由（fail-closed，避免闪现无权限内容）。
  // 放在全部 hooks 之后（rules-of-hooks）
  if (!permissionsReady) {
    return <PageLoading />
  }

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
        style={{ color: resolvedTheme === 'dark' ? '#fff' : '#4E5969' }}
      >
        <span className="header-doc-text">文档</span>
      </Button>
      <NotificationDropdown />
      <Dropdown menu={{ items: userMenuItems }} placement="bottomRight">
        <Space style={{ cursor: 'pointer' }}>
          <Avatar size="small" icon={<UserOutlined />} />
          <span style={{ color: resolvedTheme === 'dark' ? '#fff' : '#1D2129', fontSize: 14 }}>
            {user?.username || 'Admin'}
          </span>
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
      fixSiderbar
      layout={isMobile || settings.compactMode ? 'side' : 'mix'}
      theme={resolvedTheme}
      colorWeak={false}
      title={settings.siteName}
      logo={logo}
      navTheme={resolvedTheme}
      contentWidth="Fluid"
      location={{ pathname: location.pathname }}
      route={{ path: '/', routes: visibleRoutes } as ProLayoutProps['route']}
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
      <div className={settings.compactMode ? 'app-density-compact' : undefined}>
        <Suspense fallback={<PageLoading />}>
          {permDenied ? <ForbiddenPage /> : (
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/resources" element={<Resources />} />
            <Route path="/resources/*" element={<Resources />} />
            <Route path="/workbench" element={<Workbench />} />
            <Route path="/logsearch" element={<LogSearch />} />
            <Route path="/agents" element={<Navigate to="/workbench" replace />} />
            <Route path="/docker" element={<Docker />} />
            <Route path="/stacks" element={<Stacks />} />
            <Route path="/backups" element={<Backups />} />
            <Route path="/proxy" element={<Proxy />} />
            <Route path="/cron" element={<Cron />} />
            <Route path="/services" element={<Services />} />
            <Route path="/network" element={<Network />} />
            <Route path="/disk" element={<Disk />} />
            <Route path="/nas" element={<Nas />} />
            <Route path="/drift" element={<Drift />} />
            <Route path="/dns" element={<DNS />} />
            <Route path="/domains" element={<Domains />} />
            <Route path="/acme" element={<Acme />} />
            <Route path="/recordings" element={<Recordings />} />
            <Route path="/monitor" element={<Monitor />} />
            <Route path="/settings" element={<Settings />} />
            <Route path="/settings/audit-logs" element={<AuditLogs />} />
            <Route path="/access/users" element={<Users />} />
            <Route path="/access/roles" element={<Roles />} />
            <Route path="/profile" element={<Profile />} />
          </Routes>
          )}
        </Suspense>
      </div>
    </ProLayout>
  )
}

const AppShell = () => {
  const { resolvedTheme } = useSettingsContext()

  return (
    <ConfigProvider
      theme={{
        algorithm: resolvedTheme === 'dark' ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
        token: {
          colorPrimary: '#165DFF',
        },
        components: {
          Layout: {
            // 头/侧栏底色与 App.less 暗色覆盖段保持一致（容器层 #1d1d1d）
            headerBg: resolvedTheme === 'dark' ? '#1d1d1d' : '#fff',
            siderBg: resolvedTheme === 'dark' ? '#1d1d1d' : '#fff',
          },
        },
      }}
    >
      <AntdApp>
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
      </AntdApp>
    </ConfigProvider>
  )
}

const App = () => (
  <ErrorBoundary>
    <QueryClientProvider client={queryClient}>
      <UserProvider>
        <SettingsProvider>
          <AppShell />
        </SettingsProvider>
      </UserProvider>
    </QueryClientProvider>
  </ErrorBoundary>
)

export default App
