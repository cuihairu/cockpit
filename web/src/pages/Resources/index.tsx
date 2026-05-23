import { useState } from 'react'
import { Card, Tabs, Table, Tag, Button, Space, Tooltip, Input, Select } from 'antd'
import {
  ReloadOutlined,
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  PoweroffOutlined,
  PlayCircleOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type {
  ComputeInstance,
  Domain,
  Certificate,
  Service,
} from '@/types'
import { api } from '@/services/api'

const Resources = () => {
  const [loading, setLoading] = useState(false)
  const [activeTab, setActiveTab] = useState('compute')

  // 计算实例数据
  const [computeInstances, setComputeInstances] = useState<ComputeInstance[]>([])

  // 域名数据
  const [domains, setDomains] = useState<Domain[]>([])

  // 证书数据
  const [certificates, setCertificates] = useState<Certificate[]>([])

  // 服务数据
  const [services, setServices] = useState<Service[]>([])

  const fetchAll = async () => {
    setLoading(true)
    try {
      const [computeData, domainsData, certsData, servicesData] = await Promise.all([
        api.getComputeInstances(),
        api.getDomains(),
        api.getCertificates(),
        api.getServices(),
      ])
      setComputeInstances(computeData.data || [])
      setDomains(domainsData.data || [])
      setCertificates(certsData.data || [])
      setServices(servicesData.data || [])
    } finally {
      setLoading(false)
    }
  }

  // 计算实例列定义
  const computeColumns: ColumnsType<ComputeInstance> = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      sorter: (a, b) => a.name.localeCompare(b.name),
    },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      width: 100,
      render: (type: string) => {
        const colorMap: Record<string, string> = {
          vm: 'blue',
          container: 'green',
          baremetal: 'orange',
        }
        return <Tag color={colorMap[type]}>{type.toUpperCase()}</Tag>
      },
    },
    {
      title: 'Agent',
      dataIndex: 'agentId',
      key: 'agentId',
      width: 120,
      ellipsis: true,
    },
    {
      title: '位置',
      key: 'location',
      width: 150,
      render: (_, record) => `${record.region || '-'}/${record.zone || '-'}`,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status: string) => {
        const colorMap: Record<string, string> = {
          running: 'success',
          stopped: 'default',
          error: 'error',
        }
        return <Tag color={colorMap[status]}>{status}</Tag>
      },
    },
    {
      title: '配置',
      key: 'config',
      width: 200,
      render: (_, record) => (
        <Space direction="vertical" size="small">
          <span>CPU: {record.cpuCores} 核</span>
          <span>内存: {record.memoryMb} MB</span>
          <span>磁盘: {record.diskGb} GB</span>
        </Space>
      ),
    },
    {
      title: 'IP',
      dataIndex: 'ipv4',
      key: 'ipv4',
      width: 140,
      render: (ip: string) => ip || '-',
    },
    {
      title: '操作',
      key: 'actions',
      width: 180,
      render: (_, record) => (
        <Space>
          <Tooltip title={record.status === 'running' ? '停止' : '启动'}>
            <Button
              type="text"
              icon={record.status === 'running' ? <PoweroffOutlined /> : <PlayCircleOutlined />}
              size="small"
            />
          </Tooltip>
          <Button type="text" icon={<EditOutlined />} size="small" />
          <Button type="text" danger icon={<DeleteOutlined />} size="small" />
        </Space>
      ),
    },
  ]

  // 域名列定义
  const domainColumns: ColumnsType<Domain> = [
    {
      title: '域名',
      dataIndex: 'domain',
      key: 'domain',
      render: (domain: string) => <a href={`https://${domain}`} target="_blank" rel="noreferrer">{domain}</a>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status: string) => {
        const colorMap: Record<string, string> = {
          active: 'success',
          expired: 'error',
          pending: 'warning',
        }
        return <Tag color={colorMap[status]}>{status}</Tag>
      },
    },
    {
      title: 'Provider',
      dataIndex: 'provider',
      key: 'provider',
      width: 120,
    },
    {
      title: '自动续费',
      dataIndex: 'autoRenew',
      key: 'autoRenew',
      width: 100,
      render: (autoRenew: boolean) => (
        <Tag color={autoRenew ? 'success' : 'default'}>{autoRenew ? '是' : '否'}</Tag>
      ),
    },
    {
      title: '过期时间',
      dataIndex: 'expiresAt',
      key: 'expiresAt',
      width: 120,
      render: (date: string) => date ? new Date(date).toLocaleDateString() : '-',
    },
    {
      title: '操作',
      key: 'actions',
      width: 120,
      render: () => (
        <Space>
          <Button type="text" icon={<EditOutlined />} size="small" />
          <Button type="text" danger icon={<DeleteOutlined />} size="small" />
        </Space>
      ),
    },
  ]

  // 证书列定义
  const certificateColumns: ColumnsType<Certificate> = [
    {
      title: '域名',
      dataIndex: 'domain',
      key: 'domain',
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status: string) => {
        const colorMap: Record<string, string> = {
          valid: 'success',
          expiring: 'warning',
          expired: 'error',
        }
        return <Tag color={colorMap[status]}>{status}</Tag>
      },
    },
    {
      title: '签发者',
      dataIndex: 'issuer',
      key: 'issuer',
      width: 150,
    },
    {
      title: '过期时间',
      dataIndex: 'expiresAt',
      key: 'expiresAt',
      width: 120,
      render: (date: string) => date ? new Date(date).toLocaleDateString() : '-',
    },
    {
      title: '自动续费',
      dataIndex: 'autoRenew',
      key: 'autoRenew',
      width: 100,
      render: (autoRenew: boolean) => (
        <Tag color={autoRenew ? 'success' : 'default'}>{autoRenew ? '是' : '否'}</Tag>
      ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 120,
      render: () => (
        <Space>
          <Button type="text" icon={<EditOutlined />} size="small" />
          <Button type="text" danger icon={<DeleteOutlined />} size="small" />
        </Space>
      ),
    },
  ]

  // 服务列定义
  const serviceColumns: ColumnsType<Service> = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      width: 100,
    },
    {
      title: 'URL',
      dataIndex: 'url',
      key: 'url',
      ellipsis: true,
      render: (url: string) => (
        url ? <a href={url} target="_blank" rel="noreferrer">{url}</a> : '-'
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status: string) => {
        const colorMap: Record<string, string> = {
          up: 'success',
          down: 'error',
          degraded: 'warning',
        }
        return <Tag color={colorMap[status]}>{status}</Tag>
      },
    },
    {
      title: '响应时间',
      dataIndex: 'responseTimeMs',
      key: 'responseTimeMs',
      width: 100,
      render: (ms: number) => ms ? `${ms}ms` : '-',
    },
    {
      title: '操作',
      key: 'actions',
      width: 120,
      render: () => (
        <Space>
          <Button type="text" icon={<EditOutlined />} size="small" />
          <Button type="text" danger icon={<DeleteOutlined />} size="small" />
        </Space>
      ),
    },
  ]

  const tabItems = [
    {
      key: 'compute',
      label: `计算实例 (${computeInstances.length})`,
      children: (
        <Table
          columns={computeColumns}
          dataSource={computeInstances}
          rowKey="id"
          loading={loading}
          pagination={{ pageSize: 20 }}
        />
      ),
    },
    {
      key: 'domains',
      label: `域名 (${domains.length})`,
      children: (
        <Table
          columns={domainColumns}
          dataSource={domains}
          rowKey="id"
          loading={loading}
          pagination={{ pageSize: 20 }}
        />
      ),
    },
    {
      key: 'certificates',
      label: `证书 (${certificates.length})`,
      children: (
        <Table
          columns={certificateColumns}
          dataSource={certificates}
          rowKey="id"
          loading={loading}
          pagination={{ pageSize: 20 }}
        />
      ),
    },
    {
      key: 'services',
      label: `服务 (${services.length})`,
      children: (
        <Table
          columns={serviceColumns}
          dataSource={services}
          rowKey="id"
          loading={loading}
          pagination={{ pageSize: 20 }}
        />
      ),
    },
  ]

  return (
    <Card
      title="资源管理"
      extra={
        <Space>
          <Button icon={<ReloadOutlined />} onClick={fetchAll} loading={loading}>
            刷新
          </Button>
          <Button type="primary" icon={<PlusOutlined />}>
            添加资源
          </Button>
        </Space>
      }
    >
      <Tabs activeKey={activeTab} onChange={setActiveTab} items={tabItems} />
    </Card>
  )
}

export default Resources
