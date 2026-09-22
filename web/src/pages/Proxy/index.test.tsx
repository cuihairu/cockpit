import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import Proxy from './index'
import type { Agent } from '@/types'

// Proxy：agent 下拉标签与禁用 / nginx 与 traefik 两分支（Descriptions/Alert/extra）/
// 站点表与配置预览 / 新建校验与编辑保存 / 删除

const apiMock = vi.hoisted(() => ({
  getAgents: vi.fn(),
  getProxyStatus: vi.fn(),
  getProxySites: vi.fn(),
  getProxySite: vi.fn(),
  applyProxySite: vi.fn(),
  deleteProxySite: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))
vi.mock('@/hooks/usePerm', () => ({ usePerm: () => true }))

const msgSuccess = vi.spyOn(message, 'success')
const msgError = vi.spyOn(message, 'error')

const mkAgent = (id: string, hostname: string, cap: string | null, offline = false, version = '1.24'): Agent =>
  ({
    id,
    hostname,
    ip: '1.2.3.4',
    status: offline ? 'offline' : 'online',
    lastSeen: '0',
    capabilities: cap ? [{ type: cap, metadata: { version } }] : [],
  }) as unknown as Agent

const agents = [
  mkAgent('ag-1', 'gw-nginx', 'nginx-proxy'),
  mkAgent('ag-2', 'gw-traefik', 'traefik-proxy', false, 'v3.1'),
  mkAgent('ag-3', 'gw-offline', 'nginx-proxy', true),
  mkAgent('ag-4', 'plain', null),
]

const status = { backend: 'nginx' as const, installed: true, version: '1.24.0', confDir: '/etc/nginx/conf.d', siteCount: 2, reloadMode: 'signal' as const }
const sites = {
  sites: [
    { name: 'blog', serverNames: ['blog.example.com', 'www.example.com'], upstream: '127.0.0.1:3000', scheme: 'https' as const, tlsCert: '/c.pem', tlsKey: '/k.pem', websocket: true },
    { name: 'api', serverNames: ['api.example.com'], upstream: '127.0.0.1:9000', scheme: 'http' as const },
  ],
}

const renderPage = () => {
  apiMock.getAgents.mockResolvedValue(agents)
  apiMock.getProxyStatus.mockResolvedValue(status)
  apiMock.getProxySites.mockResolvedValue(sites)
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <Proxy />
    </QueryClientProvider>,
  )
}

const rowOf = (cell: string) =>
  Array.from(document.querySelectorAll<HTMLTableRowElement>('tr.ant-table-row')).find((tr) =>
    tr.textContent?.includes(cell)) as HTMLTableRowElement

// 选网关主机（卡片 extra 的 Select，DOM 里第一个）
const pickAgent = async (label: string) => {
  fireEvent.mouseDown(document.querySelector('.ant-select-selector')!)
  const opt = await waitFor(() => {
    const el = Array.from(document.querySelectorAll('.ant-select-item-option')).find(
      (o) => o.textContent?.startsWith(label))
    if (!el) throw new Error(`option not found: ${label}`)
    return el as HTMLElement
  })
  fireEvent.click(opt)
}

const modalOk = async () => {
  await act(async () => {
    fireEvent.click(document.querySelector('.ant-modal-footer .ant-btn-primary')!)
  })
}

