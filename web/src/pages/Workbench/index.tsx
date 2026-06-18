import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, Card, Col, Layout, Row, Space, Tabs, message } from 'antd'
import {
  CodeOutlined,
  DesktopOutlined,
  EyeOutlined,
  SettingOutlined,
} from '@ant-design/icons'
import { api } from '@/services/api'
import TerminalModal from '@/components/TerminalModal'
import DesktopModal from '@/components/DesktopModal'
import VNCModal from '@/components/VNCModal'
import AgentSidebar from '@/workbench/AgentSidebar'
import ConnectionPanel from '@/workbench/ConnectionPanel'
import OverviewPanel from '@/workbench/OverviewPanel'
import { getRemoteServices } from '@/workbench/services'
import type { SessionConfig, WorkbenchTab } from '@/workbench/types'

const protocolTabs: Array<{ key: WorkbenchTab; label: string; icon: React.ReactNode }> = [
  { key: 'overview', label: '概览', icon: <SettingOutlined /> },
  { key: 'ssh', label: 'SSH', icon: <CodeOutlined /> },
  { key: 'rdp', label: 'RDP', icon: <DesktopOutlined /> },
  { key: 'vnc', label: 'VNC', icon: <EyeOutlined /> },
]

const Workbench = () => {
  const [selectedAgentId, setSelectedAgentId] = useState('')
  const [query, setQuery] = useState('')
  const [tab, setTab] = useState<WorkbenchTab>('overview')
  const [terminalConfig, setTerminalConfig] = useState<SessionConfig | null>(null)
  const [desktopConfig, setDesktopConfig] = useState<SessionConfig | null>(null)
  const [vncConfig, setVncConfig] = useState<SessionConfig | null>(null)

  const { data: agents = [], isFetching: loading, refetch: loadAgents } = useQuery({
    queryKey: ['workbench-agents'],
    queryFn: () => api.getAgents(),
  })

  const refreshAgents = () => {
    void loadAgents()
  }

  const filteredAgents = useMemo(() => {
    const keyword = query.trim().toLowerCase()
    if (!keyword) return agents
    return agents.filter((agent) => {
      return (
        agent.id.toLowerCase().includes(keyword) ||
        (agent.hostname || '').toLowerCase().includes(keyword) ||
        (agent.ip || '').toLowerCase().includes(keyword) ||
        (agent.location?.region || '').toLowerCase().includes(keyword)
      )
    })
  }, [agents, query])

  const effectiveSelectedAgentId = selectedAgentId || filteredAgents[0]?.id || ''
  const selectedAgent = filteredAgents.find((agent) => agent.id === effectiveSelectedAgentId) || null
  const remoteServices = useMemo(() => getRemoteServices(selectedAgent), [selectedAgent])

  const openConnection = (protocol: WorkbenchTab) => {
    setTab(protocol)
    if (!selectedAgent) {
      message.warning('请先选择一台服务器')
      return
    }
    if (protocol === 'overview') return

    const service = remoteServices.find((item) => item.protocol === protocol)
    if (!service) {
      message.warning(`未检测到可用的 ${protocol.toUpperCase()} 服务`)
      return
    }

    const config: SessionConfig = {
      agentId: selectedAgent.id,
      host: service.host,
      port: service.port,
      protocol: service.protocol,
      title: `${service.protocol.toUpperCase()} - ${selectedAgent.hostname || selectedAgent.id}`,
    }

    if (service.protocol === 'rdp') {
      setDesktopConfig(config)
    } else if (service.protocol === 'vnc') {
      setVncConfig(config)
    } else {
      setTerminalConfig(config)
    }
  }

  return (
    <Layout style={{ minHeight: 'calc(100vh - 120px)', background: 'transparent' }}>
      <Row gutter={16} style={{ flex: 1 }}>
        <Col xs={24} lg={7} xl={6} style={{ display: 'flex' }}>
          <AgentSidebar
            agents={filteredAgents}
            loading={loading}
            query={query}
            selectedAgentId={effectiveSelectedAgentId}
            onQueryChange={setQuery}
            onRefresh={refreshAgents}
            onSelect={setSelectedAgentId}
          />
        </Col>

        <Col xs={24} lg={17} xl={18} style={{ display: 'flex' }}>
          <Card title={selectedAgent ? selectedAgent.hostname || selectedAgent.id : '工作台'} style={{ width: '100%' }}>
            <Space style={{ marginBottom: 16 }} wrap>
              {protocolTabs.map((item) => (
                <Button
                  key={item.key}
                  icon={item.icon}
                  type={tab === item.key ? 'primary' : 'default'}
                  onClick={() => openConnection(item.key)}
                >
                  {item.label}
                </Button>
              ))}
            </Space>

            <Tabs
              activeKey={tab}
              onChange={(key) => setTab(key as WorkbenchTab)}
              items={[
                {
                  key: 'overview',
                  label: '概览',
                  children: <OverviewPanel agent={selectedAgent} />,
                },
                {
                  key: 'ssh',
                  label: 'SSH',
                  children: (
                    <ConnectionPanel
                      protocol="ssh"
                      service={remoteServices.find((item) => item.protocol === 'ssh')}
                      onConnect={() => openConnection('ssh')}
                    />
                  ),
                },
                {
                  key: 'rdp',
                  label: 'RDP',
                  children: (
                    <ConnectionPanel
                      protocol="rdp"
                      service={remoteServices.find((item) => item.protocol === 'rdp')}
                      onConnect={() => openConnection('rdp')}
                    />
                  ),
                },
                {
                  key: 'vnc',
                  label: 'VNC',
                  children: (
                    <ConnectionPanel
                      protocol="vnc"
                      service={remoteServices.find((item) => item.protocol === 'vnc')}
                      onConnect={() => openConnection('vnc')}
                    />
                  ),
                },
              ]}
            />
          </Card>
        </Col>
      </Row>

      {terminalConfig && (
        <TerminalModal
          visible={Boolean(terminalConfig)}
          onClose={() => setTerminalConfig(null)}
          agentId={terminalConfig.agentId}
          host={terminalConfig.host}
          port={terminalConfig.port}
          protocol={terminalConfig.protocol}
          title={terminalConfig.title}
        />
      )}

      {desktopConfig && (
        <DesktopModal
          visible={Boolean(desktopConfig)}
          onClose={() => setDesktopConfig(null)}
          agentId={desktopConfig.agentId}
          host={desktopConfig.host}
          port={desktopConfig.port}
          title={desktopConfig.title}
        />
      )}

      {vncConfig && (
        <VNCModal
          visible={Boolean(vncConfig)}
          onClose={() => setVncConfig(null)}
          agentId={vncConfig.agentId}
          host={vncConfig.host}
          port={vncConfig.port}
          title={vncConfig.title}
        />
      )}
    </Layout>
  )
}

export default Workbench
