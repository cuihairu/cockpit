import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
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
  Tabs,
  Tag,
  Typography,
  message,
} from 'antd'
import { DeleteOutlined, EditOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import type { DNSRecord, DNSRecordInput, DNSZone } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'
import DDNSPanel from './DDNSPanel'

// DNS 管理：Tab1 记录管理（按 dns.provider 分派 cloudflare/dnspod/alidns，
// server 直连对应 API，不落库，见 docs/guide/dns-design.md）+ Tab2 DDNS
// 动态域名（ddns-design.md）。未配置凭据时两 Tab 共用同一张引导卡片。

// 未配置引导卡按 provider 分流（M2 D17，文案与 server 503 一致）
const PROVIDER_GUIDE: Record<string, { keys: string; env: string; note: string }> = {
  dnspod: {
    keys: 'dns.dnspod.login_token',
    env: 'DNSPOD_LOGIN_TOKEN',
    note: '值为「ID,Token」合并格式，在腾讯云 API 密钥管理页创建。',
  },
  alidns: {
    keys: 'dns.alidns.access_key + dns.alidns.secret_key',
    env: 'ALIYUN_ACCESS_KEY / ALIYUN_ACCESS_KEY_SECRET',
    note: 'AccessKey 只需要云解析 DNS 的读写权限。',
  },
  cloudflare: {
    keys: 'dns.cloudflare.api_token',
    env: 'CLOUDFLARE_API_TOKEN',
    note: 'token 只需要 Zone.DNS 编辑权限。',
  },
}

// 与 server 端 dns.AllowedTypes 同规则（双端校验，D8）
const RECORD_TYPES = ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'NS', 'SRV', 'CAA']

// proxied（橙云）仅代理 DNS 流量的类型有效
const PROXIABLE = new Set(['A', 'AAAA', 'CNAME'])

interface RecordFormValues {
  type: string
  name: string
  content: string
  ttl: number | null
  proxied: boolean
}

const emptyForm: RecordFormValues = { type: 'A', name: '', content: '', ttl: null, proxied: false }

