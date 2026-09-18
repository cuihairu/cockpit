import { useEffect, useMemo, useState } from 'react'
import { Alert, Button, Modal, Spin, Typography } from 'antd'
import dayjs from 'dayjs'
import { api } from '@/services/api'
import type { DriftCheckItem, DriftDiffResult } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'
import type { DiffLine } from '@/utils/lineDiff'
import { lineDiff } from '@/utils/lineDiff'
import { KIND_LABEL } from './shared'

interface DiffModalProps {
  agentId: string
  target: DriftCheckItem | null
  onClose: () => void
}

// 行号沟槽：基线侧 / 当前侧各一列，缺失侧留空
const gutter = (l: DiffLine) =>
  `${String(l.oldLine ?? '').padStart(4)} ${String(l.newLine ?? '').padStart(4)} `

const rowBackground = (type: DiffLine['type']): string | undefined =>
  type === 'del' ? 'rgba(255, 77, 79, 0.12)' : type === 'add' ? 'rgba(82, 196, 26, 0.14)' : undefined

// 漂移 diff 视图（drift-design M3/D22）：基线原文 vs 磁盘当前，行级统一 diff。
// 失败透出 agent 原因（无基线 / 基线无原文 / 两侧过大）。
const DiffModal = ({ agentId, target, onClose }: DiffModalProps) => {
  const [data, setData] = useState<DriftDiffResult | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!target) return
    let cancelled = false
    setLoading(true)
    setError('')
    setData(null)
    api
      .driftDiff(agentId, target.kind, target.name)
      .then((res) => {
        if (!cancelled) setData(res)
      })
      .catch((err) => {
        if (!cancelled) setError(getApiErrorMessage(err, '获取差异失败'))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [agentId, target])

  const diff = useMemo(() => (data ? lineDiff(data.expected, data.current) : null), [data])

  return (
    <Modal
      title={target ? `${KIND_LABEL[target.kind]} · ${target.name}` : '内容差异'}
      open={!!target}
      onCancel={onClose}
      width="min(960px, 96vw)"
      footer={
        <Button onClick={onClose} disabled={loading}>
          关闭
        </Button>
      }
    >
      {loading && (
        <div style={{ textAlign: 'center', padding: '48px 0' }}>
          <Spin tip="正在读取两侧内容…" />
        </div>
      )}
      {!loading && error && (
        <Alert
          type="error"
          showIcon
          message="获取差异失败"
          description={
            <>
              {error}
              {error.includes('no content') && ' 到对应管理页重新保存一次即可补全基线原文。'}
            </>
          }
        />
      )}
      {!loading && !error && diff && (
        <>
          {diff.truncated && (
            <Alert
              type="info"
              showIcon
              style={{ marginBottom: 8 }}
              message="内容过长，仅对比前 1000 行"
            />
          )}
          <div
            style={{
              maxHeight: '60vh',
              overflow: 'auto',
              border: '1px solid rgba(128, 128, 128, 0.35)',
              borderRadius: 6,
              fontFamily:
                "'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace",
              fontSize: 12,
              lineHeight: '20px',
            }}
          >
            {diff.lines.map((l, idx) => (
              <div
                key={idx}
                style={{ display: 'flex', background: rowBackground(l.type) }}
              >
                <span style={{ color: 'rgba(128, 128, 128, 0.75)', userSelect: 'none', whiteSpace: 'pre' }}>
                  {gutter(l)}
                </span>
                <span
                  style={{
                    userSelect: 'none',
                    whiteSpace: 'pre',
                    color: l.type === 'del' ? '#cf1322' : l.type === 'add' ? '#389e0d' : undefined,
                  }}
                >
                  {l.type === 'del' ? '-' : l.type === 'add' ? '+' : ' '}
                </span>
                <span style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>{l.text}</span>
              </div>
            ))}
          </div>
          <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 8 }}>
            基线记录于 {data ? dayjs.unix(data.baseline_updated_at).format('YYYY-MM-DD HH:mm:ss') : ''}；
            核对后到对应管理页重新保存可消除漂移。
          </Typography.Text>
        </>
      )}
    </Modal>
  )
}

export default DiffModal
