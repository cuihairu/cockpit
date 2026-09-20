import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  Breadcrumb,
  Button,
  Checkbox,
  Empty,
  Form,
  Input,
  InputNumber,
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
  SafetyOutlined,
  SearchOutlined,
  SwapOutlined,
  UploadOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { api } from '@/services/api'
import { usePerm } from '@/hooks/usePerm'
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

// 跨刷新断点续传（file-manager-design.md M5 D23-D25）：挂起上传登记在
// localStorage（单浏览器语义，per-viewer 便利），续传游标仍以远端文件实际
// size 为准（D16 探测同源）——服务端零新状态、零新端点
interface PendingUpload {
  agentId: string
  dir: string
  name: string
  size: number
  lastModified: number
  uploaded: number
  ts: number
}
const PENDING_UPLOADS_KEY = 'cockpit-pending-uploads'
const PENDING_UPLOADS_MAX = 10 // FIFO 容量
const PENDING_UPLOADS_TTL_MS = 7 * 24 * 3600 * 1000

const readPendingUploads = (): PendingUpload[] => {
  try {
    const raw = localStorage.getItem(PENDING_UPLOADS_KEY)
    if (!raw) return []
    const now = Date.now()
    return (JSON.parse(raw) as PendingUpload[]).filter((e) => now - e.ts < PENDING_UPLOADS_TTL_MS)
  } catch {
    return []
  }
}

const writePendingUploads = (list: PendingUpload[]) => {
  try {
    localStorage.setItem(PENDING_UPLOADS_KEY, JSON.stringify(list.slice(-PENDING_UPLOADS_MAX)))
  } catch {
    // 隐私窗口/存储禁用：续传登记静默缺席，主上传流程不受影响
  }
}

const persistPendingUpload = (e: PendingUpload) => {
  const list = readPendingUploads().filter(
    (x) => !(x.agentId === e.agentId && x.dir === e.dir && x.name === e.name),
  )
  list.push(e)
  writePendingUploads(list)
}

const removePendingUpload = (agentId: string, dir: string, name: string) => {
  writePendingUploads(
    readPendingUploads().filter((x) => !(x.agentId === agentId && x.dir === dir && x.name === name)),
  )
}

// Modal.confirm 的 Promise 封装（续传校验矩阵里语义模糊处交用户裁决）
const confirmAsync = (title: string, content: string): Promise<boolean> =>
  new Promise((resolve) => {
    Modal.confirm({ title, content, onOk: () => resolve(true), onCancel: () => resolve(false) })
  })

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

// 权限九宫格：三组（属主/属组/其他）× r/w/x 的位
const PERM_GRID: Array<{ label: string; bits: [number, number, number] }> = [
  { label: '属主', bits: [0o400, 0o200, 0o100] },
  { label: '属组', bits: [0o040, 0o020, 0o010] },
  { label: '其他', bits: [0o004, 0o002, 0o001] },
]
const PERM_HEADS = ['读', '写', '执行']

