import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Card, Table, Tag, Button, Space, Input, Select, DatePicker, Statistic, Row, Col, message } from 'antd'
import {
  ReloadOutlined,
  SearchOutlined,
  DownloadOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { RangePickerProps } from 'antd/es/date-picker'
import dayjs from 'dayjs'
import { logger } from '@/utils/logger'
import './index.less'

const { RangePicker } = DatePicker

interface AuditLog {
  id: number
  user_id: string
  username: string
  action: string
  resource: string
  resource_id: string
  details: string
  ip: string
  user_agent: string
  status: string
  created_at: string
}

interface RemoteAuditDetails {
  protocol?: string
  agent_id?: string
  host?: string
  port?: number
  session_id?: string
  egress?: string
  duration?: string
  reason?: string
}

interface AuditLogStats {
  total_logs: number
  today_logs: number
  failed_logs: number
  by_action: Record<string, number>
  by_resource: Record<string, number>
  by_user: Record<string, number>
}

const ACTION_MAP: Record<string, { text: string; color: string }> = {
  login: { text: '登录', color: 'blue' },
  logout: { text: '登出', color: 'default' },
  create: { text: '创建', color: 'green' },
  update: { text: '更新', color: 'orange' },
  delete: { text: '删除', color: 'red' },
  view: { text: '查看', color: 'default' },
  export: { text: '导出', color: 'purple' },
  import: { text: '导入', color: 'cyan' },
  start: { text: '启动', color: 'green' },
  stop: { text: '停止', color: 'orange' },
}

const RESOURCE_MAP: Record<string, string> = {
  user: '用户',
  agent: 'Agent',
  domain: '域名',
  certificate: '证书',
  service: '服务',
  compute: '计算实例',
  gateway: '网关',
  storage: '存储',
  settings: '设置',
  remote_session: '远控会话',
}

function parseAuditDetails(details: string): Record<string, unknown> | null {
  if (!details) return null
  try {
    return JSON.parse(details) as Record<string, unknown>
  } catch {
    return null
  }
}

function isRemoteAuditDetails(details: Record<string, unknown> | null): details is Record<string, unknown> & RemoteAuditDetails {
  return Boolean(details && ('protocol' in details || 'agent_id' in details || 'egress' in details || 'reason' in details))
}

function renderRemoteAuditDetails(details: RemoteAuditDetails) {
  return (
    <Space direction="vertical" size={2} style={{ fontSize: 12 }}>
      {details.protocol && (
        <span>
          <strong>协议:</strong> {String(details.protocol).toUpperCase()}
        </span>
      )}
      {details.agent_id && (
        <span>
          <strong>Agent:</strong> {details.agent_id}
        </span>
      )}
      {(details.host || details.port) && (
        <span>
          <strong>目标:</strong> {details.host || '-'}{details.port ? `:${details.port}` : ''}
        </span>
      )}
      {details.egress && (
        <span>
          <strong>出口:</strong> {details.egress}
        </span>
      )}
      {details.reason && (
        <span style={{ color: '#F53F3F' }}>
          <strong>原因:</strong> {details.reason}
        </span>
      )}
      {details.duration && (
        <span>
          <strong>时长:</strong> {details.duration}
        </span>
      )}
    </Space>
  )
}

function renderGenericAuditDetails(details: Record<string, unknown> | null, raw: string) {
  if (!details) {
    return <span style={{ fontSize: 12 }}>{raw}</span>
  }

  return (
    <pre
      style={{
        margin: 0,
        fontSize: 12,
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-word',
        fontFamily: 'SFMono-Regular, Consolas, "Liberation Mono", Menlo, monospace',
      }}
    >
      {JSON.stringify(details, null, 2)}
    </pre>
  )
}

const fetchAuditLogs = async (
  filters: Record<string, string | undefined>,
  page: number,
  pageSize: number,
) => {
  const params = new URLSearchParams({
    page: page.toString(),
    page_size: pageSize.toString(),
  })

  Object.entries(filters).forEach(([key, value]) => {
    if (value) {
      params.set(key, value)
    }
  })

  const response = await fetch(`/api/admin/audit/logs?${params}`)
  return response.json()
}

const fetchAuditStats = async (): Promise<AuditLogStats> => {
  const response = await fetch('/api/admin/audit/stats')
  return response.json()
}

const exportAuditLogs = async (filters: Record<string, string | undefined>) => {
  const params = new URLSearchParams()
  Object.entries(filters).forEach(([key, value]) => {
    if (value) {
      params.set(key, value)
    }
  })

  const token = localStorage.getItem('token')
  const response = await fetch(`/api/admin/audit/export?${params}`, {
    headers: token ? { Authorization: `Bearer ${token}` } : undefined,
  })

  if (!response.ok) {
    throw new Error(`导出失败: ${response.status}`)
  }

  const blob = await response.blob()
  const url = window.URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = `audit-logs-${dayjs().format('YYYYMMDD-HHmmss')}.csv`
  document.body.appendChild(anchor)
  anchor.click()
  document.body.removeChild(anchor)
  window.URL.revokeObjectURL(url)
}

const isRemoteSessionFilterActive = (filters: {
  action?: string
  resource?: string
  username?: string
  status?: string
  start_time?: string
  end_time?: string
}) => filters.resource === 'remote_session'

const isFailedRemoteFilterActive = (filters: {
  action?: string
  resource?: string
  username?: string
  status?: string
  start_time?: string
  end_time?: string
}) => filters.resource === 'remote_session' && filters.status === 'failure'

const AuditLogs = () => {
  const [pagination, setPagination] = useState({ current: 1, pageSize: 20, total: 0 })
  const [filters, setFilters] = useState<{
    action?: string
    resource?: string
    username?: string
    status?: string
    start_time?: string
    end_time?: string
  }>({})

  const logsQuery = useQuery({
    queryKey: ['audit-logs', filters, pagination.current, pagination.pageSize],
    queryFn: () => fetchAuditLogs(filters, pagination.current, pagination.pageSize),
  })

  const statsQuery = useQuery({
    queryKey: ['audit-stats', filters],
    queryFn: fetchAuditStats,
  })

  const logs = (logsQuery.data?.data || []) as AuditLog[]
  const stats = statsQuery.data || null
  const loading = logsQuery.isFetching
  const total = logsQuery.data?.pagination?.total || 0

  useEffect(() => {
    if (statsQuery.error) {
      logger.error('Failed to fetch stats:', statsQuery.error)
    }
  }, [statsQuery.error])

  const refreshLogs = () => {
    void logsQuery.refetch()
    void statsQuery.refetch()
  }

  const handleExport = async () => {
    try {
      await exportAuditLogs(filters)
      void message.success('导出成功')
    } catch (error) {
      logger.error('Failed to export audit logs:', error)
      void message.error('导出失败')
    }
  }

  const applyRemoteSessionFilter = () => {
    setFilters({
      ...filters,
      resource: 'remote_session',
    })
    setPagination({ ...pagination, current: 1 })
  }

  const applyFailedRemoteFilter = () => {
    setFilters({
      ...filters,
      resource: 'remote_session',
      status: 'failure',
    })
    setPagination({ ...pagination, current: 1 })
  }

  const handleDateRangeChange: RangePickerProps['onChange'] = (dates) => {
    if (dates && dates[0] && dates[1]) {
      setFilters({
        ...filters,
        start_time: dates[0].startOf('day').toISOString(),
        end_time: dates[1].endOf('day').toISOString(),
      })
      setPagination({ ...pagination, current: 1 })
    } else {
      const nextFilters = { ...filters }
      delete nextFilters.start_time
      delete nextFilters.end_time
      setFilters(nextFilters)
      setPagination({ ...pagination, current: 1 })
    }
  }

  const columns: ColumnsType<AuditLog> = [
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      width: 80,
    },
    {
      title: '时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 180,
      render: (date: string) => {
        const d = new Date(date)
        return (
          <Space direction="vertical" size="small">
            <span>{dayjs(d).format('YYYY-MM-DD')}</span>
            <span style={{ color: '#86909C', fontSize: 12 }}>{dayjs(d).format('HH:mm:ss')}</span>
          </Space>
        )
      },
      sorter: true,
    },
    {
      title: '用户',
      dataIndex: 'username',
      key: 'username',
      width: 120,
    },
    {
      title: '操作',
      dataIndex: 'action',
      key: 'action',
      width: 100,
      render: (action: string) => {
        const config = ACTION_MAP[action] || { text: action, color: 'default' }
        return <Tag color={config.color}>{config.text}</Tag>
      },
    },
    {
      title: '资源',
      dataIndex: 'resource',
      key: 'resource',
      width: 100,
      render: (resource: string) => RESOURCE_MAP[resource] || resource,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status: string) => (
        <Tag
          icon={status === 'success' ? <CheckCircleOutlined /> : <CloseCircleOutlined />}
          color={status === 'success' ? 'success' : 'error'}
        >
          {status === 'success' ? '成功' : '失败'}
        </Tag>
      ),
    },
    {
      title: 'IP地址',
      dataIndex: 'ip',
      key: 'ip',
      width: 140,
      ellipsis: true,
    },
    {
      title: '详情',
      dataIndex: 'details',
      key: 'details',
      ellipsis: true,
      render: (details: string) => {
        if (!details) return '-'

        const parsed = parseAuditDetails(details)
        if (isRemoteAuditDetails(parsed)) {
          return renderRemoteAuditDetails(parsed)
        }

        return renderGenericAuditDetails(parsed, details)
      },
    },
  ]

  return (
    <div className="audit-logs-container">
      {/* 统计卡片 */}
      {stats && (
        <Row gutter={16} style={{ marginBottom: 24 }}>
          <Col span={6}>
            <Card>
              <Statistic title="总日志数" value={stats.total_logs} />
            </Card>
          </Col>
          <Col span={6}>
            <Card>
              <Statistic title="今日日志" value={stats.today_logs} valueStyle={{ color: '#1366EC' }} />
            </Card>
          </Col>
          <Col span={6}>
            <Card>
              <Statistic title="失败操作" value={stats.failed_logs} valueStyle={{ color: '#F53F3F' }} />
            </Card>
          </Col>
          <Col span={6}>
            <Card>
              <Statistic
                title="成功率"
                value={stats.total_logs > 0 ? ((stats.total_logs - stats.failed_logs) / stats.total_logs * 100).toFixed(1) : 100}
                suffix="%"
                valueStyle={{ color: '#00A870' }}
              />
            </Card>
          </Col>
        </Row>
      )}

      {/* 主表格 */}
      <Card
        title="审计日志"
        extra={
          <Space>
            <Button
              type={isRemoteSessionFilterActive(filters) && !isFailedRemoteFilterActive(filters) ? 'primary' : 'default'}
              onClick={applyRemoteSessionFilter}
            >
              远控会话
            </Button>
            <Button
              danger
              type={isFailedRemoteFilterActive(filters) ? 'primary' : 'default'}
              onClick={applyFailedRemoteFilter}
            >
              失败远控
            </Button>
            <Input
              placeholder="搜索用户名"
              prefix={<SearchOutlined />}
              style={{ width: 200 }}
              value={filters.username}
              onChange={(e) => {
                setFilters({ ...filters, username: e.target.value })
                setPagination({ ...pagination, current: 1 })
              }}
              onPressEnter={refreshLogs}
              allowClear
            />
            <Select
              placeholder="操作类型"
              style={{ width: 120 }}
              value={filters.action}
              onChange={(value) => {
                setFilters({ ...filters, action: value })
                setPagination({ ...pagination, current: 1 })
              }}
              allowClear
            >
              {Object.entries(ACTION_MAP).map(([key, { text }]) => (
                <Select.Option key={key} value={key}>{text}</Select.Option>
              ))}
            </Select>
            <Select
              placeholder="资源类型"
              style={{ width: 120 }}
              value={filters.resource}
              onChange={(value) => {
                setFilters({ ...filters, resource: value })
                setPagination({ ...pagination, current: 1 })
              }}
              allowClear
            >
              {Object.entries(RESOURCE_MAP).map(([key, value]) => (
                <Select.Option key={key} value={key}>{value}</Select.Option>
              ))}
            </Select>
            <RangePicker
              style={{ width: 280 }}
              onChange={handleDateRangeChange}
            />
            <Button icon={<ReloadOutlined />} onClick={refreshLogs}>
              刷新
            </Button>
            <Button icon={<DownloadOutlined />} onClick={() => void handleExport()}>
              导出
            </Button>
          </Space>
        }
      >
        <Table
          columns={columns}
          dataSource={logs}
          rowKey="id"
          loading={loading}
          pagination={{
            current: pagination.current,
            pageSize: pagination.pageSize,
            total,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (total) => `共 ${total} 条`,
            onChange: (page, pageSize) => {
              setPagination({ current: page, pageSize, total })
            },
          }}
          size="small"
        />
      </Card>
    </div>
  )
}

export default AuditLogs
