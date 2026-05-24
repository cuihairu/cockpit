import React, { useState, useEffect, useCallback } from 'react';
import { PageContainer, ProCard } from '@ant-design/pro-components';
import { Row, Col, Select, Spin, Alert, Empty } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import SystemInfoCard from '@/components/SystemInfoCard';
import MetricsChart from '@/components/MetricsChart';
import {
  getSystemSnapshots,
  getSystemSnapshot,
  getMetricsHistory,
  formatBytes,
  type SystemInfoSnapshot,
  type SystemMetric,
} from '@/services/metrics';

const Monitor: React.FC = () => {
  const [snapshots, setSnapshots] = useState<SystemInfoSnapshot[]>([]);
  const [selectedAgentId, setSelectedAgentId] = useState<string>('');
  const [currentSnapshot, setCurrentSnapshot] = useState<SystemInfoSnapshot | null>(null);
  const [metricsHistory, setMetricsHistory] = useState<SystemMetric[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);

  // 获取所有快照
  const fetchSnapshots = useCallback(async () => {
    try {
      const data = await getSystemSnapshots();
      setSnapshots(data || []);
      if (data && data.length > 0 && !selectedAgentId) {
        setSelectedAgentId(data[0].agentId);
      }
    } catch (error) {
      console.error('Failed to fetch snapshots:', error);
    }
  }, [selectedAgentId]);

  // 获取单个 Agent 的系统信息
  const fetchSnapshot = useCallback(async (agentId: string) => {
    try {
      const data = await getSystemSnapshot(agentId);
      setCurrentSnapshot(data);
    } catch (error) {
      console.error('Failed to fetch snapshot:', error);
    }
  }, []);

  // 获取历史指标
  const fetchMetricsHistory = useCallback(async (agentId: string) => {
    try {
      const end = Date.now();
      const start = end - 24 * 60 * 60 * 1000; // 最近24小时
      const response = await getMetricsHistory(agentId, { start, end, limit: 1000 });
      setMetricsHistory(response.data || []);
    } catch (error) {
      console.error('Failed to fetch metrics history:', error);
    }
  }, []);

  // 刷新数据
  const refresh = useCallback(async () => {
    setRefreshing(true);
    await fetchSnapshots();
    if (selectedAgentId) {
      await Promise.all([
        fetchSnapshot(selectedAgentId),
        fetchMetricsHistory(selectedAgentId),
      ]);
    }
    setRefreshing(false);
  }, [selectedAgentId, fetchSnapshots, fetchSnapshot, fetchMetricsHistory]);

  // 初始化
  useEffect(() => {
    const init = async () => {
      setLoading(true);
      await fetchSnapshots();
      setLoading(false);
    };
    init();
  }, [fetchSnapshots]);

  // 选择 Agent 变化时
  useEffect(() => {
    if (selectedAgentId) {
      fetchSnapshot(selectedAgentId);
      fetchMetricsHistory(selectedAgentId);
    }
  }, [selectedAgentId, fetchSnapshot, fetchMetricsHistory]);

  // 定时刷新
  useEffect(() => {
    const interval = setInterval(() => {
      refresh();
    }, 30000); // 每30秒刷新

    return () => clearInterval(interval);
  }, [refresh]);

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
          value={selectedAgentId}
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