// 远程文件浏览器：按 agent 作用域，浏览/编辑/上传/下载/重命名/删除/权限
// （见 docs/guide/file-manager-design.md；全文件系统可见，安全靠双端路径校验）
const FileBrowser = ({ agentId }: { agentId: string }) => {
  const queryClient = useQueryClient()
  // 浏览/预览/下载/搜索是读；编辑/权限/重命名/删除/新建目录/上传是写
  const canWrite = usePerm('files:write')
  const [cwd, setCwd] = useState('/')
  const [jump, setJump] = useState('')
  const [mkdirOpen, setMkdirOpen] = useState(false)
  const [renameTarget, setRenameTarget] = useState<FileEntry | null>(null)
  const [editTarget, setEditTarget] = useState<FileEntry | null>(null)
  const [editContent, setEditContent] = useState('')
  const [editSaving, setEditSaving] = useState(false)
  const [deleteDir, setDeleteDir] = useState<FileEntry | null>(null)
  const [deleteConfirm, setDeleteConfirm] = useState('')
  // 权限编辑（file-manager-design.md M4 D22）：permBits 是九宫格合成的权限位
  const [permTarget, setPermTarget] = useState<FileEntry | null>(null)
  const [permBits, setPermBits] = useState(0)
  const [permUid, setPermUid] = useState<number>()
  const [permGid, setPermGid] = useState<number>()
  const [permSaving, setPermSaving] = useState(false)
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

  // ---- 权限编辑（file-manager-design.md M4 D22）----
  // 打开时从八进制 mode 反解权限位；uid/gid 为 -1（windows）或缺失时显示空
  const openPerm = (f: FileEntry) => {
    setPermBits(parseInt(f.mode, 8) || 0)
    setPermUid(typeof f.uid === 'number' && f.uid >= 0 ? f.uid : undefined)
    setPermGid(typeof f.gid === 'number' && f.gid >= 0 ? f.gid : undefined)
    setPermTarget(f)
  }

  // 应用时按变化分流：权限位变调 chmod；uid/gid 变调 chown（两者需同时有值）
  const applyPerm = async () => {
    if (!permTarget) return
    const origBits = parseInt(permTarget.mode, 8) || 0
    const origUid = typeof permTarget.uid === 'number' && permTarget.uid >= 0 ? permTarget.uid : undefined
    const origGid = typeof permTarget.gid === 'number' && permTarget.gid >= 0 ? permTarget.gid : undefined
    const ownerChanged = permUid !== origUid || permGid !== origGid
    if (ownerChanged && (permUid === undefined || permGid === undefined)) {
      message.warning('uid 与 gid 需同时填写才能修改归属')
      return
    }
    setPermSaving(true)
    try {
      const path = joinPath(cwd, permTarget.name)
      if (permBits !== origBits) await api.chmodFile(agentId, path, permBits)
      if (ownerChanged) await api.chownFile(agentId, path, permUid!, permGid!)
      message.success('权限已更新')
      setPermTarget(null)
      invalidate()
    } catch (err) {
      message.error(getApiErrorMessage(err, '权限修改失败'))
    } finally {
      setPermSaving(false)
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

  // dir 显式传入（挂起续传时来自登记而非当前 cwd）；resume 为跨刷新续传的
  // 起始偏移（M5 D24）。登记生命周期归本函数：起始 upsert、每块更新进度、
  // 成功移除；重试耗尽的最终失败保留记录（D23——断网关页面正是要续传的场景）
  const uploadChunked = async (
    raw: File,
    dir: string,
    token: number,
    resume?: { uploaded: number; first: boolean },
  ) => {
    const path = joinPath(dir, raw.name)
    let uploaded = resume?.uploaded ?? 0
    let first = resume?.first ?? true
    let retries = 0
    const entry = (): PendingUpload => ({
      agentId,
      dir,
      name: raw.name,
      size: raw.size,
      lastModified: raw.lastModified,
      uploaded,
      ts: Date.now(),
    })
    persistPendingUpload(entry())
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
        persistPendingUpload(entry())
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
          persistPendingUpload(entry())
        } else if (remote === null || remote === 0) {
          uploaded = 0 // 目标丢失/为空 → 首块重建
          first = true
          persistPendingUpload(entry())
        } else {
          throw err // 其他大小（并发写入）→ 不覆盖别人的改动
        }
      }
    }
    removePendingUpload(agentId, dir, raw.name)
  }

  const cancelUpload = () => {
    uploadTokenRef.current++
    const cur = uploading
    setUploading(null)
    // 清理半成品（分块留下的截断文件）与挂起登记，失败静默
    if (cur && cur.uploaded > 0) {
      api.deleteRemoteFile(agentId, joinPath(cwd, cur.name)).catch(() => {})
    }
    if (cur) {
      removePendingUpload(agentId, cwd, cur.name)
      refreshPendingUploads()
    }
  }

  // 挂起上传（跨刷新续传，M5 D24）：同 agent 的未完成上传，Alert 呈现
  const [pendingUploads, setPendingUploads] = useState<PendingUpload[]>([])
  const refreshPendingUploads = () =>
    setPendingUploads(readPendingUploads().filter((e) => e.agentId === agentId))
  useEffect(() => {
    refreshPendingUploads()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agentId])

  // 续传校验矩阵（D24）：size 一致才续；mtime 不符确认混合内容风险；远端
  // 对齐则从游标续、已完整清记录、不符确认覆盖重传
  const resumeUpload = async (entry: PendingUpload, raw: File) => {
    if (raw.size !== entry.size) {
      message.error(
        `${raw.name} 与记录大小不一致（${formatBytes(raw.size)} ≠ ${formatBytes(entry.size)}），无法续传`,
      )
      return
    }
    if (raw.lastModified !== entry.lastModified) {
      const ok = await confirmAsync(
        '本地文件已修改？',
        `${raw.name} 的修改时间与上传记录不同，续传将产出新旧混合内容，建议重新上传整文件。仍要继续吗？`,
      )
      if (!ok) return
    }
    let resume: { uploaded: number; first: boolean } = { uploaded: 0, first: true }
    try {
      const remote = await probeUploadedSize(entry.dir, entry.name)
      if (remote === entry.size) {
        removePendingUpload(agentId, entry.dir, entry.name)
        refreshPendingUploads()
        invalidate()
        message.success(`${entry.name} 远端已完整（${formatBytes(remote)}），已清除续传记录`)
        return
      }
      if (remote !== null && remote > 0 && remote < entry.size && remote % UPLOAD_CHUNK_BYTES === 0) {
        resume = { uploaded: remote, first: false }
      } else if (remote !== null && remote > 0) {
        // 非对齐（半块截断）或超出记录大小 → 语义模糊，交用户裁决
        const ok = await confirmAsync(
          '远端文件与记录不符',
          `${entry.name} 远端大小 ${formatBytes(remote)} 与续传记录不符（可能不完整或被改动）。从头重传将覆盖该文件，是否继续？`,
        )
        if (!ok) return
      }
    } catch {
      // 探测失败：从 truncate 幂等重来（首块清空旧内容）
    }
    try {
      await uploadChunked(raw, entry.dir, uploadTokenRef.current, resume)
      message.success(`${raw.name} 已上传`)
      refreshPendingUploads()
      invalidate()
    } catch (err) {
      if (err instanceof Error && err.message === 'cancelled') return
      // 记录保留（D23）：断网/失败后仍可刷新页面继续
      message.error(getApiErrorMessage(err, `续传 ${raw.name} 失败`))
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
          await uploadChunked(raw, cwd, uploadTokenRef.current)
        }
        message.success(`${raw.name} 已上传`)
        invalidate()
      } catch (err) {
        if (err instanceof Error && err.message === 'cancelled') return
        message.error(getApiErrorMessage(err, `上传 ${raw.name} 失败`))
      }
    }
    invalidate()
    refreshPendingUploads()
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
            {canWrite && !f.isDir && !f.isSymlink && (
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
            {canWrite && (
              <Button
                type="link"
                size="small"
                icon={<SafetyOutlined />}
                title={f.isSymlink ? '符号链接不支持权限编辑' : '权限'}
                disabled={f.isSymlink}
                onClick={() => openPerm(f)}
              />
            )}
            {canWrite && (
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
            )}
            {canWrite &&
              (f.isDir && !f.isSymlink ? (
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
              ))}
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
          {canWrite && (
            <Button size="small" icon={<FolderAddOutlined />} onClick={() => {
              form.setFieldsValue({ name: '' })
              setMkdirOpen(true)
            }}>
              新建目录
            </Button>
          )}
          <Button size="small" icon={<SearchOutlined />} onClick={openSearch}>
            搜索
          </Button>
          {canWrite && (
            <Upload
              multiple
              showUploadList={false}
              beforeUpload={() => false}
              onChange={({ fileList }) => uploadFiles(fileList)}
            >
              <Button size="small" icon={<UploadOutlined />}>上传</Button>
            </Upload>
          )}
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

      {/* 挂起上传（跨刷新续传，M5 D24）：进度以远端文件为准，重选本地文件继续 */}
      {!uploading && pendingUploads.length > 0 && (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 12 }}
          message="有未完成的上传——可重选本地文件继续（进度以远端文件为准）"
          description={
            <Space direction="vertical" size={8} style={{ width: '100%' }}>
              {pendingUploads.map((e) => (
                <Space key={`${e.dir}/${e.name}`} size={8} wrap>
                  <Progress
                    percent={Math.floor((e.uploaded * 100) / Math.max(e.size, 1))}
                    size="small"
                    style={{ width: 90, marginBottom: 0 }}
                  />
                  <Typography.Text style={{ fontSize: 12 }}>
                    {e.name}
                    <Typography.Text type="secondary">
                      {' '}
                      · {e.dir} · {formatBytes(e.uploaded)}/{formatBytes(e.size)}
                    </Typography.Text>
                  </Typography.Text>
                  <Upload
                    showUploadList={false}
                    beforeUpload={() => false}
                    onChange={({ file }) => {
                      const raw = file.originFileObj as File | undefined
                      if (raw) resumeUpload(e, raw)
                    }}
                  >
                    <Button size="small" icon={<UploadOutlined />}>
                      继续
                    </Button>
                  </Upload>
                  <Popconfirm
                    title="放弃续传记录？"
                    description="远端半成品不删除，可手动清理或重新上传覆盖。"
                    onConfirm={() => {
                      removePendingUpload(agentId, e.dir, e.name)
                      refreshPendingUploads()
                    }}
                  >
                    <Button size="small" type="text" danger>
                      放弃
                    </Button>
                  </Popconfirm>
                </Space>
              ))}
            </Space>
          }
        />
      )}

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
        title={`权限：${permTarget?.name ?? ''}`}
        open={!!permTarget}
        onCancel={() => setPermTarget(null)}
        onOk={applyPerm}
        okText="应用"
        confirmLoading={permSaving}
        width={460}
        destroyOnClose
      >
        {permTarget && (
          <>
            <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 12 }}>
              当前 {permTarget.mode}
              {typeof permTarget.uid === 'number' && permTarget.uid >= 0
                ? ` · ${permTarget.uid}:${permTarget.gid}`
                : ''}
              ；应用后合成八进制 {(permBits & 0o777).toString(8).padStart(4, '0')}
            </Typography.Paragraph>
            <div style={{ marginBottom: 4, display: 'flex' }}>
              <span style={{ width: 44 }} />
              {PERM_HEADS.map((h) => (
                <span key={h} style={{ width: 64, fontSize: 13, color: 'rgba(128,128,128,0.85)' }}>
                  {h}
                </span>
              ))}
            </div>
            {PERM_GRID.map((g) => (
              <div key={g.label} style={{ display: 'flex', alignItems: 'center', marginBottom: 4 }}>
                <span style={{ width: 44, fontSize: 13 }}>{g.label}</span>
                {g.bits.map((bit, i) => (
                  <Checkbox
                    key={i}
                    checked={(permBits & bit) !== 0}
                    onChange={() => setPermBits((b) => b ^ bit)}
                    style={{ width: 64 }}
                  >
                    {PERM_HEADS[i]}
                  </Checkbox>
                ))}
              </div>
            ))}
            <div style={{ display: 'flex', gap: 16, marginTop: 12 }}>
              <span>
                属主 UID{' '}
                <InputNumber
                  size="small"
                  min={0}
                  max={4294967295}
                  precision={0}
                  value={permUid}
                  placeholder="—"
                  onChange={(v) => setPermUid(v ?? undefined)}
                />
              </span>
              <span>
                属组 GID{' '}
                <InputNumber
                  size="small"
                  min={0}
                  max={4294967295}
                  precision={0}
                  value={permGid}
                  placeholder="—"
                  onChange={(v) => setPermGid(v ?? undefined)}
                />
              </span>
            </div>
            <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 8 }}>
              仅权限位变化时执行 chmod；uid/gid 变化时执行 chown（需 agent 主机权限，失败原因原样提示）。
            </Typography.Text>
          </>
        )}
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
