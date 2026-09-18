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
import { ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import { api } from '@/services/api'
import type { LogsQueryResult } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'

// LogsPanel 远程日志查询（见 docs/guide/logs-design.md）：
// 源选择（systemd 服务 / docker 容器）+ tail/since/grep 过滤 → agent 实时执行
// journalctl / docker logs，整段文本回显（级别着色 + grep 高亮 + 自动滚底）。

const SINCE_OPTIONS = [
  { value: 0, label: '不限' },
  { value: 5, label: '最近 5 分钟' },
  { value: 60, label: '最近 1 小时' },
  { value: 360, label: '最近 6 小时' },
  { value: 1440, label: '最近 24 小时' },
]

// 行级着色：ERROR/FATAL 红、WARN 橙；返回 null 用默认色
const lineTone = (line: string): string | null => {
  const upper = line.toUpperCase()
  if (upper.includes('ERROR') || upper.includes('FATAL') || upper.includes('PANIC')) return '#ff6b6b'
  if (upper.includes('WARN')) return '#ffa940'
  return null
}

// GrepLine 单行渲染：grep 命中片段高亮（大小写敏感，与 agent contains 行为一致）
const GrepLine = ({ line, grep }: { line: string; grep: string }) => {
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

  // 新结果自动滚动到底部（最新日志在末尾）
  useEffect(() => {
    if (viewRef.current) {
      viewRef.current.scrollTop = viewRef.current.scrollHeight
    }
  }, [result])

  // 外部预选源：logs.status 就绪后自动首查一次（useRef 守卫防重复）
  const autoQueried = useRef(false)
  useEffect(() => {
    if (!initialSource || autoQueried.current || !status) return
    autoQueried.current = true
    if (typeAvailable.systemd && source) void runQuery()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [status, initialSource, source, typeAvailable])

  const lines = useMemo(() => (result ? result.lines.split('\n') : []), [result])

  return (
    <Card
      type="inner"
      title="日志"
      loading={statusLoading}
      extra={
        <Button icon={<ReloadOutlined />} onClick={() => void runQuery()} loading={querying}>
          刷新
        </Button>
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
        />
        <Select
          value={sinceMinutes}
          onChange={setSinceMinutes}
          options={SINCE_OPTIONS}
          style={{ width: 130 }}
        />
        <InputNumber
          min={1}
          max={2000}
          value={tail}
          onChange={(v) => setTail(v ?? 200)}
          addonAfter="行"
          style={{ width: 120 }}
        />
        <Input
          allowClear
          placeholder="关键词过滤"
          prefix={<SearchOutlined />}
          value={grep}
          onChange={(e) => setGrep(e.target.value)}
          onPressEnter={() => void runQuery()}
          style={{ width: 180 }}
        />
        <Button type="primary" onClick={() => void runQuery()} loading={querying}>
          查询
        </Button>
      </Space>

      {result ? (
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
            {lines.map((line, i) => (
              <GrepLine key={i} line={line} grep={grep} />
            ))}
          </div>
          <Space size={8} style={{ marginTop: 8 }}>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              共 {lines.filter((l) => l !== '').length} 行
              {elapsedMs != null && ` · 耗时 ${elapsedMs}ms`}
            </Typography.Text>
            {result.truncated && (
              <Tag color="warning">输出过大已截断，建议缩小行数或时间范围</Tag>
            )}
          </Space>
        </>
      ) : (
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          选择日志源后点击「查询」；journal 顺带覆盖 cron/ssh 等系统日志。实时尾随与服务端聚合在后续版本。
        </Typography.Text>
      )}
    </Card>
  )
}

export default LogsPanel
