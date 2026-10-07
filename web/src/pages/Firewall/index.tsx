import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Alert,
  Badge,
  Card,
  Collapse,
  Empty,
  Space,
  Spin,
  Table,
  Tag,
  Tooltip,
  Typography,
} from 'antd'
import { SafetyOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import type { Agent, FirewallChain, FirewallRule, FirewallStatus, FirewallTable } from '@/types'

// 防火墙观测：iptables/nftables 规则集只读快照（见 docs/guide/firewall-design.md）。
// server 纯转发不落库，总览由前端逐 agent 拉取后聚合（磁盘健康/存储池同款）。
// M1 只观测：无巡检配置、无任何规则写路径。

// 基础链默认策略徽标：accept 用警示色——「默认策略是不是 accept」是巡检第一眼
const POLICY_META: Record<string, { label: string; status: 'success' | 'warning' | 'default' }> = {
  accept: { label: 'accept', status: 'warning' },
  drop: { label: 'drop', status: 'success' },
}

const BASE_CHAINS = ['input', 'forward', 'output']

// 基础链策略：跨后端归一（nft 小写、iptables 大写），缺省不显示
const basePolicies = (tables: FirewallTable[]): { name: string; policy: string }[] => {
  const found: { name: string; policy: string }[] = []
  for (const table of tables) {
    for (const chain of table.chains ?? []) {
      const name = chain.name.toLowerCase()
      if (BASE_CHAINS.includes(name) && chain.policy && !found.some((f) => f.name === name)) {
        found.push({ name, policy: chain.policy.toLowerCase() })
      }
    }
  }
  return found.sort((a, b) => BASE_CHAINS.indexOf(a.name) - BASE_CHAINS.indexOf(b.name))
}

const PolicyBadges: React.FC<{ status: FirewallStatus }> = ({ status }) => {
  const policies = basePolicies(status.tables ?? [])
  if (policies.length === 0) return <span>—</span>
  return (
    <Space wrap size={4}>
      {policies.map((p) => {
        const meta = POLICY_META[p.policy] ?? { label: p.policy, status: 'default' as const }
        return (
          <Badge
            key={p.name}
            status={meta.status}
            text={<Typography.Text style={{ fontSize: 12 }}>{`${p.name} ${meta.label}`}</Typography.Text>}
          />
        )
      })}
    </Space>
  )
}

const formatCounters = (r: FirewallRule) => {
  if (!r.packets && !r.bytes) return '—'
  return `${r.packets ?? 0} / ${r.bytes ?? 0}`
}

// 单主机规则明细（Collapse 内）：family×table → chain → 规则列表
const AgentFirewallPanel: React.FC<{ agentId: string }> = ({ agentId }) => {
  const { data, isLoading, isError } = useQuery({
    queryKey: ['firewall-status', agentId],
    queryFn: () => api.getFirewallStatus(agentId),
    staleTime: 30_000,
    retry: false,
  })

  if (isLoading) {
    return (
      <div style={{ textAlign: 'center', padding: 24 }}>
        <Spin />
      </div>
    )
  }
  if (isError || !data) {
    return <Alert type="warning" showIcon message="该主机防火墙状态获取失败" />
  }
  // 说明态：工具在但读数失败（典型为非 root 部署）
  if (!data.available) {
    return (
      <Alert
        type="warning"
        showIcon
        message={`防火墙规则集不可读${data.backend ? `（${data.backend}）` : ''}`}
        description={data.error || 'Agent 需以 root 运行才能读取防火墙规则集'}
      />
    )
  }

  const tables = data.tables ?? []
  if (tables.length === 0) {
    return <Empty description="规则集为空（未配置任何规则）" />
  }

  const ruleColumns: ColumnsType<FirewallRule & { key: string }> = [
    {
      title: '规则',
      dataIndex: 'text',
      render: (v: string, r) =>
        r.ownedByCockpit ? (
          <Space size={6}>
            <Tooltip title="由 cockpit 管理的规则">
              <Tag color="blue">cockpit</Tag>
            </Tooltip>
            <Typography.Text code>{v || '（无表达式）'}</Typography.Text>
          </Space>
        ) : (
          <Typography.Text code>{v || '（无表达式）'}</Typography.Text>
        ),
    },
    { title: '包/字节', key: 'counters', width: 170, render: (_: unknown, r) => formatCounters(r) },
  ]

  const tableItems = tables.map((t, ti) => ({
    key: `${t.family}:${t.name}:${ti}`,
    label: (
      <Space size={8}>
        <Typography.Text strong>{t.name}</Typography.Text>
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          {t.family}
        </Typography.Text>
      </Space>
    ),
    children: (
      <Space direction="vertical" style={{ width: '100%' }} size={12}>
        {(t.chains ?? []).map((chain: FirewallChain, ci: number) => {
          const policy = chain.policy ? chain.policy.toLowerCase() : ''
          const meta = POLICY_META[policy]
          return (
            <div key={`${chain.name}:${ci}`}>
              <Space size={8} style={{ marginBottom: 4 }}>
                <Typography.Text code>{chain.name}</Typography.Text>
                {meta && <Badge status={meta.status} text={policy} />}
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  {chain.rules?.length ?? 0} 条规则
                </Typography.Text>
              </Space>
              <Table<FirewallRule & { key: string }>
                size="small"
                rowKey={(r, idx) => `${r.handle ?? idx}`}
                dataSource={(chain.rules ?? []).map((r, ri) => ({ ...r, key: `${r.handle ?? 'x'}:${ri}` }))}
                pagination={false}
                columns={ruleColumns}
                locale={{ emptyText: '空链' }}
              />
            </div>
          )
        })}
      </Space>
    ),
  }))

  return (
    <Space direction="vertical" style={{ width: '100%' }} size={8}>
      {data.error && (
        <Alert type="info" showIcon message={data.error} />
      )}
      <Collapse items={tableItems} size="small" />
    </Space>
  )
}

const Firewall = () => {
  const { data: agents = [] } = useQuery({ queryKey: ['agents'], queryFn: () => api.getAgents() })

  // 带 firewall capability 的在线 agent
  const firewallAgents = useMemo(
    () => agents.filter((a) => a.status !== 'offline' && (a.capabilities ?? []).some((c) => c.type === 'firewall')),
    [agents],
  )

  const statuses = useQuery({
    queryKey: ['firewall-overview', firewallAgents.map((a) => a.id).join(',')],
    queryFn: async () => {
      const results = await Promise.allSettled(
        firewallAgents.map(async (a) => ({ agent: a, status: await api.getFirewallStatus(a.id) })),
      )
      return results
        .filter((r) => r.status === 'fulfilled')
        .map((r) => (r as PromiseFulfilledResult<{ agent: Agent; status: FirewallStatus }>).value)
    },
    enabled: firewallAgents.length > 0,
    staleTime: 30_000,
    retry: false,
  })

  const overviewColumns: ColumnsType<{ agent: Agent; status: FirewallStatus; key: string }> = [
    { title: '主机', key: 'agent', width: 150, ellipsis: true, render: (_: unknown, r) => r.agent.hostname || r.agent.id },
    {
      title: '后端',
      key: 'backend',
      width: 130,
      render: (_: unknown, r) => {
        if (!r.status.available) return '—'
        const variant = r.status.iptablesVariant
        return (
          <Space size={6}>
            <Typography.Text code>{r.status.backend}</Typography.Text>
            {variant && (
              <Tooltip title={`iptables 后端：${variant}`}>
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  {variant}
                </Typography.Text>
              </Tooltip>
            )}
          </Space>
        )
      },
    },
    {
      title: '版本',
      key: 'version',
      width: 170,
      ellipsis: true,
      render: (_: unknown, r) => {
        const v = r.status.backendVersion
        if (!v) return '—'
        return (
          <Tooltip title={v}>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              {v}
            </Typography.Text>
          </Tooltip>
        )
      },
    },
    { title: '规则总数', dataIndex: ['status', 'totalRules'], width: 90 },
    {
      title: '默认策略',
      key: 'policies',
      render: (_: unknown, r) =>
        r.status.available ? <PolicyBadges status={r.status} /> : <span>—</span>,
    },
    {
      title: '状态',
      key: 'state',
      width: 220,
      render: (_: unknown, r) => {
        if (!r.status.available) {
          return (
            <Tooltip title={r.status.error || 'Agent 需以 root 运行'}>
              <Typography.Text type="danger" style={{ fontSize: 12 }}>
                不可读{r.status.error ? `：${r.status.error}` : ''}
              </Typography.Text>
            </Tooltip>
          )
        }
        if (r.status.error) {
          return (
            <Tooltip title={r.status.error}>
              <Typography.Text type="warning" style={{ fontSize: 12 }}>
                部分读取
              </Typography.Text>
            </Tooltip>
          )
        }
        return <Badge status="success" text="正常" />
      },
    },
  ]

  const panelItems = firewallAgents.map((a) => ({
    key: a.id,
    label: a.hostname || a.id,
    children: <AgentFirewallPanel agentId={a.id} />,
  }))

  const truncatedHosts = (statuses.data ?? []).filter((r) => r.status.truncated)

  return (
    <div style={{ padding: 24 }}>
      <Card
        title={
          <Space>
            <SafetyOutlined />
            防火墙
          </Space>
        }
      >
          <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 16 }}>
            观测来源为 nftables（nft list ruleset）或 iptables-save 规则集快照，只读不落库；默认策略 accept 意味着未显式拒绝的流量一律放行，巡检时优先确认。
          </Typography.Text>

          {firewallAgents.length === 0 ? (
            <Empty description="暂无带防火墙工具的在线主机——装有 nftables 或 iptables 的 Linux 主机运行 Agent 即可" />
          ) : statuses.isLoading ? (
            <div style={{ textAlign: 'center', padding: 32 }}>
              <Spin tip="正在收集各主机防火墙规则集…" />
            </div>
          ) : (
            <>
              {truncatedHosts.length > 0 && (
                <Alert
                  type="warning"
                  showIcon
                  style={{ marginBottom: 16 }}
                  message={`${truncatedHosts.map((r) => r.agent.hostname || r.agent.id).join('、')} 规则集过大，仅显示前 4MB`}
                />
              )}
              <Table<{ agent: Agent; status: FirewallStatus; key: string }>
                rowKey="key"
                columns={overviewColumns}
                dataSource={(statuses.data ?? []).map((r) => ({ ...r, key: r.agent.id }))}
                pagination={false}
                size="small"
                locale={{ emptyText: '各主机暂无防火墙读数' }}
                style={{ marginBottom: 16 }}
              />
            </>
          )}
        </Card>

      {firewallAgents.length > 0 && (
        <Card title="按主机查看" style={{ marginTop: 16 }}>
          <Collapse items={panelItems} />
        </Card>
      )}
    </div>
  )
}

export default Firewall
