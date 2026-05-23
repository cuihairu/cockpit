import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import ProLayout from '@ant-design/pro-layout'
import {
  DashboardOutlined,
  CloudServerOutlined,
  ApiOutlined,
  SettingOutlined,
} from '@ant-design/icons'
import Dashboard from './pages/Dashboard'
import Resources from './pages/Resources'
import Agents from './pages/Agents'
import Settings from './pages/Settings'
import logo from '@/assets/logo.svg'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
})

const App = () => {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <ProLayout
          title="Cockpit"
          logo={logo}
          layout="mix"
          splitMenus={true}
          navTheme="light"
          headerTheme="light"
          contentWidth="Fluid"
          location={{
            pathname: window.location.pathname,
          }}
          route={{
            path: '/',
            routes: [
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
            ],
          }}
          menuItemRender={(menuItemProps, defaultDom) => {
            return (
              <a href={menuItemProps.path} onClick={(e) => {
                e.preventDefault()
                window.history.pushState({}, '', menuItemProps.path)
                window.dispatchEvent(new PopStateEvent('popstate'))
              }}>
                {defaultDom}
              </a>
            )
          }}
        >
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/resources" element={<Resources />} />
            <Route path="/agents" element={<Agents />} />
            <Route path="/settings" element={<Settings />} />
          </Routes>
        </ProLayout>
      </BrowserRouter>
    </QueryClientProvider>
  )
}

export default App
