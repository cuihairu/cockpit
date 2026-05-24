import React, { useState, useEffect } from 'react';
import { Card, List, Tag, Button, Space, Modal, Form, Input, Select, message, Tooltip } from 'antd';
import {
  CodeOutlined,
  DesktopOutlined,
  EyeOutlined,
  CloudServerOutlined,
  LinkOutlined,
} from '@ant-design/icons';

// 远程服务类型
export type RemoteProtocol = 'ssh' | 'rdp' | 'vnc' | 'telnet' | 'ftp';

export interface RemoteService {
  protocol: RemoteProtocol;
  host: string;
  port: number;
  name: string;
  running: boolean;
}

interface RemoteServicesCardProps {
  agentId?: string;
  services?: RemoteService[];
  loading?: boolean;
  onConnect?: (protocol: RemoteProtocol, host: string, port: number) => void;
}

// 协议配置
const PROTOCOL_CONFIG: Record<RemoteProtocol, { name: string; icon: React.ReactNode; color: string; defaultPort: number }> = {
  ssh: { name: 'SSH', icon: <CodeOutlined />, color: 'blue', defaultPort: 22 },
  rdp: { name: 'RDP', icon: <DesktopOutlined />, color: 'green', defaultPort: 3389 },
  vnc: { name: 'VNC', icon: <EyeOutlined />, color: 'orange', defaultPort: 5900 },
  telnet: { name: 'Telnet', icon: <CodeOutlined />, color: 'default', defaultPort: 23 },
  ftp: { name: 'FTP', icon: <CloudServerOutlined />, color: 'cyan', defaultPort: 21 },
};

const RemoteServicesCard: React.FC<RemoteServicesCardProps> = ({
  agentId,
  services = [],
  loading = false,
  onConnect,
}) => {
  const [connectModalVisible, setConnectModalVisible] = useState(false);
  const [selectedService, setSelectedService] = useState<RemoteService | null>(null);
  const [form] = Form.useForm();

  // 过滤出运行中的服务
  const activeServices = services.filter((s) => s.running);

  const handleConnect = (service: RemoteService) => {
    setSelectedService(service);
    form.setFieldsValue({
      protocol: service.protocol,
      host: service.host,
      port: service.port,
    });
    setConnectModalVisible(true);
  };

  const handleQuickConnect = () => {
    setSelectedService(null);
    form.resetFields();
    setConnectModalVisible(true);
  };

  const handleConnectSubmit = () => {
    form.validateFields().then((values) => {
      const { protocol, host, port } = values;
      if (onConnect) {
        onConnect(protocol, host, port);
      } else {
        // 默认行为：复制连接命令
        copyConnectCommand(protocol, host, port);
      }
      setConnectModalVisible(false);
    });
  };

  const copyConnectCommand = (protocol: RemoteProtocol, host: string, port: number) => {
    let command = '';
    switch (protocol) {
      case 'ssh':
        command = `ssh -p ${port} ${host}`;
        break;
      case 'vnc':
        command = `vncviewer ${host}:${port}`;
        break;
      case 'rdp':
        command = `rdesktop ${host}:${port}`;
        break;
      default:
        command = `${protocol}://${host}:${port}`;
    }

    navigator.clipboard.writeText(command).then(() => {
      message.success('连接命令已复制到剪贴板');
    });
  };

  return (
    <Card
      title="远程服务"
      extra={
        <Button type="primary" size="small" icon={<LinkOutlined />} onClick={handleQuickConnect}>
          自定义连接
        </Button>
      }
      size="small"
    >
      {activeServices.length === 0 ? (
        <div style={{ textAlign: 'center', padding: '20px 0', color: '#999' }}>
          未检测到远程服务
        </div>
      ) : (
        <List
          size="small"
          dataSource={activeServices}
          renderItem={(service) => {
            const config = PROTOCOL_CONFIG[service.protocol];
            return (
              <List.Item
                actions={[
                  <Button
                    type="link"
                    size="small"
                    onClick={() => handleConnect(service)}
                  >
                    连接
                  </Button>,
                ]}
              >
                <List.Item.Meta
                  avatar={
                    <div style={{ fontSize: 20, color: config.color }}>
                      {config.icon}
                    </div>
                  }
                  title={
                    <Space>
                      <Tag color={config.color}>{config.name}</Tag>
                      <span>{service.name}</span>
                    </Space>
                  }
                  description={`${service.host}:${service.port}`}
                />
              </List.Item>
            );
          }}
        />
      )}

      <Modal
        title="远程连接"
        open={connectModalVisible}
        onOk={handleConnectSubmit}
        onCancel={() => setConnectModalVisible(false)}
        width={500}
      >
        <Form form={form} layout="vertical">
          <Form.Item
            label="协议"
            name="protocol"
            rules={[{ required: true, message: '请选择协议' }]}
          >
            <Select>
              {Object.entries(PROTOCOL_CONFIG).map(([key, config]) => (
                <Select.Option key={key} value={key}>
                  <Space>
                    {config.icon}
                    {config.name}
                  </Space>
                </Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item
            label="主机"
            name="host"
            rules={[{ required: true, message: '请输入主机地址' }]}
          >
            <Input placeholder="例如: 192.168.1.100" />
          </Form.Item>
          <Form.Item
            label="端口"
            name="port"
            rules={[{ required: true, message: '请输入端口' }]}
          >
            <Input type="number" placeholder="例如: 22" />
          </Form.Item>
        </Form>
        <div style={{ marginTop: 16, padding: 12, background: '#f5f5f5', borderRadius: 4 }}>
          <div style={{ fontSize: 12, color: '#666', marginBottom: 8 }}>
            <strong>连接命令：</strong>
          </div>
          <div style={{ fontFamily: 'monospace', fontSize: 12 }}>
            {Form.useWatch('protocol', form) === 'ssh' && (
              <span>ssh -p {Form.useWatch('port', form) || 22} {Form.useWatch('host', form) || 'host'}</span>
            )}
            {Form.useWatch('protocol', form) === 'vnc' && (
              <span>vncviewer {Form.useWatch('host', form) || 'host'}:{Form.useWatch('port', form) || 5900}</span>
            )}
            {Form.useWatch('protocol', form) === 'rdp' && (
              <span>rdesktop {Form.useWatch('host', form) || 'host'}:{Form.useWatch('port', form) || 3389}</span>
            )}
          </div>
        </div>
      </Modal>
    </Card>
  );
};

export default RemoteServicesCard;
