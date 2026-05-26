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
  CheckCircleOutlined,
  CloseCircleOutlined,
  ExclamationCircleOutlined,
  CodeOutlined,
  CloudServerOutlined,
  DesktopOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { Agent } from '@/types'
import { api } from '@/services/api'
import RemoteServicesCard from '@/components/RemoteServices'
import TerminalModal from '@/components/TerminalModal'
import type { RemoteProtocol } from '@/services/remote'
import DesktopModal from '@/components/DesktopModal'
import VNCModal from '@/components/VNCModal'

// 虚拟化类型显示配置
const virtTypeConfig: Record<string, { label: string; icon: React.ReactNode; color: string }> = {
  kvm: { label: 'KVM', icon: <CloudServerOutlined />, color: 'blue' },
  vmware: { label: 'VMware', icon: <CloudServerOutlined />, color: 'green' },
  qemu: { label: 'QEMU', icon: <CloudServerOutlined />, color: 'cyan' },
  xen: { label: 'Xen', icon: <CloudServerOutlined />, color: 'purple' },
  virtualbox: { label: 'VirtualBox', icon: <CloudServerOutlined />, color: 'orange' },
  hyperv: { label: 'Hyper-V', icon: <CloudServerOutlined />, color: 'blue' },
  docker: { label: 'Docker', icon: <CloudServerOutlined />, color: 'blue' },
  lxc: { label: 'LXC', icon: <CloudServerOutlined />, color: 'geekblue' },
  container: { label: '容器', icon: <CloudServerOutlined />, color: 'magenta' },
  none: { label: '物理机', icon: <DesktopOutlined />, color: 'default' },
}

// 获取虚拟化显示信息
const getVirtDisplay = (agent: Agent) => {
  if (agent.virtRole === 'host' || agent.virtType === 'none') {
    return virtTypeConfig.none
  }
  if (agent.virtType && virtTypeConfig[agent.virtType]) {
    return virtTypeConfig[agent.virtType]
  }
  return { label: '未知', icon: <ExclamationCircleOutlined />, color: 'default' }
}

// 从 Agent 的 capabilities 中提取远程服务
const getRemoteServices = (agent: Agent | null) => {
  if (!agent || !agent.capabilities) return []
  const remoteCap = agent.capabilities.find((cap) => cap.type === 'remote-services')
  if (!remoteCap || !remoteCap.metadata) return []

  const services: Array<{ protocol: 'ssh' | 'rdp' | 'vnc' | 'telnet' | 'ftp'; host: string; port: number; name: string; running: boolean }> = []

  for (const [key, value] of Object.entries(remoteCap.metadata)) {
    if (typeof value === 'object' && value !== null && 'running' in value) {
      const service = value as { host: string; port: number; name: string; running: boolean }
      if (service.running && ['ssh', 'rdp', 'vnc', 'telnet', 'ftp'].includes(key)) {
        services.push({
          protocol: key as 'ssh' | 'rdp' | 'vnc' | 'telnet' | 'ftp',
          host: service.host || '127.0.0.1',
          port: service.port,
          name: service.name || `${key.toUpperCase()} Server`,
          running: true,
        })
      }
    }
  }

  return services
}

