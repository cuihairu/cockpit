import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Alert,
  Button,
  Card,
  Input,
  InputNumber,
  Segmented,
  Select,
  Space,
  Tag,
  Typography,
  message,
} from 'antd'
import { PauseCircleOutlined, PlayCircleOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import { api } from '@/services/api'
import type { LogsQueryResult } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'

// LogsPanel 远程日志查询（见 docs/guide/logs-design.md）：
// 源选择（systemd 服务 / docker 容器）+ tail/since/grep 过滤 → agent 实时执行
// journalctl / docker logs，整段文本回显（级别着色 + grep 高亮 + 自动滚底）。
// M2 实时尾随：fetch NDJSON 流（F4/F7），agent 侧 journalctl -f / docker logs -f。

// eof reason → 展示文案（reason 来自 agent close 通知，见 logs-design.md F3/F6）
const FOLLOW_EOF_TEXT: Record<string, string> = {
  exited: '日志源已停止输出',
  limit: '输出达上限（4MB），已自动停止',
  replaced: '会话被新的尾随替换',
  stopped: '已停止',
  disconnected: '连接中断',
}

const SINCE_OPTIONS = [
  { value: 0, label: '不限' },
  { value: 5, label: '最近 5 分钟' },
  { value: 60, label: '最近 1 小时' },
  { value: 360, label: '最近 6 小时' },
  { value: 1440, label: '最近 24 小时' },
]

// 行级着色：ERROR/FATAL 红、WARN 橙；返回 null 用默认色（LogSearch 跨机检索页复用）
export const lineTone = (line: string): string | null => {
  const upper = line.toUpperCase()
  if (upper.includes('ERROR') || upper.includes('FATAL') || upper.includes('PANIC')) return '#ff6b6b'
  if (upper.includes('WARN')) return '#ffa940'
  return null
}

// GrepLine 单行渲染：grep 命中片段高亮（大小写敏感，与 agent contains 行为一致）
export const GrepLine = ({ line, grep }: { line: string; grep: string }) => {
  const color = lineTone(line)
  const style: React.CSSProperties = color ? { color } : {}

  if (!grep || !line.includes(grep)) {
    return <div style={style}>{line || ' '}</div>
  }
  const parts: React.ReactNode[] = []
  let rest = line
  let key = 0
  while (rest) {
    const idx = rest.indexOf(grep)
    if (idx < 0) {
      parts.push(rest)
      break
    }
    if (idx > 0) parts.push(rest.slice(0, idx))
    parts.push(
      <mark key={key++} style={{ background: '#613400', color: '#ffd666', padding: '0 1px' }}>
        {grep}
      </mark>,
    )
    rest = rest.slice(idx + grep.length)
  }
  return <div>{parts}</div>
}

interface LogsPanelProps {
  agentId: string
  // 外部预选源（服务页 journal 跳转传入 unit 名）：作为 pickedSource 初值，
  // 且源派生时无条件优先——该 unit 可能不在 sources 列表（logs.sources 只
  // 枚举 list-units 在册 unit，未加载服务不在），但 journalctl -u 仍能查到
  // 历史日志，回落会查错对象；传入时 status 就绪后自动首查一次
  initialSource?: string
}

const LogsPanel = ({ agentId, initialSource }: LogsPanelProps) => {
  const [sourceType, setSourceType] = useState<'systemd' | 'docker'>('systemd')
  const [pickedSource, setPickedSource] = useState<string>(initialSource ?? '')
  const [sinceMinutes, setSinceMinutes] = useState(0)
  const [tail, setTail] = useState(200)
  const [grep, setGrep] = useState('')
  const [querying, setQuerying] = useState(false)
  const [result, setResult] = useState<LogsQueryResult | null>(null)
  const [elapsedMs, setElapsedMs] = useState<number | null>(null)
  // 实时尾随（F7）：following 中锁定源/参数；eof 后保留内容展示
  const [following, setFollowing] = useState(false)
  const [followLines, setFollowLines] = useState<string[]>([])
  const [followEof, setFollowEof] = useState<string | null>(null)
  const followAbortRef = useRef<AbortController | null>(null)
  const viewRef = useRef<HTMLDivElement>(null)

  const { data: status, isLoading: statusLoading } = useQuery({
    queryKey: ['logs-status', agentId],
    queryFn: () => api.getLogsStatus(agentId),
  })
  const { data: sources, isLoading: sourcesLoading } = useQuery({
    queryKey: ['logs-sources', agentId],
    queryFn: () => api.getLogsSources(agentId),
  })

  const typeAvailable = useMemo(() => {
    if (!status) return { systemd: true, docker: true }
    return { systemd: status.journalctl, docker: status.docker }
  }, [status])

  const sourceOptions = useMemo(
    () => (sources ? sources[sourceType] : []),
    [sources, sourceType],
  )

  // 选中源派生：用户手选的源不在当前类型列表里（切类型/列表到达）时回落到第一项；
  // 切回原类型时保留原选择；外部预选（initialSource）无条件优先
  const source =
    pickedSource && (pickedSource === initialSource || sourceOptions.includes(pickedSource))
      ? pickedSource
      : (sourceOptions[0] ?? '')

  const runQuery = async () => {
    if (!source) {
      message.warning('请选择日志源')
      return
    }
    setQuerying(true)
    const started = performance.now()
    try {
      const res = await api.queryLogs(agentId, {
        type: sourceType,
        source,
        tail,
        since_minutes: sinceMinutes,
        grep,
      })
      setResult(res)
      setElapsedMs(Math.round(performance.now() - started))
    } catch (err) {
      message.error(getApiErrorMessage(err, '查询日志失败'))
    } finally {
      setQuerying(false)
    }
  }

  // 新结果/尾随新行自动滚动到底部（最新日志在末尾）
  useEffect(() => {
    if (viewRef.current) {
      viewRef.current.scrollTop = viewRef.current.scrollHeight
    }
  }, [result, followLines])

  // 组件卸载断开尾随流（server 感知断开补发 follow.stop）
  useEffect(() => () => followAbortRef.current?.abort(), [])

  const stopFollow = () => {
    followAbortRef.current?.abort()
    followAbortRef.current = null
    setFollowing(false)
  }

  const startFollow = async () => {
    if (!source) {
      message.warning('请选择日志源')
      return
    }
    setFollowLines([])
    setFollowEof(null)
    setResult(null)
    setElapsedMs(null)
    const abort = new AbortController()
    followAbortRef.current = abort
    setFollowing(true)
    try {
      const resp = await api.followLogs(
        agentId,
        { type: sourceType, source, tail, since_minutes: 0, grep },
        abort.signal,
      )
      if (!resp.ok) {
        let msg = `尾随请求失败（HTTP ${resp.status}）`
        try {
          const body = (await resp.json()) as { error?: string }
          if (body?.error) msg = body.error
        } catch {
          // 非 JSON 错误体，保留 HTTP 状态提示
        }
        throw new Error(msg)
      }
      const reader = resp.body?.getReader()
      if (!reader) throw new Error('当前浏览器不支持流式响应')
      const decoder = new TextDecoder()
      let buf = ''
      for (;;) {
        const { done, value } = await reader.read()
        if (done) break
        buf += decoder.decode(value, { stream: true })
        const frames = buf.split('\n')
        buf = frames.pop() ?? ''
        for (const f of frames) {
          if (!f.trim()) continue
          let frame: { data?: string; eof?: boolean; reason?: string }
          try {
            frame = JSON.parse(f) as { data?: string; eof?: boolean; reason?: string }
          } catch {
            continue
          }
          if (frame.eof) {
            setFollowEof(frame.reason ?? 'stopped')
            followAbortRef.current = null
            setFollowing(false)
            return
          }
          if (frame.data != null) {
            // 行带尾换行（agent 按 "\n" 结尾推送），去掉尾空元素
            setFollowLines((prev) => [...prev, ...frame.data!.replace(/\n$/, '').split('\n')])
          }
        }
      }
      // 流结束但未见 eof 帧（异常断开）
      setFollowEof((prev) => prev ?? 'disconnected')
      followAbortRef.current = null
      setFollowing(false)
    } catch (err) {
      if ((err as Error).name === 'AbortError') return // 主动停止不算错误
      message.error(getApiErrorMessage(err, '实时尾随失败'))
      followAbortRef.current = null
      setFollowing(false)
    }
  }

  // 外部预选源：logs.status 就绪后自动首查一次（useRef 守卫防重复）
  const autoQueried = useRef(false)
  useEffect(() => {
    if (!initialSource || autoQueried.current || !status) return
    autoQueried.current = true
    if (typeAvailable.systemd && source) void runQuery()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [status, initialSource, source, typeAvailable])

  const lines = useMemo(() => (result ? result.lines.split('\n') : []), [result])

  // 尾随视图优先：尾随中/eof 后展示 followLines，否则展示查询结果
  const viewLines = following || followLines.length > 0 ? followLines : lines

  return (
    <Card
      type="inner"
      title="日志"
      loading={statusLoading}
      extra={
        <Space size={8}>
          <Button
            icon={following ? <PauseCircleOutlined /> : <PlayCircleOutlined />}
            danger={following}
            disabled={!source}
            onClick={() => (following ? stopFollow() : void startFollow())}
          >
            {following ? '停止尾随' : '实时尾随'}
          </Button>
          <Button
            icon={<ReloadOutlined />}
            onClick={() => void runQuery()}
            loading={querying}
            disabled={following}
          >
            刷新
          </Button>
        </Space>
      }
    >
      {status && !typeAvailable[sourceType] && (
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 12 }}
          message={
            sourceType === 'systemd'
              ? '该主机没有 journalctl（非 systemd 或未安装）'
              : '该主机没有 docker 命令'
          }
        />
      )}

      <Space wrap size={8} style={{ marginBottom: 12 }}>
        <Segmented
          value={sourceType}
          onChange={(v) => setSourceType(v as 'systemd' | 'docker')}
          disabled={following}
          options={[
            { label: 'systemd 服务', value: 'systemd', disabled: !typeAvailable.systemd },
            { label: 'docker 容器', value: 'docker', disabled: !typeAvailable.docker },
          ]}
        />
        <Select
          showSearch
          style={{ minWidth: 220 }}
          placeholder={sourcesLoading ? '加载源…' : '选择日志源'}
          value={source || undefined}
          onChange={(v) => setPickedSource(v)}
          options={sourceOptions.map((s) => ({ value: s, label: s }))}
          notFoundContent={sourcesLoading ? '加载中…' : '无运行中的对象'}
          disabled={following}
        />
        <Select
          value={sinceMinutes}
          onChange={setSinceMinutes}
          options={SINCE_OPTIONS}
          style={{ width: 130 }}
          disabled={following}
        />
        <InputNumber
          min={1}
          max={2000}
          value={tail}
          onChange={(v) => setTail(v ?? 200)}
          addonAfter="行"
          style={{ width: 120 }}
          disabled={following}
        />
        <Input
          allowClear
          placeholder="关键词过滤"
          prefix={<SearchOutlined />}
          value={grep}
          onChange={(e) => setGrep(e.target.value)}
          onPressEnter={() => void runQuery()}
          style={{ width: 180 }}
          disabled={following}
        />
        <Button type="primary" onClick={() => void runQuery()} loading={querying} disabled={following}>
          查询
        </Button>
      </Space>

      {result || following || followLines.length > 0 ? (
        <>
          <div
            ref={viewRef}
            style={{
              background: '#0d1117',
              color: '#c9d1d9',
              fontFamily: 'SFMono-Regular, Consolas, monospace',
              fontSize: 12,
              lineHeight: 1.6,
              padding: '8px 12px',
              borderRadius: 6,
              maxHeight: 480,
              overflow: 'auto',
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-all',
            }}
          >
            {viewLines.map((line, i) => (
              <GrepLine key={i} line={line} grep={grep} />
            ))}
          </div>
          <Space size={8} style={{ marginTop: 8 }}>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              共 {viewLines.filter((l) => l !== '').length} 行
              {following && ' · 实时尾随中'}
              {!following && elapsedMs != null && ` · 耗时 ${elapsedMs}ms`}
            </Typography.Text>
            {following && <Tag color="processing">尾随中</Tag>}
            {followEof && (
              <Tag color={followEof === 'limit' ? 'warning' : 'default'}>
                {FOLLOW_EOF_TEXT[followEof] ?? followEof}
              </Tag>
            )}
            {!following && result?.truncated && (
              <Tag color="warning">输出过大已截断，建议缩小行数或时间范围</Tag>
            )}
          </Space>
        </>
      ) : (
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          选择日志源后点击「查询」；journal 顺带覆盖 cron/ssh 等系统日志。点击「实时尾随」可持续追踪新日志输出。
        </Typography.Text>
      )}
    </Card>
  )
}

export default LogsPanel
