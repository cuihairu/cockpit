import React from 'react';
import { Card, Row, Col, Statistic, Progress, Tag, Typography, Space } from 'antd';
import {
  CloudServerOutlined,
  DatabaseOutlined,
  ClockCircleOutlined,
  DesktopOutlined,
  HddOutlined,
} from '@ant-design/icons';
import { formatBytes, formatUptime } from '@/services/metrics';
import type { SystemInfoSnapshot } from '@/services/metrics';

const { Text } = Typography;

interface SystemInfoCardProps {
  systemInfo: SystemInfoSnapshot;
  loading?: boolean;
}

const SystemInfoCard: React.FC<SystemInfoCardProps> = ({ systemInfo, loading = false }) => {
  return (
    <Card
      title={
        <Space>
          <DesktopOutlined />
          <Text strong>{systemInfo.hostname}</Text>
          <Tag color="blue">{systemInfo.osName}</Tag>
          <Tag>{systemInfo.arch}</Tag>
        </Space>
      }
      loading={loading}
    >
      <Row gutter={[16, 16]}>
        {/* CPU 使用率 */}
        <Col xs={24} sm={12} md={8}>
          <Card size="small">
            <Statistic
              title={<><CloudServerOutlined /> CPU 使用率</>}
              value={systemInfo.cpuUsage}
              precision={1}
              suffix="%"
              valueStyle={{ color: systemInfo.cpuUsage > 80 ? '#cf1322' : '#3f8600' }}
            />
            <Progress
              percent={systemInfo.cpuUsage}
              status={systemInfo.cpuUsage > 80 ? 'exception' : 'active'}
              strokeColor={systemInfo.cpuUsage > 80 ? '#cf1322' : '#1890ff'}
              size="small"
            />
            <Text type="secondary">核心数: {systemInfo.cpuCores}</Text>
          </Card>
        </Col>

        {/* 内存使用率 */}
        <Col xs={24} sm={12} md={8}>
          <Card size="small">
            <Statistic
              title={<><HddOutlined /> 内存使用率</>}
              value={systemInfo.memUsagePercent}
              precision={1}
              suffix="%"
              valueStyle={{ color: systemInfo.memUsagePercent > 80 ? '#cf1322' : '#3f8600' }}
            />
            <Progress
              percent={systemInfo.memUsagePercent}
              status={systemInfo.memUsagePercent > 80 ? 'exception' : 'active'}
              strokeColor={systemInfo.memUsagePercent > 80 ? '#cf1322' : '#52c41a'}
              size="small"
            />
            <Text type="secondary">
              {formatBytes(systemInfo.memUsed)} / {formatBytes(systemInfo.memTotal)}
            </Text>
          </Card>
        </Col>

        {/* 磁盘使用率 */}
        <Col xs={24} sm={12} md={8}>
          <Card size="small">
            <Statistic
              title={<><DatabaseOutlined /> 磁盘使用率</>}
              value={systemInfo.diskUsagePercent}
              precision={1}
              suffix="%"
              valueStyle={{ color: systemInfo.diskUsagePercent > 80 ? '#cf1322' : '#3f8600' }}
            />
            <Progress
              percent={systemInfo.diskUsagePercent}
              status={systemInfo.diskUsagePercent > 80 ? 'exception' : 'active'}
              strokeColor={systemInfo.diskUsagePercent > 80 ? '#cf1322' : '#faad14'}
              size="small"
            />
            <Text type="secondary">
              {formatBytes(systemInfo.diskUsed)} / {formatBytes(systemInfo.diskTotal)}
            </Text>
          </Card>
        </Col>

        {/* 系统负载 */}
        <Col xs={24} sm={12} md={8}>
          <Card size="small" title="系统负载">
            <Row gutter={8}>
              <Col span={8}>
                <Statistic title="1分钟" value={systemInfo.load1} precision={2} />
              </Col>
              <Col span={8}>
                <Statistic title="5分钟" value={systemInfo.load5} precision={2} />
              </Col>
              <Col span={8}>
                <Statistic title="15分钟" value={systemInfo.load15} precision={2} />
              </Col>
            </Row>
          </Card>
        </Col>

        {/* 运行时间 */}
        <Col xs={24} sm={12} md={8}>
          <Card size="small">
            <Statistic
              title={<><ClockCircleOutlined /> 运行时间</>}
              value={formatUptime(systemInfo.uptime)}
            />
          </Card>
        </Col>

        {/* 网络流量 */}
        <Col xs={24} sm={12} md={8}>
          <Card size="small" title="网络流量">
            <Row gutter={8}>
              <Col span={12}>
                <Statistic
                  title="上传"
                  value={formatBytes(systemInfo.netBytesSent)}
                  valueStyle={{ fontSize: '14px' }}
                />
              </Col>
              <Col span={12}>
                <Statistic
                  title="下载"
                  value={formatBytes(systemInfo.netBytesRecv)}
                  valueStyle={{ fontSize: '14px' }}
                />
              </Col>
            </Row>
          </Card>
        </Col>
      </Row>
    </Card>
  );
};

export default SystemInfoCard;
