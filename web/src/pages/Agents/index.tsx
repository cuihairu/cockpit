import { useState, useEffect } from 'react'
import {
  Card,
  Table,
  Tag,
  Button,
  Space,
  Input,
  Select,
  Tooltip,
  Modal,
  Descriptions,
} from 'antd'
import {
  ReloadOutlined,
  SearchOutlined,
  EnvironmentOutlined,
  ApiOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  ExclamationCircleOutlined,
  CodeOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { Agent } from '@/types'
import { api } from '@/services/api'

const Agents = () => {
  const [loading, setLoading] = useState(false)
  const [agents, setAgents] = useState<Agent[]>([])
  const [filteredAgents, setFilteredAgents] = useState<Agent[]>([])
  const [searchText, setSearchText] = useState('')
  const [regionFilter, setRegionFilter] = useState<string | undefined>()
  const [statusFilter, setStatusFilter] = useState<string | undefined>()
  const [selectedAgent, setSelectedAgent] = useState<Agent | null>(null)
  const [detailVisible, setDetailVisible] = useState(false)

  const fetchAgents = async () => {
    setLoading(true)
    try {
      const data = await api.getAgents()
      setAgents(data)
      setFilteredAgents(data)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchAgents()
  }, [])

  // 过滤逻辑
  useEffect(() => {
    let filtered = [...agents]

    if (searchText) {
      filtered = filtered.filter(
        (agent) =>
          agent.hostname?.toLowerCase().includes(searchText.toLowerCase()) ||
          agent.ip?.includes(searchText) ||
          agent.id?.toLowerCase().includes(searchText.toLowerCase())
      )
    }

    if (regionFilter) {
      filtered = filtered.filter((agent) => agent.location?.region === regionFilter)
    }

    if (statusFilter) {
      filtered = filtered.filter((agent) => agent.status === statusFilter)
    }

    setFilteredAgents(filtered)
  }, [searchText, regionFilter, statusFilter, agents])

  // 获取所有地域
  const regions = Array.from(new Set(agents.map((a) => a.location?.region || 'unknown').filter(Boolean)))

  const columns: ColumnsType<Agent> = [
    {
      title: 'Agent ID',
      dataIndex: 'id',
      key: 'id',
      width: 200,
      ellipsis: true,
      render: (id: string) => (
        <Space>
          <CodeOutlined />
          <span style={{ fontFamily: 'monospace' }}>{id}</span>
        </Space>
      ),
    },
    {
      title: '主机名',
      dataIndex: 'hostname',
      key: 'hostname',
      sorter: (a, b) => (a.hostname || '').localeCompare(b.hostname || ''),
      render: (hostname: string, record) => (
        <Space direction="vertical" size="small">
          <span>{hostname || '-'}</span>
          <span style={{ fontSize: 12, color: '#999' }}>{record.ip || '-'}</span>
        </Space>
      ),
    },
    {
      title: '位置',
      key: 'location',
      width: 180,
      render: (_, record) => (
        <Space direction="vertical" size="small">
          <Space>
            <EnvironmentOutlined />
            <span>{record.location?.region || 'unknown'}</span>
          </Space>
          <span style={{ fontSize: 12, color: '#999', marginLeft: 20 }}>
            {record.location?.zone || '-'}
          </span>
        </Space>
      ),
    },
    {
      title: '能力',
      dataIndex: 'capabilities',
      key: 'capabilities',
      width: 200,
      render: (capabilities: string[]) => (
        <Space size="small" wrap>
          {(capabilities || []).slice(0, 3).map((cap) => (
            <Tag key={cap} color="blue">
              {cap}
            </Tag>
          ))}
          {(capabilities || []).length > 3 && (
            <Tooltip title={capabilities.slice(3).join(', ')}>
              <Tag>+{(capabilities || []).length - 3}</Tag>
            </Tooltip>
          )}
        </Space>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      sorter: (a, b) => a.status?.localeCompare(b.status || ''),
      render: (status: string) => {
        const statusConfig: Record<string, { icon: React.ReactNode; color: string; text: string }> = {
          online: { icon: <CheckCircleOutlined />, color: 'success', text: '在线' },
          offline: { icon: <CloseCircleOutlined />, color: 'default', text: '离线' },
          error: { icon: <ExclamationCircleOutlined />, color: 'error', text: '异常' },
        }
        const config = statusConfig[status] || statusConfig.offline
        return (
          <Tag icon={config.icon} color={config.color}>
            {config.text}
          </Tag>
        )
      },
    },
    {
      title: '最后连接',
      dataIndex: 'lastSeen',
      key: 'lastSeen',
      width: 120,
      render: (timestamp: number) => {
        if (!timestamp) return '-'
        const date = new Date(timestamp * 1000)
        const now = new Date()
        const diff = Math.floor((now.getTime() - date.getTime()) / 1000 / 60)
        if (diff < 1) return '刚刚'
        if (diff < 60) return `${diff} 分钟前`
        if (diff < 1440) return `${Math.floor(diff / 60)} 小时前`
        return date.toLocaleDateString()
      },
    },
    {
      title: '操作',
      key: 'actions',
      width: 120,
      render: (_, record) => (
        <Button type="link" onClick={() => showDetail(record)}>
          详情
        </Button>
      ),
    },
  ]

  const showDetail = (agent: Agent) => {
    setSelectedAgent(agent)
    setDetailVisible(true)
  }

  return (
    <Card
      title="Agent 管理"
      extra={
        <Space>
          <Button icon={<ReloadOutlined />} onClick={fetchAgents} loading={loading}>
            刷新
          </Button>
        </Space>
      }
    >
      <Space style={{ marginBottom: 16 }} size="middle">
        <Input
          placeholder="搜索主机名、IP 或 Agent ID"
          prefix={<SearchOutlined />}
          style={{ width: 300 }}
          value={searchText}
          onChange={(e) => setSearchText(e.target.value)}
          allowClear
        />
        <Select
          placeholder="筛选地域"
          style={{ width: 150 }}
          value={regionFilter}
          onChange={setRegionFilter}
          allowClear
        >
          {regions.map((r) => (
            <Select.Option key={r} value={r}>
              {r}
            </Select.Option>
          ))}
        </Select>
        <Select
          placeholder="筛选状态"
          style={{ width: 120 }}
          value={statusFilter}
          onChange={setStatusFilter}
          allowClear
        >
          <Select.Option value="online">在线</Select.Option>
          <Select.Option value="offline">离线</Select.Option>
        </Select>
      </Space>

      <Table
        columns={columns}
        dataSource={filteredAgents}
        rowKey="id"
        loading={loading}
        pagination={{ pageSize: 20 }}
      />

      <Modal
        title="Agent 详情"
        open={detailVisible}
        onCancel={() => setDetailVisible(false)}
        footer={null}
        width={800}
      >
        {selectedAgent && (
          <Descriptions bordered column={2}>
            <Descriptions.Item label="Agent ID" span={2}>
              <code>{selectedAgent.id}</code>
            </Descriptions.Item>
            <Descriptions.Item label="主机名">{selectedAgent.hostname || '-'}</Descriptions.Item>
            <Descriptions.Item label="IP 地址">{selectedAgent.ip || '-'}</Descriptions.Item>
            <Descriptions.Item label="地域">{selectedAgent.location?.region || '-'}</Descriptions.Item>
            <Descriptions.Item label="可用区">{selectedAgent.location?.zone || '-'}</Descriptions.Item>
            <Descriptions.Item label="状态">
              <Tag
                color={selectedAgent.status === 'online' ? 'success' : 'default'}
              >
                {selectedAgent.status === 'online' ? '在线' : '离线'}
              </Tag>
            </Descriptions.Item>
            <Descriptions.Item label="最后连接">
              {selectedAgent.lastSeen
                ? new Date(selectedAgent.lastSeen * 1000).toLocaleString()
                : '-'}
            </Descriptions.Item>
            <Descriptions.Item label="能力" span={2}>
              <Space size="small" wrap>
                {(selectedAgent.capabilities || []).map((cap) => (
                  <Tag key={cap} color="blue">
                    {cap}
                  </Tag>
                ))}
              </Space>
            </Descriptions.Item>
          </Descriptions>
        )}
      </Modal>
    </Card>
  )
}

export default Agents
