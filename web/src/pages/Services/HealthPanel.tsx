import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  Badge,
  Button,
  Card,
  Drawer,
  Empty,
  Form,
  Input,
  InputNumber,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd'
import { PlusOutlined, ReloadOutlined, SettingOutlined, ThunderboltOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import type { HealthProbe, HealthStateEntry } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'
import { usePerm } from '@/hooks/usePerm'

// 服务健康探针面板（见 docs/guide/service-health-design.md）：探针运行态
// （在线实时拉取 drain=false，离线回落 server 灰态缓存）+ 配置编辑（探针/
// 白名单，server 保存即校验并在在线时推送，D3/D6）+ 立即探测（诊断动作，
// 读语义不审计）。自愈三态的审计/告警在 server 归集循环落库，这里只展示
// 最近一次自愈决策（lastHeal，agent 侧状态快照自带）。

// 探针状态 → 徽标
const STATUS_META: Record<string, { status: 'success' | 'error' | 'default'; label: string }> = {
  ok: { status: 'success', label: '正常' },
  fail: { status: 'error', label: '失败' },
  unknown: { status: 'default', label: '待测' },
}

// 自愈结果 → Tag（restarted/failed 来自 agent 事件；blocked/backoff 只告警）
const HEAL_META: Record<string, { color: string; label: string }> = {
  restarted: { color: 'green', label: '已自愈' },
  failed: { color: 'red', label: '自愈失败' },
  blocked: { color: 'orange', label: '白名单拦截' },
  backoff: { color: 'orange', label: '退避中' },
}

const PROBE_TYPE_LABELS: Record<HealthProbe['type'], string> = {
  http: 'HTTP 状态码',
  tcp: 'TCP 端口',
  systemd: 'systemd active',
}

// 表单里的探针行（数值控件可能给 null，提交前归一为 undefined 让 0 缺省
// 语义在 server 侧生效）
type ProbeFormRow = Omit<HealthProbe, 'expectStatus' | 'intervalSec' | 'timeoutSec' | 'failThreshold' | 'backoffWindowSec' | 'maxRestartsInWindow'> & {
  expectStatus?: number | null
  intervalSec?: number | null
  timeoutSec?: number | null
  failThreshold?: number | null
  backoffWindowSec?: number | null
  maxRestartsInWindow?: number | null
}

const normalizeProbe = (p: ProbeFormRow): HealthProbe => ({
  ...p,
  expectStatus: p.expectStatus ?? undefined,
  intervalSec: p.intervalSec ?? undefined,
  timeoutSec: p.timeoutSec ?? undefined,
  failThreshold: p.failThreshold ?? undefined,
  backoffWindowSec: p.backoffWindowSec ?? undefined,
  maxRestartsInWindow: p.maxRestartsInWindow ?? undefined,
})

const HealthPanel = ({ agentId }: { agentId: string }) => {
  const canWrite = usePerm('services:write')
  const queryClient = useQueryClient()
  const [form] = Form.useForm<{ probes: ProbeFormRow[]; whitelist: string[] }>()
  const [editing, setEditing] = useState(false)
  const [checking, setChecking] = useState<string>()

  const { data, isLoading } = useQuery({
    queryKey: ['agent-health', agentId],
    queryFn: () => api.getAgentHealth(agentId),
    refetchInterval: 30_000,
  })

  const refresh = () => queryClient.invalidateQueries({ queryKey: ['agent-health', agentId] })

  const saveMut = useMutation({
    mutationFn: (body: { probes: HealthProbe[]; whitelist: string[] }) => api.saveAgentHealth(agentId, body),
    onSuccess: (res) => {
      message.success(
        res.pushed
          ? `已保存并下发（${res.applied} 条探针）`
          : `已保存（${res.applied} 条探针）；agent 离线，上线后自动补推`,
      )
      setEditing(false)
      refresh()
    },
    onError: (err) => message.error(getApiErrorMessage(err, '保存失败')),
  })

  const checkMut = useMutation({
    mutationFn: (probeId: string) => api.checkAgentProbe(agentId, probeId),
    onSuccess: (res) => {
      if (res.status === 'ok') message.success(`探针 ${res.probe} 本次探测正常`)
      else message.warning(`探针 ${res.probe} 本次探测失败${res.lastError ? `：${res.lastError}` : ''}`)
      refresh()
    },
    onError: (err) => message.error(getApiErrorMessage(err, '探测失败')),
    onSettled: (_res, _err, probeId) => {
      if (checking === probeId) setChecking(undefined)
    },
  })

  const openEditor = () => {
    form.setFieldsValue({
      probes: data?.config?.probes ?? [],
      whitelist: data?.config?.whitelist ?? [],
    })
    setEditing(true)
  }

  const save = async () => {
    let values: { probes?: ProbeFormRow[]; whitelist?: string[] }
    try {
      values = await form.validateFields()
    } catch {
      return // 校验失败：antd 已在字段上标错
    }
    saveMut.mutate({
      probes: (values.probes ?? []).map(normalizeProbe),
      whitelist: values.whitelist ?? [],
    })
  }

  // 状态表按配置里的探针为基准（探针从未跑过也有 unknown 一行）
  const rows = useMemo(() => {
    const probes = data?.config?.probes ?? []
    return probes.map((p) => ({
      probe: p,
      state: data?.states?.[p.id] ?? ({ status: 'unknown' } as HealthStateEntry),
    }))
  }, [data])

  const columns: ColumnsType<{ probe: HealthProbe; state: HealthStateEntry }> = [
    {
      title: '探针',
      dataIndex: ['probe', 'id'],
      key: 'id',
      width: 240,
      render: (v: string, r) => (
        <Space size={8}>
          <Typography.Text code style={{ fontSize: 12 }}>
            {v}
          </Typography.Text>
          <Tag>{PROBE_TYPE_LABELS[r.probe.type]}</Tag>
          {r.probe.heal && (
            <Tooltip title={`自愈目标 ${r.probe.unit ?? r.probe.target}（连续失败达阈值自动重启）`}>
              <Tag color="volcano">自愈</Tag>
            </Tooltip>
          )}
        </Space>
      ),
    },
    {
      title: '目标',
      dataIndex: ['probe', 'target'],
      key: 'target',
      width: 260,
      ellipsis: true,
      render: (v: string, r) => (
        <Typography.Text copyable={false} style={{ fontSize: 12 }} ellipsis={{ tooltip: v }}>
          {r.probe.type === 'http' ? `${v}（期望 ${r.probe.expectStatus ?? 200}）` : v}
        </Typography.Text>
      ),
    },
    {
      title: '状态',
      key: 'status',
      width: 180,
      render: (_, r) => {
        const meta = STATUS_META[r.state.status] ?? { status: 'default' as const, label: r.state.status }
        const badge = <Badge status={meta.status} text={meta.label} />
        const fails = r.state.consecutiveFails ?? 0
        return (
          <Space size={8}>
            {r.state.lastError ? (
              <Tooltip title={<pre style={{ margin: 0, whiteSpace: 'pre-wrap', fontSize: 12 }}>{r.state.lastError}</pre>}>
                {badge}
              </Tooltip>
            ) : (
              badge
            )}
            {r.state.status === 'fail' && fails > 1 && <Tag color="red">连续 {fails} 次</Tag>}
          </Space>
        )
      },
    },
    {
      title: '最近检查',
      dataIndex: ['state', 'lastCheck'],
      key: 'lastCheck',
      width: 150,
      render: (v?: number) => (v ? dayjs.unix(v).format('YYYY-MM-DD HH:mm:ss') : '—'),
    },
    {
      title: '最近自愈',
      dataIndex: ['state', 'lastHeal'],
      key: 'lastHeal',
      width: 220,
      render: (_, r) => {
        const h = r.state.lastHeal
        if (!h) return '—'
        const meta = HEAL_META[h.result] ?? { color: 'default', label: h.result }
        return (
          <Tooltip title={`${h.detail}（耗时 ${h.durationMs}ms）`}>
            <Tag color={meta.color}>
              {meta.label} {dayjs.unix(h.time).format('MM-DD HH:mm')}
            </Tag>
          </Tooltip>
        )
      },
    },
    {
      title: '操作',
      key: 'actions',
      width: 110,
      render: (_, r) =>
        canWrite && (
          <Tooltip title="立即探测一次（诊断，不影响周期探测）">
            <Button
              size="small"
              type="text"
              icon={<ThunderboltOutlined />}
              loading={checking === r.probe.id}
              onClick={() => {
                setChecking(r.probe.id)
                checkMut.mutate(r.probe.id)
              }}
            >
              探测
            </Button>
          </Tooltip>
        ),
    },
  ]

  return (
    <Card
      title="健康探针"
      extra={
        <Space>
          {canWrite && (
            <Button icon={<SettingOutlined />} onClick={openEditor}>
              编辑探针
            </Button>
          )}
          <Button icon={<ReloadOutlined />} onClick={refresh} />
        </Space>
      }
    >
      {!data?.config ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="该主机尚未配置健康探针">
          {canWrite && (
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              点右上角「编辑探针」添加：HTTP 状态码 / TCP 端口 / systemd active 三类，连续失败可触发白名单内服务自动重启
            </Typography.Text>
          )}
        </Empty>
      ) : (
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
          {!data.online && (
            <Alert
              type="warning"
              showIcon
              message="主机离线：以下为最近一次归集的状态快照（探针在 agent 侧照常运行，重连后恢复实时）"
            />
          )}
          {data.config.updatedAt ? (
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              配置更新于 {dayjs.unix(data.config.updatedAt).format('YYYY-MM-DD HH:mm')}
              {data.config.updatedBy ? `（by ${data.config.updatedBy}）` : ''}
              ；自愈目标白名单：{data.config.whitelist.length ? data.config.whitelist.join('、') : '无'}
            </Typography.Text>
          ) : null}
          <Table
            rowKey={(r) => r.probe.id}
            size="small"
            columns={columns}
            dataSource={rows}
            loading={isLoading}
            pagination={false}
          />
        </Space>
      )}

      <Drawer
        title="编辑健康探针"
        width="min(880px, 96vw)"
        open={editing}
        onClose={() => setEditing(false)}
        footer={
          <Space style={{ float: 'right' }}>
            <Button onClick={() => setEditing(false)}>取消</Button>
            <Button type="primary" loading={saveMut.isPending} onClick={() => void save()}>
              保存
            </Button>
          </Space>
        }
      >
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
          message="配置保存即校验并推送；自愈目标必须在白名单内（双端各校验一道，非白名单只告警不动手）。连续失败达阈值才触发自愈，重启按 unit 记退避额度防风暴。"
        />
        <Form form={form} layout="vertical" requiredMark={false}>
          <Form.List name="probes">
            {(fields, { add, remove }) => (
              <>
                {fields.map((field) => (
                  <Space key={field.key} align="baseline" wrap size={4} style={{ display: 'flex', marginBottom: 4 }}>
                    <Form.Item
                      name={[field.name, 'id']}
                      rules={[
                        { required: true, message: '必填' },
                        { pattern: /^[a-z0-9][a-z0-9-]{0,63}$/, message: '小写字母/数字/短横线' },
                      ]}
                      style={{ width: 150, marginBottom: 4 }}
                    >
                      <Input placeholder="id（如 cloudflared-ready）" size="small" />
                    </Form.Item>
                    <Form.Item name={[field.name, 'type']} initialValue="http" rules={[{ required: true }]} style={{ width: 130, marginBottom: 4 }}>
                      <Select
                        size="small"
                        options={[
                          { value: 'http', label: 'HTTP 状态码' },
                          { value: 'tcp', label: 'TCP 端口' },
                          { value: 'systemd', label: 'systemd active' },
                        ]}
                        onChange={() => form.setFieldValue(['probes', field.name, 'target'], '')}
                      />
                    </Form.Item>
                    <Form.Item noStyle shouldUpdate={(prev, cur) => prev.probes?.[field.name]?.type !== cur.probes?.[field.name]?.type}>
                      {() => {
                        const t = form.getFieldValue(['probes', field.name, 'type'])
                        return (
                          <Form.Item
                            name={[field.name, 'target']}
                            rules={[{ required: true, message: '必填' }]}
                            style={{ width: t === 'http' ? 300 : 200, marginBottom: 4 }}
                          >
                            <Input
                              size="small"
                              placeholder={
                                t === 'http'
                                  ? 'http://127.0.0.1:20241/ready'
                                  : t === 'tcp'
                                    ? '127.0.0.1:443'
                                    : 'cloudflared.service'
                              }
                            />
                          </Form.Item>
                        )
                      }}
                    </Form.Item>
                    <Form.Item noStyle shouldUpdate={(prev, cur) => prev.probes?.[field.name]?.type !== cur.probes?.[field.name]?.type}>
                      {() => {
                        const t = form.getFieldValue(['probes', field.name, 'type'])
                        return t === 'http' ? (
                          <Form.Item name={[field.name, 'expectStatus']} style={{ width: 110, marginBottom: 4 }}>
                            <InputNumber size="small" min={100} max={599} placeholder="期望状态码" style={{ width: '100%' }} />
                          </Form.Item>
                        ) : null
                      }}
                    </Form.Item>
                    <Form.Item name={[field.name, 'intervalSec']} style={{ width: 100, marginBottom: 4 }}>
                      <InputNumber size="small" min={5} max={3600} placeholder="间隔秒" style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name={[field.name, 'failThreshold']} style={{ width: 100, marginBottom: 4 }}>
                      <InputNumber size="small" min={1} max={60} placeholder="失败阈值" style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name={[field.name, 'heal']} valuePropName="checked" style={{ marginBottom: 4 }}>
                      <Switch size="small" checkedChildren="自愈" unCheckedChildren="只告警" />
                    </Form.Item>
                    <Form.Item noStyle shouldUpdate={(prev, cur) => prev.probes?.[field.name]?.heal !== cur.probes?.[field.name]?.heal}>
                      {() =>
                        form.getFieldValue(['probes', field.name, 'heal']) ? (
                          <Form.Item
                            name={[field.name, 'unit']}
                            rules={[{ pattern: /^[A-Za-z0-9@._+-]+\.service$/, message: '*.service' }]}
                            style={{ width: 190, marginBottom: 4 }}
                          >
                            <Input size="small" placeholder="自愈目标 *.service" />
                          </Form.Item>
                        ) : null
                      }
                    </Form.Item>
                    <Button size="small" type="text" danger onClick={() => remove(field.name)}>
                      删除
                    </Button>
                  </Space>
                ))}
                <Button size="small" icon={<PlusOutlined />} onClick={() => add({ type: 'http' })}>
                  添加探针
                </Button>
              </>
            )}
          </Form.List>
          <Form.Item
            name="whitelist"
            label="自愈白名单（*.service，白名单内的 unit 才允许自动重启）"
            style={{ marginTop: 16 }}
          >
            <Select mode="tags" tokenSeparators={[',', ' ']} placeholder="如 cloudflared.service" open={false} />
          </Form.Item>
        </Form>
      </Drawer>
    </Card>
  )
}

export default HealthPanel
