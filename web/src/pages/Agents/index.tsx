import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Alert, Button, Card, Input, Select, Space, Table, Tooltip, Typography } from 'antd'
import { ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { Agent } from '@/types'
import { api } from '@/services/api'
import { PermGuard } from '@/components/PermGuard'
import CleanupOfflineButton from '@/components/CleanupOfflineButton'
import TerminalModal from '@/components/TerminalModal'
import GuacamoleModal from '@/components/GuacamoleModal'
import type { RemoteProtocol } from '@/services/remote'
import { formatOfflineThreshold } from '@/utils/format'
import { buildAgentColumns } from './columns'
import { AgentDetailModal } from './AgentDetailModal'
import { isAgentExpired } from './helpers'
import { useRemoteModals } from './useRemoteModals'

const PAGE_SIZE = 20

const { Text } = Typography

const Agents = () => {
  const [searchText, setSearchText] = useState('')
  const [regionFilter, setRegionFilter] = useState<string | undefined>()
  const [statusFilter, setStatusFilter] = useState<string | undefined>()
  const [virtFilter, setVirtFilter] = useState<string | undefined>()
  const [selectedAgent, setSelectedAgent] = useState<Agent | null>(null)
  const [detailVisible, setDetailVisible] = useState(false)
  // 过期行默认隐藏（D-2026-10-08-3），可切回全量展示
  const [showExpired, setShowExpired] = useState(false)

  const modals = useRemoteModals()

  const { data: agents = [], isFetching: loading, refetch: fetchAgents } = useQuery({
    queryKey: ['agents'],
    queryFn: () => api.getAgents(),
  })

  // 过期阈值来自服务端（/api/status 下发，env AGENT_EXPIRE_MINUTES 可配）
  const { data: status } = useQuery({
    queryKey: ['status'],
    queryFn: () => api.getStatus(),
  })
  const expireMinutes = status?.agentExpireMinutes ?? 0

  const refreshAgents = () => {
    void fetchAgents()
  }

  // 过滤逻辑
  const filteredAgents = useMemo(() => {
    let filtered = [...agents]
    if (!showExpired) {
      filtered = filtered.filter((agent) => !isAgentExpired(agent, expireMinutes))
    }
    if (searchText) {
      filtered = filtered.filter(
        (agent) =>
          agent.hostname?.toLowerCase().includes(searchText.toLowerCase()) ||
          agent.ip?.includes(searchText) ||
          agent.id?.toLowerCase().includes(searchText.toLowerCase()),
      )
    }
    if (regionFilter) {
      filtered = filtered.filter((agent) => agent.region === regionFilter)
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
  }, [searchText, regionFilter, statusFilter, virtFilter, agents, expireMinutes, showExpired])

  // 过期行计数（Alert 提示与「显示/隐藏」切换用）
  const expiredCount = useMemo(
    () => (expireMinutes > 0 ? agents.filter((a) => isAgentExpired(a, expireMinutes)).length : 0),
    [agents, expireMinutes],
  )

  const regions = Array.from(
    new Set(agents.map((a) => a.region || 'unknown').filter(Boolean)),
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
            {status?.agentExpireMinutes !== undefined && (
              <Tooltip title="心跳/上报超过该时长即判过期：服务端自动标离线并释放连接，列表默认隐藏；env AGENT_EXPIRE_MINUTES 可配">
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {expireMinutes > 0
                    ? `过期阈值：${formatOfflineThreshold(expireMinutes)}`
                    : '自动过期已关闭'}
                </Text>
              </Tooltip>
            )}
            <PermGuard perm="inventory:write">
              <CleanupOfflineButton onCleaned={refreshAgents} />
            </PermGuard>
            <Button icon={<ReloadOutlined />} onClick={refreshAgents} loading={loading}>
              刷新
            </Button>
          </Space>
        }
      >
        {expireMinutes > 0 && (
          <Alert
            type={expiredCount > 0 ? 'warning' : 'info'}
            showIcon
            style={{ marginBottom: 16 }}
            message={
              expiredCount === 0
                ? `过期 agent 自动隐藏已开启（阈值 ${formatOfflineThreshold(expireMinutes)}），当前无过期 agent`
                : showExpired
                  ? `正在显示 ${expiredCount} 台过期 agent（心跳超过 ${formatOfflineThreshold(expireMinutes)}）`
                  : `已隐藏 ${expiredCount} 台过期 agent（心跳超过 ${formatOfflineThreshold(expireMinutes)}），物理清除请用「清理离线 agent」`
            }
            action={
              expiredCount > 0 ? (
                // autoInsertSpace={false}：antd 默认给两字中文按钮插空格（显 示），
                // 可访问名与测试断言都用紧凑写法
                <Button size="small" autoInsertSpace={false} onClick={() => setShowExpired(!showExpired)}>
                  {showExpired ? '隐藏' : '显示'}
                </Button>
              ) : undefined
            }
          />
        )}
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

      {modals.guacConfig && (
        <GuacamoleModal
          visible={modals.guacVisible}
          onClose={() => modals.setGuacVisible(false)}
          agentId={modals.guacConfig.agentId}
          host={modals.guacConfig.host}
          port={modals.guacConfig.port}
          protocol={modals.guacConfig.protocol}
          title={modals.guacConfig.title}
        />
      )}
    </div>
  )
}

export default Agents
