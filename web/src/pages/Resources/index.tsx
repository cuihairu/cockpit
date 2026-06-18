import { useState } from 'react'
import { Button, Card, Space, Tabs, Table } from 'antd'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { useResources } from './useResources'
import {
  certificateColumns,
  computeColumns,
  domainColumns,
  gatewayColumns,
  serviceColumns,
  storageColumns,
} from './columns'

const PAGE_SIZE = 20

const Resources = () => {
  const [activeTab, setActiveTab] = useState('compute')
  const { loading, computeInstances, domains, certificates, services, gateways, storages, fetchAll } =
    useResources()

  // 统一构建资源表格，消除重复 JSX（DRY）
  const buildTable = <T extends { id: string }>(
    columns: ColumnsType<T>,
    dataSource: T[],
  ) => (
    <Table
      columns={columns}
      dataSource={dataSource}
      rowKey="id"
      loading={loading}
      pagination={{ pageSize: PAGE_SIZE }}
    />
  )

  const tabItems = [
    {
      key: 'compute',
      label: `计算实例 (${computeInstances.length})`,
      children: buildTable(computeColumns, computeInstances),
    },
    {
      key: 'domains',
      label: `域名 (${domains.length})`,
      children: buildTable(domainColumns, domains),
    },
    {
      key: 'certificates',
      label: `证书 (${certificates.length})`,
      children: buildTable(certificateColumns, certificates),
    },
    {
      key: 'services',
      label: `服务 (${services.length})`,
      children: buildTable(serviceColumns, services),
    },
    {
      key: 'gateways',
      label: `网关 (${gateways.length})`,
      children: buildTable(gatewayColumns, gateways),
    },
    {
      key: 'storages',
      label: `存储 (${storages.length})`,
      children: buildTable(storageColumns, storages),
    },
  ]

  return (
    <div className="page-container">
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
    </div>
  )
}

export default Resources
