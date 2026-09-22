import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import DiffModal from './DiffModal'
import type { DriftCheckItem } from '@/types'

// DiffModal：对照/统一双视图、纯增删行的缺位侧半栏、着色行有/无颜色 token、
// 失败透出、target 空标题兜底与关闭回调

const apiMock = vi.hoisted(() => ({ driftDiff: vi.fn() }))
vi.mock('@/services/api', () => ({ api: apiMock }))

const target = {
  kind: 'nginx', name: 'blog.example.com', status: 'drifted',
  baseline_sha: 'a', current_sha: 'b',
} as unknown as DriftCheckItem

const renderModal = (t: DriftCheckItem | null = target) => {
  const onClose = vi.fn()
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const view = render(
    <QueryClientProvider client={qc}>
      <DiffModal agentId="ag-1" target={t} onClose={onClose} />
    </QueryClientProvider>,
  )
  return { ...view, onClose }
}

describe('DiffModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('target 为空：不发请求（标题兜底在渲染期求值）', () => {
    renderModal(null)
    expect(apiMock.driftDiff).not.toHaveBeenCalled()
    expect(document.querySelector('.ant-modal')).toBeNull()
  })

  it('对照视图：纯删行右半栏缺位、纯增行左半栏缺位', async () => {
    // 纯删：current 少一行 → 右侧 Half 无内容（signOf/Highlight 兜底空格）
    apiMock.driftDiff.mockResolvedValueOnce({
      expected: 'keep\ndrop-me', current: 'keep', baseline_updated_at: 1759000000,
    })
    const first = renderModal()
    await screen.findByText('反代站点 · blog.example.com', { selector: '.ant-modal-title' })
    await waitFor(() => expect(apiMock.driftDiff).toHaveBeenCalledWith('ag-1', 'nginx', 'blog.example.com'))
    await waitFor(() => {
      expect(document.querySelector('.ant-modal')!.textContent).toContain('drop-me')
    })
    first.unmount()

    // 纯增：current 多一行 → 左侧 Half 无内容
    apiMock.driftDiff.mockResolvedValueOnce({
      expected: 'keep', current: 'keep\nadd-me', baseline_updated_at: 1759000000,
    })
    renderModal()
    await screen.findByText('反代站点 · blog.example.com', { selector: '.ant-modal-title' })
    await waitFor(() => {
      expect(document.querySelector('.ant-modal')!.textContent).toContain('add-me')
    })
  })

  it('统一视图：注释/键着色 token 与纯文本无色 token 并存', async () => {
    apiMock.driftDiff.mockResolvedValue({
      expected: '# cmt\nkey: v', current: '# cmt\nkey: v2', baseline_updated_at: 1759000000,
    })
    renderModal()
    await screen.findByText('反代站点 · blog.example.com', { selector: '.ant-modal-title' })
    fireEvent.click(await screen.findByText('统一'))
    await waitFor(() => {
      const modal = document.querySelector('.ant-modal')!
      const colored = Array.from(modal.querySelectorAll('span[style*="color"]'))
        .map((s) => s.textContent)
        .join('')
      // 注释行整行着色；键名着色（无色 token 由 'v'/'v2' 等纯文本路径覆盖）
      expect(colored).toContain('# cmt')
      expect(colored).toContain('key')
      expect(modal.textContent).toContain('v2')
    })
  })

  it('footer 关闭按钮触发 onClose；失败且非 no content 不带补全引导', async () => {
    apiMock.driftDiff.mockRejectedValue(Object.assign(new Error('agent offline'), {
      isAxiosError: true, response: { status: 502, data: { error: 'agent offline' } },
    }))
    const { onClose } = renderModal()
    await screen.findByText('反代站点 · blog.example.com', { selector: '.ant-modal-title' })
    expect(await screen.findByText('获取差异失败')).toBeInTheDocument()
    expect(screen.getByText(/agent offline/)).toBeInTheDocument()
    expect(screen.queryByText(/到对应管理页重新保存/)).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /^关\s*闭$/ }))
    expect(onClose).toHaveBeenCalled()
  })
})
