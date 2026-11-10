// 常量与静态配置（自 FileBrowser/index.tsx 按职责拆分；纯数据无副作用）
export const EDIT_MAX_BYTES = 1024 * 1024 // 编辑器只允许 ≤1MB 文本（与 server 校验一致）
// 上传（file-manager-design.md M3 D15-D17）：≤10MB 单块快速路径；更大走分块
// （单块 1MB = agent fileWriteChunkLimit 满额，首块 truncate 建文件续块 append）
export const UPLOAD_MAX_BYTES = 10 * 1024 * 1024
export const UPLOAD_CHUNK_MAX_BYTES = 2 * 1024 * 1024 * 1024
export const UPLOAD_CHUNK_BYTES = 1024 * 1024
// 图片预览（file-manager-design.md M2 D14）：白名单扩展名 + 20MB 上限
export const PREVIEW_MAX_BYTES = 20 * 1024 * 1024
export const PREVIEW_CHUNK_BYTES = 1024 * 1024 // agent file.read 单块上限
export const IMAGE_MIME: Record<string, string> = {
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

// 权限九宫格：三组（属主/属组/其他）× r/w/x 的位
export const PERM_GRID: Array<{ label: string; bits: [number, number, number] }> = [
  { label: '属主', bits: [0o400, 0o200, 0o100] },
  { label: '属组', bits: [0o040, 0o020, 0o010] },
  { label: '其他', bits: [0o004, 0o002, 0o001] },
]
export const PERM_HEADS = ['读', '写', '执行']
