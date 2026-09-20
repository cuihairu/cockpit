import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Button,
  Card,
  Form,
  Input,
  Modal,
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
import {
  CheckCircleOutlined,
  CopyOutlined,
  EditOutlined,
  PlusOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { api } from '@/services/api'
import type {
  Agent,
  DomainApplyStep,
  DomainBinding,
  DomainDriftCheck,
  DomainDriftReport,
} from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'

// 服务域名绑定（domain-binding-design.md）：「域名 → agent 目标服务」唯一
// 事实源。列表 + 登记/编辑 + apply 三联动 + D6 漂移检查 + D7 配置片段。
// 漂移为实时检查（DNS 打 provider API、proxy 打 agent RPC），按需触发。

interface BindingFormValues {
  domain: string
  agentId: string
  target: string
  enabled: boolean
  autoDns: boolean
  autoProxy: boolean
  autoCert: boolean
}

const emptyForm: BindingFormValues = {
  domain: '',
  agentId: '',
  target: '',
  enabled: true,
  autoDns: true,
  autoProxy: true,
  autoCert: true,
}

// D6 漂移三态 Tag：绿 ok / 红 确认漂移（missing|mismatch|foreign）/ 黄 检查失败
const DRIFT_STATUS_LABEL: Record<string, string> = {
  missing: '缺失',
  mismatch: '不符',
  foreign: '被占',
}

function DriftTag({ label, check }: { label: string; check?: DomainDriftCheck }) {
  if (!check || !check.checked) {
    return (
      <Tag color="default" style={{ marginInlineEnd: 0 }}>
        {label}·未启用
      </Tag>
    )
  }
  if (check.ok) {
    return (
      <Tag color="success" style={{ marginInlineEnd: 0 }}>
        {label}
      </Tag>
    )
  }
  if (check.error) {
    return (
      <Tooltip title={check.error}>
        <Tag color="warning" style={{ marginInlineEnd: 0 }}>
          {label}·异常
        </Tag>
      </Tooltip>
    )
  }
  const tip = `期望 ${check.expected ?? '?'} / 实际 ${check.actual ?? '?'}`
  return (
    <Tooltip title={tip}>
      <Tag color="error" style={{ marginInlineEnd: 0 }}>
        {label}·{DRIFT_STATUS_LABEL[check.status ?? ''] ?? '漂移'}
      </Tag>
    </Tooltip>
  )
}

const DomainsPage = () => {
  const queryClient = useQueryClient()
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<DomainBinding | null>(null)
  const [form] = Form.useForm<BindingFormValues>()
  // D7 片段弹窗：选中的 agentId + 拉取到的文本
  const [snippetFor, setSnippetFor] = useState<string | null>(null)
  // apply 结果弹窗（steps 明细，排障用）
  const [applySteps, setApplySteps] = useState<DomainApplyStep[] | null>(null)
  // D6 漂移检查结果按 domain 索引（表格「漂移」列渲染）
  const [driftMap, setDriftMap] = useState<Record<string, DomainDriftReport>>({})

  const { data: bindings = [], isLoading } = useQuery({
    queryKey: ['domain-bindings'],
    queryFn: () => api.getDomainBindings(),
  })
  const { data: agents = [] } = useQuery({
    queryKey: ['agents'],
    queryFn: () => api.getAgents(),
  })
  const agentName = useMemo(
    () => Object.fromEntries(agents.map((a: Agent) => [a.id, a.hostname || a.id])),
    [agents],
  )

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['domain-bindings'] })

  // D6 漂移：按需实时检查，结果按 domain 存 map 供表格列渲染
  const driftMutation = useMutation({
    mutationFn: () => api.getDomainDrift(),
    onSuccess: (resp) => {
      setDriftMap(Object.fromEntries(resp.items.map((r) => [r.domain, r])))
      message.success(`漂移检查完成（${resp.items.length} 条绑定）`)
    },
    onError: (err) => message.error(getApiErrorMessage(err, '漂移检查失败')),
  })

  const saveMutation = useMutation({
    mutationFn: (values: BindingFormValues) =>
      api.saveDomainBinding({
        domain: values.domain.trim(),
        agentId: values.agentId,
        target: values.target.trim(),
        enabled: values.enabled,
        autoDns: values.autoDns,
        autoProxy: values.autoProxy,
        autoCert: values.autoCert,
      }),
    onSuccess: () => {
      message.success(editing ? '绑定已更新' : '绑定已登记')
      setModalOpen(false)
      invalidate()
    },
    onError: (err) => message.error(getApiErrorMessage(err, '保存失败')),
  })

  const applyMutation = useMutation({
    mutationFn: (domain: string) => api.applyDomainBinding(domain),
    onSuccess: (resp) => {
      setApplySteps(resp.steps ?? [])
      if (resp.status === 'ok') message.success('联动完成')
      else message.warning('联动部分失败，详见步骤明细')
      invalidate()
    },
    onError: (err) => message.error(getApiErrorMessage(err, '联动失败')),
  })

  const deleteMutation = useMutation({
    mutationFn: (domain: string) => api.deleteDomainBinding(domain),
    onSuccess: () => {
      message.success('已删除登记（已下发的 DNS/站点/监控项保留，需手动清理）')
      invalidate()
    },
    onError: (err) => message.error(getApiErrorMessage(err, '删除失败')),
  })

  const { data: snippet } = useQuery({
    queryKey: ['agent-domains-snippet', snippetFor],
    queryFn: () => api.getAgentDomainsSnippet(snippetFor!),
    enabled: !!snippetFor,
  })

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    setModalOpen(true)
  }
  const openEdit = (b: DomainBinding) => {
    setEditing(b)
    form.setFieldsValue({
      domain: b.domain,
      agentId: b.agentId,
      target: b.target,
      enabled: b.enabled,
      autoDns: b.autoDns,
      autoProxy: b.autoProxy,
      autoCert: b.autoCert,
    })
    setModalOpen(true)
  }

  const columns: ColumnsType<DomainBinding> = [
    {
      title: '域名',
      dataIndex: 'domain',
      render: (v: string) => <Typography.Text copyable={{ text: v }}>{v}</Typography.Text>,
    },
    {
      title: 'Agent',
      dataIndex: 'agentId',
      width: 140,
      render: (v: string) => <Tooltip title={v}>{agentName[v] ?? v}</Tooltip>,
    },
    {
      title: '目标服务',
      dataIndex: 'target',
      width: 160,
      render: (v: string) => <Typography.Text code>{v}</Typography.Text>,
    },
    {
      title: '自动化',
      key: 'autos',
      width: 170,
      render: (_, b) => (
        <Space size={4} wrap>
          {!b.enabled && <Tag color="default">停用</Tag>}
          {b.autoDns && <Tag color="blue">DNS</Tag>}
          {b.autoProxy && <Tag color="purple">反代</Tag>}
          {b.autoCert && <Tag color="cyan">证书</Tag>}
        </Space>
      ),
    },
    {
      title: '最近 apply',
      key: 'lastApply',
      width: 130,
      render: (_, b) => {
        if (!b.appliedAt) return <Tag>从未</Tag>
        const ok = b.lastApplyStatus === 'ok'
        return (
          <Tooltip title={ok ? undefined : b.lastError || '失败'}>
            <Tag color={ok ? 'success' : 'error'}>
              {ok ? '成功' : '失败'}·{dayjs.unix(b.appliedAt).format('MM-DD HH:mm')}
            </Tag>
          </Tooltip>
        )
      },
    },
    {
      title: '漂移',
      key: 'drift',
      width: 220,
      render: (_, b) => {
        const report = driftMap[b.domain]
        if (!report) return <Typography.Text type="secondary">未检查</Typography.Text>
        return (
          <Space size={4}>
            <DriftTag label="DNS" check={report.dns} />
            <DriftTag label="反代" check={report.proxy} />
            <DriftTag label="证书" check={report.cert} />
          </Space>
        )
      },
    },
    {
      title: '操作',
      key: 'actions',
      width: 200,
      render: (_, b) => (
        <Space size={0}>
          <Button
            type="link"
            size="small"
            icon={<ThunderboltOutlined />}
            loading={applyMutation.isPending && applyMutation.variables === b.domain}
            onClick={() => applyMutation.mutate(b.domain)}
          >
            Apply
          </Button>
          <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEdit(b)}>
            编辑
          </Button>
          <Popconfirm
            title="删除登记？"
            description="只删登记，已下发的 DNS/站点/监控项保留"
            onConfirm={() => deleteMutation.mutate(b.domain)}
          >
            <Button type="link" size="small" danger>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <Card
      title="服务域名绑定"
      extra={
        <Space>
          <Select
            placeholder="选择 agent 生成配置片段"
            style={{ minWidth: 200 }}
            value={undefined}
            onChange={(v: string) => setSnippetFor(v)}
            options={agents.map((a: Agent) => ({ value: a.id, label: a.hostname || a.id }))}
            allowClear
          />
          <Button
            icon={<SafetyCertificateOutlined />}
            loading={driftMutation.isPending}
            onClick={() => driftMutation.mutate()}
          >
            漂移检查
          </Button>
          <Button icon={<PlusOutlined />} type="primary" onClick={openCreate}>
            登记绑定
          </Button>
          <Button icon={<ReloadOutlined />} onClick={() => invalidate()} />
        </Space>
      }
    >
      <Table
        rowKey="domain"
        columns={columns}
        dataSource={bindings}
        loading={isLoading}
        pagination={false}
      />

      <Modal
        title={editing ? `编辑绑定 · ${editing.domain}` : '登记绑定'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => form.validateFields().then((v) => saveMutation.mutate(v))}
        confirmLoading={saveMutation.isPending}
        destroyOnClose
      >
        <Form form={form} initialValues={emptyForm} layout="vertical">
          <Form.Item
            name="domain"
            label="域名"
            rules={[
              { required: true },
              {
                pattern: /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$/,
                message: '小写多级域名，如 blog.example.com',
              },
            ]}
          >
            <Input placeholder="blog.example.com" disabled={!!editing} />
          </Form.Item>
          <Form.Item name="agentId" label="Agent" rules={[{ required: true }]}>
            <Select
              options={agents.map((a: Agent) => ({ value: a.id, label: a.hostname || a.id }))}
              placeholder="域名指向的节点"
            />
          </Form.Item>
          <Form.Item
            name="target"
            label="目标服务"
            rules={[
              { required: true },
              {
                validator: (_, v: string) => {
                  if (!v) return Promise.resolve()
                  if (/^[A-Za-z0-9][A-Za-z0-9.-]*:[0-9]{1,5}$/.test(v)) return Promise.resolve()
                  if (/^docker:\/\/[A-Za-z0-9][A-Za-z0-9_.-]*(:[0-9]{1,5})?$/.test(v))
                    return Promise.resolve()
                  return Promise.reject(new Error('host:port 或 docker://服务名[:端口]'))
                },
              },
            ]}
          >
            <Input placeholder="127.0.0.1:8080 或 docker://web" />
          </Form.Item>
          <Form.Item name="enabled" label="启用" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Space size="large">
            <Form.Item name="autoDns" label="联动 DNS" valuePropName="checked" style={{ marginBottom: 0 }}>
              <Switch />
            </Form.Item>
            <Form.Item name="autoProxy" label="联动反代" valuePropName="checked" style={{ marginBottom: 0 }}>
              <Switch />
            </Form.Item>
            <Form.Item name="autoCert" label="联动证书监控" valuePropName="checked" style={{ marginBottom: 0 }}>
              <Switch />
            </Form.Item>
          </Space>
        </Form>
      </Modal>

      <Modal
        title="apply 步骤明细"
        open={applySteps !== null}
        onCancel={() => setApplySteps(null)}
        footer={null}
      >
        {applySteps?.map((s) => (
          <div key={s.name}>
            {s.ok ? (
              <CheckCircleOutlined style={{ color: '#52c41a', marginRight: 8 }} />
            ) : (
              <SafetyCertificateOutlined style={{ color: '#ff4d4f', marginRight: 8 }} />
            )}
            <Typography.Text strong>{s.name}</Typography.Text>
            {!s.ok && <Typography.Text type="danger"> — {s.error}</Typography.Text>}
          </div>
        ))}
        {applySteps?.length === 0 && (
          <Typography.Text type="warning">未启用任何 auto 联动开关</Typography.Text>
        )}
      </Modal>

      <Modal
        title={`域名配置片段 · ${agentName[snippetFor ?? ''] ?? snippetFor}`}
        open={!!snippetFor}
        onCancel={() => setSnippetFor(null)}
        footer={[
          <Button
            key="copy"
            icon={<CopyOutlined />}
            onClick={() => {
              navigator.clipboard.writeText(snippet ?? '')
              message.success('已复制')
            }}
          >
            复制
          </Button>,
        ]}
      >
        <Typography.Text>
          <pre style={{ maxHeight: 320, overflow: 'auto', margin: 0 }}>{snippet}</pre>
        </Typography.Text>
      </Modal>
    </Card>
  )
}

export default DomainsPage