// Tab1：记录管理（原 DNS 页主体；仅在凭据已配置时渲染）。
// proxied（橙云）是 Cloudflare 专属概念，非 CF provider 全链路隐藏（D15/D17）
const RecordsPanel = ({ provider }: { provider: string }) => {
  const isCF = provider === 'cloudflare'
  const [zoneId, setZoneId] = useState('')
  const [typeFilter, setTypeFilter] = useState<string | undefined>(undefined)
  const [page, setPage] = useState(1)
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<DNSRecord | null>(null)
  const [form] = Form.useForm<RecordFormValues>()
  const queryClient = useQueryClient()

  const { data: zones = [], isLoading: zonesLoading } = useQuery({
    queryKey: ['dns-zones'],
    queryFn: () => api.getDNSZones(),
  })

  const zone: DNSZone | undefined = useMemo(
    () => zones.find((z) => z.id === zoneId),
    [zones, zoneId],
  )

  const { data: recordsPage, isLoading: recordsLoading } = useQuery({
    queryKey: ['dns-records', zoneId, typeFilter, page],
    queryFn: () => api.getDNSRecords(zoneId, typeFilter, page),
    enabled: !!zoneId,
  })

  const invalidateRecords = () =>
    queryClient.invalidateQueries({ queryKey: ['dns-records', zoneId] })

  const saveMutation = useMutation({
    mutationFn: (values: RecordFormValues) => {
      const input: DNSRecordInput = {
        type: values.type,
        name: values.name.trim(),
        content: values.content.trim(),
        ttl: values.ttl ?? 0, // 0 = auto（server 归一化为 1）
        proxied: isCF && PROXIABLE.has(values.type) ? values.proxied : false,
      }
      return editing
        ? api.updateDNSRecord(zoneId, editing.id, input)
        : api.createDNSRecord(zoneId, input)
    },
    onSuccess: () => {
      message.success(editing ? '记录已更新' : '记录已创建')
      setModalOpen(false)
      invalidateRecords()
    },
    onError: (err) => message.error(getApiErrorMessage(err, '操作失败')),
  })

  const deleteMutation = useMutation({
    mutationFn: (record: DNSRecord) => api.deleteDNSRecord(zoneId, record.id),
    onSuccess: () => {
      message.success('记录已删除')
      invalidateRecords()
    },
    onError: (err) => message.error(getApiErrorMessage(err, '操作失败')),
  })

  const openCreate = () => {
    setEditing(null)
    form.setFieldsValue(emptyForm)
    setModalOpen(true)
  }

  const openEdit = (record: DNSRecord) => {
    setEditing(record)
    form.setFieldsValue({
      type: record.type,
      name: record.name,
      content: record.content,
      ttl: record.ttl === 1 ? null : record.ttl,
      proxied: record.proxied,
    })
    setModalOpen(true)
  }

  const columns: ColumnsType<DNSRecord> = [
    { title: '类型', dataIndex: 'type', width: 90, render: (t: string) => <Tag>{t}</Tag> },
    { title: '名称', dataIndex: 'name', ellipsis: true },
    { title: '内容', dataIndex: 'content', ellipsis: true },
    {
      title: 'TTL',
      dataIndex: 'ttl',
      width: 80,
      render: (ttl: number) => (ttl === 1 ? 'auto' : ttl),
    },
    ...(isCF
      ? [
          {
            title: '代理',
            dataIndex: 'proxied',
            width: 80,
            render: (p: boolean) =>
              p ? <Tag color="orange">已代理</Tag> : <Tag color="default">仅 DNS</Tag>,
          },
        ]
      : []),
    {
      title: '操作',
      key: 'actions',
      width: 130,
      render: (_, record) => (
        <Space>
          <Button size="small" icon={<EditOutlined />} onClick={() => openEdit(record)}>
            编辑
          </Button>
          <Popconfirm
            title={`删除记录 ${record.name}？`}
            description="删除立即生效，不可恢复"
            onConfirm={() => deleteMutation.mutate(record)}
          >
            <Button size="small" danger icon={<DeleteOutlined />}>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  // hooks 必须先于条件 return（Form.useWatch 拿当前类型控制 proxied 开关）
  const selectedType = Form.useWatch('type', form)

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Card>
        <Space wrap>
          <Select
            style={{ minWidth: 260 }}
            placeholder="选择域名（zone）"
            loading={zonesLoading}
            value={zoneId || undefined}
            onChange={(v) => {
              setZoneId(v)
              setPage(1)
            }}
            options={zones.map((z) => ({
              value: z.id,
              label: `${z.name}${z.in_cmdb ? '' : '（未登记 CMDB）'}`,
            }))}
            showSearch
            optionFilterProp="label"
          />
          {zone && !zone.in_cmdb && (
            <Typography.Text type="warning">
              该域名未登记在「资源 → 域名」，探测与证书管理不会覆盖它
            </Typography.Text>
          )}
          <Select
            style={{ width: 120 }}
            allowClear
            placeholder="类型"
            value={typeFilter}
            onChange={(v) => {
              setTypeFilter(v)
              setPage(1)
            }}
            options={RECORD_TYPES.map((t) => ({ value: t, label: t }))}
          />
          <Button
            icon={<ReloadOutlined />}
            onClick={() => queryClient.invalidateQueries({ queryKey: ['dns-records', zoneId] })}
          >
            刷新
          </Button>
          <Button type="primary" icon={<PlusOutlined />} disabled={!zoneId} onClick={openCreate}>
            新建记录
          </Button>
        </Space>
      </Card>

      <Card title={zone ? `${zone.name} 的 DNS 记录` : 'DNS 记录'}>
        <Table<DNSRecord>
          rowKey="id"
          size="middle"
          columns={columns}
          dataSource={recordsPage?.records ?? []}
          loading={recordsLoading}
          locale={{ emptyText: zoneId ? '暂无记录' : '请先选择域名（zone）' }}
          pagination={{
            current: page,
            pageSize: 50,
            total: (recordsPage?.total_pages ?? 1) * 50,
            showSizeChanger: false,
            onChange: setPage,
          }}
        />
      </Card>

      <Modal
        title={editing ? `编辑记录 ${editing.name}` : '新建 DNS 记录'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={saveMutation.isPending}
        destroyOnClose
      >
        <Form
          form={form}
          layout="vertical"
          onFinish={(values) => saveMutation.mutate(values)}
          initialValues={emptyForm}
        >
          <Form.Item name="type" label="类型" rules={[{ required: true }]}>
            <Select options={RECORD_TYPES.map((t) => ({ value: t, label: t }))} disabled={!!editing} />
          </Form.Item>
          <Form.Item
            name="name"
            label="名称"
            rules={[{ required: true, message: '记录名不能为空' }]}
            extra={zone ? `如 www（会补全为 www.${zone.name}，也可填全名）` : undefined}
          >
            <Input placeholder="www" autoComplete="off" />
          </Form.Item>
          <Form.Item
            name="content"
            label="内容"
            rules={[{ required: true, message: '内容不能为空' }]}
          >
            <Input placeholder="1.2.3.4 / target.example.com" autoComplete="off" />
          </Form.Item>
          <Form.Item name="ttl" label="TTL（秒，留空 = auto）">
            <InputNumber min={60} max={86400} style={{ width: '100%' }} placeholder="auto" />
          </Form.Item>
          {isCF && (
            <Form.Item
              name="proxied"
              label="Cloudflare 代理（橙云）"
              valuePropName="checked"
              extra="仅 A / AAAA / CNAME 支持代理"
            >
              <Switch disabled={!!selectedType && !PROXIABLE.has(selectedType)} />
            </Form.Item>
          )}
        </Form>
      </Modal>
    </Space>
  )
}

const DNS = () => {
  const { data: status } = useQuery({
    queryKey: ['dns-status'],
    queryFn: () => api.getDNSStatus(),
  })

  // 未配置凭据：按 provider 分流的引导卡（M2 D17），而非报错堆叠
  if (status && !status.configured) {
    const guide = PROVIDER_GUIDE[status.provider] ?? PROVIDER_GUIDE.cloudflare
    return (
      <Card>
        <Alert
          type="info"
          showIcon
          message="DNS 服务商未配置"
          description={
            <Typography.Paragraph style={{ marginBottom: 0 }}>
              在 config.yaml 设置 <Typography.Text code>{guide.keys}</Typography.Text>
              ，或用环境变量 <Typography.Text code>{guide.env}</Typography.Text> 注入
              （推荐，secret 不落文件）后重启 server。{guide.note}
            </Typography.Paragraph>
          }
        />
      </Card>
    )
  }

  return (
    <Tabs
      defaultActiveKey="records"
      items={[
        {
          key: 'records',
          label: '记录管理',
          children: <RecordsPanel provider={status?.provider ?? 'cloudflare'} />,
        },
        { key: 'ddns', label: 'DDNS', children: <DDNSPanel /> },
      ]}
    />
  )
}

export default DNS
