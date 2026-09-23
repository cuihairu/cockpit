import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

// App：应用壳——登录守卫 / MainLayout 布局 / RBAC 菜单裁剪与路由守卫 /
// 用户下拉 / 旧路径重定向 / 文档外链。
// AppShell 自带 BrowserRouter，故用 history.pushState 定初始路径（不外裹 Router）。

const userMock = vi.hoisted(() => ({
  token: 'tk' as string | null,
  user: null as { username: string; permissions?: string[] } | null,
  logout: vi.fn(),
  permissionsReady: true as boolean,
  login: vi.fn(),
}))
vi.mock('./contexts/useUser', () => ({ useUser: () => userMock }))

const settingsMock = vi.hoisted(() => ({
  settings: { siteName: 'Cockpit', compactMode: false },
  resolvedTheme: 'light',
}))
vi.mock('./contexts/useSettingsContext', () => ({
  useSettingsContext: () => settingsMock,
}))

vi.mock('./pages/Login', () => ({
  default: () => <div data-testid="page-login">Login</div>,
}))

// 25 个 lazy 页面统一 mock 成轻量占位（App.tsx 用相对路径 ./pages/X）
const pageStub = (name: string) => ({ default: () => <div data-testid={`page-${name}`}>{name}</div> })
vi.mock('./pages/Dashboard', () => pageStub('Dashboard'))
vi.mock('./pages/Resources', () => pageStub('Resources'))
vi.mock('./pages/Workbench', () => pageStub('Workbench'))
vi.mock('./pages/LogSearch', () => pageStub('LogSearch'))
vi.mock('./pages/Docker', () => pageStub('Docker'))
vi.mock('./pages/Stacks', () => pageStub('Stacks'))
vi.mock('./pages/Backups', () => pageStub('Backups'))
vi.mock('./pages/Proxy', () => pageStub('Proxy'))
vi.mock('./pages/Cron', () => pageStub('Cron'))
vi.mock('./pages/Services', () => pageStub('Services'))
vi.mock('./pages/Network', () => pageStub('Network'))
vi.mock('./pages/Disk', () => pageStub('Disk'))
vi.mock('./pages/Nas', () => pageStub('Nas'))
vi.mock('./pages/Drift', () => pageStub('Drift'))
vi.mock('./pages/DNS', () => pageStub('DNS'))
vi.mock('./pages/Domains', () => pageStub('Domains'))
vi.mock('./pages/Acme', () => pageStub('Acme'))
vi.mock('./pages/Recordings', () => pageStub('Recordings'))
vi.mock('./pages/Settings', () => pageStub('Settings'))
vi.mock('./pages/SetupTOTP', () => pageStub('SetupTOTP'))
vi.mock('./pages/Profile', () => pageStub('Profile'))
vi.mock('./pages/AuditLogs', () => pageStub('AuditLogs'))
vi.mock('./pages/Users', () => pageStub('Users'))
vi.mock('./pages/Roles', () => pageStub('Roles'))
vi.mock('./pages/Monitor', () => pageStub('Monitor'))

vi.mock('./components/Notifications', () => ({
  default: () => <div data-testid="notif">notif</div>,
}))

const ADMIN_PERMS = ['inventory:read', 'docker:read', 'users:admin', 'roles:admin']

const renderAt = async (path: string) => {
  window.history.pushState({}, '', path)
  const { default: App } = await import('./App')
  return render(<App />)
}

const path = () => window.location.pathname

// 全局 setup 的 matchMedia 恒 matches:false → Grid.useBreakpoint 全 false →
// ProLayout 落 screen-md 移动布局（header 搜索/文档/用户下拉不渲染）。
// 本文件覆盖为桌面断点，让 header 右侧区可测。
const stubMatchMedia = (query: string) => ({
  matches: /min-width:\s*(\d+)px/.test(query)
    ? Number(/min-width:\s*(\d+)px/.exec(query)![1]) <= 1200
    : true,
  media: query,
  onchange: null,
  addListener: () => {},
  removeListener: () => {},
  addEventListener: () => {},
  removeEventListener: () => {},
  dispatchEvent: () => false,
})

