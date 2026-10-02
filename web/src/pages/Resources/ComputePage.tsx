import { Button, Card, Space, Table, Tooltip } from 'antd'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import { useSettingsContext } from '@/contexts/useSettingsContext'
import { useResources } from './useResources'
import { computeColumns } from './columns'

const ComputePage = () => {
  const { settings } = useSettingsContext()
  const { loading, computeInstances, fetchAll } = useResources()

  return (
    <div className="page-container">
      <Card
        title="计算实例"
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
          columns={computeColumns}
          dataSource={computeInstances}
          rowKey="id"
          loading={loading}
          pagination={{ pageSize: 20 }}
          size="small"
        />
      </Card>
    </div>
  )
}

export default ComputePage