describe('Proxy', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('下拉标签：后端与版本、离线与未检测提示；未选时空态', async () => {
    renderPage()
    expect(await screen.findByText('选择一台安装了 Nginx 或 Traefik 的主机开始管理')).toBeInTheDocument()
    expect(apiMock.getProxyStatus).not.toHaveBeenCalled()
    fireEvent.mouseDown(document.querySelector('.ant-select-selector')!)
    await waitFor(() =>
      expect(document.querySelectorAll('.ant-select-item-option').length).toBeGreaterThan(0))
    const optText = (label: string) => {
      const el = Array.from(document.querySelectorAll('.ant-select-item-option')).find(
        (o) => o.textContent?.startsWith(label))
      return { el, text: el?.textContent ?? '' }
    }
    expect(optText('gw-nginx').text).toBe('gw-nginx（Nginx 1.24）')
    expect(optText('gw-traefik').text).toBe('gw-traefik（Traefik v3.1）')
    expect(optText('gw-offline').text).toBe('gw-offline（Nginx 1.24）（离线）')
    expect(optText('plain').text).toBe('plain（未检测到 Nginx/Traefik）')
    expect((optText('gw-offline').el as HTMLElement).classList.contains('ant-select-item-option-disabled')).toBe(true)
    expect((optText('plain').el as HTMLElement).classList.contains('ant-select-item-option-disabled')).toBe(true)
  })

  it('选 nginx 主机：Descriptions、信息 Alert 与站点表', async () => {
    renderPage()
    await screen.findByText('选择一台安装了 Nginx 或 Traefik 的主机开始管理')
    await act(async () => {
      await pickAgent('gw-nginx')
    })
    expect(await screen.findByText('blog')).toBeInTheDocument()
    expect(screen.getByText('Nginx')).toBeInTheDocument()
    expect(screen.getByText('1.24.0')).toBeInTheDocument()
    expect(screen.getByText('/etc/nginx/conf.d')).toBeInTheDocument()
    expect(screen.getByText('nginx -s reload')).toBeInTheDocument() // reloadMode=signal 默认文案
    expect(screen.getByText(/应用前先 nginx -t 校验/)).toBeInTheDocument()
    // 站点行：域名 Tag、HTTPS Tag、WebSocket 支持
    expect(screen.getByText('blog.example.com')).toBeInTheDocument()
    expect(screen.getByText('HTTPS')).toBeInTheDocument()
    expect(screen.getByText('支持')).toBeInTheDocument()
    expect(screen.getByText('api.example.com')).toBeInTheDocument()
    expect(screen.getByText('HTTP')).toBeInTheDocument()
  })

  it('traefik 分支：热加载文案、YAML 自检 Alert 与高级指令禁用', async () => {
    renderPage()
    // renderPage 内部已 mock nginx 状态，此处覆盖为 traefik（调用发生在选主机之后）
    apiMock.getProxyStatus.mockResolvedValue({
      backend: 'traefik', installed: true, version: 'v3.1', confDir: '/etc/traefik/dynamic', siteCount: 1, reloadMode: 'hot',
    })
    await screen.findByText('选择一台安装了 Nginx 或 Traefik 的主机开始管理')
    await act(async () => {
      await pickAgent('gw-traefik')
    })
    expect(await screen.findByText('Traefik')).toBeInTheDocument()
    expect(screen.getByText('热加载（file provider）')).toBeInTheDocument()
    expect(screen.getByText(/经 YAML 自检，失败不落盘/)).toBeInTheDocument()
    // 新建弹窗：extra TextArea 禁用
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /新建站点/ }))
    })
    const extra = await screen.findByPlaceholderText('client_max_body_size 50m;')
    expect((extra as HTMLTextAreaElement).disabled).toBe(true)
  })

  it('配置预览与读取失败 fallback', async () => {
    apiMock.getProxySite.mockResolvedValue({ name: 'blog', content: 'server { listen 443; }' })
    renderPage()
    await screen.findByText('选择一台安装了 Nginx 或 Traefik 的主机开始管理')
    await act(async () => {
      await pickAgent('gw-nginx')
    })
    expect(await screen.findByText('blog')).toBeInTheDocument()
    await act(async () => {
      fireEvent.click(within(rowOf('blog')).getByRole('button', { name: /配\s*置/ }))
    })
    expect(await screen.findByText('配置预览：blog')).toBeInTheDocument()
    expect(screen.getByText(/server \{ listen 443; \}/)).toBeInTheDocument()
    fireEvent.click(document.querySelector('.ant-modal-close')!)
    apiMock.getProxySite.mockRejectedValue(new Error('boom'))
    await act(async () => {
      fireEvent.click(within(rowOf('blog')).getByRole('button', { name: /配\s*置/ }))
    })
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('读取配置失败'))
  })

  it('编辑：回填且名称禁用，https 保存含证书', async () => {
    apiMock.applyProxySite.mockResolvedValue({})
    renderPage()
    await screen.findByText('选择一台安装了 Nginx 或 Traefik 的主机开始管理')
    await act(async () => {
      await pickAgent('gw-nginx')
    })
    expect(await screen.findByText('blog')).toBeInTheDocument()
    await act(async () => {
      fireEvent.click(within(rowOf('blog')).getByRole('button', { name: /编辑/ }))
    })
    expect(await screen.findByText('编辑站点 blog')).toBeInTheDocument()
    expect((screen.getByPlaceholderText('如 blog') as HTMLInputElement).disabled).toBe(true)
    await modalOk()
    await waitFor(() =>
      expect(apiMock.applyProxySite).toHaveBeenCalledWith('ag-1', expect.objectContaining({
        name: 'blog',
        serverNames: ['blog.example.com', 'www.example.com'],
        scheme: 'https',
        tlsCert: '/c.pem',
        tlsKey: '/k.pem',
        websocket: true,
      })))
    expect(msgSuccess).toHaveBeenCalledWith('站点 blog 已应用')
  })

  it('新建：名称与上游格式校验；https 切换出证书必填', async () => {
    renderPage()
    await screen.findByText('选择一台安装了 Nginx 或 Traefik 的主机开始管理')
    await act(async () => {
      await pickAgent('gw-nginx')
    })
    await screen.findByText('blog')
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /新建站点/ }))
    })
    await modalOk()
    // 必填
    expect(await screen.findByText('请输入站点名称')).toBeInTheDocument()
    // 名称格式 + 上游格式
    fireEvent.change(screen.getByPlaceholderText('如 blog'), { target: { value: 'Bad Name' } })
    fireEvent.change(screen.getByPlaceholderText('127.0.0.1:3000'), { target: { value: 'http://x' } })
    await modalOk()
    expect(await screen.findByText('小写字母/数字开头，可用 - 和 _，最长 64 字符')).toBeInTheDocument()
    expect(screen.getByText('形如 127.0.0.1:3000')).toBeInTheDocument()
    // 切 https 再 submit：证书/私钥必填错误出现（message 需校验触发）
    // Radio 的 HTTPS 与表格列 Tag 撞名，按结构定位第二个 radio 按钮
    fireEvent.click(document.querySelectorAll('.ant-radio-button-wrapper')[1])
    expect(await screen.findByText('证书路径')).toBeInTheDocument()
    await modalOk()
    expect(await screen.findByText('https 需要证书绝对路径')).toBeInTheDocument()
    expect(screen.getByText('https 需要私钥绝对路径')).toBeInTheDocument()
  })

  it('应用失败：applyError Alert 原样展示；删除走 Popconfirm', async () => {
    apiMock.applyProxySite.mockRejectedValue(Object.assign(new Error('x'), { response: { data: { error: 'nginx: [emerg] invalid host' } } }))
    apiMock.deleteProxySite.mockResolvedValue({})
    renderPage()
    await screen.findByText('选择一台安装了 Nginx 或 Traefik 的主机开始管理')
    await act(async () => {
      await pickAgent('gw-nginx')
    })
    expect(await screen.findByText('blog')).toBeInTheDocument()
    await act(async () => {
      fireEvent.click(within(rowOf('api')).getByRole('button', { name: /编辑/ }))
    })
    await screen.findByText('编辑站点 api')
    await modalOk()
    // apply 失败：Modal 保持打开且错误摘要展示
    expect(await screen.findByText(/nginx: \[emerg\] invalid host/)).toBeInTheDocument()
    fireEvent.click(document.querySelector('.ant-modal-close')!)
    // 删除：Popconfirm 确认文案含后端名
    await act(async () => {
      fireEvent.click(within(rowOf('blog')).getByRole('button', { name: /删\s*除/ }))
    })
    expect(await screen.findByText('将从 Nginx 移除 blog，确认？')).toBeInTheDocument()
    await act(async () => {
      fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    })
    await waitFor(() => expect(apiMock.deleteProxySite).toHaveBeenCalledWith('ag-1', 'blog'))
    expect(msgSuccess).toHaveBeenCalledWith('站点 blog 已删除')
  })
})
