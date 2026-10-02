import { Button, Card, Space, Table, Tooltip } from 'antd'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import { useResources } from './useResources'
import { storageColumns } from './columns'

const StoragesPage = () => {
  const { loading, storages, fetchAll } = useResources()

  return (
    <div className="page-container">
      <Card
        title="存储"
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
        <Table
          columns={storageColumns}
          dataSource={storages}
          rowKey="id"
          loading={loading}
          pagination={{ pageSize: 20 }}
          size="small"
        />
      </Card>
    </div>
  )
}

export default StoragesPage