const Agents = () => {
  const [loading, setLoading] = useState(false)
  const [agents, setAgents] = useState<Agent[]>([])
  const [filteredAgents, setFilteredAgents] = useState<Agent[]>([])
  const [searchText, setSearchText] = useState('')
  const [regionFilter, setRegionFilter] = useState<string | undefined>()
  const [statusFilter, setStatusFilter] = useState<string | undefined>()
  const [virtFilter, setVirtFilter] = useState<string | undefined>()
  const [selectedAgent, setSelectedAgent] = useState<Agent | null>(null)
  const [detailVisible, setDetailVisible] = useState(false)

  // 终端相关状态
  const [terminalVisible, setTerminalVisible] = useState(false)
  const [terminalConfig, setTerminalConfig] = useState<{
    agentId: string
    host: string
    port: number
    protocol: RemoteProtocol
    title: string
  } | null>(null)

  // 桌面相关状态
  const [desktopVisible, setDesktopVisible] = useState(false)
  const [desktopConfig, setDesktopConfig] = useState<{
    agentId: string
    host: string
    port: number
    title: string
  } | null>(null)

  // VNC 相关状态
  const [vncVisible, setVncVisible] = useState(false)
  const [vncConfig, setVncConfig] = useState<{
    agentId: string
    host: string
    port: number
    title: string
  } | null>(null)

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

    if (virtFilter) {
      filtered = filtered.filter((agent) => {
        if (virtFilter === 'physical') return agent.virtRole === 'host' || agent.virtType === 'none'
        return agent.virtType === virtFilter
      })
    }

    setFilteredAgents(filtered)
  }, [searchText, regionFilter, statusFilter, virtFilter, agents])

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
      title: '类型',
      key: 'virtualization',
      width: 120,
      sorter: (a, b) => (a.virtType || '').localeCompare(b.virtType || ''),
      render: (_, record) => {
        const config = getVirtDisplay(record)
        return (
          <Tag icon={config.icon} color={config.color}>
            {config.label}
          </Tag>
        )
      },
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

  // 打开远程连接（按协议分流）
  const openTerminal = (protocol: RemoteProtocol, host: string, port: number) => {
    if (protocol === 'rdp') {
      setDesktopConfig({
        agentId: selectedAgent?.id || '',
        host,
        port,
        title: `RDP - ${host}:${port}`,
      })
      setDesktopVisible(true)
    } else if (protocol === 'vnc') {
      setVncConfig({
        agentId: selectedAgent?.id || '',
        host,
        port,
        title: `VNC - ${host}:${port}`,
      })
      setVncVisible(true)
    } else {
      setTerminalConfig({
        agentId: selectedAgent?.id || '',
        host,
        port,
        protocol,
        title: `${protocol.toUpperCase()} - ${host}:${port}`,
      })
      setTerminalVisible(true)
    }
  }

  return (
    <div className="page-container">
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
        <Select
          placeholder="筛选类型"
          style={{ width: 140 }}
          value={virtFilter}
          onChange={setVirtFilter}
          allowClear
        >
          <Select.Option value="physical">物理机</Select.Option>
          <Select.Option value="kvm">KVM</Select.Option>
          <Select.Option value="vmware">VMware</Select.Option>
          <Select.Option value="docker">Docker</Select.Option>
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
        width={900}
      >
        {selectedAgent && (
          <>
            <Descriptions bordered column={2} style={{ marginBottom: 16 }}>
              <Descriptions.Item label="Agent ID" span={2}>
                <code>{selectedAgent.id}</code>
              </Descriptions.Item>
              <Descriptions.Item label="主机名">{selectedAgent.hostname || '-'}</Descriptions.Item>
              <Descriptions.Item label="IP 地址">{selectedAgent.ip || '-'}</Descriptions.Item>
              <Descriptions.Item label="地域">{selectedAgent.location?.region || '-'}</Descriptions.Item>
              <Descriptions.Item label="可用区">{selectedAgent.location?.zone || '-'}</Descriptions.Item>
              <Descriptions.Item label="系统类型">
                {(() => {
                  const config = getVirtDisplay(selectedAgent)
                  return (
                    <Tag icon={config.icon} color={config.color}>
                      {config.label} ({selectedAgent.virtRole === 'guest' ? '虚拟机' : '宿主机'})
                    </Tag>
                  )
                })()}
              </Descriptions.Item>
              <Descriptions.Item label="状态">
                <Tag
                  color={selectedAgent.status === 'online' ? 'success' : 'default'}
                >
                  {selectedAgent.status === 'online' ? '在线' : '离线'}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label="最后连接">
                {selectedAgent.lastSeen
                  ? new Date(Number(selectedAgent.lastSeen) * 1000).toLocaleString()
                  : '-'}
              </Descriptions.Item>
              <Descriptions.Item label="能力" span={2}>
                <Space size="small" wrap>
                  {(selectedAgent.capabilities || []).map((cap) => (
                    <Tag key={cap.type} color="blue">
                      {cap.type}
                    </Tag>
                  ))}
                </Space>
              </Descriptions.Item>
              <Descriptions.Item label="标签" span={2}>
                <LabelsDisplay labels={selectedAgent.labels} />
              </Descriptions.Item>
            </Descriptions>

            {/* 远程服务卡片 */}
            <RemoteServicesCard
              agentId={selectedAgent.id}
	              services={getRemoteServices(selectedAgent)}
              loading={loading}
              onConnect={openTerminal}
            />
          </>
        )}
      </Modal>

      {/* 终端模态框 */}
      {terminalConfig && (
        <TerminalModal
          visible={terminalVisible}
          onClose={() => setTerminalVisible(false)}
          agentId={terminalConfig.agentId}
          host={terminalConfig.host}
          port={terminalConfig.port}
          protocol={terminalConfig.protocol}
          title={terminalConfig.title}
        />
      )}
    </Card>

      {/* 桌面模态框 */}
      {desktopConfig && (
        <DesktopModal
          visible={desktopVisible}
          onClose={() => setDesktopVisible(false)}
          agentId={desktopConfig.agentId}
          host={desktopConfig.host}
          port={desktopConfig.port}
          title={desktopConfig.title}
        />
      )}

      {/* VNC 模态框 */}
      {vncConfig && (
        <VNCModal
          visible={vncVisible}
          onClose={() => setVncVisible(false)}
          agentId={vncConfig.agentId}
          host={vncConfig.host}
          port={vncConfig.port}
          title={vncConfig.title}
        />
      )}
  </div>
  )
}

// 显示 Labels 的辅助组件
const LabelsDisplay: React.FC<{ labels?: Record<string, any> }> = ({ labels }) => {
  if (!labels || Object.keys(labels).length === 0) {
    return <span style={{ color: '#999' }}>-</span>
  }

  return (
    <Space size="small" wrap>
      {Object.entries(labels).map(([key, value]) => {
        let displayValue: string
        let color = 'default'

        if (Array.isArray(value)) {
          displayValue = value.join(', ')
          color = 'geekblue'
        } else if (typeof value === 'boolean') {
          displayValue = value ? '是' : '否'
          color = value ? 'success' : 'default'
        } else {
          displayValue = String(value)
          color = 'blue'
        }

        return (
          <Tag key={key} color={color}>
            <strong>{key}</strong>: {displayValue}
          </Tag>
        )
      })}
    </Space>
  )
}

export default Agents
