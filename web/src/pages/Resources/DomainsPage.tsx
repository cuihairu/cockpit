import { Button, Card, Space, Table, Tabs, Tooltip } from 'antd'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import { useResources } from './useResources'
import { domainColumns, certificateColumns } from './columns'

const DomainsPage = () => {
  const { loading, domains, certificates, fetchAll } = useResources()

  return (
    <div className="page-container">
      <Card
        title="域名与证书"
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={fetchAll} loading={loading} size="small">
              刷新
            </Button>
            <Tooltip title="请通过 inventory.yaml 管理">
              <Button type="primary" icon={<PlusOutlined />} size="small" disabled>
                添加
              </Button>
            </Tooltip>
          </Space>
        }
      >
        <Tabs
          items={[
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
                  size="small"
                />
              ),
            },
            {
              key: 'certs',
              label: `证书 (${certificates.length})`,
              children: (
                <Table
                  columns={certificateColumns}
                  dataSource={certificates}
                  rowKey="id"
                  loading={loading}
                  pagination={{ pageSize: 20 }}
                  size="small"
                />
              ),
            },
          ]}
        />
      </Card>
    </div>
  )
}

export default DomainsPage