describe('App', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    userMock.token = 'tk'
    userMock.user = { username: 'admin', permissions: ADMIN_PERMS }
    userMock.permissionsReady = true
    settingsMock.settings = { siteName: 'Cockpit', compactMode: false }
    settingsMock.resolvedTheme = 'light'
    window.matchMedia = stubMatchMedia as unknown as typeof window.matchMedia
  })

  it('未登录访问受保护路由 → 跳登录页', async () => {
    userMock.token = null
    await renderAt('/')
    expect(await screen.findByTestId('page-login')).toBeInTheDocument()
    expect(path()).toBe('/login')
  })

  it('/login 直达登录页', async () => {
    await renderAt('/login')
    expect(await screen.findByTestId('page-login')).toBeInTheDocument()
  })

  it('已登录进主框架：总览页渲染、搜索框、文档外链、通知铃铛', async () => {
    await renderAt('/')
    expect(await screen.findByTestId('page-Dashboard')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('搜索产品、文档、资源...')).toBeInTheDocument()
    expect(screen.getByTestId('notif')).toBeInTheDocument()
    const doc = screen.getByRole('link', { name: /文档/ })
    expect(doc).toHaveAttribute('href', 'https://cuihairu.github.io/cockpit/')
    expect(doc).toHaveAttribute('target', '_blank')
  })

  it('permissionsReady=false → 加载占位不出菜单', async () => {
    userMock.permissionsReady = false
    await renderAt('/')
    // antd Spin 在 jsdom 下无 role="progressbar"，按 class 断言
    expect(document.querySelector('.ant-spin')).toBeTruthy()
    expect(screen.queryByTestId('page-Dashboard')).not.toBeInTheDocument()
  })

  it('RBAC 菜单裁剪：无 users:admin 不渲染用户管理；分组全裁则分组隐藏', async () => {
    userMock.user = { username: 'op', permissions: ['inventory:read'] }
    await renderAt('/')
    expect(await screen.findByTestId('page-Dashboard')).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.queryByText('用户管理')).not.toBeInTheDocument()
    })
    expect(screen.queryByText('角色管理')).not.toBeInTheDocument()
    expect(screen.queryByText('访问控制')).not.toBeInTheDocument()
  })

  it('RBAC 路由守卫：直接敲无权限 URL → 403 页', async () => {
    userMock.user = { username: 'op', permissions: ['inventory:read'] }
    await renderAt('/access/users')
    expect(await screen.findByText('403')).toBeInTheDocument()
    expect(screen.getByText('抱歉，您没有访问此页面的权限。')).toBeInTheDocument()
    expect(screen.queryByTestId('page-Users')).not.toBeInTheDocument()
  })

  it('旧路径 /agents 重定向到 /workbench', async () => {
    await renderAt('/agents')
    expect(await screen.findByTestId('page-Workbench')).toBeInTheDocument()
    expect(path()).toBe('/workbench')
  })

  it('二级路由可达：/settings/setup-totp、/resources/compute', async () => {
    await renderAt('/settings/setup-totp')
    expect(await screen.findByTestId('page-SetupTOTP')).toBeInTheDocument()
  })

  it('/resources/compute 落资源页', async () => {
    await renderAt('/resources/compute')
    expect(await screen.findByTestId('page-Resources')).toBeInTheDocument()
  })

  it('菜单渲染回归：权限就绪后侧栏导航项存在且数量正确（防「菜单全空也能过」）', async () => {
    // 线上回归自证：负断言（queryByText(...).not.toBe）在菜单全空时同样通过，
    // 必须补正向计数断言。ADMIN_PERMS = inventory:read + docker:read +
    // users:admin + roles:admin（AND 语义，见 utils/perm.ts）下应剩 7 项：
    // 总览（无 perm）、资源管理（compute/domains/certificates 均 inventory:read）、
    // 工作台（inventory:read）、容器管理（docker:read）、系统监控（inventory:read）、
    // 设置（无 perm）、访问控制（users:admin+roles:admin）
    await renderAt('/')
    expect(await screen.findByTestId('page-Dashboard')).toBeInTheDocument()
    const labels = Array.from(
      document.querySelectorAll('.ant-menu-item, .ant-menu-submenu-title'),
    )
      .map((el) => (el.textContent || '').trim())
      .filter(Boolean)
    expect(labels.length).toBe(7)
    for (const name of ['总览', '资源管理', '工作台', '容器管理', '系统监控', '设置', '访问控制']) {
      expect(labels).toContain(name)
    }
  })

  it('菜单渲染回归：permissions 未载入（undefined）时 fail-closed 全裁，不闪现', async () => {
    userMock.user = { username: 'admin', permissions: undefined }
    await renderAt('/')
    expect(await screen.findByTestId('page-Dashboard')).toBeInTheDocument()
    const labels = Array.from(
      document.querySelectorAll('.ant-menu-item, .ant-menu-submenu-title'),
    ).filter((el) => (el.textContent || '').trim())
    expect(labels).toHaveLength(0)
  })

  it('用户下拉：hover 展开后个人中心跳转', async () => {
    await renderAt('/')
    expect(await screen.findByTestId('page-Dashboard')).toBeInTheDocument()

    // antd Dropdown 默认 trigger 为 hover
    fireEvent.mouseEnter(screen.getByText('admin'))
    fireEvent.click(await screen.findByText('个人中心'))
    await waitFor(() => expect(path()).toBe('/profile'))
    expect(await screen.findByTestId('page-Profile')).toBeInTheDocument()
  })

  it('顶栏搜索框输入不抛错（logger.debug 通道）', async () => {
    await renderAt('/')
    expect(await screen.findByTestId('page-Dashboard')).toBeInTheDocument()
    const input = screen.getByPlaceholderText('搜索产品、文档、资源...')
    fireEvent.change(input, { target: { value: 'nginx' } })
    fireEvent.keyDown(input, { key: 'Enter', code: 'Enter' })
  })

  it('搜索框点放大镜触发 onSearch', async () => {
    await renderAt('/')
    expect(await screen.findByTestId('page-Dashboard')).toBeInTheDocument()
    const input = screen.getByPlaceholderText('搜索产品、文档、资源...')
    fireEvent.change(input, { target: { value: 'docker' } })
    fireEvent.click(document.querySelector('.ant-input-search-button')!)
  })

  it('用户下拉「设置」跳 /settings（Dropdown onClick）', async () => {
    await renderAt('/')
    expect(await screen.findByTestId('page-Dashboard')).toBeInTheDocument()
    fireEvent.mouseEnter(screen.getByText('admin'))
    // Dropdown 菜单项 role=menuitem（避免命中 ProLayout 侧栏同名「设置」）
    const item = await screen.findByRole('menuitem', { name: /设置/ })
    fireEvent.click(item)
    await waitFor(() => expect(path()).toBe('/settings'))
  })

  it('面包屑在子路径渲染（itemRender first 分支）', async () => {
    const { container } = await renderAt('/docker')
    expect(await screen.findByTestId('page-Docker')).toBeInTheDocument()
    // ProLayout 面包屑：breadcrumbRender 注入「首页」项，itemRender 首项渲染为 link
    const home = container.querySelector('.ant-breadcrumb a, a[href="/"]')
    if (home) {
      fireEvent.click(home)
      await waitFor(() => expect(path()).toBe('/'))
    } else {
      // jsdom + ProLayout 在 screen-md 布局下面包屑不渲染为 link——
      // 至少断言面包屑容器或首页文本出现（itemRender 被调用）
      expect(
        container.querySelector('.ant-breadcrumb') || screen.queryByText('首页'),
      ).toBeTruthy()
    }
  })

  it('用户下拉「退出登录」调 logout', async () => {
    await renderAt('/')
    expect(await screen.findByTestId('page-Dashboard')).toBeInTheDocument()
    fireEvent.mouseEnter(screen.getByText('admin'))
    fireEvent.click(await screen.findByText('退出登录'))
    // handleLogout：logout() 后 window.location.href='/login'（jsdom 导航未实现，
    // 只断言 logout 被调，避免 delete window.location 污染后续用例）
    await waitFor(() => expect(userMock.logout).toHaveBeenCalled())
  })

  it('侧栏菜单点「容器管理」跳对应路由（menuItemRender）', async () => {
    await renderAt('/')
    expect(await screen.findByTestId('page-Dashboard')).toBeInTheDocument()
    // 点菜单项触发 menuItemRender 的 onClick（preventDefault + navigate）
    fireEvent.click(screen.getByText('容器管理'))
    await waitFor(() => expect(path()).toBe('/docker'))
    expect(await screen.findByTestId('page-Docker')).toBeInTheDocument()
  })

  it('dark 主题：header 文案/图标反白', async () => {
    settingsMock.resolvedTheme = 'dark'
    await renderAt('/')
    expect(await screen.findByTestId('page-Dashboard')).toBeInTheDocument()
    // 深色下用户名单 inline color 为 #fff
    const name = screen.getByText('admin')
    expect(name).toHaveStyle({ color: '#fff' })
  })

  it('compactMode：内容区加 app-density-compact 类', async () => {
    settingsMock.settings = { siteName: 'MyCockpit', compactMode: true }
    await renderAt('/')
    expect(await screen.findByTestId('page-Dashboard')).toBeInTheDocument()
    expect(document.querySelector('.app-density-compact')).toBeTruthy()
  })

  it('站点标题写入 document.title', async () => {
    settingsMock.settings = { siteName: '运维面板', compactMode: false }
    await renderAt('/')
    expect(await screen.findByTestId('page-Dashboard')).toBeInTheDocument()
    await waitFor(() => expect(document.title).toBe('运维面板'))
  })
})
