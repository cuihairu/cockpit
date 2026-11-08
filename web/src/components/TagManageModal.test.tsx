import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { message } from 'antd'
import TagManageModal from './TagManageModal'
import { api } from '@/services/api'
import type { AgentTagRef } from '@/types'

vi.mock('@/services/api', () => ({
  api: {
    listAgentTags: vi.fn(),
    createAgentTag: vi.fn(),
    updateAgentTag: vi.fn(),
    deleteAgentTag: vi.fn(),
  },
}))

vi.spyOn(message, 'success')
vi.spyOn(message, 'warning')
vi.spyOn(message, 'error')

const apiMock = vi.mocked(api, true)

type TagRow = AgentTagRef & { agentCount?: number }

const tags: TagRow[] = [
  { id: 't1', name: 'prod', color: '#ee33ee', agentCount: 3 },
  { id: 't2', name: 'env', color: '', agentCount: 1 },
  { id: 't3', name: 'web', color: '#52c41a' },
]

const renderModal = (onChanged?: () => void) => {
  const onClose = vi.fn()
  const changed = onChanged ?? vi.fn()
  const view = render(<TagManageModal visible onClose={onClose} onChanged={changed} />)
  return { view, onClose, onChanged: changed }
}

const rowOf = (name: string) => screen.getByText(name).closest('tr') as HTMLElement

// 展示态行内按钮：[编辑, 删除]；编辑态：[保存, 取消]
const rowButtons = (name: string) => within(rowOf(name)).getAllByRole('button')

const popconfirmOk = () =>
  document.querySelector('.ant-popover .ant-btn-primary') as HTMLButtonElement

