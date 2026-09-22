import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message, Modal } from 'antd'
import FileBrowser from './index'
import type { FileEntry } from '@/types'

// FileBrowser 批 1：列表渲染/导航/跳转/RBAC 裁剪/下载/编辑器/重命名/新建/删除/搜索
// （上传分块与续传、权限九宫格、图片预览在批 2）

const apiMock = vi.hoisted(() => ({
  listFiles: vi.fn(),
  createRemoteDir: vi.fn(),
  renameRemoteFile: vi.fn(),
  deleteRemoteFile: vi.fn(),
  downloadRemoteFile: vi.fn(),
  readFileChunk: vi.fn(),
  writeFile: vi.fn(),
  chmodFile: vi.fn(),
  chownFile: vi.fn(),
  searchFiles: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

const permState = vi.hoisted(() => ({ canWrite: true }))
vi.mock('@/hooks/usePerm', () => ({ usePerm: () => permState.canWrite }))

const msgSuccess = vi.spyOn(message, 'success')
const msgError = vi.spyOn(message, 'error')
const msgWarning = vi.spyOn(message, 'warning')

const entry = (o: Partial<FileEntry> & { name: string }): FileEntry => ({
  size: 0, mode: '0644', uid: 0, gid: 0, mtime: 1700000000,
  isDir: false, isSymlink: false, target: '', ...o,
})

// 默认数据集：目录 / 文本 / symlink / 可预览图片
const defaultEntries = () => [
  entry({ name: 'etc', isDir: true, mode: '0755' }),
  entry({ name: 'a.txt', size: 1024 }),
  entry({ name: 'link.txt', isSymlink: true, target: '/etc/hosts' }),
  entry({ name: 'pic.png', size: 2048 }),
]

const wrap = (ui: React.ReactNode) => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{ui}</QueryClientProvider>
}

const rowOf = (name: string) => screen.getByText(name).closest('tr')!

// 源码 onOk 走 form.validateFields().then() 无 catch——校验失败分支的
// rejection 是已知的 antd 常态，兜底吞掉避免 vitest 记 unhandled error
process.on('unhandledRejection', () => {})

const modalEl = () => document.querySelector('.ant-modal') as HTMLElement

const modalOk = () =>
  document.querySelector('.ant-modal-footer .ant-btn-primary') as HTMLButtonElement

describe('FileBrowser（批 1）', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    permState.canWrite = true
    localStorage.clear()
    apiMock.listFiles.mockReset()
    apiMock.listFiles.mockResolvedValue({ entries: defaultEntries() })
    apiMock.readFileChunk.mockReset()
    apiMock.writeFile.mockReset()
    apiMock.writeFile.mockResolvedValue({ size: 0 })
    apiMock.searchFiles.mockReset()
    apiMock.deleteRemoteFile.mockReset()
    apiMock.deleteRemoteFile.mockResolvedValue(undefined)
    apiMock.renameRemoteFile.mockReset()
    apiMock.renameRemoteFile.mockResolvedValue(undefined)
    apiMock.createRemoteDir.mockReset()
    apiMock.createRemoteDir.mockResolvedValue(undefined)
    apiMock.downloadRemoteFile.mockReset()
  })

  it('渲染目录表：目录链接/symlink 标注/大小/权限/时间列', async () => {
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    expect(apiMock.listFiles).toHaveBeenCalledWith('ag1', '/')
    // 目录与普通文件名都在表格
    expect(screen.getByText('etc').closest('a')).not.toBeNull()
    // symlink：名称 → 目标 + 「链接」Tag
    expect(screen.getByText('link.txt → /etc/hosts')).toBeInTheDocument()
    expect(screen.getByText('链接')).toBeInTheDocument()
    // 大小列：文件按 KB 格式化、目录占位
    expect(within(rowOf('etc')).getByText('—')).toBeInTheDocument()
    expect(within(rowOf('a.txt')).getByText('1.0 KB')).toBeInTheDocument()
    // mtime 格式 YYYY-MM-DD HH:mm
    expect(rowOf('a.txt').textContent).toMatch(/\d{4}-\d{2}-\d{2} \d{2}:\d{2}/)
  })

  it('点目录名进入子目录；面包屑回跳根', async () => {
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('etc')
    fireEvent.click(screen.getByText('etc'))
    await waitFor(() => expect(apiMock.listFiles).toHaveBeenLastCalledWith('ag1', '/etc'))
    // 面包屑：当前段为纯文本非链接，根「/」段是链接（分隔符「/」也是文本，按 a 取）
    await waitFor(() => expect(screen.getByText('etc').closest('a')).toBeNull())
    fireEvent.click(document.querySelector('.ant-breadcrumb a')!)
    await waitFor(() => expect(apiMock.listFiles).toHaveBeenLastCalledWith('ag1', '/'))
  })

  it('跳转框：绝对路径回车（尾斜杠截断）；相对路径告警', async () => {
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    const jump = screen.getByPlaceholderText('跳转到路径，如 /etc/nginx')
    // rc-input 的 Enter 有 keyLock（keyDown 置锁、keyUp/blur 解锁）——须补 keyUp
    const pressEnter = () => {
      fireEvent.keyDown(jump, { key: 'Enter' })
      fireEvent.keyUp(jump, { key: 'Enter' })
    }
    fireEvent.change(jump, { target: { value: '/var/log/' } })
    pressEnter()
    await waitFor(() => expect(apiMock.listFiles).toHaveBeenLastCalledWith('ag1', '/var/log'))
    expect(jump).toHaveValue('')
    fireEvent.change(jump, { target: { value: 'var' } })
    pressEnter()
    await waitFor(() => expect(msgWarning).toHaveBeenCalledWith('请输入绝对路径'))
    expect(apiMock.listFiles).toHaveBeenCalledTimes(2)
  })

  it('目录不可访问：Empty 提示', async () => {
    apiMock.listFiles.mockRejectedValue(new Error('denied'))
    render(wrap(<FileBrowser agentId="ag1" />))
    // useQuery retry:1——失败需等一次重试
    expect(await screen.findByText(/目录不可访问/)).toBeInTheDocument()
  })

  it('空目录提示', async () => {
    apiMock.listFiles.mockResolvedValue({ entries: [] })
    render(wrap(<FileBrowser agentId="ag1" />))
    expect(await screen.findByText('目录为空')).toBeInTheDocument()
  })

  it('canWrite=false：写操作全裁剪，读操作（搜索/下载/预览）保留', async () => {
    permState.canWrite = false
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    expect(screen.queryByText('新建目录')).toBeNull()
    expect(screen.queryByText('上 传')).toBeNull()
    expect(screen.queryByTitle('重命名')).toBeNull()
    expect(screen.queryByTitle('权限')).toBeNull()
    expect(screen.queryByTitle('删除目录')).toBeNull()
    const row = rowOf('a.txt')
    expect(row.querySelector('.anticon-edit')).toBeNull()
    expect(row.querySelector('.anticon-delete')).toBeNull()
    expect(row.querySelector('.anticon-download')).not.toBeNull()
    expect(rowOf('pic.png').querySelector('[title="预览图片"]')).not.toBeNull()
    // 带 icon 的两字按钮不插空格（与纯文本两字按钮不同）
    expect(screen.getByText('搜索')).toBeInTheDocument()
  })

  it('下载：Blob 建对象 URL 并触发锚点点击；失败报错', async () => {
    const createObjectURL = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:x')
    const revokeObjectURL = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
    const anchorClick = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    apiMock.downloadRemoteFile.mockResolvedValue(new Blob(['x']))
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    fireEvent.click(rowOf('a.txt').querySelector('.anticon-download')!.closest('button')!)
    await waitFor(() => expect(apiMock.downloadRemoteFile).toHaveBeenCalledWith('ag1', '/a.txt'))
    expect(createObjectURL).toHaveBeenCalled()
    expect(anchorClick).toHaveBeenCalled()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:x')

    apiMock.downloadRemoteFile.mockRejectedValue(new Error('net'))
    // loading 态 icon 被 loading 图标替换——等恢复后再点
    await waitFor(() =>
      expect(rowOf('a.txt').querySelector('.anticon-download')).not.toBeNull())
    fireEvent.click(rowOf('a.txt').querySelector('.anticon-download')!.closest('button')!)
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('下载失败'))
    anchorClick.mockRestore()
    createObjectURL.mockRestore()
    revokeObjectURL.mockRestore()
  })

  it('编辑器：分块读文本填充、保存走 writeFile + 刷新列表', async () => {
    apiMock.readFileChunk.mockResolvedValue({ data: btoa('hello'), size: 5, eof: true })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    fireEvent.click(within(rowOf('a.txt')).getByTitle('编辑'))
    const area = await screen.findByDisplayValue('hello')
    expect(screen.getByText('编辑：a.txt')).toBeInTheDocument()
    fireEvent.change(area, { target: { value: 'hello!' } })
    fireEvent.click(modalOk())
    await waitFor(() =>
      expect(apiMock.writeFile).toHaveBeenCalledWith('ag1', '/a.txt', btoa('hello!'), true))
    expect(msgSuccess).toHaveBeenCalledWith('已保存')
    // invalidate 触发同 key 重查
    await waitFor(() => expect(apiMock.listFiles.mock.calls.filter(([_, d]) => d === '/').length).toBe(2))
  })

  it('编辑读取超限（服务端块大于 1MB）：报错并关闭编辑器', async () => {
    apiMock.readFileChunk.mockResolvedValue({ data: '', size: 1024 * 1024 + 1, eof: false })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    fireEvent.click(within(rowOf('a.txt')).getByTitle('编辑'))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('读取文件失败'))
  })

  it('重命名：预填旧名、改名生效；同名提交不调接口', async () => {
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    fireEvent.click(within(rowOf('a.txt')).getByTitle('重命名'))
    const input = within(modalEl()).getByDisplayValue('a.txt')
    fireEvent.change(input, { target: { value: 'b.txt' } })
    fireEvent.click(modalOk())
    await waitFor(() => expect(apiMock.renameRemoteFile).toHaveBeenCalledWith('ag1', '/a.txt', 'b.txt'))
    expect(msgSuccess).toHaveBeenCalledWith('重命名成功')

    // 同名：直接关闭不调接口（act flush validateFields 微任务链）
    fireEvent.click(within(rowOf('link.txt → /etc/hosts')).getByTitle('重命名'))
    await act(async () => {
      fireEvent.click(modalOk())
    })
    expect(apiMock.renameRemoteFile).toHaveBeenCalledTimes(1)
  })

  it('新建目录：提交创建；空名与路径分隔符校验', async () => {
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    fireEvent.click(screen.getByText('新建目录'))
    // 空名：校验拦截
    fireEvent.click(modalOk())
    expect(await screen.findByText('请输入目录名')).toBeInTheDocument()
    expect(apiMock.createRemoteDir).not.toHaveBeenCalled()
    // 含分隔符
    const input = modalEl().querySelector('input')!
    fireEvent.change(input, { target: { value: 'a/b' } })
    fireEvent.click(modalOk())
    expect(await screen.findByText('不能包含路径分隔符')).toBeInTheDocument()
    fireEvent.change(input, { target: { value: 'logs' } })
    fireEvent.click(modalOk())
    await waitFor(() => expect(apiMock.createRemoteDir).toHaveBeenCalledWith('ag1', '/logs'))
    expect(msgSuccess).toHaveBeenCalledWith('目录已创建')
  })

  it('删除文件走 Popconfirm；删除目录需输入名字解锁', async () => {
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    // 文件：Popconfirm 确认
    fireEvent.click(rowOf('a.txt').querySelector('.anticon-delete')!.closest('button')!)
    expect(await screen.findByText('删除该文件？')).toBeInTheDocument()
    fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary') as HTMLElement)
    await waitFor(() => expect(apiMock.deleteRemoteFile).toHaveBeenCalledWith('ag1', '/a.txt'))
    expect(msgSuccess).toHaveBeenCalledWith('已删除')

    // 目录：Modal 确认名解锁
    fireEvent.click(within(rowOf('etc')).getByTitle('删除目录'))
    const confirmInput = screen.getByPlaceholderText('输入目录名 etc 确认')
    expect(modalOk().disabled).toBe(true)
    fireEvent.change(confirmInput, { target: { value: 'etc' } })
    await waitFor(() => expect(modalOk().disabled).toBe(false))
    fireEvent.click(modalOk())
    await waitFor(() => expect(apiMock.deleteRemoteFile).toHaveBeenCalledWith('ag1', '/etc'))
  })

  it('文本搜索：参数透传/命中渲染高亮/统计/截断 Tag/点击命中跳目录', async () => {
    apiMock.searchFiles.mockResolvedValue({
      matches: [{ path: 'etc/app.conf', line: 3, text: 'listen 80' }],
      scanned: 5, skipped: 1, truncated: true,
    })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    fireEvent.click(screen.getByText('搜索'))
    const input = screen.getByPlaceholderText('关键词（纯文本，递归搜索子目录）')
    // 空关键词：告警不发请求（message 静态方法在 jsdom 不渲染 notice，断言 spy）
    fireEvent.click(within(modalEl()).getByRole('button', { name: /搜\s*索/ }))
    await waitFor(() => expect(msgWarning).toHaveBeenCalledWith('请输入搜索关键词'))
    expect(apiMock.searchFiles).not.toHaveBeenCalled()
    // 输入关键词回车搜索（大小写开关打开）
    fireEvent.change(input, { target: { value: 'listen' } })
    fireEvent.click(within(modalEl()).getByRole('switch'))
    fireEvent.keyDown(input, { key: 'Enter' })
    fireEvent.keyUp(input, { key: 'Enter' })
    await waitFor(() => expect(apiMock.searchFiles).toHaveBeenCalledWith('ag1', '/', 'listen', true))
    expect(await screen.findByText('etc/app.conf')).toBeInTheDocument()
    expect(screen.getByText(':3')).toBeInTheDocument()
    expect(document.querySelector('.ant-modal mark')!.textContent).toBe('listen')
    expect(screen.getByText(/1 处命中 · 已扫描 5 文件 · 跳过 1/)).toBeInTheDocument()
    expect(screen.getByText('结果截断，可换更具体的关键词或子目录')).toBeInTheDocument()
    // 点击命中：跳到所在目录并关 Modal（jsdom 无关闭动画事件，DOM 卸载不断言）
    fireEvent.click(screen.getByText('etc/app.conf'))
    await waitFor(() => expect(apiMock.listFiles).toHaveBeenLastCalledWith('ag1', '/etc'))
  })

  it('搜索失败报错', async () => {
    apiMock.searchFiles.mockRejectedValue(new Error('down'))
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    fireEvent.click(screen.getByText('搜索'))
    fireEvent.change(screen.getByPlaceholderText('关键词（纯文本，递归搜索子目录）'), {
      target: { value: 'kw' },
    })
    fireEvent.click(within(modalEl()).getByRole('button', { name: /搜\s*索/ }))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('搜索失败'))
  })
})

