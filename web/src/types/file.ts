// 远程文件域类型
// Agent 远程文件条目（目录直属，不递归）
export interface FileEntry {
  name: string
  size: number
  mode: string // 八进制权限，如 "0644"
  uid?: number // 属主 uid（unix；windows/旧 agent 无为 -1 或缺省）
  gid?: number
  mtime: number
  isDir: boolean
  isSymlink: boolean
  target: string // symlink 目标（仅 isSymlink 时有值）
}

// file.read 分块结果
export interface FileReadResult {
  data: string // base64
  size: number
  eof: boolean
}

// file.search 文本搜索结果（见 file-manager-design.md D10-D11）
export interface FileSearchMatch {
  path: string // 相对搜索根目录
  line: number
  text: string // 命中行（agent 侧已截断 200 字符）
}

export interface FileSearchResult {
  matches: FileSearchMatch[]
  truncated: boolean
  scanned: number
  skipped: number
}