describe('TagManageModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMock.listAgentTags.mockResolvedValue(tags)
  })

  afterEach(() => {
    cleanup()
    document.body.innerHTML = '' // Modal/Popconfirm 挂 body 的弹层跨用例清理
  })

  it('loads and renders tag list on open', async () => {
    const { view, onClose, onChanged } = renderModal()
    expect(await screen.findByText('prod')).toBeInTheDocument()
    expect(screen.getByText('env')).toBeInTheDocument()
    expect(screen.getByText('web')).toBeInTheDocument()
    expect(screen.getByText('预览')).toBeInTheDocument() // 输入为空时的预览文案
    expect(screen.getByText('3')).toBeInTheDocument() // 服务器数
    expect(screen.getByText('#52c41a')).toBeInTheDocument() // 颜色列文本
    expect(screen.getByText('-')).toBeInTheDocument() // 空颜色回退
    expect(apiMock.listAgentTags).toHaveBeenCalledTimes(1)
    expect(onChanged).not.toHaveBeenCalled()

    // 点关闭回调 onClose
    fireEvent.click(document.querySelector('.ant-modal-close') as HTMLElement)
    expect(onClose).toHaveBeenCalled()

    // visible=false 后不再重复拉取
    view.rerender(<TagManageModal visible={false} onClose={onClose} onChanged={onChanged} />)
    await act(async () => {})
    expect(apiMock.listAgentTags).toHaveBeenCalledTimes(1)
  })

  it('shows error when loading tags fails', async () => {
    apiMock.listAgentTags.mockRejectedValue(new Error('boom'))
    renderModal()
    await waitFor(() => expect(message.error).toHaveBeenCalledWith('加载标签失败'))
  })

  it('create requires a name', async () => {
    renderModal()
    await screen.findByText('prod')
    fireEvent.click(screen.getByRole('button', { name: /新\s*建/ }))
    await waitFor(() => expect(message.warning).toHaveBeenCalledWith('请输入标签名'))
    expect(apiMock.createAgentTag).not.toHaveBeenCalled()
  })

  it('creates tag with trimmed name and color, then reloads', async () => {
    const { onChanged } = renderModal()
    await screen.findByText('prod')
    fireEvent.change(screen.getByPlaceholderText('新标签名'), { target: { value: '  stage  ' } })
    fireEvent.change(screen.getByDisplayValue('#1677ff'), { target: { value: '#00ff00' } })
    fireEvent.click(screen.getByRole('button', { name: /新\s*建/ }))
    await waitFor(() => expect(apiMock.createAgentTag).toHaveBeenCalledWith('stage', '#00ff00'))
    await waitFor(() => expect(onChanged).toHaveBeenCalled())
    expect(await screen.findByText('预览')).toBeInTheDocument() // 输入框已清空
    expect(screen.getByPlaceholderText('新标签名')).toHaveValue('')
    expect(apiMock.listAgentTags).toHaveBeenCalledTimes(2)
  })

  it('reports duplicate tag name on 409', async () => {
    apiMock.createAgentTag.mockRejectedValue({ response: { status: 409 } })
    renderModal()
    await screen.findByText('prod')
    fireEvent.change(screen.getByPlaceholderText('新标签名'), { target: { value: 'prod' } })
    fireEvent.click(screen.getByRole('button', { name: /新\s*建/ }))
    await waitFor(() => expect(message.error).toHaveBeenCalledWith('标签名已存在'))
  })

  it('reports generic create failure', async () => {
    apiMock.createAgentTag.mockRejectedValue(new Error('boom'))
    renderModal()
    await screen.findByText('prod')
    fireEvent.change(screen.getByPlaceholderText('新标签名'), { target: { value: 'x' } })
    fireEvent.click(screen.getByRole('button', { name: /新\s*建/ }))
    await waitFor(() => expect(message.error).toHaveBeenCalledWith('创建失败'))
  })

  it('creates on Enter in the name input', async () => {
    renderModal()
    await screen.findByText('prod')
    const input = screen.getByPlaceholderText('新标签名')
    fireEvent.change(input, { target: { value: 'edge' } })
    fireEvent.keyDown(input, { key: 'Enter', keyCode: 13 })
    await waitFor(() => expect(apiMock.createAgentTag).toHaveBeenCalledWith('edge', '#1677ff'))
  })

  it('saves edit with new name and color', async () => {
    const { onChanged } = renderModal()
    apiMock.updateAgentTag.mockResolvedValue(undefined)
    await screen.findByText('prod')
    const row = rowOf('prod')
    fireEvent.click(within(row).getAllByRole('button')[0])
    fireEvent.change(within(row).getByDisplayValue('prod'), { target: { value: 'prod-2' } })
    fireEvent.change(within(row).getByDisplayValue('#ee33ee'), { target: { value: '#123456' } })
    fireEvent.click(within(row).getByRole('button', { name: /保\s*存/ }))
    await waitFor(() => expect(apiMock.updateAgentTag).toHaveBeenCalledWith('t1', 'prod-2', '#123456'))
    await waitFor(() => expect(onChanged).toHaveBeenCalled())
    expect(apiMock.listAgentTags).toHaveBeenCalledTimes(2)
    // 保存后回到展示态
    expect(await within(row).findByText('prod')).toBeInTheDocument()
    expect(within(row).queryByDisplayValue('prod-2')).not.toBeInTheDocument()
  })

  it('rejects empty tag name on save', async () => {
    renderModal()
    await screen.findByText('prod')
    const row = rowOf('prod')
    fireEvent.click(within(row).getAllByRole('button')[0])
    fireEvent.change(within(row).getByDisplayValue('prod'), { target: { value: '   ' } })
    fireEvent.click(within(row).getByRole('button', { name: /保\s*存/ }))
    await waitFor(() => expect(message.warning).toHaveBeenCalledWith('标签名不能为空'))
    expect(apiMock.updateAgentTag).not.toHaveBeenCalled()
  })

  it('reports update failure and stays in edit mode', async () => {
    apiMock.updateAgentTag.mockRejectedValue(new Error('boom'))
    renderModal()
    await screen.findByText('prod')
    const row = rowOf('prod')
    fireEvent.click(within(row).getAllByRole('button')[0])
    fireEvent.click(within(row).getByRole('button', { name: /保\s*存/ }))
    await waitFor(() => expect(message.error).toHaveBeenCalledWith('更新失败'))
    expect(within(row).getByDisplayValue('prod')).toBeInTheDocument()
    expect(apiMock.listAgentTags).toHaveBeenCalledTimes(1)
  })

  it('cancel restores display mode', async () => {
    renderModal()
    await screen.findByText('prod')
    const row = rowOf('prod')
    fireEvent.click(within(row).getAllByRole('button')[0])
    fireEvent.click(within(row).getByRole('button', { name: /取\s*消/ }))
    expect(within(row).queryByDisplayValue('prod')).not.toBeInTheDocument()
    expect(within(row).getByText('prod')).toBeInTheDocument()
    expect(apiMock.updateAgentTag).not.toHaveBeenCalled()
  })

  it('falls back to default color when tag has none', async () => {
    renderModal()
    await screen.findByText('env')
    const row = rowOf('env')
    fireEvent.click(within(row).getAllByRole('button')[0])
    expect(within(row).getByDisplayValue('#1677ff')).toBeInTheDocument()
  })

  it('deletes tag after popconfirm', async () => {
    const { onChanged } = renderModal()
    apiMock.deleteAgentTag.mockResolvedValue(undefined)
    await screen.findByText('web')
    fireEvent.click(rowButtons('web')[1])
    expect(await screen.findByText('删除后只摘除关联，不删服务器')).toBeInTheDocument()
    await act(async () => {
      fireEvent.click(popconfirmOk())
    })
    await waitFor(() => expect(apiMock.deleteAgentTag).toHaveBeenCalledWith('t3'))
    await waitFor(() => expect(onChanged).toHaveBeenCalled())
    expect(apiMock.listAgentTags).toHaveBeenCalledTimes(2)
  })

  it('reports delete failure', async () => {
    apiMock.deleteAgentTag.mockRejectedValue(new Error('boom'))
    renderModal()
    await screen.findByText('prod')
    fireEvent.click(rowButtons('prod')[1])
    await screen.findByText('删除后只摘除关联，不删服务器')
    await act(async () => {
      fireEvent.click(popconfirmOk())
    })
    await waitFor(() => expect(message.error).toHaveBeenCalledWith('删除失败'))
    expect(apiMock.listAgentTags).toHaveBeenCalledTimes(1)
  })

  it('works without the optional onChanged callback', async () => {
    apiMock.createAgentTag.mockResolvedValue(undefined)
    apiMock.updateAgentTag.mockResolvedValue(undefined)
    apiMock.deleteAgentTag.mockResolvedValue(undefined)
    render(<TagManageModal visible onClose={vi.fn()} />)
    await screen.findByText('prod')

    // 新建
    fireEvent.change(screen.getByPlaceholderText('新标签名'), { target: { value: 'x1' } })
    fireEvent.click(screen.getByRole('button', { name: /新\s*建/ }))
    await waitFor(() => expect(apiMock.createAgentTag).toHaveBeenCalledWith('x1', '#1677ff'))

    // 行内编辑保存
    await screen.findByText('prod') // 新建触发的 load 完成
    let row = rowOf('prod')
    fireEvent.click(within(row).getAllByRole('button')[0])
    fireEvent.change(within(row).getByDisplayValue('prod'), { target: { value: 'prod2' } })
    fireEvent.click(within(row).getByRole('button', { name: /保\s*存/ }))
    await waitFor(() => expect(apiMock.updateAgentTag).toHaveBeenCalledWith('t1', 'prod2', '#ee33ee'))

    // 删除确认
    await screen.findByText('prod') // 保存触发的 load 完成、回到展示态
    row = rowOf('prod')
    fireEvent.click(within(row).getAllByRole('button')[1])
    await screen.findByText('删除后只摘除关联，不删服务器')
    await act(async () => {
      fireEvent.click(popconfirmOk())
    })
    await waitFor(() => expect(apiMock.deleteAgentTag).toHaveBeenCalledWith('t1'))
  })
})
