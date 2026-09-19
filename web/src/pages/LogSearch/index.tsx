import { useMemo, useState } from 'react'
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
import { SearchOutlined } from '@ant-design/icons'
import { api } from '@/services/api'
import type { LogsSearchResult } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'
import { GrepLine } from '@/workbench/LogsPanel'

// 跨机日志联邦检索（logs-design.md M3，web 侧 D16）：
// server 对全部在线且带 logs capability 的 agent 并行转发既有 logs.query，
// 按主机分组返回，server 零存储。与 Workbench 单机「日志」Tab 是两条动线
// （单机深查与实时尾随留在 Workbench）；source 手填——跨机源下拉需 N 次
// sources 枚举（扇出放大器），设计上回避。

const MONO = 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace'
const MAX_TAIL = 500 // server 同款收窄上限（D11）

const SINCE_OPTIONS = [
  { value: 0, label: '不限' },
  { value: 5, label: '最近 5 分钟' },
  { value: 60, label: '最近 1 小时' },
  { value: 360, label: '最近 6 小时' },
  { value: 1440, label: '最近 24 小时' },
]

const SKIPPED_REASON: Record<string, string> = {
  offline: '离线',
  'no-logs': '无日志能力（journalctl/docker 均不可用）',
}

const LogSearch = () => {
  // 可选目标：在线且带 logs capability 的 agent（server 侧同规则二次过滤）
  const { data: agents } = useQuery({ queryKey: ['agents'], queryFn: () => api.getAgents() })
  const logAgents = useMemo(
    () => (agents ?? []).filter((a) => (a.capabilities || []).some((c) => c.type === 'logs')),
    [agents],
  )

  const [type, setType] = useState<'systemd' | 'docker'>('systemd')
  const [source, setSource] = useState('')
  const [tail, setTail] = useState(200)
  const [since, setSince] = useState(0)
  const [grep, setGrep] = useState('')
  const [agentIds, setAgentIds] = useState<string[]>([])

  const [searching, setSearching] = useState(false)
  const [result, setResult] = useState<LogsSearchResult | null>(null)

  const submit = async () => {
    const src = source.trim()
    if (!src) {
      message.warning('请填写日志源（systemd unit 名或 docker 容器名）')
      return
    }
    setSearching(true)
    try {
      // 未指定子集时不传 agents = 全部目标（与 server 缺省语义一致）
      const res = await api.searchLogs({
        type,
        source: src,
        tail,
        since_minutes: since,
        grep: grep.trim() || undefined,
        agents: agentIds.length > 0 && agentIds.length < logAgents.length ? agentIds : undefined,
      })
      setResult(res)
    } catch (e) {
      message.error(getApiErrorMessage(e, '检索失败'))
    } finally {
      setSearching(false)
    }
  }

  const totalLines =
    result?.results.reduce((n, r) => n + (r.ok ? (r.lines ?? '').split('\n').filter(Boolean).length : 0), 0) ?? 0

  return (
    <div style={{ maxWidth: 1080, margin: '0 auto', padding: 24 }}>
      <Typography.Title level={4}>日志检索</Typography.Title>
      <Typography.Paragraph type="secondary">
        在全部（或选定）主机的同一日志源上并行执行一次查询，按主机分组返回——适合跨机排查同一服务的错误。
        单机深查与实时尾随请到「工作台 → 日志」。
      </Typography.Paragraph>

      <Card style={{ marginBottom: 16 }}>
        <Space wrap size={8}>
          <Segmented
            value={type}
            onChange={(v) => setType(v as 'systemd' | 'docker')}
            options={[
              { label: 'systemd', value: 'systemd' },
              { label: 'docker', value: 'docker' },
            ]}
          />
          <Input
            style={{ width: 240 }}
            placeholder="日志源，如 nginx.service"
            value={source}
            onChange={(e) => setSource(e.target.value)}
            onPressEnter={submit}
            allowClear
          />
          <Select style={{ width: 130 }} value={since} onChange={setSince} options={SINCE_OPTIONS} />
          <InputNumber
            min={1}
            max={MAX_TAIL}
            value={tail}
            onChange={(v) => setTail(v ?? 200)}
            addonAfter={`行 ≤${MAX_TAIL}`}
            style={{ width: 150 }}
          />
          <Input
            style={{ width: 200 }}
            placeholder="过滤关键字（可选）"
            value={grep}
            onChange={(e) => setGrep(e.target.value)}
            onPressEnter={submit}
            allowClear
          />
          <Select
            mode="multiple"
            style={{ minWidth: 220 }}
            placeholder="全部主机"
            value={agentIds}
            onChange={setAgentIds}
            options={logAgents.map((a) => ({ value: a.id, label: a.hostname || a.id }))}
            allowClear
          />
          <Button type="primary" icon={<SearchOutlined />} loading={searching} onClick={submit}>
            检索
          </Button>
        </Space>
      </Card>

      {result && (
        <>
          {result.skipped.length > 0 && (
            <Alert
              type="warning"
              showIcon
              style={{ marginBottom: 12 }}
              message={`${result.skipped.length} 台主机已跳过`}
              description={result.skipped
                .map((s) => `${s.agentId}：${SKIPPED_REASON[s.reason] ?? s.reason}`)
                .join('；')}
            />
          )}
          {result.results.length === 0 ? (
            <Alert type="info" showIcon message="没有可检索的主机（全部离线或无日志能力）" />
          ) : (
            <Typography.Paragraph type="secondary" style={{ marginBottom: 12 }}>
              {result.results.filter((r) => r.ok).length}/{result.results.length} 台返回，共 {totalLines} 行
            </Typography.Paragraph>
          )}
          {result.results.map((r) => (
            <Card
              key={r.agentId}
              size="small"
              style={{ marginBottom: 12 }}
              title={
                <Space size={8}>
                  <span>{r.hostname || r.agentId}</span>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    {r.agentId}
                  </Typography.Text>
                  {r.ok ? (
                    <Tag color={r.truncated ? 'orange' : 'success'}>
                      {r.truncated ? '已截断（超 1MB）' : `${(r.lines ?? '').split('\n').filter(Boolean).length} 行`}
                    </Tag>
                  ) : (
                    <Tag color="error">失败</Tag>
                  )}
                </Space>
              }
            >
              {r.ok ? (
                <div
                  style={{
                    background: '#001529',
                    color: '#d9d9d9',
                    fontFamily: MONO,
                    fontSize: 12,
                    lineHeight: 1.7,
                    padding: '8px 12px',
                    borderRadius: 6,
                    maxHeight: 360,
                    overflow: 'auto',
                    whiteSpace: 'pre-wrap',
                    wordBreak: 'break-all',
                  }}
                >
                  {(r.lines ?? '').split('\n').map((line, i) => (
                    <GrepLine key={i} line={line} grep={grep.trim()} />
                  ))}
                </div>
              ) : (
                <Typography.Text type="danger">{r.error || '未知错误'}</Typography.Text>
              )}
            </Card>
          ))}
        </>
      )}
    </div>
  )
}

export default LogSearch
