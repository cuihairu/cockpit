import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  Badge,
  Button,
  Card,
  Form,
  Input,
  InputNumber,
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
import { CheckCircleOutlined, DeleteOutlined, EditOutlined, PlusOutlined } from '@ant-design/icons'
import { PermGuard } from '@/components/PermGuard'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import type { Agent, DDNSConfig } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'

// DDNS（动态域名解析）Tab（见 docs/guide/ddns-design.md D11）：
// 绑定 agent 探测公网 IP，server 巡检比对并写 Cloudflare，
// 记录缺失自动创建。失败产生告警（真去重），成功静默自愈。

const STATUS_META: Record<DDNSConfig['lastStatus'], { label: string; badge: 'success' | 'error' | 'default' }> = {
  ok: { label: '正常', badge: 'success' },
  failed: { label: '失败', badge: 'error' },
  never: { label: '未检查', badge: 'default' },
}

interface DDNSFormValues {
  agentId: string
  zoneId: string
  recordName: string
  type: 'A' | 'AAAA'
  enabled: boolean
}

const formatCheckedAt = (unix: number) => {
  if (!unix) return '—'
  return new Date(unix * 1000).toLocaleString()
}

const DDNSPanel: React.FC = () => {
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState<DDNSConfig | null>(null)
  const [modalOpen, setModalOpen] = useState(false)
  const [form] = Form.useForm<DDNSFormValues>()
  // 巡检设置编辑态：改动才落本地，未编辑从服务端派生（与 Drift/Disk 页同模式）
  const [scanEdit, setScanEdit] = useState<{ on?: boolean; minutes?: number } | null>(null)
  const [savingScan, setSavingScan] = useState(false)

  const { data: configs = [], isLoading } = useQuery({ queryKey: ['ddns-configs'], queryFn: () => api.getDDNSConfigs() })
  const { data: zones = [] } = useQuery({ queryKey: ['dns-zones'], queryFn: () => api.getDNSZones() })
  const { data: agents = [] } = useQuery({ queryKey: ['ddns-agents'], queryFn: () => api.getAgents() })
  const { data: scanCfg } = useQuery({ queryKey: ['ddns-scan-config'], queryFn: () => api.getDDNSScanConfig() })

  const savedInterval = scanCfg?.scan_interval_seconds ?? 0
  const scanOn = scanEdit?.on ?? savedInterval > 0
  const scanMinutes = scanEdit?.minutes ?? (savedInterval > 0 ? Math.round(savedInterval / 60) : 5)

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['ddns-configs'] })

  const saveScanConfig = async () => {
    const seconds = scanOn ? scanMinutes * 60 : 0
    if (scanOn && (scanMinutes < 1 || scanMinutes > 1440)) {
      message.warning('间隔需在 1～1440 分钟之间')
      return
    }
    setSavingScan(true)
    try {
      await api.putDDNSScanConfig(seconds)
      setScanEdit(null)
      await queryClient.invalidateQueries({ queryKey: ['ddns-scan-config'] })
      message.success(scanOn ? '已开启自动巡检' : '已关闭自动巡检')
    } catch (err) {
      message.error(getApiErrorMessage(err, '保存失败'))
    } finally {
      setSavingScan(false)
    }
  }

  const checkMut = useMutation({
    mutationFn: (id: number) => api.checkDDNSConfig(id),
    onSuccess: (res) => {
      void invalidate()
      if (res.status === 'ok') {
        message.success(res.changed ? `已更新 DNS 记录 → ${res.ip}` : `IP 无变化（${res.ip}）`)
      } else {
        message.error(res.error || '检查失败')
      }
    },
    onError: (err) => message.error(getApiErrorMessage(err, '检查失败')),
  })

  const deleteMut = useMutation({
    mutationFn: (id: number) => api.deleteDDNSConfig(id),
    onSuccess: () => {
      void invalidate()
      message.success('已删除')
    },
    onError: (err) => message.error(getApiErrorMessage(err, '删除失败')),
  })

  const openCreate = () => {
    setEditing(null)
    form.setFieldsValue({ agentId: undefined, zoneId: undefined, recordName: '', type: 'A', enabled: true })
    setModalOpen(true)
  }

  const openEdit = (cfg: DDNSConfig) => {
    setEditing(cfg)
    form.setFieldsValue({
      agentId: cfg.agentId,
      zoneId: cfg.zoneId,
      recordName: cfg.recordName,
      type: cfg.type,
      enabled: cfg.enabled,
    })
    setModalOpen(true)
  }

  const submit = async () => {
    const values = await form.validateFields()
    const zone = zones.find((z) => z.id === values.zoneId)
    const input = {
      agentId: values.agentId,
      zoneId: values.zoneId,
      zoneName: zone?.name ?? '',
      recordName: values.recordName.trim(),
      type: values.type,
      enabled: values.enabled,
    }
    try {
      if (editing) {
        await api.updateDDNSConfig(editing.id, input)
        message.success('已更新')
      } else {
        await api.createDDNSConfig(input)
        message.success('已创建')
      }
      setModalOpen(false)
      await invalidate()
    } catch (err) {
      message.error(getApiErrorMessage(err, '保存失败'))
    }
  }

  // 在线的 agent 排前面（DDNS 源主机需要保持在线）
  const agentOptions = [...agents]
    .sort((a, b) => Number(a.status === 'online') - Number(b.status === 'online'))
    .map((a: Agent) => ({
      value: a.id,
      label: `${a.hostname || a.id}${a.status === 'offline' ? '（离线）' : ''}`,
    }))

  const columns: ColumnsType<DDNSConfig> = [
    {
      title: '状态',
      dataIndex: 'lastStatus',
      width: 90,
      render: (v: DDNSConfig['lastStatus'], r) => {
        const meta = STATUS_META[v] ?? STATUS_META.never
        const badge = <Badge status={meta.badge} text={meta.label} />
        return v === 'failed' && r.lastError ? <Tooltip title={r.lastError}>{badge}</Tooltip> : badge
      },
    },
    {
      title: '记录',
      dataIndex: 'recordName',
      render: (v: string, r) => (
        <Space size={6}>
          <Typography.Text code>{v}</Typography.Text>
          <Tag>{r.type}</Tag>
        </Space>
      ),
    },
    { title: '当前 IP', dataIndex: 'lastIP', width: 150, render: (v: string) => (v ? <Typography.Text code>{v}</Typography.Text> : '—') },
    {
      title: '绑定主机',
      dataIndex: 'agentId',
      width: 150,
      ellipsis: true,
      render: (v: string) => {
        const a = agents.find((x) => x.id === v)
        return a ? a.hostname || a.id : v
      },
    },
    { title: '最后检查', dataIndex: 'checkedAt', width: 170, render: (v: number) => <span style={{ fontSize: 12 }}>{formatCheckedAt(v)}</span> },
    {
      title: '操作',
      key: 'actions',
      width: 200,
      render: (_, r) => (
        <PermGuard perm="ddns:write" fallback={<span style={{ color: '#999' }}>只读</span>}>
          <Space size={4}>
            <Tooltip title="立即检查（不等巡检周期）">
              <Button
                size="small"
                type="text"
                icon={<CheckCircleOutlined />}
                loading={checkMut.isPending && checkMut.variables === r.id}
                onClick={() => checkMut.mutate(r.id)}
              />
            </Tooltip>
            <Button size="small" type="text" icon={<EditOutlined />} onClick={() => openEdit(r)} />
            <Popconfirm title="删除该 DDNS 配置？（不影响 DNS 上已有的记录）" onConfirm={() => deleteMut.mutate(r.id)}>
              <Button size="small" type="text" danger icon={<DeleteOutlined />} />
            </Popconfirm>
          </Space>
        </PermGuard>
      ),
    },
  ]

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Space wrap align="center">
        <Typography.Text strong>自动巡检</Typography.Text>
        <Tooltip title="开启后 server 定时让绑定的主机探测公网 IP，发现变化即更新 Cloudflare 记录；失败会产生告警（同一记录未处理期间只提醒一次）">
          <Switch checked={scanOn} onChange={(on) => setScanEdit((e) => ({ ...e, on }))} loading={!scanCfg} />
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
        <PermGuard perm="ddns:write">
          <Button size="small" onClick={() => void saveScanConfig()} loading={savingScan}>
            保存
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
            新建 DDNS
          </Button>
        </PermGuard>
      </Space>
      <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block' }}>
        将域名记录绑定到一台内网主机：巡检时该主机探测自己的公网出口 IP，记录缺失自动创建、IP 变化自动更新（保留记录的 TTL 与代理设置）。
      </Typography.Text>

      <Card size="small">
        <Table<DDNSConfig>
          rowKey="id"
          loading={isLoading}
          columns={columns}
          dataSource={configs}
          pagination={false}
          size="small"
          locale={{ emptyText: '暂无 DDNS 配置——新建一条，把家庭宽带的动态 IP 绑定到域名' }}
        />
      </Card>

      {configs.some((c) => c.lastStatus === 'failed') && (
        <Alert
          type="warning"
          showIcon
          message="部分 DDNS 配置最近一次检查失败，请查看状态列的错误说明（主机离线 / 无该地址族 / Cloudflare 报错）"
        />
      )}

      <Modal
        title={editing ? '编辑 DDNS 配置' : '新建 DDNS 配置'}
        open={modalOpen}
        onOk={() => void submit()}
        onCancel={() => setModalOpen(false)}
        okText="保存"
        destroyOnClose
      >
        <Form form={form} layout="vertical">
          <Form.Item name="agentId" label="绑定主机（IP 探测源）" rules={[{ required: true, message: '请选择主机' }]}>
            <Select showSearch optionFilterProp="label" placeholder="选择探测公网 IP 的主机" options={agentOptions} />
          </Form.Item>
          <Form.Item name="zoneId" label="域名（zone）" rules={[{ required: true, message: '请选择域名' }]}>
            <Select
              showSearch
              optionFilterProp="label"
              placeholder="选择 Cloudflare 托管域名"
              options={zones.map((z) => ({ value: z.id, label: z.name }))}
            />
          </Form.Item>
          <Form.Item
            name="recordName"
            label="记录名"
            rules={[
              { required: true, message: '请填写记录名' },
              { pattern: /^[a-zA-Z0-9]([a-zA-Z0-9-]*\.)+[a-zA-Z]{2,}$/, message: '填写完整记录名，如 home.example.com' },
            ]}
          >
            <Input placeholder="home.example.com" />
          </Form.Item>
          <Form.Item name="type" label="记录类型" rules={[{ required: true }]}>
            <Select
              options={[
                { value: 'A', label: 'A（IPv4）' },
                { value: 'AAAA', label: 'AAAA（IPv6）' },
              ]}
            />
          </Form.Item>
          <Form.Item name="enabled" label="启用" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </Space>
  )
}

export default DDNSPanel