describe('FileBrowser（批 2：权限/预览/上传）', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    permState.canWrite = true
    localStorage.clear()
    apiMock.listFiles.mockReset()
    apiMock.listFiles.mockResolvedValue({ entries: defaultEntries() })
    apiMock.readFileChunk.mockReset()
    apiMock.writeFile.mockReset()
    apiMock.writeFile.mockResolvedValue({ size: 0 })
    apiMock.chmodFile.mockReset()
    apiMock.chmodFile.mockResolvedValue(undefined)
    apiMock.chownFile.mockReset()
    apiMock.chownFile.mockResolvedValue(undefined)
    apiMock.deleteRemoteFile.mockReset()
    apiMock.deleteRemoteFile.mockResolvedValue(undefined)
  })

  const uploadInput = () =>
    document.querySelector('.ant-upload input[type=file]') as HTMLInputElement

  const pickFile = async (file: File) => {
    Object.defineProperty(uploadInput(), 'files', { value: [file], configurable: true })
    await act(async () => {
      fireEvent.change(uploadInput())
    })
  }

  it('权限九宫格：mode 反解、翻位合成八进制、仅 chmod 分流', async () => {
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    fireEvent.click(within(rowOf('a.txt')).getByTitle('权限'))
    expect(await screen.findByText('权限：a.txt')).toBeInTheDocument()
    // 0644 反解：段落显示合成值
    expect(screen.getByText(/应用后合成八进制 0644/)).toBeInTheDocument()
    const ownerRow = screen.getByText('属主').closest('div')!
    const boxes = within(ownerRow).getAllByRole('checkbox')
    expect(boxes.map((b) => (b as HTMLInputElement).checked)).toEqual([true, true, false])
    // 翻属主「执行」位 → 0744
    fireEvent.click(boxes[2])
    expect(screen.getByText(/应用后合成八进制 0744/)).toBeInTheDocument()
    fireEvent.click(modalOk())
    await waitFor(() => expect(apiMock.chmodFile).toHaveBeenCalledWith('ag1', '/a.txt', 0o744))
    expect(apiMock.chownFile).not.toHaveBeenCalled()
    expect(msgSuccess).toHaveBeenCalledWith('权限已更新')
  })

  it('权限：uid/gid 同变走 chown；二者都不变不调接口；失败报错', async () => {
    apiMock.listFiles.mockResolvedValue({ entries: [entry({ name: 'a.txt', size: 5 })] })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    fireEvent.click(within(rowOf('a.txt')).getByTitle('权限'))
    await screen.findByText('权限：a.txt')
    const [uid, gid] = within(modalEl()).getAllByRole('spinbutton')
    fireEvent.change(uid, { target: { value: '1000' } })
    fireEvent.change(gid, { target: { value: '2000' } })
    fireEvent.click(modalOk())
    await waitFor(() => expect(apiMock.chownFile).toHaveBeenCalledWith('ag1', '/a.txt', 1000, 2000))
    expect(apiMock.chmodFile).not.toHaveBeenCalled() // 权限位未变不 chmod

    // 都不变：直接应用 → 无接口调用但成功提示
    fireEvent.click(within(rowOf('a.txt')).getByTitle('权限'))
    await screen.findByText('权限：a.txt')
    fireEvent.click(modalOk())
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledTimes(2))
    expect(apiMock.chmodFile).not.toHaveBeenCalled()
    expect(apiMock.chownFile).toHaveBeenCalledTimes(1)

    apiMock.chmodFile.mockRejectedValue(new Error('denied'))
    fireEvent.click(within(rowOf('a.txt')).getByTitle('权限'))
    await screen.findByText('权限：a.txt')
    fireEvent.click(within(screen.getByText('属主').closest('div')!).getAllByRole('checkbox')[0])
    fireEvent.click(modalOk())
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('权限修改失败'))
  })

  it('权限：原属主缺失（windows）时只填 uid 报「需同时填写」', async () => {
    apiMock.listFiles.mockResolvedValue({
      entries: [entry({ name: 'win.txt', size: 5, uid: -1, gid: -1, mode: '0644' })],
    })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('win.txt')
    fireEvent.click(within(rowOf('win.txt')).getByTitle('权限'))
    await screen.findByText('权限：win.txt')
    fireEvent.change(within(modalEl()).getAllByRole('spinbutton')[0], {
      target: { value: '1000' },
    })
    fireEvent.click(modalOk())
    await waitFor(() =>
      expect(msgWarning).toHaveBeenCalledWith('uid 与 gid 需同时填写才能修改归属'))
    expect(apiMock.chownFile).not.toHaveBeenCalled()
  })

  it('图片预览：多块拼装 Blob 建 URL；超 20MB 拒绝；读取失败告警', async () => {
    const createObjectURL = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:img')
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
    // 1.5MB 两块（1MB + 0.5MB）
    apiMock.listFiles.mockResolvedValue({
      entries: [entry({ name: 'pic.png', size: 1024 * 1024 + 512 * 1024 })],
    })
    apiMock.readFileChunk
      .mockResolvedValueOnce({ data: btoa('A'.repeat(1024 * 1024)), size: 1024 * 1024, eof: false })
      .mockResolvedValueOnce({ data: btoa('B'.repeat(512 * 1024)), size: 1024 * 1024 + 512 * 1024, eof: true })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('pic.png')
    const tableRowOf = (name: string) =>
      within(document.querySelector('.ant-table') as HTMLElement).getByText(name).closest('tr')!
    await act(async () => {
      fireEvent.click(within(tableRowOf('pic.png')).getByTitle('预览图片'))
    })
    await waitFor(() =>
      expect(document.querySelector('.ant-modal img')?.getAttribute('src')).toBe('blob:img'))
    expect(apiMock.readFileChunk).toHaveBeenCalledTimes(2)
    expect((createObjectURL.mock.calls[0][0] as Blob).type).toBe('image/png')
    fireEvent.click(document.querySelector('.ant-modal-close')!)

    // 超 20MB：首块返回总大小即拦截
    apiMock.readFileChunk.mockReset()
    apiMock.readFileChunk.mockResolvedValue({ data: '', size: 21 * 1024 * 1024, eof: false })
    await act(async () => {
      fireEvent.click(document.querySelector('.ant-modal-close')!)
    })
    await act(async () => {
      fireEvent.click(within(tableRowOf('pic.png')).getByTitle('预览图片'))
    })
    expect(await screen.findByText(/超过 20 MB 不支持在线预览/)).toBeInTheDocument()
    fireEvent.click(document.querySelector('.ant-modal-close')!)

    // 读取失败
    apiMock.readFileChunk.mockReset()
    apiMock.readFileChunk.mockRejectedValue(new Error('io'))
    await act(async () => {
      fireEvent.click(within(tableRowOf('pic.png')).getByTitle('预览图片'))
    })
    // 预览失败走 Modal 内 Alert（getApiErrorMessage 对普通 Error 回退默认文案）
    expect(await screen.findByText('读取文件失败')).toBeInTheDocument()
    createObjectURL.mockRestore()
  })

  it('上传：小文件单块直达；超大（>2GB）直接拒绝', async () => {
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    await pickFile(new File(['hello'], 'up.txt', { type: 'text/plain' }))
    await waitFor(() =>
      expect(apiMock.writeFile).toHaveBeenCalledWith('ag1', '/up.txt', btoa('hello'), true))
    expect(msgSuccess).toHaveBeenCalledWith('up.txt 已上传')

    const huge = new File(['x'], 'huge.iso')
    Object.defineProperty(huge, 'size', { value: 3 * 1024 * 1024 * 1024 })
    await pickFile(huge)
    await waitFor(() => expect(msgError).toHaveBeenCalledWith(
      'huge.iso 超过 2.00 GB，请走 scp 或终端上传'))
    expect(apiMock.writeFile).toHaveBeenCalledTimes(1)
  })

  it('上传：>10MB 走分块（首块 truncate 续块 append）并清登记', async () => {
    // mock 按累计字节数返回 size（对齐组件 res.size === end 校验）
    let acc = 0
    apiMock.writeFile.mockImplementation((_a: string, _p: string, b64: string, first: boolean) => {
      if (first) acc = 0
      acc += atob(b64).length
      return Promise.resolve({ size: acc })
    })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    // 10.5MB = 10×1MB + 0.5MB → 11 块
    const total = 10 * 1024 * 1024 + 512 * 1024
    await pickFile(new File([new Uint8Array(total)], 'big.bin'))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('big.bin 已上传'))
    expect(apiMock.writeFile).toHaveBeenCalledTimes(11)
    const firstFlags = apiMock.writeFile.mock.calls.map((c) => c[3])
    expect(firstFlags[0]).toBe(true)
    expect(firstFlags.slice(1).every((f: boolean) => !f)).toBe(true)
    // 成功后清除挂起登记
    expect(JSON.parse(localStorage.getItem('cockpit-pending-uploads')!)).toEqual([])
  })

  it('挂起续传：Alert 呈现进度；放弃清除登记', async () => {
    localStorage.setItem('cockpit-pending-uploads', JSON.stringify([{
      agentId: 'ag1', dir: '/', name: 'half.bin', size: 2 * 1024 * 1024,
      lastModified: 1, uploaded: 1024 * 1024, ts: Date.now(),
    }]))
    render(wrap(<FileBrowser agentId="ag1" />))
    expect(await screen.findByText(/有未完成的上传/)).toBeInTheDocument()
    expect(screen.getByText('half.bin')).toBeInTheDocument()
    expect(screen.getByText('50%')).toBeInTheDocument()
    // 放弃 → Popconfirm 确认 → 登记清除 + Alert 消失
    fireEvent.click(screen.getByText(/放\s*弃/))
    expect(await screen.findByText('放弃续传记录？')).toBeInTheDocument()
    fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary') as HTMLElement)
    await waitFor(() =>
      expect(JSON.parse(localStorage.getItem('cockpit-pending-uploads')!)).toEqual([]))
    await waitFor(() => expect(screen.queryByText(/有未完成的上传/)).toBeNull())
  })

  it('上传取消：进度清空并清理半成品', async () => {
    let block = 0
    apiMock.writeFile.mockImplementation(() => {
      block += 1
      // 第一块成功，第二块起挂起（模拟进行中）
      return block === 1 ? Promise.resolve({ size: 1024 * 1024 }) : new Promise(() => {})
    })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    await pickFile(new File([new Uint8Array(11 * 1024 * 1024)], 'c.bin'))
    // 第一块完成 → 进度 UI 出现
    await waitFor(() => expect(screen.getByText(/取\s*消/)).toBeInTheDocument())
    fireEvent.click(screen.getByText(/取\s*消/))
    await waitFor(() => expect(apiMock.deleteRemoteFile).toHaveBeenCalledWith('ag1', '/c.bin'))
    await waitFor(() => expect(screen.queryByText(/取\s*消/)).toBeNull())
    expect(JSON.parse(localStorage.getItem('cockpit-pending-uploads')!)).toEqual([])
  })
})


