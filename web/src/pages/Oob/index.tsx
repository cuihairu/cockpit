import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  Button,
  Card,
  Empty,
  Form,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  message,
} from 'antd'
import { ApiOutlined, PlusOutlined, ThunderboltOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { Agent, OobRedirectRule, OobRedirectSpec } from '@/types'
import { api } from '@/services/api'
import { getApiErrorMessage } from '@/utils/apiError'
import { PermGuard } from '@/components/PermGuard'

// 带外兜底转发编排（docs/design/oob-openwrt-bootstrap.md 片C）：
// server 纯转发不落库，凭据走 vault oob/<name> 条目（用户在此输入条目名），
// 写动作一律双确认——确认字段必须与目标逐字一致（后端逐字校验，误触无法触发）。
// RBAC：读走 inventory:read，写走 inventory:write（/api/oob 前缀规则）。

// 规则名形如 cockpit_<label>_<expiryUnix>（internal/oob/redirect.RuleName）；
// 过期账在名字里，这里反解出过期时刻用于展示与「已过期」标注。
const parseExpiry = (name: string): number | null => {
  const m = /^cockpit_.+_(\d{10})$/.exec(name)
  return m ? Number(m[1]) : null
}

// 规则名里的过期账是否已到点（模块级 helper：渲染体内不直接调 Date.now）。
const isExpired = (name: string): boolean => {
  const exp = parseExpiry(name)
  return exp !== null && exp * 1000 < Date.now()
}

// 后端只允许拆除 cockpit_ 前缀规则（redirect.Owned，否则 400）。UI 必须与
// 后端同口径：非 cockpit 前缀规则不渲染拆除入口，避免发起必败动作（审计）。
const isOwned = (name: string): boolean => name.startsWith('cockpit_')

// 原始 ubus 规则字段的摘要展示（字段名与设备同形，防御式取值）。
const ruleSummary = (r: OobRedirectRule): string => {
  const parts: string[] = []
  const proto = r['proto']
  const src = r['src']
  const dest = r['dest']
  if (typeof proto === 'string') parts.push(proto)
  if (typeof src === 'string') parts.push(src)
  if (typeof dest === 'string') parts.push(`→${dest}`)
  const dstIP = r['dest_ip']
  const dstPort = r['dest_port']
  if (typeof dstIP === 'string' && dstIP !== '') {
    parts.push(typeof dstPort === 'number' || typeof dstPort === 'string' ? `${dstIP}:${dstPort}` : dstIP)
  }
  return parts.join(' ') || '—'
}

interface CreateFormValues {
  label?: string
  proto: string
  srcZone: string
  dstZone: string
  srcIp?: string
  dstIp: string
  srcPort?: number
  dstPort: number
  ttlSeconds: number
  confirmLabel: string
}

const OobRedirectManager: React.FC = () => {
  const queryClient = useQueryClient()
  const [agentId, setAgentId] = useState<string | undefined>()
  const [oob, setOob] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const [removeTarget, setRemoveTarget] = useState<OobRedirectRule | null>(null)
  const [sweepOpen, setSweepOpen] = useState(false)
  const [createForm] = Form.useForm<CreateFormValues>()
  const [removeForm] = Form.useForm<{ confirmName: string }>()
  const [sweepForm] = Form.useForm<{ confirmOob: string }>()
  // 响应式读取 label：占位符与确认提示要随输入实时变化，getFieldValue 在
  // 渲染期读一次不更新，改用 useWatch（审计：占位符误导）。
  const labelValue = Form.useWatch('label', createForm)

  const { data: agents = [] } = useQuery({ queryKey: ['agents'], queryFn: () => api.getAgents() })
  const oobAgents = useMemo(
    () => agents.filter((a) => a.status !== 'offline' && (a.capabilities ?? []).some((c) => c.type === 'oob')),
    [agents],
  )
  // 未显式选择时渲染期派生默认选中（避免 effect 内 setState）。显式选中的
  // 主机掉线（不在 oobAgents）时回退到首个在线 oob 主机：effectiveAgentId
  // 不再指向离线机，列表查询不会被启用到已离线 agent（审计发现）。
  const effectiveAgentId = oobAgents.some((a) => a.id === agentId) ? agentId : oobAgents[0]?.id
  const selectedAgent: Agent | undefined = useMemo(
    () => oobAgents.find((a) => a.id === effectiveAgentId),
    [oobAgents, effectiveAgentId],
  )
  const oobTrimmed = oob.trim()
  // 拆除弹窗常开判定用：Modal 关闭态（destroyOnClose）也在渲染标题/属性，
  // 取值收敛到这里兜底空串，避免子树里散落可空链。
  const removeName = removeTarget?.name ?? ''

  const listQuery = useQuery({
    queryKey: ['oob-redirects', effectiveAgentId, oobTrimmed],
    queryFn: () => api.oobRedirectList(effectiveAgentId!, oobTrimmed),
    enabled: !!effectiveAgentId && oobTrimmed !== '',
    retry: false,
  })

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['oob-redirects'] })

  const createMutation = useMutation({
    mutationFn: (v: { spec: OobRedirectSpec; confirmLabel: string }) =>
      api.oobRedirectApply({ agentId: effectiveAgentId!, oob: oobTrimmed, spec: v.spec, confirmLabel: v.confirmLabel }),
    onSuccess: (r) => {
      message.success(`转发规则已下发：${r.name}`)
      setCreateOpen(false)
      void invalidate()
    },
    onError: (e) => message.error(getApiErrorMessage(e, '转发规则下发失败')),
  })

  const removeMutation = useMutation({
    mutationFn: (v: { name: string; confirmName: string }) =>
      api.oobRedirectRemove({ agentId: effectiveAgentId!, oob: oobTrimmed, name: v.name, confirmName: v.confirmName }),
    onSuccess: () => {
      message.success('转发规则已拆除')
      setRemoveTarget(null)
      void invalidate()
    },
    onError: (e) => message.error(getApiErrorMessage(e, '转发规则拆除失败')),
  })

  const sweepMutation = useMutation({
    mutationFn: (v: { confirmOob: string }) =>
      api.oobRedirectSweep({ agentId: effectiveAgentId!, oob: oobTrimmed, confirmOob: v.confirmOob }),
    onSuccess: (r) => {
      message.success(r.removed.length > 0 ? `已清扫 ${r.removed.length} 条过期规则` : '没有过期的 cockpit 规则')
      setSweepOpen(false)
      void invalidate()
    },
    onError: (e) => message.error(getApiErrorMessage(e, '过期规则清扫失败')),
  })

  const submitCreate = async () => {
    // 校验失败（如确认字段不一致）是预期路径：antd 表单就地显示错误，
    // 这里吞掉 rejection 避免 unhandled rejection（不进入 mutation）。
    const v = await createForm.validateFields().catch(() => null)
    if (!v) return
    createMutation.mutate({
      spec: {
        label: v.label ?? '',
        proto: v.proto,
        src_zone: v.srcZone,
        dst_zone: v.dstZone,
        dst_ip: v.dstIp,
        dst_port: v.dstPort,
        ttl_seconds: v.ttlSeconds,
        ...(v.srcIp ? { src_ip: v.srcIp } : {}),
        ...(v.srcPort ? { src_port: v.srcPort } : {}),
      },
      confirmLabel: v.confirmLabel ?? '',
    })
  }

  const submitRemove = async () => {
    const v = await removeForm.validateFields().catch(() => null)
    if (!v) return
    removeMutation.mutate({ name: removeName, confirmName: v.confirmName })
  }

  const submitSweep = async () => {
    const v = await sweepForm.validateFields().catch(() => null)
    if (!v) return
    sweepMutation.mutate({ confirmOob: v.confirmOob })
  }

  const columns: ColumnsType<OobRedirectRule> = [
    {
      title: '规则名',
      key: 'name',
      render: (_, r) => (
        <Space size={4}>
          {parseExpiry(r.name) !== null && <Tag color="geekblue">cockpit</Tag>}
          <Typography.Text copyable={{ text: r.name }} code>
            {r.name}
          </Typography.Text>
        </Space>
      ),
    },
    {
      title: '摘要',
      key: 'summary',
      render: (_, r) => <Typography.Text type="secondary">{ruleSummary(r)}</Typography.Text>,
    },
    {
      title: '到期',
      key: 'expiry',
      width: 180,
      render: (_, r) => {
        const exp = parseExpiry(r.name)
        if (exp === null) return <Typography.Text type="secondary">—</Typography.Text>
        return isExpired(r.name) ? <Tag color="error">已过期</Tag> : new Date(exp * 1000).toLocaleString()
      },
    },
    {
      title: '操作',
      key: 'action',
      width: 100,
      render: (_, r) => (
        <PermGuard perm="inventory:write">
          {isOwned(r.name) && (
            <Button size="small" danger onClick={() => { setRemoveTarget(r); removeForm.resetFields() }}>
              拆除
            </Button>
          )}
        </PermGuard>
      ),
    },
  ]

  const rules = listQuery.data?.redirects ?? []
  const expiredCount = rules.filter((r) => isExpired(r.name)).length

  return (
    <div style={{ padding: 24 }}>
      <Card
        title={
          <Space>
            <ApiOutlined />
            带外兜底转发
          </Space>
        }
        extra={
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            凭据取 vault 条目（oob/&lt;name&gt;），写动作需双确认
          </Typography.Text>
        }
      >
        {oobAgents.length === 0 ? (
          <Empty description="暂无带 oob capability 的在线主机——装有 ubus（OpenWrt）的主机运行 Agent 即可" />
        ) : (
          <>
            <Space wrap style={{ marginBottom: 16 }}>
              <Select
                style={{ width: 220 }}
                placeholder="选择主机"
                value={effectiveAgentId}
                onChange={setAgentId}
                options={oobAgents.map((a) => ({ value: a.id, label: a.hostname || a.id }))}
              />
              <Input
                style={{ width: 240 }}
                placeholder="vault 条目名（oob/&lt;name&gt;）"
                value={oob}
                onChange={(e) => setOob(e.target.value)}
                allowClear
              />
              <PermGuard perm="inventory:write">
                <Button
                  type="primary"
                  icon={<PlusOutlined />}
                  disabled={!effectiveAgentId || oobTrimmed === ''}
                  onClick={() => { setCreateOpen(true); createForm.resetFields() }}
                >
                  新建规则
                </Button>
              </PermGuard>
              <PermGuard perm="inventory:write">
                <Button
                  icon={<ThunderboltOutlined />}
                  disabled={!effectiveAgentId || oobTrimmed === ''}
                  onClick={() => { setSweepOpen(true); sweepForm.resetFields() }}
                >
                  清扫过期
                </Button>
              </PermGuard>
            </Space>

            {!effectiveAgentId || oobTrimmed === '' ? (
              <Alert type="info" showIcon message="选择主机并输入 vault 条目名后，可查看并管理该主机上的兜底转发规则" />
            ) : listQuery.isLoading ? (
              <Alert type="info" showIcon message="正在读取该设备转发规则…" />
            ) : listQuery.isError ? (
              <Alert type="warning" showIcon message="读取失败——请确认 Agent 在线且 vault 条目已配置（oob/&lt;name&gt; 且含 addr）" />
            ) : (
              <>
                {expiredCount > 0 && (
                  <Alert
                    type="warning"
                    showIcon
                    style={{ marginBottom: 12 }}
                    message={`该设备有 ${expiredCount} 条已过期的 cockpit 规则，可用「清扫过期」回收`}
                  />
                )}
                <Table<OobRedirectRule>
                  rowKey={(r) => r.name}
                  columns={columns}
                  dataSource={rules}
                  pagination={false}
                  locale={{ emptyText: '该设备暂无转发规则（或读取不到）' }}
                  size="small"
                />
              </>
            )}
          </>
        )}
      </Card>

      <Modal
        title={`新建转发规则：${selectedAgent?.hostname || selectedAgent?.id || ''}`}
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={() => void submitCreate()}
        confirmLoading={createMutation.isPending}
        okText="下发"
        cancelText="取消"
        width={560}
        destroyOnClose
      >
        <Form form={createForm} layout="vertical" initialValues={{ proto: 'tcp', ttlSeconds: 1800 }}>
          <Space.Compact block>
            <Form.Item
              name="label"
              label="规则标签"
              style={{ width: '50%' }}
            >
              <Input placeholder="留空 = 按 proto/目标推导" />
            </Form.Item>
            <Form.Item name="proto" label="协议" style={{ width: '50%' }} rules={[{ required: true }]}>
              <Select
                options={[
                  { value: 'tcp', label: 'tcp' },
                  { value: 'udp', label: 'udp' },
                ]}
              />
            </Form.Item>
          </Space.Compact>
          <Space.Compact block>
            <Form.Item
              name="srcZone"
              label="源区域"
              style={{ width: '50%' }}
              rules={[{ required: true, message: '请输入源区域（如 wan）' }]}
            >
              <Input placeholder="wan" />
            </Form.Item>
            <Form.Item
              name="dstZone"
              label="目标区域"
              style={{ width: '50%' }}
              rules={[{ required: true, message: '请输入目标区域（如 lan）' }]}
            >
              <Input placeholder="lan" />
            </Form.Item>
          </Space.Compact>
          <Space.Compact block>
            <Form.Item name="srcIp" label="源 IP（可选）" style={{ width: '50%' }}>
              <Input placeholder="留空 = 任意" />
            </Form.Item>
            <Form.Item
              name="dstIp"
              label="目标 IP"
              style={{ width: '50%' }}
              rules={[{ required: true, message: '请输入目标 IP' }]}
            >
              <Input placeholder="192.168.1.10" />
            </Form.Item>
          </Space.Compact>
          <Space.Compact block>
            <Form.Item name="srcPort" label="源端口（可选）" style={{ width: '50%' }}>
              <InputNumber min={0} max={65535} style={{ width: '100%' }} placeholder="留空 = 任意" />
            </Form.Item>
            <Form.Item
              name="dstPort"
              label="目标端口"
              style={{ width: '50%' }}
              rules={[{ required: true, message: '请输入目标端口' }]}
            >
              <InputNumber min={1} max={65535} style={{ width: '100%' }} placeholder="16992" />
            </Form.Item>
          </Space.Compact>
          <Form.Item
            name="ttlSeconds"
            label="有效期（秒）"
            rules={[{ required: true, message: '请输入有效期' }]}
            extra="过期账进规则名；0 = 使用服务端缺省（redirect 层收敛缺省与上限）"
          >
            <InputNumber min={0} max={86400} style={{ width: '100%' }} placeholder="1800" />
          </Form.Item>
          <Form.Item
            name="confirmLabel"
            label="确认下发：输入规则标签以确认"
            rules={[
              {
                validator: (_, v: string) =>
                  (v ?? '') === (labelValue ?? '')
                    ? Promise.resolve()
                    : Promise.reject(new Error('输入与规则标签不一致')),
              },
            ]}
          >
            <Input placeholder={labelValue || '留空（标签为空）'} allowClear />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="拆除转发规则"
        open={removeTarget !== null}
        onCancel={() => setRemoveTarget(null)}
        onOk={() => void submitRemove()}
        confirmLoading={removeMutation.isPending}
        okText="确认拆除"
        cancelText="取消"
        okButtonProps={{ danger: true }}
        width={520}
        destroyOnClose
      >
        <Alert type="warning" showIcon style={{ marginBottom: 16 }} message={`将拆除：${removeName}`} />
        <Form form={removeForm} layout="vertical">
          <Form.Item
            name="confirmName"
            label="确认拆除：输入规则名"
            rules={[
              { required: true, message: '请输入规则名' },
              {
                validator: (_, v: string) =>
                  v === removeName
                    ? Promise.resolve()
                    : Promise.reject(new Error('输入与规则名不一致')),
              },
            ]}
          >
            <Input placeholder={removeName} allowClear />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="清扫过期规则"
        open={sweepOpen}
        onCancel={() => setSweepOpen(false)}
        onOk={() => void submitSweep()}
        confirmLoading={sweepMutation.isPending}
        okText="确认清扫"
        cancelText="取消"
        okButtonProps={{ danger: true }}
        width={520}
        destroyOnClose
      >
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
          message={`将拆除该设备上全部已过期的 cockpit 规则（凭据条目：${oobTrimmed}）`}
        />
        <Form form={sweepForm} layout="vertical">
          <Form.Item
            name="confirmOob"
            label="确认清扫：输入 vault 条目名"
            rules={[
              { required: true, message: '请输入 vault 条目名' },
              {
                validator: (_, v: string) =>
                  v === oobTrimmed
                    ? Promise.resolve()
                    : Promise.reject(new Error('输入与 vault 条目名不一致')),
              },
            ]}
          >
            <Input placeholder={oobTrimmed} allowClear />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default OobRedirectManager
