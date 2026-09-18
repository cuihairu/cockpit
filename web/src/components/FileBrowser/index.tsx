import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  Breadcrumb,
  Button,
  Empty,
  Form,
  Input,
  Modal,
  Popconfirm,
  Progress,
  Space,
  Spin,
  Switch,
  Table,
  Tag,
  Typography,
  Upload,
  message,
} from 'antd'
import type { UploadFile } from 'antd'
import {
  DeleteOutlined,
  DownloadOutlined,
  EditOutlined,
  EyeOutlined,
  FileOutlined,
  FolderAddOutlined,
  FolderOutlined,
  LinkOutlined,
  RedoOutlined,
  SearchOutlined,
  SwapOutlined,
  UploadOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { api } from '@/services/api'
import type { FileEntry, FileSearchResult } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'

const EDIT_MAX_BYTES = 1024 * 1024 // 编辑器只允许 ≤1MB 文本（与 server 校验一致）
// 上传（file-manager-design.md M3 D15-D17）：≤10MB 单块快速路径；更大走分块
// （单块 1MB = agent fileWriteChunkLimit 满额，首块 truncate 建文件续块 append）
const UPLOAD_MAX_BYTES = 10 * 1024 * 1024
const UPLOAD_CHUNK_MAX_BYTES = 2 * 1024 * 1024 * 1024
const UPLOAD_CHUNK_BYTES = 1024 * 1024
// 图片预览（file-manager-design.md M2 D14）：白名单扩展名 + 20MB 上限
const PREVIEW_MAX_BYTES = 20 * 1024 * 1024
const PREVIEW_CHUNK_BYTES = 1024 * 1024 // agent file.read 单块上限
const IMAGE_MIME: Record<string, string> = {
  jpg: 'image/jpeg',
  jpeg: 'image/jpeg',
  png: 'image/png',
  gif: 'image/gif',
  webp: 'image/webp',
  bmp: 'image/bmp',
  svg: 'image/svg+xml',
  ico: 'image/x-icon',
  avif: 'image/avif',
}

const imageExt = (name: string): string | null => {
  const idx = name.lastIndexOf('.')
  if (idx < 0) return null
  const ext = name.slice(idx + 1).toLowerCase()
  return IMAGE_MIME[ext] ? ext : null
}

const formatBytes = (n: number): string => {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`
}

// 路径 → 面包屑段（绝对路径按 / 切分）
const pathSegments = (p: string): Array<{ label: string; path: string }> => {
  const segs = [{ label: '/', path: '/' }]
  let acc = ''
  for (const part of p.split('/').filter(Boolean)) {
    acc += `/${part}`
    segs.push({ label: part, path: acc })
  }
  return segs
}

const joinPath = (dir: string, name: string) =>
  dir === '/' ? `/${name}` : `${dir}/${name}`

// 命中行关键词高亮（大小写按搜索模式切分，纯展示不递归）
const HighlightText = ({ text, query, caseSensitive }: { text: string; query: string; caseSensitive: boolean }) => {
  if (!query) return <>{text}</>
  const needle = caseSensitive ? query : query.toLowerCase()
  const parts: React.ReactNode[] = []
  let rest = text
  let key = 0
  while (rest) {
    const h = caseSensitive ? rest : rest.toLowerCase()
    const idx = h.indexOf(needle)
    if (idx < 0) {
      parts.push(rest)
      break
    }
    if (idx > 0) parts.push(rest.slice(0, idx))
    parts.push(
      <mark key={key++} style={{ background: '#613400', color: '#ffd666', padding: '0 1px' }}>
        {rest.slice(idx, idx + needle.length)}
      </mark>,
    )
    rest = rest.slice(idx + needle.length)
  }
  return <>{parts}</>
}

const toBase64 = (bytes: Uint8Array): string => {
  let bin = ''
  const chunk = 0x8000
  for (let i = 0; i < bytes.length; i += chunk) {
    bin += String.fromCharCode(...bytes.subarray(i, i + chunk))
  }
  return btoa(bin)
}

// 读完整文本文件（≤EDIT_MAX_BYTES），超出拒绝——编辑器场景不需要大文件
const readFullText = async (agentId: string, path: string): Promise<string> => {
  let offset = 0
  let out = ''
  for (;;) {
    const chunk = await api.readFileChunk(agentId, path, offset, EDIT_MAX_BYTES)
    if (chunk.size > EDIT_MAX_BYTES) {
      throw new Error(`文件超过 ${formatBytes(EDIT_MAX_BYTES)}，不支持在线编辑，请下载后本地编辑`)
    }
    const bytes = Uint8Array.from(atob(chunk.data), (c) => c.charCodeAt(0))
    out += new TextDecoder().decode(bytes)
    if (chunk.eof) return out
    offset = chunk.size
  }
}

// 远程文件浏览器：按 agent 作用域，浏览/编辑/上传/下载/重命名/删除
// （见 docs/guide/file-manager-design.md；全文件系统可见，安全靠双端路径校验）
const FileBrowser = ({ agentId }: { agentId: string }) => {
  const queryClient = useQueryClient()
  const [cwd, setCwd] = useState('/')
  const [jump, setJump] = useState('')
  const [mkdirOpen, setMkdirOpen] = useState(false)
  const [renameTarget, setRenameTarget] = useState<FileEntry | null>(null)
  const [editTarget, setEditTarget] = useState<FileEntry | null>(null)
  const [editContent, setEditContent] = useState('')
  const [editSaving, setEditSaving] = useState(false)
  const [deleteDir, setDeleteDir] = useState<FileEntry | null>(null)
  const [deleteConfirm, setDeleteConfirm] = useState('')
  // 文本搜索（file-manager-design.md M2 D13）：以打开时的目录为根
  const [searchOpen, setSearchOpen] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const [searchCase, setSearchCase] = useState(false)
  const [searching, setSearching] = useState(false)
  const [searchResult, setSearchResult] = useState<FileSearchResult | null>(null)
  const [searchRoot, setSearchRoot] = useState('/')
  const [form] = Form.useForm<{ name: string }>()

  const filesQuery = useQuery({
    queryKey: ['agent-files', agentId, cwd],
    queryFn: () => api.listFiles(agentId, cwd),
    retry: 1,
  })
  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['agent-files', agentId, cwd] })

  const openSearch = () => {
    setSearchQuery('')
    setSearchResult(null)
    setSearchRoot(cwd)
    setSearchOpen(true)
  }

  const runSearch = async () => {
    const q = searchQuery.trim()
    if (!q) {
      message.warning('请输入搜索关键词')
      return
    }
    setSearching(true)
    try {
      setSearchResult(await api.searchFiles(agentId, searchRoot, q, searchCase))
    } catch (err) {
      message.error(getApiErrorMessage(err, '搜索失败'))
    } finally {
      setSearching(false)
    }
  }

  // 点击命中路径：跳转到该文件所在目录（相对搜索根解析为绝对路径）
  const gotoMatch = (relPath: string) => {
    const abs = joinPath(searchRoot, relPath)
    const dir = abs.slice(0, abs.lastIndexOf('/')) || '/'
    setCwd(dir)
    setSearchOpen(false)
  }

  const mkdirMutation = useMutation({
    mutationFn: (path: string) => api.createRemoteDir(agentId, path),
    onSuccess: () => {
      message.success('目录已创建')
      setMkdirOpen(false)
      invalidate()
    },
    onError: (err) => message.error(getApiErrorMessage(err, '创建目录失败')),
  })

  const renameMutation = useMutation({
    mutationFn: ({ path, name }: { path: string; name: string }) =>
      api.renameRemoteFile(agentId, path, name),
    onSuccess: () => {
      message.success('重命名成功')
      setRenameTarget(null)
      invalidate()
    },
    onError: (err) => message.error(getApiErrorMessage(err, '重命名失败')),
  })

  const deleteMutation = useMutation({
    mutationFn: (path: string) => api.deleteRemoteFile(agentId, path),
    onSuccess: () => {
      message.success('已删除')
      setDeleteDir(null)
      setDeleteConfirm('')
      invalidate()
    },
    onError: (err) => message.error(getApiErrorMessage(err, '删除失败')),
  })

  const [downloading, setDownloading] = useState<string | null>(null)
  const download = async (path: string) => {
    setDownloading(path)
    try {
      const blob = await api.downloadRemoteFile(agentId, path)
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = path.split('/').pop() || 'download'
      a.click()
      URL.revokeObjectURL(url)
    } catch (err) {
      message.error(getApiErrorMessage(err, '下载失败'))
    } finally {
      setDownloading(null)
    }
  }

  const openEditor = async (f: FileEntry) => {
    setEditTarget(f)
    setEditContent('')
    try {
      setEditContent(await readFullText(agentId, joinPath(cwd, f.name)))
    } catch (err) {
      setEditTarget(null)
      message.error(getApiErrorMessage(err, '读取文件失败'))
    }
  }

  const saveEdit = async () => {
    if (!editTarget) return
    setEditSaving(true)
    try {
      const bytes = new TextEncoder().encode(editContent)
      await api.writeFile(agentId, joinPath(cwd, editTarget.name), toBase64(bytes), true)
      message.success('已保存')
      setEditTarget(null)
      invalidate()
    } catch (err) {
      message.error(getApiErrorMessage(err, '保存失败'))
    } finally {
      setEditSaving(false)
    }
  }

  // ---- 图片预览（file-manager-design.md M2 D14）----
  const [previewTarget, setPreviewTarget] = useState<FileEntry | null>(null)
  const [previewUrl, setPreviewUrl] = useState<string | null>(null)
  const [previewLoading, setPreviewLoading] = useState(false)
  const [previewError, setPreviewError] = useState<string | null>(null)
  const [previewDims, setPreviewDims] = useState<{ w: number; h: number } | null>(null)
  const previewTokenRef = useRef(0)

  const closePreview = () => {
    previewTokenRef.current += 1 // 中止进行中的分块拉取
    setPreviewTarget(null)
    setPreviewError(null)
    setPreviewDims(null)
    setPreviewUrl((old) => {
      if (old) URL.revokeObjectURL(old)
      return null
    })
  }
  // 组件卸载时兜底回收
  useEffect(() => () => closePreview(), [])
  // 卸载兜底：在途分块上传立即中止（不留半途请求）
  useEffect(() => () => {
    uploadTokenRef.current++
  }, [])

  // 分块拉取图片拼装 Blob（offset 按已读字节数推进；size 是总大小不是游标）
  const openPreview = async (f: FileEntry) => {
    const token = ++previewTokenRef.current
    const path = joinPath(cwd, f.name)
    const ext = imageExt(f.name)
    const mime = ext ? IMAGE_MIME[ext] : 'application/octet-stream'
    setPreviewTarget(f)
    setPreviewError(null)
    setPreviewDims(null)
    setPreviewUrl((old) => {
      if (old) URL.revokeObjectURL(old)
      return null
    })
    setPreviewLoading(true)
    try {
      const parts: Array<Uint8Array<ArrayBuffer>> = []
      let offset = 0
      let total = 0
      for (;;) {
        const chunk = await api.readFileChunk(agentId, path, offset, PREVIEW_CHUNK_BYTES)
        if (token !== previewTokenRef.current) return // Modal 已关闭/切换
        total = chunk.size
        if (total > PREVIEW_MAX_BYTES) {
          setPreviewError(`文件 ${formatBytes(total)}，超过 20 MB 不支持在线预览，请下载后查看`)
          return
        }
        const bytes = Uint8Array.from(atob(chunk.data), (c) => c.charCodeAt(0))
        parts.push(bytes)
        offset += bytes.length
        if (chunk.eof) break
      }
      const url = URL.createObjectURL(new Blob(parts, { type: mime }))
      if (token !== previewTokenRef.current) {
        URL.revokeObjectURL(url)
        return
      }
      setPreviewUrl(url)
    } catch (err) {
      if (token === previewTokenRef.current) {
        setPreviewError(getApiErrorMessage(err, '读取文件失败'))
      }
    } finally {
      if (token === previewTokenRef.current) setPreviewLoading(false)
    }
  }

  // 上传（file-manager-design.md M3）：≤10MB 单块；更大分块（首块 truncate
  // 建文件、续块 append），进度/取消走 token 递增；失败探测续传 ≤3 次
  const [uploading, setUploading] = useState<{ name: string; uploaded: number; total: number } | null>(null)
  const uploadTokenRef = useRef(0)
  const handledUploadUids = useRef(new Set<string>())

  // 探测目标文件当前大小（续传游标）；null = 不存在
  const probeUploadedSize = async (dir: string, name: string): Promise<number | null> => {
    const list = await api.listFiles(agentId, dir)
    const hit = list.entries.find((e) => e.name === name)
    return hit ? hit.size : null
  }

  const uploadChunked = async (raw: File, path: string, token: number) => {
    const dir = cwd
    let uploaded = 0
    let first = true
    let retries = 0
    while (uploaded < raw.size) {
      if (token !== uploadTokenRef.current) throw new Error('cancelled')
      const end = Math.min(uploaded + UPLOAD_CHUNK_BYTES, raw.size)
      const bytes = new Uint8Array(await raw.slice(uploaded, end).arrayBuffer())
      setUploading({ name: raw.name, uploaded, total: raw.size })
      try {
        const res = await api.writeFile(agentId, path, toBase64(bytes), first)
        // agent 返回写入后总大小：与本地进度一致才继续（防交错产出损坏文件）
        if (res.size !== end) throw new Error('size-mismatch')
        uploaded = end
        first = false
        retries = 0
      } catch (err) {
        if (token !== uploadTokenRef.current) throw new Error('cancelled')
        if (err instanceof Error && err.message === 'size-mismatch') throw err
        retries++
        if (retries > 3) throw err
        // 断点续传：以服务器文件实际大小为准决定从哪继续
        const remote = await probeUploadedSize(dir, raw.name).catch(() => null)
        if (remote === uploaded || remote === uploaded + UPLOAD_CHUNK_BYTES) {
          uploaded = remote // 响应丢失时该块可能已落盘，跳过
          first = false
        } else if (remote === null || remote === 0) {
          uploaded = 0 // 目标丢失/为空 → 首块重建
          first = true
        } else {
          throw err // 其他大小（并发写入）→ 不覆盖别人的改动
        }
      }
    }
  }

  const cancelUpload = () => {
    uploadTokenRef.current++
    const cur = uploading
    setUploading(null)
    // 清理半成品（分块留下的截断文件），失败静默
    if (cur && cur.uploaded > 0) {
      api.deleteRemoteFile(agentId, joinPath(cwd, cur.name)).catch(() => {})
    }
  }

  const uploadFiles = async (files: UploadFile[]) => {
    for (const f of files) {
      const raw = f.originFileObj as File | undefined
      if (!raw || handledUploadUids.current.has(f.uid)) continue // antd onChange 累积 fileList，按 uid 去重
      handledUploadUids.current.add(f.uid)
      if (raw.size > UPLOAD_CHUNK_MAX_BYTES) {
        message.error(`${raw.name} 超过 ${formatBytes(UPLOAD_CHUNK_MAX_BYTES)}，请走 scp 或终端上传`)
        continue
      }
      const path = joinPath(cwd, raw.name)
      try {
        if (raw.size <= UPLOAD_MAX_BYTES) {
          const bytes = new Uint8Array(await raw.arrayBuffer())
          await api.writeFile(agentId, path, toBase64(bytes), true)
        } else {
          await uploadChunked(raw, path, uploadTokenRef.current)
        }
        message.success(`${raw.name} 已上传`)
        invalidate()
      } catch (err) {
        if (err instanceof Error && err.message === 'cancelled') return
        message.error(getApiErrorMessage(err, `上传 ${raw.name} 失败`))
      }
    }
    invalidate()
  }

  const entries = useMemo(
    () => filesQuery.data?.entries ?? [],
    [filesQuery.data],
  )
  const breadcrumbItems = useMemo(() => pathSegments(cwd), [cwd])

  const columns: ColumnsType<FileEntry> = [
    {
      title: '名称',
      key: 'name',
      render: (_, f) => {
        const path = joinPath(cwd, f.name)
        const icon = f.isDir ? (
          <FolderOutlined style={{ color: '#faad14' }} />
        ) : f.isSymlink ? (
          <LinkOutlined />
        ) : (
          <FileOutlined />
        )
        const label = f.isSymlink ? `${f.name} → ${f.target}` : f.name
        return (
          <Space size={6}>
            {icon}
            {f.isDir && !f.isSymlink ? (
              <Typography.Link onClick={() => setCwd(path)}>{label}</Typography.Link>
            ) : (
              <Typography.Text title={f.isSymlink ? '符号链接不支持在线打开' : undefined}>
                {label}
              </Typography.Text>
            )}
            {f.isSymlink && <Tag style={{ fontSize: 11 }}>链接</Tag>}
          </Space>
        )
      },
    },
    {
      title: '大小',
      key: 'size',
      width: 100,
      render: (_, f) => (f.isDir ? '—' : formatBytes(f.size)),
    },
    { title: '权限', dataIndex: 'mode', key: 'mode', width: 80 },
    {
      title: '修改时间',
      dataIndex: 'mtime',
      key: 'mtime',
      width: 150,
      render: (t: number) => (t ? dayjs.unix(t).format('YYYY-MM-DD HH:mm') : '—'),
    },
    {
      title: '操作',
      key: 'actions',
      width: 210,
      render: (_, f) => {
        const path = joinPath(cwd, f.name)
        const editable = !f.isDir && !f.isSymlink && f.size <= EDIT_MAX_BYTES
        const previewable = !f.isDir && !f.isSymlink && !!imageExt(f.name)
        return (
          <Space size={0}>
            {previewable && (
              <Button
                type="link"
                size="small"
                icon={<EyeOutlined />}
                title="预览图片"
                loading={previewLoading && previewTarget?.name === f.name}
                onClick={() => void openPreview(f)}
              />
            )}
            {!f.isDir && !f.isSymlink && (
              <Button
                type="link"
                size="small"
                icon={<EditOutlined />}
                disabled={!editable}
                title={editable ? '编辑' : '仅 ≤1MB 文件支持在线编辑'}
                onClick={() => openEditor(f)}
              />
            )}
            {!f.isDir && !f.isSymlink && (
              <Button
                type="link"
                size="small"
                icon={<DownloadOutlined />}
                loading={downloading === path}
                onClick={() => download(path)}
              />
            )}
            <Button
              type="link"
              size="small"
              icon={<SwapOutlined />}
              title="重命名"
              onClick={() => {
                setRenameTarget(f)
                form.setFieldsValue({ name: f.name })
              }}
            />
            {f.isDir && !f.isSymlink ? (
              <Button
                type="link"
                size="small"
                danger
                icon={<DeleteOutlined />}
                title="删除目录"
                onClick={() => {
                  setDeleteDir(f)
                  setDeleteConfirm('')
                }}
              />
            ) : (
              <Popconfirm
                title="删除该文件？"
                description="此操作直接作用于 Agent 主机，不可恢复"
                onConfirm={() => deleteMutation.mutate(path)}
              >
                <Button type="link" size="small" danger icon={<DeleteOutlined />} />
              </Popconfirm>
            )}
          </Space>
        )
      },
    },
  ]

  return (
    <div>
      <Space style={{ marginBottom: 12, width: '100%', justifyContent: 'space-between' }} wrap>
        <Space wrap>
          <Breadcrumb
            items={breadcrumbItems.map((s, i) => ({
              title:
                i === breadcrumbItems.length - 1 ? (
                  s.label
                ) : (
                  <a onClick={() => setCwd(s.path)}>{s.label}</a>
                ),
            }))}
          />
          <Button type="text" size="small" icon={<RedoOutlined />} onClick={() => invalidate()} />
        </Space>
        <Space wrap>
          <Input
            size="small"
            style={{ width: 220 }}
            placeholder="跳转到路径，如 /etc/nginx"
            value={jump}
            onChange={(e) => setJump(e.target.value)}
            onPressEnter={() => {
              const p = jump.trim()
              if (p.startsWith('/')) {
                setCwd(p.replace(/\/+$/, '') || '/')
                setJump('')
              } else if (p) {
                message.warning('请输入绝对路径')
              }
            }}
          />
          <Button size="small" icon={<FolderAddOutlined />} onClick={() => {
            form.setFieldsValue({ name: '' })
            setMkdirOpen(true)
          }}>
            新建目录
          </Button>
          <Button size="small" icon={<SearchOutlined />} onClick={openSearch}>
            搜索
          </Button>
          <Upload
            multiple
            showUploadList={false}
            beforeUpload={() => false}
            onChange={({ fileList }) => uploadFiles(fileList)}
          >
            <Button size="small" icon={<UploadOutlined />}>上传</Button>
          </Upload>
          {uploading && (
            <Space size={4}>
              <Progress
                percent={Math.floor((uploading.uploaded * 100) / Math.max(uploading.total, 1))}
                size="small"
                style={{ width: 110, marginBottom: 0 }}
              />
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                {uploading.name} {formatBytes(uploading.uploaded)}/{formatBytes(uploading.total)}
              </Typography.Text>
              <Button size="small" type="text" danger onClick={cancelUpload}>
                取消
              </Button>
            </Space>
          )}
        </Space>
      </Space>

      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 12 }}
        message="符号链接不支持在线打开；删除直接作用于 Agent 主机且不可恢复"
      />

      {filesQuery.isLoading ? null : filesQuery.isError ? (
        <Empty description={`目录不可访问：${getApiErrorMessage(filesQuery.error, '未知错误')}`} />
      ) : entries.length === 0 ? (
        <Empty description="目录为空" />
      ) : (
        <Table
          columns={columns}
          dataSource={entries}
          rowKey="name"
          size="small"
          pagination={{ pageSize: 50, hideOnSinglePage: true }}
        />
      )}

      <Modal
        title={`新建目录：${cwd}`}
        open={mkdirOpen}
        onCancel={() => setMkdirOpen(false)}
        onOk={() =>
          form.validateFields().then(({ name }) => {
            mkdirMutation.mutate(joinPath(cwd, name.trim()))
          })
        }
        confirmLoading={mkdirMutation.isPending}
        destroyOnClose
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="name"
            label="目录名"
            rules={[
              { required: true, message: '请输入目录名' },
              { pattern: /^[^/\\]+$/, message: '不能包含路径分隔符' },
            ]}
          >
            <Input placeholder="logs" onPressEnter={() =>
              form.validateFields().then(({ name }) => mkdirMutation.mutate(joinPath(cwd, name.trim())))
            } />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={`重命名：${renameTarget?.name ?? ''}`}
        open={!!renameTarget}
        onCancel={() => setRenameTarget(null)}
        onOk={() =>
          form.validateFields().then(({ name }) => {
            if (renameTarget && name.trim() && name.trim() !== renameTarget.name) {
              renameMutation.mutate({ path: joinPath(cwd, renameTarget.name), name: name.trim() })
            } else {
              setRenameTarget(null)
            }
          })
        }
        confirmLoading={renameMutation.isPending}
        destroyOnClose
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="name"
            label="新名称"
            rules={[
              { required: true, message: '请输入新名称' },
              { pattern: /^[^/\\]+$/, message: '不能包含路径分隔符' },
            ]}
          >
            <Input />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={`编辑：${editTarget?.name ?? ''}`}
        open={!!editTarget}
        onCancel={() => setEditTarget(null)}
        onOk={saveEdit}
        confirmLoading={editSaving}
        width={720}
        okText="保存"
        destroyOnClose
      >
        <Input.TextArea
          value={editContent}
          onChange={(e) => setEditContent(e.target.value)}
          rows={18}
          style={{
            fontFamily: 'monospace',
            fontSize: 12,
            whiteSpace: 'pre',
            overflowX: 'auto',
          }}
        />
      </Modal>

      {/* 图片预览 Modal（D14）：<img> 渲染（SVG secure static 模式无脚本） */}
      <Modal
        title={previewTarget?.name ?? ''}
        open={!!previewTarget}
        onCancel={closePreview}
        footer={
          <Space size={8}>
            {previewDims && (
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                {previewDims.w}×{previewDims.h} · {previewTarget ? formatBytes(previewTarget.size) : ''}
              </Typography.Text>
            )}
            {previewTarget && (
              <Button
                size="small"
                icon={<DownloadOutlined />}
                loading={downloading === joinPath(cwd, previewTarget.name)}
                onClick={() => download(joinPath(cwd, previewTarget.name))}
              >
                下载
              </Button>
            )}
          </Space>
        }
        width={760}
        destroyOnClose
      >
        {previewError ? (
          <Alert type="warning" showIcon message={previewError} />
        ) : previewLoading || !previewUrl ? (
          <div style={{ textAlign: 'center', padding: 48 }}>
            <Spin tip="加载中…" />
          </div>
        ) : (
          <div
            style={{
              textAlign: 'center',
              // 透明底纹：透明 PNG/SVG 可见
              background:
                'repeating-conic-gradient(rgba(128,128,128,0.18) 0% 25%, transparent 0% 50%) 50% / 16px 16px',
              borderRadius: 4,
              padding: 8,
            }}
          >
            <img
              src={previewUrl}
              alt={previewTarget?.name ?? ''}
              style={{ maxWidth: '100%', maxHeight: '60vh', objectFit: 'contain' }}
              onLoad={(e) =>
                setPreviewDims({
                  w: e.currentTarget.naturalWidth,
                  h: e.currentTarget.naturalHeight,
                })
              }
            />
          </div>
        )}
      </Modal>

      <Modal
        title={`删除目录：${deleteDir?.name ?? ''}`}
        open={!!deleteDir}
        onCancel={() => {
          setDeleteDir(null)
          setDeleteConfirm('')
        }}
        okButtonProps={{ danger: true, disabled: deleteConfirm !== deleteDir?.name }}
        onOk={() => deleteDir && deleteMutation.mutate(joinPath(cwd, deleteDir.name))}
        okText="永久删除"
        destroyOnClose
      >
        <Alert
          type="error"
          showIcon
          style={{ marginBottom: 12 }}
          message="目录及其全部内容将被递归删除，不可恢复"
        />
        <Input
          placeholder={`输入目录名 ${deleteDir?.name ?? ''} 确认`}
          value={deleteConfirm}
          onChange={(e) => setDeleteConfirm(e.target.value)}
        />
      </Modal>

      {/* 文本搜索 Modal（D13）：当前目录为根递归 contains */}
      <Modal
        title={`搜索文件内容：${searchRoot}`}
        open={searchOpen}
        onCancel={() => setSearchOpen(false)}
        footer={null}
        width={760}
        destroyOnClose
      >
        <Space.Compact style={{ width: '100%', marginBottom: 8 }}>
          <Input
            placeholder="关键词（纯文本，递归搜索子目录）"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            onPressEnter={() => void runSearch()}
            prefix={<SearchOutlined />}
            autoFocus
          />
          <Button type="primary" loading={searching} onClick={() => void runSearch()}>
            搜索
          </Button>
        </Space.Compact>
        <Space size={8} style={{ marginBottom: 12 }}>
          <Switch checked={searchCase} onChange={setSearchCase} size="small" />
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            区分大小写（默认忽略；跳过 symlink/二进制/大于 1MB 文件，最多返回 200 条）
          </Typography.Text>
        </Space>
        {searching ? (
          <div style={{ textAlign: 'center', padding: 24 }}>
            <Spin tip="扫描中…" />
          </div>
        ) : searchResult ? (
          <>
            <Space size={8} style={{ marginBottom: 8 }}>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                {searchResult.matches.length} 处命中 · 已扫描 {searchResult.scanned} 文件
                {searchResult.skipped > 0 && ` · 跳过 ${searchResult.skipped}`}
              </Typography.Text>
              {searchResult.truncated && (
                <Tag color="warning">结果截断，可换更具体的关键词或子目录</Tag>
              )}
            </Space>
            {searchResult.matches.length === 0 ? (
              <Empty description="无命中" image={Empty.PRESENTED_IMAGE_SIMPLE} />
            ) : (
              <div style={{ maxHeight: 420, overflow: 'auto' }}>
                {searchResult.matches.map((m, i) => (
                  <div key={`${m.path}:${m.line}:${i}`} style={{ marginBottom: 6 }}>
                    <div>
                      <a onClick={() => gotoMatch(m.path)} style={{ fontSize: 12 }}>
                        {m.path}
                      </a>
                      <Typography.Text type="secondary" style={{ fontSize: 12, marginLeft: 8 }}>
                        :{m.line}
                      </Typography.Text>
                    </div>
                    <div
                      style={{
                        fontFamily: 'SFMono-Regular, Consolas, monospace',
                        fontSize: 12,
                        lineHeight: 1.5,
                        background: 'rgba(128,128,128,0.08)',
                        padding: '2px 8px',
                        borderRadius: 4,
                        whiteSpace: 'pre-wrap',
                        wordBreak: 'break-all',
                      }}
                    >
                      <HighlightText text={m.text} query={searchQuery.trim()} caseSensitive={searchCase} />
                    </div>
                  </div>
                ))}
              </div>
            )}
          </>
        ) : (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            以 {searchRoot} 为根递归搜索文件内容；适合定位「配置项写在哪个文件」。
          </Typography.Text>
        )}
      </Modal>
    </div>
  )
}

export default FileBrowser