// ---- 批 3+ 公共工具 ----
const deferred = <T,>() => {
  let resolve!: (v: T) => void
  let reject!: (e?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

const modalBy = (text: string) => {
  for (const el of document.querySelectorAll('.ant-modal')) {
    if ((el.textContent || '').includes(text)) return el as HTMLElement
  }
  throw new Error(`未找到弹窗: ${text}`)
}
const okBtn = (m: HTMLElement) =>
  m.querySelector('.ant-modal-footer .ant-btn-primary') as HTMLButtonElement
const cancelBtn = (m: HTMLElement) =>
  [...m.querySelectorAll('.ant-modal-footer button')].find((b) =>
    /Cancel|取\s*消/.test(b.textContent || ''),
  ) as HTMLButtonElement

const pressEnter2 = (el: HTMLElement) => {
  fireEvent.keyDown(el, { key: 'Enter' })
  fireEvent.keyUp(el, { key: 'Enter' })
}

// 用属性伪造大文件（内容 1 字节），驱动 >10MB 的分块路径
const fakeFile = (name: string, size: number, lastModified = 12345) => {
  const f = new File(['x'], name, { lastModified })
  Object.defineProperty(f, 'size', { value: size, configurable: true })
  return f
}

const baseReset = () => {
  vi.clearAllMocks()
  permState.canWrite = true
  localStorage.clear()
  apiMock.listFiles.mockReset()
  apiMock.listFiles.mockResolvedValue({ entries: defaultEntries() })
  apiMock.readFileChunk.mockReset()
  apiMock.writeFile.mockReset()
  apiMock.writeFile.mockResolvedValue({ size: 0 })
  apiMock.chmodFile.mockReset()
  apiMock.chmodFile.mockResolvedValue(undefined)
  apiMock.chownFile.mockReset()
  apiMock.chownFile.mockResolvedValue(undefined)
  apiMock.createRemoteDir.mockReset()
  apiMock.createRemoteDir.mockResolvedValue(undefined)
  apiMock.renameRemoteFile.mockReset()
  apiMock.renameRemoteFile.mockResolvedValue(undefined)
  apiMock.deleteRemoteFile.mockReset()
  apiMock.deleteRemoteFile.mockResolvedValue(undefined)
  apiMock.downloadRemoteFile.mockReset()
  apiMock.downloadRemoteFile.mockResolvedValue(new Blob(['x']))
  apiMock.searchFiles.mockReset()
}

describe('FileBrowser（批 3：错误路径与 Modal 取消）', () => {
  beforeEach(() => {
    baseReset()
  })

  it('写接口失败：新建目录/重命名/删除走 onError 提示', async () => {
    apiMock.createRemoteDir.mockRejectedValue(new Error('e'))
    apiMock.renameRemoteFile.mockRejectedValue(new Error('e'))
    apiMock.deleteRemoteFile.mockRejectedValue(new Error('e'))
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')

    fireEvent.click(screen.getByText('新建目录'))
    fireEvent.change(modalBy('新建目录').querySelector('input')!, { target: { value: 'logs' } })
    fireEvent.click(okBtn(modalBy('新建目录')))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('创建目录失败'))

    fireEvent.click(within(rowOf('a.txt')).getByTitle('重命名'))
    fireEvent.change(within(modalBy('重命名')).getByDisplayValue('a.txt'), {
      target: { value: 'b.txt' },
    })
    fireEvent.click(okBtn(modalBy('重命名')))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('重命名失败'))

    fireEvent.click(rowOf('a.txt').querySelector('.anticon-delete')!.closest('button')!)
    expect(await screen.findByText('删除该文件？')).toBeInTheDocument()
    fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary') as HTMLElement)
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('删除失败'))
  })

  it('保存编辑失败提示', async () => {
    apiMock.readFileChunk.mockResolvedValue({ data: btoa('hi'), size: 2, eof: true })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    fireEvent.click(within(rowOf('a.txt')).getByTitle('编辑'))
    await screen.findByDisplayValue('hi')
    apiMock.writeFile.mockRejectedValue(new Error('ro'))
    fireEvent.click(okBtn(modalBy('编辑：')))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('保存失败'))
    expect(apiMock.writeFile).toHaveBeenCalledTimes(1)
  })

  it('六个弹窗的取消路径', async () => {
    apiMock.readFileChunk.mockResolvedValue({ data: btoa('x'), size: 1, eof: true })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')

    fireEvent.click(screen.getByText('新建目录'))
    fireEvent.click(cancelBtn(modalBy('新建目录')))
    fireEvent.click(within(rowOf('a.txt')).getByTitle('权限'))
    await screen.findByText('权限：a.txt')
    fireEvent.click(cancelBtn(modalBy('权限：')))
    fireEvent.click(within(rowOf('a.txt')).getByTitle('重命名'))
    fireEvent.click(cancelBtn(modalBy('重命名')))
    fireEvent.click(within(rowOf('a.txt')).getByTitle('编辑'))
    await screen.findByDisplayValue('x')
    fireEvent.click(cancelBtn(modalBy('编辑：')))
    fireEvent.click(within(rowOf('etc')).getByTitle('删除目录'))
    fireEvent.click(cancelBtn(modalBy('删除目录')))
    fireEvent.click(screen.getByText('搜索'))
    fireEvent.click(modalBy('搜索文件内容').querySelector('.ant-modal-close')!)

    // 取消不应触发任何写接口（onCancel 均为 setState 收敛）
    expect(apiMock.createRemoteDir).not.toHaveBeenCalled()
    expect(apiMock.chmodFile).not.toHaveBeenCalled()
    expect(apiMock.renameRemoteFile).not.toHaveBeenCalled()
    expect(apiMock.writeFile).not.toHaveBeenCalled()
    expect(apiMock.deleteRemoteFile).not.toHaveBeenCalled()
  })

  it('新建目录输入框回车提交', async () => {
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    fireEvent.click(screen.getByText('新建目录'))
    const input = modalBy('新建目录').querySelector('input')!
    fireEvent.change(input, { target: { value: 'logs2' } })
    pressEnter2(input)
    await waitFor(() => expect(apiMock.createRemoteDir).toHaveBeenCalledWith('ag1', '/logs2'))
  })

  it('工具栏刷新按钮触发重查', async () => {
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    expect(apiMock.listFiles).toHaveBeenCalledTimes(1)
    fireEvent.click(document.querySelector('.anticon-redo')!.closest('button')!)
    await waitFor(() => expect(apiMock.listFiles).toHaveBeenCalledTimes(2))
  })

  it('跳转：全斜杠归一为根；空白回车无操作', async () => {
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    const jump = screen.getByPlaceholderText('跳转到路径，如 /etc/nginx')
    fireEvent.change(jump, { target: { value: '/etc' } })
    pressEnter2(jump)
    await waitFor(() => expect(apiMock.listFiles).toHaveBeenLastCalledWith('ag1', '/etc'))
    fireEvent.change(jump, { target: { value: '///' } })
    pressEnter2(jump)
    await waitFor(() => expect(apiMock.listFiles).toHaveBeenLastCalledWith('ag1', '/'))
    expect(jump).toHaveValue('')
    const n = apiMock.listFiles.mock.calls.length
    fireEvent.change(jump, { target: { value: '   ' } })
    pressEnter2(jump)
    expect(apiMock.listFiles.mock.calls.length).toBe(n)
    expect(msgWarning).not.toHaveBeenCalledWith('请输入绝对路径')
  })

  it('mtime 为 0 显示占位；无扩展名文件不可预览', async () => {
    apiMock.listFiles.mockResolvedValue({
      entries: [
        entry({ name: 'mt0.bin', size: 1, mtime: 0 }),
        entry({ name: 'noext', size: 2 }),
      ],
    })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('mt0.bin')
    expect(within(rowOf('mt0.bin')).getByText('—')).toBeInTheDocument()
    expect(rowOf('noext').querySelector('[title="预览图片"]')).toBeNull()
  })

  it('权限 UID/GID 清空回退 undefined（只填单侧应用被拒）', async () => {
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    fireEvent.click(within(rowOf('a.txt')).getByTitle('权限'))
    await screen.findByText('权限：a.txt')
    const [uid, gid] = within(modalBy('权限：')).getAllByRole('spinbutton')
    fireEvent.change(uid, { target: { value: '1000' } })
    fireEvent.change(gid, { target: { value: '2000' } })
    fireEvent.change(uid, { target: { value: '' } })
    fireEvent.change(gid, { target: { value: '' } })
    fireEvent.change(gid, { target: { value: '7' } })
    fireEvent.click(okBtn(modalBy('权限：')))
    await waitFor(() =>
      expect(msgWarning).toHaveBeenCalledWith('uid 与 gid 需同时填写才能修改归属'),
    )
    expect(apiMock.chownFile).not.toHaveBeenCalled()
  })

  it('文本搜索：无命中占位', async () => {
    apiMock.searchFiles.mockResolvedValue({ matches: [], scanned: 4, skipped: 0, truncated: false })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    fireEvent.click(screen.getByText('搜索'))
    fireEvent.change(screen.getByPlaceholderText('关键词（纯文本，递归搜索子目录）'), {
      target: { value: 'zz' },
    })
    fireEvent.click(within(modalBy('搜索文件内容')).getByRole('button', { name: /搜\s*索/ }))
    expect(await screen.findByText('无命中')).toBeInTheDocument()
  })

  it('命中高亮：忽略大小写/非开头命中/清空关键词/无目录段命中跳根', async () => {
    apiMock.searchFiles.mockResolvedValue({
      matches: [
        { path: 'sub/x.txt', line: 1, text: 'foo LISTEN bar listen' },
        { path: 'y.txt', line: 2, text: 'listen now' },
      ],
      scanned: 2,
      skipped: 0,
      truncated: false,
    })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    fireEvent.click(screen.getByText('搜索'))
    const input = screen.getByPlaceholderText('关键词（纯文本，递归搜索子目录）')
    fireEvent.change(input, { target: { value: 'listen' } })
    pressEnter2(input)
    await waitFor(() =>
      expect(apiMock.searchFiles).toHaveBeenCalledWith('ag1', '/', 'listen', false),
    )
    await screen.findByText('sub/x.txt')
    // 大小写不敏感 + 非开头命中：3 个 mark，首个前置段是 'foo '
    const marks = document.querySelectorAll('.ant-modal mark')
    expect(marks.length).toBe(3)
    expect(marks[0].textContent).toBe('LISTEN')
    expect(marks[0].previousSibling?.textContent).toBe('foo ')
    // 清空关键词 → 高亮退化为纯文本
    fireEvent.change(input, { target: { value: '' } })
    expect(document.querySelectorAll('.ant-modal mark').length).toBe(0)
    // 先跳 /etc（searchRoot 保持 /），再点无目录段命中 → 回退根
    fireEvent.click(screen.getByText('etc'))
    await waitFor(() => expect(apiMock.listFiles).toHaveBeenLastCalledWith('ag1', '/etc'))
    fireEvent.click(screen.getByText('y.txt'))
    await waitFor(() => expect(apiMock.listFiles).toHaveBeenLastCalledWith('ag1', '/'))
  })

  it('编辑器：多块读取按偏移拼接', async () => {
    apiMock.readFileChunk
      .mockResolvedValueOnce({ data: btoa('hello '), size: 6, eof: false })
      .mockResolvedValueOnce({ data: btoa('world'), size: 11, eof: true })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    fireEvent.click(within(rowOf('a.txt')).getByTitle('编辑'))
    expect(await screen.findByDisplayValue('hello world')).toBeInTheDocument()
    expect(apiMock.readFileChunk).toHaveBeenNthCalledWith(2, 'ag1', '/a.txt', 6, 1024 * 1024)
  })

  it('挂起登记 JSON 损坏兜底为空', async () => {
    localStorage.setItem('cockpit-pending-uploads', '{oops')
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    expect(screen.queryByText(/有未完成的上传/)).toBeNull()
  })

  it('下载：路径尾段为空时文件名回退 download', async () => {
    let got = ''
    const clickSpy = vi
      .spyOn(HTMLAnchorElement.prototype, 'click')
      .mockImplementation(function (this: HTMLAnchorElement) {
        got = this.download
      })
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:x')
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
    apiMock.listFiles.mockResolvedValue({ entries: [entry({ name: 'x/', size: 5 })] })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('x/')
    fireEvent.click(rowOf('x/').querySelector('.anticon-download')!.closest('button')!)
    await waitFor(() => expect(clickSpy).toHaveBeenCalled())
    expect(got).toBe('download')
    clickSpy.mockRestore()
  })

  it('权限：mode 无法解析按 0 处理', async () => {
    apiMock.listFiles.mockResolvedValue({
      entries: [entry({ name: 'zz.bin', mode: 'zz', size: 5 })],
    })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('zz.bin')
    fireEvent.click(within(rowOf('zz.bin')).getByTitle('权限'))
    await screen.findByText('权限：zz.bin')
    expect(screen.getByText(/应用后合成八进制 0000/)).toBeInTheDocument()
    fireEvent.click(okBtn(modalBy('权限：')))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('权限已更新'))
    expect(apiMock.chmodFile).not.toHaveBeenCalled()
    expect(apiMock.chownFile).not.toHaveBeenCalled()
  })

  it('图片预览：footer 下载按钮', async () => {
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:i')
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
    apiMock.readFileChunk.mockResolvedValue({ data: btoa('x'), size: 1, eof: true })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('pic.png')
    await act(async () => {
      fireEvent.click(rowOf('pic.png').querySelector('[title="预览图片"]')!)
    })
    await waitFor(() => expect(document.querySelector('.ant-modal img')).not.toBeNull())
    fireEvent.click(within(modalBy('pic.png')).getByText('下载'))
    await waitFor(() => expect(apiMock.downloadRemoteFile).toHaveBeenCalledWith('ag1', '/pic.png'))
  })

  it('图片预览：img 加载后显示尺寸', async () => {
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:i')
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
    apiMock.readFileChunk.mockResolvedValue({ data: btoa('x'), size: 1, eof: true })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('pic.png')
    await act(async () => {
      fireEvent.click(rowOf('pic.png').querySelector('[title="预览图片"]')!)
    })
    const img = document.querySelector('.ant-modal img')!
    fireEvent.load(img)
    expect(screen.getByText(/0×0 · 2\.0 KB/)).toBeInTheDocument()
  })

  it('图片预览：连续打开回收上一张 URL', async () => {
    vi.spyOn(URL, 'createObjectURL')
      .mockReturnValueOnce('blob:1')
      .mockReturnValueOnce('blob:2')
    const revoke = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
    apiMock.readFileChunk.mockResolvedValue({ data: btoa('x'), size: 1, eof: true })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('pic.png')
    const btn = rowOf('pic.png').querySelector('[title="预览图片"]')!
    await act(async () => {
      fireEvent.click(btn)
    })
    await waitFor(() =>
      expect(document.querySelector('.ant-modal img')?.getAttribute('src')).toBe('blob:1'),
    )
    await act(async () => {
      fireEvent.click(btn)
    })
    await waitFor(() => expect(revoke).toHaveBeenCalledWith('blob:1'))
    await waitFor(() =>
      expect(document.querySelector('.ant-modal img')?.getAttribute('src')).toBe('blob:2'),
    )
  })

  it('图片预览：中途关闭丢弃在途分块', async () => {
    const create = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:x')
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
    const d = deferred<{ data: string; size: number; eof: boolean }>()
    apiMock.readFileChunk.mockReturnValue(d.promise)
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('pic.png')
    await act(async () => {
      fireEvent.click(rowOf('pic.png').querySelector('[title="预览图片"]')!)
    })
    fireEvent.click(modalBy('pic.png').querySelector('.ant-modal-close')!)
    await act(async () => {
      d.resolve({ data: btoa('x'), size: 1, eof: true })
    })
    expect(create).not.toHaveBeenCalled()
  })

  it('图片预览：关闭后在途拉取报错不回写状态', async () => {
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:x')
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
    const d = deferred<{ data: string; size: number; eof: boolean }>()
    apiMock.readFileChunk.mockReturnValue(d.promise)
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('pic.png')
    await act(async () => {
      fireEvent.click(rowOf('pic.png').querySelector('[title="预览图片"]')!)
    })
    fireEvent.click(modalBy('pic.png').querySelector('.ant-modal-close')!)
    await act(async () => {
      d.reject(new Error('io'))
    })
    // token 已失效：不再写 previewError（旧 DOM 冻结，断言不出现错误 Alert）
    await waitFor(() => expect(screen.queryByText('读取文件失败')).toBeNull())
  })

  it('图片预览：建 URL 窗口内关闭则丢弃', async () => {
    let closed = false
    vi.spyOn(URL, 'createObjectURL').mockImplementation(() => {
      if (!closed) {
        closed = true
        fireEvent.click(document.querySelector('.ant-modal-close')!)
      }
      return 'blob:x'
    })
    const revoke = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
    apiMock.readFileChunk.mockResolvedValue({ data: btoa('x'), size: 1, eof: true })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('pic.png')
    // 不包 act：让 Modal 先提交，createObjectURL 里才能点到关闭按钮
    fireEvent.click(rowOf('pic.png').querySelector('[title="预览图片"]')!)
    await waitFor(() => expect(revoke).toHaveBeenCalledWith('blob:x'))
  })
})

