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
import {
  DeleteOutlined,
  DownloadOutlined,
  EditOutlined,
  PlusOutlined,
  ReloadOutlined,
  UploadOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import type { DNSRecord, DNSRecordInput, DNSZone } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'
import { PermGuard } from '@/components/PermGuard'
import DDNSPanel from './DDNSPanel'
import ImportModal from './ImportModal'

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
  const [importOpen, setImportOpen] = useState(false)
  const [editing, setEditing] = useState<DNSRecord | null>(null)
  const [form] = Form.useForm<RecordFormValues>()
  const queryClient = useQueryClient()

  const { data: zonesResult, isLoading: zonesLoading } = useQuery({
    queryKey: ['dns-zones'],
    queryFn: () => api.getDNSZones(),
  })
  const zones = zonesResult?.zones ?? []
  // 反向对账：DNS 来源台账行但所属 zone 已不在 provider 列表（M3 D20）
  const orphans = zonesResult?.orphans ?? []

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

  // 台账写联动（dns-design M3 D18/D20）：zone 显式登记 / 整 zone 移除
  // （移除同时清该 zone 的记录级跟随行，也是孤儿清理出口）；同名行已被
  // inventory/手工占用时 server 409，错误文案直接透出
  const registerMutation = useMutation({
    mutationFn: (z: DNSZone) => api.registerDNSZoneCMDB(z.id),
    onSuccess: () => {
      message.success('已登记到「资源 → 域名」')
      queryClient.invalidateQueries({ queryKey: ['dns-zones'] })
    },
    onError: (err) => message.error(getApiErrorMessage(err, '登记失败')),
  })

  const unregisterMutation = useMutation({
    mutationFn: (zoneID: string) => api.unregisterDNSZoneCMDB(zoneID),
    onSuccess: () => {
      message.success('已移除登记')
      queryClient.invalidateQueries({ queryKey: ['dns-zones'] })
    },
    onError: (err) => message.error(getApiErrorMessage(err, '移除失败')),
  })

  // 批量导出（M4 D22）：Blob 下载 <zone>-records.json（Acme 下载同款手法）
  const exportMutation = useMutation({
    mutationFn: () => api.exportDNSRecords(zoneId),
    onSuccess: (data) => {
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `${zone?.name ?? zoneId}-records.json`
      a.click()
      URL.revokeObjectURL(url)
      message.success(`已导出 ${data.count} 条记录`)
    },
    onError: (err) => message.error(getApiErrorMessage(err, '导出失败')),
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
        <PermGuard perm="dns:write">
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
        </PermGuard>
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
          {zone && (
            <PermGuard perm="dns:write">
              {zone.in_cmdb ? (
                <Popconfirm
                  title={`移除登记 ${zone.name}？`}
                  description="将同时清除该域名在台账里的记录级跟随行（仅 DNS 来源行，inventory 声明不受影响）"
                  onConfirm={() => unregisterMutation.mutate(zone.id)}
                >
                  <Button danger loading={unregisterMutation.isPending}>
                    移除登记
                  </Button>
                </Popconfirm>
              ) : (
                <Button
                  loading={registerMutation.isPending}
                  onClick={() => registerMutation.mutate(zone)}
                >
                  登记台账
                </Button>
              )}
            </PermGuard>
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
          <Button
            icon={<DownloadOutlined />}
            disabled={!zoneId}
            loading={exportMutation.isPending}
            onClick={() => exportMutation.mutate()}
          >
            导出
          </Button>
          <PermGuard perm="dns:write">
            <Button icon={<UploadOutlined />} disabled={!zoneId} onClick={() => setImportOpen(true)}>
              导入
            </Button>
          </PermGuard>
          <PermGuard perm="dns:write">
            <Button type="primary" icon={<PlusOutlined />} disabled={!zoneId} onClick={openCreate}>
              新建记录
            </Button>
          </PermGuard>
        </Space>
      </Card>

      {orphans.length > 0 && (
        <Alert
          type="warning"
          showIcon
          message="发现孤儿台账行"
          description={
            <Space direction="vertical" size={4} style={{ width: '100%' }}>
              {orphans.map((o) => (
                <Space key={o.id} wrap>
                  <Typography.Text code>{o.domain}</Typography.Text>
                  {o.zone_id ? (
                    <Popconfirm
                      title={`清理 ${o.domain} 的残留台账行？`}
                      description="仅删除 DNS 来源行（该 zone 的登记与记录跟随行），inventory 声明不受影响"
                      onConfirm={() => unregisterMutation.mutate(o.zone_id)}
                    >
                      <Button size="small">清理</Button>
                    </Popconfirm>
                  ) : (
                    <Typography.Text type="secondary">
                      无 zone 信息，请在「资源 → 域名」手工处理
                    </Typography.Text>
                  )}
                </Space>
              ))}
              <Typography.Text type="secondary">
                这些 DNS 来源台账行的域名已不在当前 provider 的 zone 列表（可能在面板外删除或切换过
                provider）
              </Typography.Text>
            </Space>
          }
        />
      )}

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

      <ImportModal
        zoneId={zoneId}
        zoneName={zone?.name}
        open={importOpen}
        onClose={() => setImportOpen(false)}
      />
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
