import { useQuery } from '@tanstack/react-query'
import { Alert, Button, Space, Table, Tag, Tooltip, Typography } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import type { AgentConsistency, ConsistencyStatus } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'

// CMDB 一致性（drift-design.md M6 D28-D30）：inventory 声明 vs agent 实报
// 的按需读模型比对。浏览类查询，30s staleTime 内不重复请求。

const STATUS_META: Record<ConsistencyStatus, { label: string; color: string; hint: string }> = {
  ok: { label: '一致', color: 'success', hint: '实报与 inventory 声明一致' },
  mismatch: { label: '不一致', color: 'error', hint: '实报 hostname/IP 与声明不符（展开看字段级对照）' },
  unregistered: { label: '未注册', color: 'warning', hint: '声明了但库内无实报：未上线，或声明先于装机' },
  undeclared: { label: '未声明', color: 'purple', hint: '库内有实报但 inventory YAML 未声明（CMDB 完整性缺口）' },
}

// 声明/实报事实快照的单侧呈现（hostname 为主、ip 并列可缺）
const Facts = ({ facts }: { facts?: { hostname?: string; ip?: string } }) =>
  !facts || (!facts.hostname && !facts.ip) ? (
    <Typography.Text type="secondary">-</Typography.Text>
  ) : (
    <Typography.Text style={{ fontSize: 13 }}>
      {facts.hostname || '-'}
      {facts.ip && (
        <Typography.Text type="secondary" style={{ fontSize: 12, marginLeft: 6 }}>
          {facts.ip}
        </Typography.Text>
      )}
    </Typography.Text>
  )

const ConsistencyTab = () => {
  const { data: report, isLoading, isError, error, refetch, isFetching } = useQuery({
    queryKey: ['inventory-consistency'],
    queryFn: () => api.getInventoryConsistency(),
    staleTime: 30_000,
  })

  if (isError) {
    return (
      <Alert
        type="warning"
        showIcon
        message="CMDB 一致性不可用"
        description={getApiErrorMessage(error, '获取一致性报告失败')}
      />
    )
  }

  const summary = report?.summary
  const agents = report?.agents ?? []
  const attention = (summary?.mismatch ?? 0) + (summary?.unregistered ?? 0) + (summary?.undeclared ?? 0)

  const columns: ColumnsType<AgentConsistency> = [
    { title: 'Agent', dataIndex: 'id', ellipsis: true },
    {
      title: '状态',
      dataIndex: 'status',
      width: 110,
      render: (status: ConsistencyStatus) => {
        const meta = STATUS_META[status]
        return (
          <Tooltip title={meta.hint}>
            <Tag color={meta.color}>{meta.label}</Tag>
          </Tooltip>
        )
      },
    },
    {
      title: '声明（inventory）',
      key: 'declared',
      width: 220,
      render: (_: unknown, it: AgentConsistency) => <Facts facts={it.declared} />,
    },
    {
      title: '实报（注册）',
      key: 'actual',
      width: 220,
      render: (_: unknown, it: AgentConsistency) => <Facts facts={it.actual} />,
    },
  ]

  return (
    <>
      <Space wrap style={{ marginBottom: 12 }} align="center">
        {summary && (
          <>
            <Typography.Text strong>共 {summary.total} 台</Typography.Text>
            <Tag color="success">一致 {summary.ok}</Tag>
            <Tag color="error">不一致 {summary.mismatch}</Tag>
            <Tag color="warning">未注册 {summary.unregistered}</Tag>
            <Tag color="purple">未声明 {summary.undeclared}</Tag>
          </>
        )}
        <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} loading={isFetching}>
          刷新
        </Button>
      </Space>

      {summary && attention === 0 && summary.total > 0 && (
        <Alert type="success" showIcon style={{ marginBottom: 12 }} message="声明与实报全部一致" />
      )}
      {summary && summary.total === 0 && (
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 12 }}
          message="inventory 未声明任何 agent：在 inventory YAML 的 regions/zones/agents 下声明后自动热加载"
        />
      )}

      <Table<AgentConsistency>
        rowKey="id"
        loading={isLoading}
        columns={columns}
        dataSource={agents}
        pagination={agents.length > 20 ? { pageSize: 20 } : false}
        size="small"
        expandable={{
          // mismatch 行展开声明 vs 实报的字段级对照（D30）
          rowExpandable: (it) => it.status === 'mismatch' && !!it.mismatch?.length,
          expandedRowRender: (it) => (
            <Table
              rowKey="field"
              size="small"
              pagination={false}
              columns={[
                {
                  title: '字段',
                  dataIndex: 'field',
                  width: 120,
                  render: (f: string) => (f === 'hostname' ? '主机名' : 'IP'),
                },
                { title: '声明值', dataIndex: 'declared' },
                { title: '实报值', dataIndex: 'actual' },
              ]}
              dataSource={it.mismatch ?? []}
            />
          ),
        }}
        locale={{ emptyText: '无可比对对象' }}
      />
    </>
  )
}

export default ConsistencyTab
