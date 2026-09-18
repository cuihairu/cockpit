import { useMemo, useState } from 'react'
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
const UPLOAD_MAX_BYTES = 10 * 1024 * 1024

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

  // 上传：读 File → base64 → write（truncate），>10MB 前端拒绝
  const uploadFiles = async (files: UploadFile[]) => {
    for (const f of files) {
      const raw = f.originFileObj as File | undefined
      if (!raw) continue
      if (raw.size > UPLOAD_MAX_BYTES) {
        message.error(`${raw.name} 超过 10 MB，暂不支持上传`)
        continue
      }
      try {
        const bytes = new Uint8Array(await raw.arrayBuffer())
        await api.writeFile(agentId, joinPath(cwd, raw.name), toBase64(bytes), true)
        message.success(`${raw.name} 已上传`)
      } catch (err) {
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
        return (
          <Space size={0}>
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
