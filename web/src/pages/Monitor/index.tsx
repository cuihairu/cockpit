import React, { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { PageContainer, ProCard } from '@ant-design/pro-components';
import { Row, Col, Select, Spin, Alert } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import SystemInfoCard from '@/components/SystemInfoCard';
import MetricsChart from '@/components/MetricsChart';
import { getSystemSnapshots, getSystemSnapshot, getMetricsHistory } from '@/services/metrics';

const Monitor: React.FC = () => {
  const [selectedAgentId, setSelectedAgentId] = useState<string>('');
  const snapshotsQuery = useQuery({
    queryKey: ['system-snapshots'],
    queryFn: getSystemSnapshots,
    refetchInterval: 30000,
  });

  const snapshots = snapshotsQuery.data || [];
  const effectiveAgentId = selectedAgentId || snapshots[0]?.agentId || '';

  const currentSnapshotQuery = useQuery({
    queryKey: ['system-snapshot', effectiveAgentId],
    queryFn: () => getSystemSnapshot(effectiveAgentId),
    enabled: Boolean(effectiveAgentId),
    refetchInterval: 30000,
  });

  const metricsHistoryQuery = useQuery({
    queryKey: ['metrics-history', effectiveAgentId],
    queryFn: () => {
      const end = Date.now();
      const start = end - 24 * 60 * 60 * 1000; // 最近24小时
      return getMetricsHistory(effectiveAgentId, { start, end, limit: 1000 });
    },
    enabled: Boolean(effectiveAgentId),
    refetchInterval: 30000,
  });

  const currentSnapshot = currentSnapshotQuery.data || null;
  const metricsHistory = useMemo(
    () => metricsHistoryQuery.data?.data || [],
    [metricsHistoryQuery.data],
  );
  const loading = snapshotsQuery.isLoading;
  const refreshing =
    snapshotsQuery.isFetching || currentSnapshotQuery.isFetching || metricsHistoryQuery.isFetching;

  // 刷新数据
  const refresh = () => {
    void snapshotsQuery.refetch();
    void currentSnapshotQuery.refetch();
    void metricsHistoryQuery.refetch();
  };

  // 准备图表数据
  const chartData = metricsHistory.map((m) => ({
    time: m.timestamp,
    cpuValue: m.cpuUsage,
    memValue: m.memUsagePercent,
    diskValue: m.diskUsagePercent,
    load1Value: m.load1,
  }));

  const cpuChartData = chartData.map((d) => ({ time: d.time, value: d.cpuValue }));
  const memChartData = chartData.map((d) => ({ time: d.time, value: d.memValue }));
  const diskChartData = chartData.map((d) => ({ time: d.time, value: d.diskValue }));
  const loadChartData = chartData.map((d) => ({ time: d.time, value: d.load1Value }));

  return (
    <PageContainer
      title="系统监控"
      subTitle="实时监控 Agent 系统资源使用情况"
      extra={[
        <Select
          key="agent-select"
          style={{ width: 250, marginRight: 16 }}
          placeholder="选择 Agent"
          value={effectiveAgentId}
          onChange={setSelectedAgentId}
          loading={loading}
        >
          {snapshots.map((s) => (
            <Select.Option key={s.agentId} value={s.agentId}>
              {s.hostname} ({s.osName} {s.arch})
            </Select.Option>
          ))}
        </Select>,
        <ReloadIcon onClick={refresh} loading={refreshing} />,
      ]}
    >
      <Spin spinning={loading}>
        {snapshots.length === 0 ? (
          <Alert
            message="暂无在线 Agent"
            description="请确保至少有一个 Agent 正在运行并连接到服务器"
            type="info"
            showIcon
          />
        ) : (
          <>
            {/* 实时系统信息卡片 */}
            {currentSnapshot && (
              <SystemInfoCard
                key={currentSnapshot.agentId}
                systemInfo={currentSnapshot}
                loading={refreshing}
              />
            )}

            {/* 历史趋势图表 */}
            <ProCard title="历史趋势 (24小时)" headerBordered collapsible defaultCollapsed={false}>
              <Row gutter={[16, 16]}>
                <Col xs={24} lg={12}>
                  <MetricsChart
                    title="CPU 使用率"
                    data={cpuChartData}
                    unit="%"
                    color="#1890ff"
                    loading={refreshing}
                  />
                </Col>
                <Col xs={24} lg={12}>
                  <MetricsChart
                    title="内存使用率"
                    data={memChartData}
                    unit="%"
                    color="#52c41a"
                    loading={refreshing}
                  />
                </Col>
                <Col xs={24} lg={12}>
                  <MetricsChart
                    title="磁盘使用率"
                    data={diskChartData}
                    unit="%"
                    color="#faad14"
                    loading={refreshing}
                  />
                </Col>
                <Col xs={24} lg={12}>
                  <MetricsChart
                    title="系统负载 (1分钟)"
                    data={loadChartData}
                    unit=""
                    color="#722ed1"
                    loading={refreshing}
                    min={0}
                  />
                </Col>
              </Row>
            </ProCard>
          </>
        )}
      </Spin>
    </PageContainer>
  );
};

const ReloadIcon: React.FC<{ onClick: () => void; loading?: boolean }> = ({ onClick, loading }) => (
  <ReloadOutlined
    spin={loading}
    onClick={onClick}
    style={{ cursor: 'pointer', fontSize: 16 }}
  />
);

export default Monitor;
