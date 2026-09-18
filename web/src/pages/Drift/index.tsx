import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  Button,
  Card,
  InputNumber,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd'
import { CheckOutlined, DiffOutlined, SafetyCertificateOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import type { DriftCheckItem, DriftCheckResult } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'
import DiffModal from './DiffModal'
import { KIND_COLOR, KIND_LABEL } from './shared'

// 防漂移检测：面板写路径基线（nginx/cron/stack 最后一次成功保存的内容）
// vs 磁盘当前内容，按需检查（见 docs/guide/drift-design.md）。

// 状态呈现：ok 之外都是需要注意的形态
const STATUS_META: Record<DriftCheckItem['status'], { label: string; color: string; hint: string } | null> = {
  ok: { label: '一致', color: 'success', hint: '与上次面板保存的内容一致' },
  drifted: { label: '已漂移', color: 'error', hint: '磁盘内容与上次面板保存不一致（可能被手改）' },
  missing: { label: '已丢失', color: 'warning', hint: '基线存在但文件已被删除' },
  no_baseline: { label: '未登记', color: 'default', hint: '尚无基线记录，通过面板保存一次即可登记' },
  error: { label: '读取失败', color: 'error', hint: '检查时读取当前内容失败' },
  none: null, // cron 空段无基线：不产生噪音条目
}

const shortSha = (sha: string) => (sha ? sha.slice(0, 8) : '-')

// 防漂移检测页：选 agent → 检查 → 四态清单（server 纯转发，结果不落库）；
// 顶部「自动巡检」行配置 server 定时扫描与漂移告警（M2）
const Drift = () => {
  const [agentId, setAgentId] = useState('')
  const [checking, setChecking] = useState(false)
  const [result, setResult] = useState<DriftCheckResult | null>(null)
  // 巡检编辑态：用户改动才落这里，未编辑时从已保存配置派生（免 setState-in-render）
  const [scanEdit, setScanEdit] = useState<{ on?: boolean; minutes?: number } | null>(null)
  const [savingScan, setSavingScan] = useState(false)
  // diff 视图（M3）：当前打开差异 Modal 的 drifted 条目
  const [diffTarget, setDiffTarget] = useState<DriftCheckItem | null>(null)

  const { data: agents = [] } = useQuery({
    queryKey: ['drift-agents'],
    queryFn: () => api.getAgents(),
  })

  const { data: scanCfg } = useQuery({
    queryKey: ['drift-config'],
    queryFn: () => api.getDriftConfig(),
  })

  const queryClient = useQueryClient()

  const savedInterval = scanCfg?.scan_interval_seconds ?? 0
  const scanOn = scanEdit?.on ?? savedInterval > 0
  const scanMinutes = scanEdit?.minutes ?? (savedInterval > 0 ? Math.round(savedInterval / 60) : 30)

  const saveScanConfig = async () => {
    const seconds = scanOn ? scanMinutes * 60 : 0
    if (scanOn && (scanMinutes < 1 || scanMinutes > 1440)) {
      message.warning('间隔需在 1～1440 分钟之间')
      return
    }
    setSavingScan(true)
    try {
      await api.putDriftConfig(seconds)
      setScanEdit(null) // 回到从服务端配置派生
      await queryClient.invalidateQueries({ queryKey: ['drift-config'] })
      message.success(scanOn ? '已开启自动巡检' : '已关闭自动巡检')
    } catch (err) {
      message.error(getApiErrorMessage(err, '保存失败'))
    } finally {
      setSavingScan(false)
    }
  }

  // 仅呈现具备 drift capability 的 agent
  const capableAgents = agents.filter((a) =>
    (a.capabilities || []).some((c) => c.type === 'drift'),
  )

  const runCheck = async () => {
    if (!agentId) {
      message.warning('请选择服务器')
      return
    }
    setChecking(true)
    try {
      const res = await api.checkDrift(agentId)
      setResult(res)
    } catch (err) {
      message.error(getApiErrorMessage(err, '漂移检查失败'))
    } finally {
      setChecking(false)
    }
  }

  const items = (result?.items ?? []).filter((it) => it.status !== 'none')
  const driftedCount = items.filter((it) => it.status === 'drifted' || it.status === 'missing' || it.status === 'error').length

  // 手动登记基线「以当前为准」（M4）：no_baseline 纳入检测 / drifted 确认
  // 手改为新标准；成功后自动重查刷新清单
  const recordCurrent = async (it: DriftCheckItem) => {
    try {
      await api.driftRecord(agentId, it.kind, it.name)
      message.success(`已登记：${it.name}，之后的漂移检测以当前内容为标准`)
      await runCheck()
    } catch (err) {
      message.error(getApiErrorMessage(err, '登记失败'))
    }
  }

  const columns: ColumnsType<DriftCheckItem> = [
    {
      title: '类型',
      dataIndex: 'kind',
      width: 110,
      render: (kind: DriftCheckItem['kind']) => (
        <Tag color={KIND_COLOR[kind]}>{KIND_LABEL[kind]}</Tag>
      ),
    },
    { title: '对象', dataIndex: 'name', ellipsis: true },
    {
      title: '状态',
      dataIndex: 'status',
      width: 110,
      render: (status: DriftCheckItem['status']) => {
        const meta = STATUS_META[status]
        if (!meta) return null
        return (
          <Tooltip title={meta.hint}>
            <Tag color={meta.color}>{meta.label}</Tag>
          </Tooltip>
        )
      },
    },
    {
      title: '基线',
      dataIndex: 'baseline_sha',
      width: 110,
      render: shortSha,
    },
    {
      title: '当前',
      dataIndex: 'current_sha',
      width: 110,
      render: shortSha,
    },
    {
      title: '操作',
      key: 'actions',
      width: 220,
      render: (_: unknown, it: DriftCheckItem) => (
        <Space size={4}>
          {it.status === 'drifted' && (
            <Button size="small" icon={<DiffOutlined />} onClick={() => setDiffTarget(it)}>
              差异
            </Button>
          )}
          {(it.status === 'drifted' || it.status === 'no_baseline') && (
            <Popconfirm
              title={it.status === 'drifted' ? '以当前磁盘内容为新基线？' : '将当前内容登记为基线？'}
              description="之后的漂移检测将以当前磁盘内容为标准。"
              okText="确认登记"
              cancelText="取消"
              onConfirm={() => void recordCurrent(it)}
            >
              <Button size="small" icon={<CheckOutlined />}>
                {it.status === 'drifted' ? '以当前为准' : '登记'}
              </Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <Card
      title={
        <Space>
          <SafetyCertificateOutlined />
          漂移检测
        </Space>
      }
    >
      <Space wrap style={{ marginBottom: 16 }} align="center">
        <Typography.Text strong>自动巡检</Typography.Text>
        <Tooltip title="开启后 server 定时对全部支持漂移检测的主机执行检查，发现漂移/丢失即产生告警（同一主机未处理期间只提醒一次）">
          <Switch
            checked={scanOn}
            onChange={(on) => setScanEdit((e) => ({ ...e, on }))}
            loading={!scanCfg}
          />
        </Tooltip>
        {scanOn && (
          <InputNumber
            min={1}
            max={1440}
            value={scanMinutes}
            onChange={(v) => setScanEdit((e) => ({ ...e, minutes: v ?? 1 }))}
            addonAfter="分钟"
            style={{ width: 130 }}
          />
        )}
        <Button size="small" onClick={() => void saveScanConfig()} loading={savingScan}>
          保存
        </Button>
      </Space>
      <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 16 }}>
        自动巡检按间隔扫描全部支持的主机并推送漂移告警；下方为单台主机的即时检查。
      </Typography.Text>

      <Space wrap style={{ marginBottom: 16 }}>
        <Select
          showSearch
          optionFilterProp="label"
          style={{ minWidth: 240 }}
          placeholder="选择服务器"
          value={agentId || undefined}
          onChange={setAgentId}
          options={capableAgents.map((a) => ({
            value: a.id,
            label: a.hostname || a.id,
          }))}
          notFoundContent="无支持漂移检测的 Agent（需 nginx/cron/stack 任一能力）"
        />
        <Button type="primary" onClick={() => void runCheck()} loading={checking}>
          检查
        </Button>
      </Space>

      {result && (
        <>
          {driftedCount > 0 ? (
            <Alert
              type="warning"
              showIcon
              style={{ marginBottom: 12 }}
              message={`发现 ${driftedCount} 项漂移/异常，请核对是否为本人操作`}
            />
          ) : (
            <Alert
              type="success"
              showIcon
              style={{ marginBottom: 12 }}
              message="全部一致，未发现漂移"
            />
          )}
          <Table<DriftCheckItem>
            rowKey={(it) => `${it.kind}/${it.name}`}
            columns={columns}
            dataSource={items}
            pagination={false}
            size="small"
            locale={{ emptyText: '无可检测对象（通过反向代理/定时任务/应用部署页保存一次即产生基线）' }}
          />
          <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 8 }}>
            基线 = 各管理页（反向代理/定时任务/应用部署）最后一次通过面板成功保存的内容；
            「未登记」对象可通过面板保存一次，或直接「登记」以当前磁盘内容为准。
          </Typography.Text>
        </>
      )}

      <DiffModal agentId={agentId} target={diffTarget} onClose={() => setDiffTarget(null)} />
    </Card>
  )
}

export default Drift