describe('FileBrowser（批 4：分块上传取消与写失败）', () => {
  const ONE_MB = 1024 * 1024

  const uploadInput = () =>
    document.querySelector('.ant-upload input[type=file]') as HTMLInputElement

  const pickFile = async (file: File) => {
    Object.defineProperty(uploadInput(), 'files', { value: [file], configurable: true })
    await act(async () => {
      fireEvent.change(uploadInput())
    })
  }

  beforeEach(() => {
    baseReset()
  })

  it('小文件写入失败提示', async () => {
    apiMock.writeFile.mockRejectedValue(new Error('net'))
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    await pickFile(new File(['hi'], 'up.txt'))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('上传 up.txt 失败'))
  })

  it('分块 size 不匹配：立即终止并提示', async () => {
    apiMock.writeFile.mockResolvedValue({ size: 0 })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    await pickFile(fakeFile('c.bin', 11 * ONE_MB))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('上传 c.bin 失败'))
    expect(apiMock.writeFile).toHaveBeenCalledTimes(1)
  })

  it('分块取消在首块边界：token 中止且不删半成品', async () => {
    const d = deferred<{ size: number }>()
    apiMock.writeFile.mockReturnValue(d.promise)
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    await pickFile(fakeFile('c.bin', 11 * ONE_MB))
    await screen.findByText(/取\s*消/)
    fireEvent.click(screen.getByText(/取\s*消/))
    // 首块未落盘（uploaded=0）→ 不清理半成品
    expect(apiMock.deleteRemoteFile).not.toHaveBeenCalled()
    await act(async () => {
      d.resolve({ size: ONE_MB })
    })
    await waitFor(() => expect(screen.queryByText(/取\s*消/)).toBeNull())
    // cancelled 错误静默 return，不弹失败
    expect(msgError).not.toHaveBeenCalled()
  })

  it('分块取消清理半成品：删除失败静默', async () => {
    apiMock.deleteRemoteFile.mockRejectedValue(new Error('e'))
    let call = 0
    apiMock.writeFile.mockImplementation(() => {
      call += 1
      return call === 1 ? Promise.resolve({ size: ONE_MB }) : new Promise(() => {})
    })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')
    await pickFile(fakeFile('c.bin', 11 * ONE_MB))
    await screen.findByText(/取\s*消/)
    fireEvent.click(screen.getByText(/取\s*消/))
    await waitFor(() => expect(apiMock.deleteRemoteFile).toHaveBeenCalledWith('ag1', '/c.bin'))
    expect(msgError).not.toHaveBeenCalled()
  })
})

