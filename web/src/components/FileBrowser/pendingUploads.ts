// 跨刷新断点续传（file-manager-design.md M5 D23-D25）：挂起上传登记在
// localStorage（单浏览器语义，per-viewer 便利），续传游标仍以远端文件实际
// size 为准（D16 探测同源）——服务端零新状态、零新端点
export interface PendingUpload {
  agentId: string
  dir: string
  name: string
  size: number
  lastModified: number
  uploaded: number
  ts: number
}
export const PENDING_UPLOADS_KEY = 'cockpit-pending-uploads'
export const PENDING_UPLOADS_MAX = 10 // FIFO 容量
export const PENDING_UPLOADS_TTL_MS = 7 * 24 * 3600 * 1000

export const readPendingUploads = (): PendingUpload[] => {
  try {
    const raw = localStorage.getItem(PENDING_UPLOADS_KEY)
    if (!raw) return []
    const now = Date.now()
    return (JSON.parse(raw) as PendingUpload[]).filter((e) => now - e.ts < PENDING_UPLOADS_TTL_MS)
  } catch {
    return []
  }
}

export const writePendingUploads = (list: PendingUpload[]) => {
  try {
    localStorage.setItem(PENDING_UPLOADS_KEY, JSON.stringify(list.slice(-PENDING_UPLOADS_MAX)))
  } catch {
    // 隐私窗口/存储禁用：续传登记静默缺席，主上传流程不受影响
  }
}

export const persistPendingUpload = (e: PendingUpload) => {
  const list = readPendingUploads().filter(
    (x) => !(x.agentId === e.agentId && x.dir === e.dir && x.name === e.name),
  )
  list.push(e)
  writePendingUploads(list)
}

export const removePendingUpload = (agentId: string, dir: string, name: string) => {
  writePendingUploads(
    readPendingUploads().filter((x) => !(x.agentId === agentId && x.dir === dir && x.name === name)),
  )
}
