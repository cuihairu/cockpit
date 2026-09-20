import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
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
    expect(await screen.findByText(/目录不可访问/, undefined, { timeout: 4000 })).toBeInTheDocument()
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
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('big.bin 已上传'), { timeout: 10000 })
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
    await waitFor(() => expect(screen.getByText(/取\s*消/)).toBeInTheDocument(), { timeout: 5000 })
    fireEvent.click(screen.getByText(/取\s*消/))
    await waitFor(() => expect(apiMock.deleteRemoteFile).toHaveBeenCalledWith('ag1', '/c.bin'))
    await waitFor(() => expect(screen.queryByText(/取\s*消/)).toBeNull())
    expect(JSON.parse(localStorage.getItem('cockpit-pending-uploads')!)).toEqual([])
  })
})