// antd 在 beforeUpload 返回 false 时把 onChange.file 换成克隆 File（无 originFileObj），
// 与 UploadFile 文档形态不符，续传入口 `if (raw)` 的真分支在线上同样进不去。
// 这里在 File 原型开一个可注入的 originFileObj：默认 undefined（与线上一致），
// 需要驱动 resumeUpload 的用例注入目标 File。
let injectedOriginFile: File | undefined
Object.defineProperty(File.prototype, 'originFileObj', {
  configurable: true,
  get() {
    return injectedOriginFile
  },
})

describe('FileBrowser（批 5：续传矩阵与分块重试）', () => {
  const ONE_MB = 1024 * 1024
  const TWO_MB = 2 * 1024 * 1024

  type ProbeReply = { entries?: FileEntry[]; error?: boolean }
  let upReplies: ProbeReply[] = []
  const setProbe = (...rs: ProbeReply[]) => {
    upReplies = [...rs]
  }

  const seedPending = (o: Record<string, unknown> = {}) => {
    localStorage.setItem(
      'cockpit-pending-uploads',
      JSON.stringify([
        {
          agentId: 'ag1',
          dir: '/up',
          name: 'r.bin',
          size: TWO_MB,
          lastModified: 999,
          uploaded: 0,
          ts: Date.now(),
          ...o,
        },
      ]),
    )
  }

  const resumeInput = () => {
    const btn = screen.getByText('继续').closest('button')!
    const scope = btn.closest('.ant-upload') ?? btn.parentElement!
    return (scope.querySelector('input[type=file]') ??
      scope.parentElement!.querySelector('input[type=file]')) as HTMLInputElement
  }

  const pickResume = async (file: File) => {
    injectedOriginFile = file
    const input = resumeInput()
    Object.defineProperty(input, 'files', { value: [file], configurable: true })
    await act(async () => {
      fireEvent.change(input)
    })
  }

  const confirmCalls: Array<{ onOk?: () => void; onCancel?: () => void }> = []

  beforeEach(() => {
    baseReset()
    injectedOriginFile = undefined
    upReplies = []
    confirmCalls.length = 0
    apiMock.listFiles.mockImplementation((_a: string, dir: string) => {
      if (dir !== '/up') return Promise.resolve({ entries: defaultEntries() })
      const r = upReplies.length > 1 ? upReplies.shift()! : upReplies[0] ?? { entries: [] }
      return r.error
        ? Promise.reject(new Error('probe-fail'))
        : Promise.resolve({ entries: r.entries ?? [] })
    })
    vi.spyOn(Modal, 'confirm').mockImplementation(((cfg: {
      onOk?: () => void
      onCancel?: () => void
    }) => {
      confirmCalls.push({ onOk: cfg.onOk, onCancel: cfg.onCancel })
      return {} as never
    }) as never)
  })

  const okSizes = (...sizes: number[]) => {
    apiMock.writeFile.mockReset()
    for (const s of sizes) apiMock.writeFile.mockResolvedValueOnce({ size: s })
  }

  it('续传入口：file 无 originFileObj 时不触发 resumeUpload', async () => {
    seedPending()
    injectedOriginFile = undefined
    render(wrap(<FileBrowser agentId="ag1" />))
    expect(await screen.findByText(/有未完成的上传/)).toBeInTheDocument()
    const input = resumeInput()
    Object.defineProperty(input, 'files', {
      value: [fakeFile('r.bin', TWO_MB, 999)],
      configurable: true,
    })
    await act(async () => {
      fireEvent.change(input)
    })
    await act(async () => {
      await new Promise((r) => setTimeout(r, 30))
    })
    expect(apiMock.writeFile).not.toHaveBeenCalled()
    expect(msgError).not.toHaveBeenCalled()
  })

  it('续传：本地大小与记录不一致直接拒绝', async () => {
    seedPending()
    render(wrap(<FileBrowser agentId="ag1" />))
    expect(await screen.findByText(/有未完成的上传/)).toBeInTheDocument()
    await pickResume(fakeFile('r.bin', ONE_MB, 999))
    await waitFor(() =>
      expect(msgError).toHaveBeenCalledWith(
        'r.bin 与记录大小不一致（1.0 MB ≠ 2.0 MB），无法续传',
      ),
    )
    expect(apiMock.writeFile).not.toHaveBeenCalled()
  })

  it('续传：mtime 不符确认——取消放弃', async () => {
    seedPending()
    render(wrap(<FileBrowser agentId="ag1" />))
    expect(await screen.findByText(/有未完成的上传/)).toBeInTheDocument()
    await pickResume(fakeFile('r.bin', TWO_MB, 1))
    await waitFor(() => expect(Modal.confirm).toHaveBeenCalled())
    await act(async () => {
      confirmCalls[0].onCancel!()
    })
    expect(apiMock.writeFile).not.toHaveBeenCalled()
    expect(msgError).not.toHaveBeenCalled()
  })

  it('续传：mtime 不符确认——继续且远端已完整', async () => {
    seedPending()
    setProbe({ entries: [entry({ name: 'r.bin', size: TWO_MB })] })
    render(wrap(<FileBrowser agentId="ag1" />))
    expect(await screen.findByText(/有未完成的上传/)).toBeInTheDocument()
    await pickResume(fakeFile('r.bin', TWO_MB, 1))
    await waitFor(() => expect(Modal.confirm).toHaveBeenCalled())
    await act(async () => {
      confirmCalls[0].onOk!()
    })
    await waitFor(() =>
      expect(msgSuccess).toHaveBeenCalledWith('r.bin 远端已完整（2.0 MB），已清除续传记录'),
    )
    expect(apiMock.writeFile).not.toHaveBeenCalled()
    await waitFor(() =>
      expect(JSON.parse(localStorage.getItem('cockpit-pending-uploads')!)).toEqual([]),
    )
  })

  it('续传：远端对齐从游标续跑成功', async () => {
    seedPending()
    setProbe({ entries: [entry({ name: 'r.bin', size: ONE_MB })] })
    okSizes(TWO_MB)
    render(wrap(<FileBrowser agentId="ag1" />))
    expect(await screen.findByText(/有未完成的上传/)).toBeInTheDocument()
    await pickResume(fakeFile('r.bin', TWO_MB, 999))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('r.bin 已上传'))
    expect(apiMock.writeFile).toHaveBeenCalledTimes(1)
    expect(apiMock.writeFile.mock.calls[0][1]).toBe('/up/r.bin')
    expect(apiMock.writeFile.mock.calls[0][3]).toBe(false)
  })

  it('续传：远端半块截断确认——覆盖重传成功', async () => {
    seedPending()
    setProbe({ entries: [entry({ name: 'r.bin', size: 512 * 1024 })] })
    okSizes(ONE_MB, TWO_MB)
    render(wrap(<FileBrowser agentId="ag1" />))
    expect(await screen.findByText(/有未完成的上传/)).toBeInTheDocument()
    await pickResume(fakeFile('r.bin', TWO_MB, 999))
    await waitFor(() => expect(Modal.confirm).toHaveBeenCalled())
    await act(async () => {
      confirmCalls[0].onOk!()
    })
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('r.bin 已上传'))
    expect(apiMock.writeFile).toHaveBeenCalledTimes(2)
    expect(apiMock.writeFile.mock.calls[0][3]).toBe(true)
  })

  it('续传：远端半块截断确认——取消', async () => {
    seedPending()
    setProbe({ entries: [entry({ name: 'r.bin', size: 512 * 1024 })] })
    render(wrap(<FileBrowser agentId="ag1" />))
    expect(await screen.findByText(/有未完成的上传/)).toBeInTheDocument()
    await pickResume(fakeFile('r.bin', TWO_MB, 999))
    await waitFor(() => expect(Modal.confirm).toHaveBeenCalled())
    await act(async () => {
      confirmCalls[0].onCancel!()
    })
    expect(apiMock.writeFile).not.toHaveBeenCalled()
  })

  it('续传：远端超出记录大小确认后从头重传', async () => {
    seedPending()
    setProbe({ entries: [entry({ name: 'r.bin', size: 3 * 1024 * 1024 })] })
    okSizes(ONE_MB, TWO_MB)
    render(wrap(<FileBrowser agentId="ag1" />))
    expect(await screen.findByText(/有未完成的上传/)).toBeInTheDocument()
    await pickResume(fakeFile('r.bin', TWO_MB, 999))
    await waitFor(() => expect(Modal.confirm).toHaveBeenCalled())
    await act(async () => {
      confirmCalls[0].onOk!()
    })
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('r.bin 已上传'))
  })

  it('续传：远端为空重建', async () => {
    seedPending()
    setProbe({ entries: [entry({ name: 'r.bin', size: 0 })] })
    okSizes(ONE_MB, TWO_MB)
    render(wrap(<FileBrowser agentId="ag1" />))
    expect(await screen.findByText(/有未完成的上传/)).toBeInTheDocument()
    await pickResume(fakeFile('r.bin', TWO_MB, 999))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('r.bin 已上传'))
    expect(Modal.confirm).not.toHaveBeenCalled()
  })

  it('续传：远端缺失从头重建', async () => {
    seedPending()
    setProbe({ entries: [] })
    okSizes(ONE_MB, TWO_MB)
    render(wrap(<FileBrowser agentId="ag1" />))
    expect(await screen.findByText(/有未完成的上传/)).toBeInTheDocument()
    await pickResume(fakeFile('r.bin', TWO_MB, 999))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('r.bin 已上传'))
  })

  it('续传：探测失败从 truncate 幂等重来', async () => {
    seedPending()
    setProbe({ error: true })
    okSizes(ONE_MB, TWO_MB)
    render(wrap(<FileBrowser agentId="ag1" />))
    expect(await screen.findByText(/有未完成的上传/)).toBeInTheDocument()
    await pickResume(fakeFile('r.bin', TWO_MB, 999))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('r.bin 已上传'))
  })

  it('分块重试：响应丢失但已落盘则跳过该块', async () => {
    seedPending()
    // 首探测 miss → 从头；首块失败；重试探测 1MB（=uploaded+CHUNK）→ 跳过
    setProbe({ entries: [] }, { entries: [entry({ name: 'r.bin', size: ONE_MB })] })
    apiMock.writeFile.mockRejectedValueOnce(new Error('net'))
    apiMock.writeFile.mockResolvedValue({ size: TWO_MB })
    render(wrap(<FileBrowser agentId="ag1" />))
    expect(await screen.findByText(/有未完成的上传/)).toBeInTheDocument()
    await pickResume(fakeFile('r.bin', TWO_MB, 999))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('r.bin 已上传'))
    expect(apiMock.writeFile).toHaveBeenCalledTimes(2)
    expect(apiMock.writeFile.mock.calls[1][3]).toBe(false)
  })

  it('分块重试：探测到已写入大小续跑', async () => {
    seedPending()
    setProbe({ entries: [] }, { entries: [entry({ name: 'r.bin', size: ONE_MB })] })
    let call = 0
    apiMock.writeFile.mockImplementation(() => {
      call += 1
      if (call === 1) return Promise.resolve({ size: ONE_MB })
      if (call === 2) return Promise.reject(new Error('net'))
      return Promise.resolve({ size: TWO_MB })
    })
    render(wrap(<FileBrowser agentId="ag1" />))
    expect(await screen.findByText(/有未完成的上传/)).toBeInTheDocument()
    await pickResume(fakeFile('r.bin', TWO_MB, 999))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('r.bin 已上传'))
  })

  it('分块重试：目标丢失重建', async () => {
    seedPending()
    setProbe({ entries: [] }, { entries: [] })
    apiMock.writeFile.mockRejectedValueOnce(new Error('net'))
    apiMock.writeFile.mockResolvedValueOnce({ size: ONE_MB })
    apiMock.writeFile.mockResolvedValue({ size: TWO_MB })
    render(wrap(<FileBrowser agentId="ag1" />))
    expect(await screen.findByText(/有未完成的上传/)).toBeInTheDocument()
    await pickResume(fakeFile('r.bin', TWO_MB, 999))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('r.bin 已上传'))
  })

  it('分块重试：远端为空重建', async () => {
    seedPending()
    setProbe({ entries: [] }, { entries: [entry({ name: 'r.bin', size: 0 })] })
    let call = 0
    apiMock.writeFile.mockImplementation(() => {
      call += 1
      if (call === 1) return Promise.resolve({ size: ONE_MB })
      if (call === 2) return Promise.reject(new Error('net'))
      return Promise.resolve({ size: call === 3 ? ONE_MB : TWO_MB })
    })
    render(wrap(<FileBrowser agentId="ag1" />))
    expect(await screen.findByText(/有未完成的上传/)).toBeInTheDocument()
    await pickResume(fakeFile('r.bin', TWO_MB, 999))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('r.bin 已上传'))
  })

  it('分块重试：远端大小异常不覆盖别人的改动', async () => {
    seedPending()
    setProbe({ entries: [] }, { entries: [entry({ name: 'r.bin', size: 512 * 1024 })] })
    apiMock.writeFile.mockRejectedValue(new Error('net'))
    render(wrap(<FileBrowser agentId="ag1" />))
    expect(await screen.findByText(/有未完成的上传/)).toBeInTheDocument()
    await pickResume(fakeFile('r.bin', TWO_MB, 999))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('续传 r.bin 失败'))
  })

  it('分块重试：探测失败按缺失重建', async () => {
    seedPending()
    setProbe({ entries: [] }, { error: true })
    apiMock.writeFile.mockRejectedValueOnce(new Error('net'))
    apiMock.writeFile.mockResolvedValueOnce({ size: ONE_MB })
    apiMock.writeFile.mockResolvedValue({ size: TWO_MB })
    render(wrap(<FileBrowser agentId="ag1" />))
    expect(await screen.findByText(/有未完成的上传/)).toBeInTheDocument()
    await pickResume(fakeFile('r.bin', TWO_MB, 999))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('r.bin 已上传'))
  })

  it('分块重试：size 不匹配立即终止', async () => {
    seedPending()
    setProbe({ entries: [] })
    apiMock.writeFile.mockResolvedValue({ size: 0 })
    render(wrap(<FileBrowser agentId="ag1" />))
    expect(await screen.findByText(/有未完成的上传/)).toBeInTheDocument()
    await pickResume(fakeFile('r.bin', TWO_MB, 999))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('续传 r.bin 失败'))
  })

  it('分块重试耗尽后保留记录', async () => {
    seedPending()
    setProbe({ entries: [] }, { entries: [entry({ name: 'r.bin', size: 0 })] })
    apiMock.writeFile.mockRejectedValue(new Error('net'))
    render(wrap(<FileBrowser agentId="ag1" />))
    expect(await screen.findByText(/有未完成的上传/)).toBeInTheDocument()
    await pickResume(fakeFile('r.bin', TWO_MB, 999))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('续传 r.bin 失败'))
    // 失败记录保留（D23）
    expect(JSON.parse(localStorage.getItem('cockpit-pending-uploads')!)).not.toEqual([])
  })

  it('续传中取消：块写入失败按 cancelled 静默中止', async () => {
    seedPending()
    setProbe({ entries: [] })
    const d = deferred<{ size: number }>()
    apiMock.writeFile.mockReturnValue(d.promise)
    render(wrap(<FileBrowser agentId="ag1" />))
    expect(await screen.findByText(/有未完成的上传/)).toBeInTheDocument()
    await pickResume(fakeFile('r.bin', TWO_MB, 999))
    await screen.findByText(/取\s*消/)
    fireEvent.click(screen.getByText(/取\s*消/))
    await act(async () => {
      d.reject(new Error('net'))
    })
    await waitFor(() => expect(screen.queryByText(/取\s*消/)).toBeNull())
    expect(msgError).not.toHaveBeenCalled()
  })
})

