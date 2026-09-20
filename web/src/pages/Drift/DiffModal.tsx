import { useMemo, useState } from 'react'
import type { CSSProperties } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Alert, Button, Modal, Segmented, Spin, Typography } from 'antd'
import dayjs from 'dayjs'
import { api } from '@/services/api'
import type { DriftCheckItem } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'
import { highlightConfigLine } from '@/utils/configHighlight'
import type { DiffLine, DiffPair } from '@/utils/lineDiff'
import { lineDiff, pairDiffLines } from '@/utils/lineDiff'
import { KIND_LABEL } from './shared'

interface DiffModalProps {
  agentId: string
  target: DriftCheckItem | null
  onClose: () => void
}

const MONO =
  "'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace"
const GUTTER_COLOR = 'rgba(128, 128, 128, 0.75)'
const BG_DEL = 'rgba(255, 77, 79, 0.12)'
const BG_ADD = 'rgba(82, 196, 26, 0.14)'
const BG_EMPTY = 'rgba(128, 128, 128, 0.08)'
const SPLIT_BORDER = '1px solid rgba(128, 128, 128, 0.3)'

const rowBackground = (type: DiffLine['type']): string | undefined =>
  type === 'del' ? BG_DEL : type === 'add' ? BG_ADD : undefined

const signOf = (type?: DiffLine['type']): string =>
  type === 'del' ? '-' : type === 'add' ? '+' : ' '

const signColor = (type?: DiffLine['type']): string | undefined =>
  type === 'del' ? '#cf1322' : type === 'add' ? '#389e0d' : undefined

// 着色后的行内容（配置轻量词法，宁缺毋错——见 configHighlight.ts）
const Highlighted = ({ text }: { text: string }) => (
  <>
    {highlightConfigLine(text).map((t, i) =>
      t.color ? (
        <span key={i} style={{ color: t.color }}>
          {t.text}
        </span>
      ) : (
        <span key={i}>{t.text}</span>
      ),
    )}
  </>
)

const num4 = (n?: number) => String(n ?? '').padStart(4)

const gutterStyle: CSSProperties = {
  color: GUTTER_COLOR,
  userSelect: 'none',
  whiteSpace: 'pre',
  flexShrink: 0,
}

// 统一视图一行：双行号沟槽 + 符号 + 内容
const UnifiedRow = ({ l }: { l: DiffLine }) => (
  <div style={{ display: 'flex', background: rowBackground(l.type) }}>
    <span style={gutterStyle}>{`${num4(l.oldLine)} ${num4(l.newLine)} `}</span>
    <span style={{ userSelect: 'none', whiteSpace: 'pre', color: signColor(l.type), flexShrink: 0 }}>
      {signOf(l.type)}
    </span>
    <span style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
      <Highlighted text={l.text} />
    </span>
  </div>
)

// 对照视图半边：单侧行号 + 符号 + 内容；缺位侧画底纹
const Half = ({ l, right }: { l?: DiffLine; right: boolean }) => (
  <div
    style={{
      flex: '1 1 0',
      minWidth: 0,
      display: 'flex',
      background: l ? rowBackground(l.type) : BG_EMPTY,
      borderLeft: right ? SPLIT_BORDER : undefined,
    }}
  >
    <span style={gutterStyle}>{` ${num4(l?.oldLine ?? l?.newLine)} `}</span>
    <span style={{ userSelect: 'none', whiteSpace: 'pre', color: signColor(l?.type), flexShrink: 0 }}>
      {l ? signOf(l.type) : ' '}
    </span>
    <span style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
      {l ? <Highlighted text={l.text} /> : ' '}
    </span>
  </div>
)

// 漂移 diff 视图（drift-design M3/D22 + M5/D26）：基线原文 vs 磁盘当前，
// 行级 LCS diff，统一 / 双栏对照两种视图 + 配置轻量着色。
// 失败透出 agent 原因（无基线 / 基线无原文 / 两侧过大）。
const DiffModal = ({ agentId, target, onClose }: DiffModalProps) => {
  const [view, setView] = useState<'side' | 'unified'>('side')

  // 弹窗打开（target 非空）才拉取；失败透出 agent 原因
  // （无基线 / 基线无原文 / 两侧过大）。retry:false 保持与原手写链一致
  const { data, error, isLoading: loading } = useQuery({
    queryKey: ['drift-diff', agentId, target?.kind, target?.name],
    queryFn: () => api.driftDiff(agentId, target!.kind, target!.name),
    enabled: !!target,
    retry: false,
  })
  const errorText = error ? getApiErrorMessage(error, '获取差异失败') : ''

  const diff = useMemo(() => (data ? lineDiff(data.expected, data.current) : null), [data])
  const pairs = useMemo(
    () => (diff && view === 'side' ? pairDiffLines(diff.lines) : null),
    [diff, view],
  )

  return (
    <Modal
      title={target ? `${KIND_LABEL[target.kind]} · ${target.name}` : '内容差异'}
      open={!!target}
      onCancel={onClose}
      width="min(1080px, 96vw)"
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
      {!loading && errorText && (
        <Alert
          type="error"
          showIcon
          message="获取差异失败"
          description={
            <>
              {errorText}
              {errorText.includes('no content') && ' 到对应管理页重新保存一次即可补全基线原文。'}
            </>
          }
        />
      )}
      {!loading && !errorText && diff && (
        <>
          <div style={{ marginBottom: 8, display: 'flex', justifyContent: 'flex-end' }}>
            <Segmented
              value={view}
              onChange={(v) => setView(v as 'side' | 'unified')}
              options={[
                { value: 'side', label: '对照' },
                { value: 'unified', label: '统一' },
              ]}
            />
          </div>
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
              border: SPLIT_BORDER,
              borderRadius: 6,
              fontFamily: MONO,
              fontSize: 12,
              lineHeight: '20px',
            }}
          >
            {pairs
              ? pairs.map((p: DiffPair, idx) => (
                  <div key={idx} style={{ display: 'flex' }}>
                    <Half l={p.left} right={false} />
                    <Half l={p.right} right />
                  </div>
                ))
              : diff.lines.map((l, idx) => <UnifiedRow key={idx} l={l} />)}
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
