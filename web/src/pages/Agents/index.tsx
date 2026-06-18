import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, Card, Input, Select, Space, Table } from 'antd'
import { ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { Agent } from '@/types'
import { api } from '@/services/api'
import TerminalModal from '@/components/TerminalModal'
import DesktopModal from '@/components/DesktopModal'
import VNCModal from '@/components/VNCModal'
import type { RemoteProtocol } from '@/services/remote'
import { buildAgentColumns } from './columns'
import { AgentDetailModal } from './AgentDetailModal'
import { useRemoteModals } from './useRemoteModals'

const PAGE_SIZE = 20

const Agents = () => {
  const [searchText, setSearchText] = useState('')
  const [regionFilter, setRegionFilter] = useState<string | undefined>()
  const [statusFilter, setStatusFilter] = useState<string | undefined>()
  const [virtFilter, setVirtFilter] = useState<string | undefined>()
  const [selectedAgent, setSelectedAgent] = useState<Agent | null>(null)
  const [detailVisible, setDetailVisible] = useState(false)

  const modals = useRemoteModals()

  const { data: agents = [], isFetching: loading, refetch: fetchAgents } = useQuery({
    queryKey: ['agents'],
    queryFn: () => api.getAgents(),
  })

  const refreshAgents = () => {
    void fetchAgents()
  }

  // 过滤逻辑
  const filteredAgents = useMemo(() => {
    let filtered = [...agents]
    if (searchText) {
      filtered = filtered.filter(
        (agent) =>
          agent.hostname?.toLowerCase().includes(searchText.toLowerCase()) ||
          agent.ip?.includes(searchText) ||
          agent.id?.toLowerCase().includes(searchText.toLowerCase()),
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
    return filtered
  }, [searchText, regionFilter, statusFilter, virtFilter, agents])

  const regions = Array.from(
    new Set(agents.map((a) => a.location?.region || 'unknown').filter(Boolean)),
  )

  const showDetail = (agent: Agent) => {
    setSelectedAgent(agent)
    setDetailVisible(true)
  }

  const handleConnect = (protocol: RemoteProtocol, host: string, port: number) => {
    modals.open(protocol, selectedAgent?.id || '', host, port)
  }

  const columns: ColumnsType<Agent> = buildAgentColumns({ onShowDetail: showDetail })

  return (
    <div className="page-container">
      <Card
        title="Agent 管理"
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={refreshAgents} loading={loading}>
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
          pagination={{ pageSize: PAGE_SIZE }}
        />
      </Card>

      <AgentDetailModal
        open={detailVisible}
        agent={selectedAgent}
        loading={loading}
        onClose={() => setDetailVisible(false)}
        onConnect={handleConnect}
      />

      {modals.terminalConfig && (
        <TerminalModal
          visible={modals.terminalVisible}
          onClose={() => modals.setTerminalVisible(false)}
          agentId={modals.terminalConfig.agentId}
          host={modals.terminalConfig.host}
          port={modals.terminalConfig.port}
          protocol={modals.terminalConfig.protocol as RemoteProtocol}
          title={modals.terminalConfig.title}
        />
      )}

      {modals.desktopConfig && (
        <DesktopModal
          visible={modals.desktopVisible}
          onClose={() => modals.setDesktopVisible(false)}
          agentId={modals.desktopConfig.agentId}
          host={modals.desktopConfig.host}
          port={modals.desktopConfig.port}
          title={modals.desktopConfig.title}
        />
      )}

      {modals.vncConfig && (
        <VNCModal
          visible={modals.vncVisible}
          onClose={() => modals.setVncVisible(false)}
          agentId={modals.vncConfig.agentId}
          host={modals.vncConfig.host}
          port={modals.vncConfig.port}
          title={modals.vncConfig.title}
        />
      )}
    </div>
  )
}

export default Agents