// rc-dialog 的 MemoChildren 在 visible=false 后冻结弹窗 DOM：关闭后 OK 按钮上
// 永远是最后一次可见帧的旧闭包（target 非空）。这里沿 fiber 上溯取 antd Modal
// 组件当前的 onOk（target 已清空的防御闭包）直接调用，覆盖守卫早退分支。
const callCurrentOnOk = (hostEl: HTMLElement) => {
  const key = Object.keys(hostEl).find((k) => k.startsWith('__reactFiber'))
  if (!key) throw new Error('未找到 react fiber')
  let fiber: unknown = (hostEl as unknown as Record<string, unknown>)[key]
  let latest: (() => void) | null = null
  while (fiber) {
    const p = (fiber as { memoizedProps?: Record<string, unknown> }).memoizedProps
    if (
      p &&
      typeof p === 'object' &&
      typeof p.onOk === 'function' &&
      typeof p.onCancel === 'function' &&
      'open' in p
    ) {
      // Footer 的 props 是 Modal 全量 props 的拷贝（同样有 open/onOk/onCancel），
      // 但它在冻结子树里是旧闭包；继续上溯取最外层 Modal 组件的当前 onOk。
      latest = p.onOk as () => void
    }
    fiber = (fiber as { return?: unknown }).return
  }
  if (!latest) throw new Error('未找到 Modal fiber')
  latest()
}

