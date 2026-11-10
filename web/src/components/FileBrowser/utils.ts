// 纯工具/读取辅助（自 FileBrowser/index.tsx 按职责拆分；无组件状态耦合）
import { Modal } from 'antd'
import { api } from '@/services/api'
import { formatBytes } from '@/utils/format'
import { EDIT_MAX_BYTES, IMAGE_MIME } from './constants'

// Modal.confirm 的 Promise 封装（续传校验矩阵里语义模糊处交用户裁决）
export const confirmAsync = (title: string, content: string): Promise<boolean> =>
  new Promise((resolve) => {
    Modal.confirm({ title, content, onOk: () => resolve(true), onCancel: () => resolve(false) })
  })

export const imageExt = (name: string): string | null => {
  const idx = name.lastIndexOf('.')
  if (idx < 0) return null
  const ext = name.slice(idx + 1).toLowerCase()
  return IMAGE_MIME[ext] ? ext : null
}

// 路径 → 面包屑段（绝对路径按 / 切分）
export const pathSegments = (p: string): Array<{ label: string; path: string }> => {
  const segs = [{ label: '/', path: '/' }]
  let acc = ''
  for (const part of p.split('/').filter(Boolean)) {
    acc += `/${part}`
    segs.push({ label: part, path: acc })
  }
  return segs
}

export const joinPath = (dir: string, name: string) =>
  dir === '/' ? `/${name}` : `${dir}/${name}`

export const toBase64 = (bytes: Uint8Array): string => {
  let bin = ''
  const chunk = 0x8000
  for (let i = 0; i < bytes.length; i += chunk) {
    bin += String.fromCharCode(...bytes.subarray(i, i + chunk))
  }
  return btoa(bin)
}

// 读完整文本文件（≤EDIT_MAX_BYTES），超出拒绝——编辑器场景不需要大文件
export const readFullText = async (agentId: string, path: string): Promise<string> => {
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
