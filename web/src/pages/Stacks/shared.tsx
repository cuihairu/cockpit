import axios from 'axios'
import dayjs from 'dayjs'

// tail 行数选项（与容器日志保持一致）
export const tailOptions = [
  { value: 50, label: '50 行' },
  { value: 100, label: '100 行' },
  { value: 200, label: '200 行' },
  { value: 500, label: '500 行' },
  { value: 2000, label: '2000 行' },
]

// Stack 名校验：小写字母、数字、中划线、下划线
export const STACK_NAME_PATTERN = /^[a-z0-9][a-z0-9_-]*$/

// 新建 Stack 的默认 compose.yml 模板（带注释的 nginx 示例）
export const DEFAULT_COMPOSE_TEMPLATE = `# nginx 示例 Stack
# 编辑后点击「保存」，再通过「启动」按钮部署服务
services:
  nginx:
    image: nginx:latest
    restart: unless-stopped
    ports:
      - "8080:80"
`

// 从 axios 错误中提取后端返回的错误信息（支持多行文本，如 docker compose config 输出）
export const extractApiError = (err: unknown, fallback = '请求失败'): string => {
  if (axios.isAxiosError(err)) {
    const data: unknown = err.response?.data
    if (typeof data === 'string' && data.trim()) return data
    if (data && typeof data === 'object') {
      const obj = data as { error?: unknown; message?: unknown }
      const msg = obj.error ?? obj.message
      if (typeof msg === 'string' && msg.trim()) return msg
    }
    if (err.message) return err.message
  }
  if (err instanceof Error && err.message) return err.message
  return fallback
}

// Unix 秒级时间戳 → 可读时间
export const formatTimestamp = (ts: number): string => {
  if (!ts) return '-'
  return dayjs.unix(ts).format('YYYY-MM-DD HH:mm:ss')
}

// 服务运行比例 → Tag 颜色：全部 running=绿、部分=橙、全停=灰
export const serviceStatusColor = (running: number, total: number): 'success' | 'warning' | 'default' => {
  if (total > 0 && running >= total) return 'success'
  if (running > 0) return 'warning'
  return 'default'
}

// 最近部署状态 → Tag 颜色
export const lastStatusColor = (status: string): string => {
  switch (status) {
    case 'success':
    case 'ok':
      return 'success'
    case 'failed':
    case 'error':
      return 'error'
    case 'running':
      return 'processing'
    default:
      return 'default'
  }
}

// 服务 docker 状态 → Tag 颜色（对齐容器管理页配色）
export const serviceStateColor: Record<string, string> = {
  running: 'green',
  created: 'blue',
  paused: 'orange',
  exited: 'default',
  dead: 'red',
  restarting: 'gold',
}

// Stack 异步任务动作 → 中文标签
export const taskActionLabel: Record<string, string> = {
  up: '启动',
  down: '停止',
  delete: '删除',
}