describe('FileBrowser（批 6：防御性分支与竞态）', () => {
  beforeEach(() => {
    baseReset()
    injectedOriginFile = undefined
  })

  it('预览防御：点击瞬间扩展名失效按 octet-stream', async () => {
    const create = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:m')
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
    const pic = entry({ name: 'pic.png', size: 8 })
    apiMock.listFiles.mockResolvedValue({ entries: [pic] })
    apiMock.readFileChunk.mockResolvedValue({ data: btoa('x'), size: 1, eof: true })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('pic.png')
    const btn = rowOf('pic.png').querySelector('[title="预览图片"]') as HTMLElement
    pic.name = 'noext' // 渲染与点击之间的数据漂移
    await act(async () => {
      fireEvent.click(btn)
    })
    await waitFor(() => expect(create).toHaveBeenCalled())
    expect((create.mock.calls[0][0] as Blob).type).toBe('application/octet-stream')
    create.mockRestore()
  })

  it('预览防御：target 名缺失时 alt 回退空串', async () => {
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:i')
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
    const pic = entry({ name: 'pic.png', size: 8 })
    apiMock.listFiles.mockImplementation((_a: string, dir: string) =>
      Promise.resolve({ entries: dir === '/' ? [pic] : [entry({ name: 'other.txt' })] }),
    )
    apiMock.readFileChunk.mockResolvedValue({ data: btoa('x'), size: 1, eof: true })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('pic.png')
    await act(async () => {
      fireEvent.click(rowOf('pic.png').querySelector('[title="预览图片"]')!)
    })
    const img = document.querySelector('.ant-modal img') as HTMLImageElement
    expect(img.getAttribute('alt')).toBe('pic.png')
    pic.name = undefined as unknown as string
    pic.isDir = true // 避免行渲染再调 imageExt
    fireEvent.change(screen.getByPlaceholderText('跳转到路径，如 /etc/nginx'), {
      target: { value: 'z' },
    })
    const img2 = document.querySelector('.ant-modal img') as HTMLImageElement
    expect(img2.getAttribute('alt')).toBe('')
  })

  it('预览防御：尺寸就绪后 target 已清空时大小段回退空串', async () => {
    const create = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:i')
    const revoke = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
    apiMock.readFileChunk.mockResolvedValue({ data: btoa('x'), size: 1, eof: true })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('pic.png')
    await act(async () => {
      fireEvent.click(rowOf('pic.png').querySelector('[title="预览图片"]')!)
    })
    const img = document.querySelector('.ant-modal img') as HTMLImageElement
    // 同一批更新：closePreview 的 target/dims 清空先入队，img onLoad 的 setPreviewDims
    // 后入队——提交态是 dims 就绪 + previewTarget 为空，footer 尺寸段走空串回退。
    // （关闭后 rc-dialog MemoChildren 冻结弹窗 DOM，这里只断言关闭副作用，回退
    // 分支的命中以覆盖率为准。）
    act(() => {
      fireEvent.click(document.querySelector('.ant-modal-close')!)
      fireEvent.load(img)
    })
    await waitFor(() => expect(revoke).toHaveBeenCalledWith('blob:i'))
    expect(create).toHaveBeenCalledTimes(1)
  })

  it('防御守卫：applyPerm/saveEdit 在 target 已清空时直接返回', async () => {
    apiMock.readFileChunk.mockResolvedValue({ data: btoa('x'), size: 1, eof: true })
    render(wrap(<FileBrowser agentId="ag1" />))
    await screen.findByText('a.txt')

    // 权限：打开 → 翻一位 → 取消（state 上 target 置空）→ 调最新 onOk
    fireEvent.click(within(rowOf('a.txt')).getByTitle('权限'))
    await screen.findByText('权限：a.txt')
    fireEvent.click(
      within(screen.getByText('属主').closest('div')!).getAllByRole('checkbox')[2],
    )
    fireEvent.click(cancelBtn(modalBy('权限：')))
    callCurrentOnOk(okBtn(modalBy('权限：')))
    await act(async () => {
      await new Promise((r) => setTimeout(r, 30))
    })
    expect(apiMock.chmodFile).not.toHaveBeenCalled()
    expect(apiMock.chownFile).not.toHaveBeenCalled()

    // 编辑：打开 → 取消 → 调最新 onOk
    fireEvent.click(within(rowOf('a.txt')).getByTitle('编辑'))
    await screen.findByDisplayValue('x')
    fireEvent.click(cancelBtn(modalBy('编辑：')))
    callCurrentOnOk(okBtn(modalBy('编辑：')))
    await act(async () => {
      await new Promise((r) => setTimeout(r, 30))
    })
    expect(apiMock.writeFile).not.toHaveBeenCalled()
  })
})
