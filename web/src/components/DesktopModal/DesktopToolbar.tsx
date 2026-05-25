import React from 'react';
import { Space, Button, Select, Tooltip, Badge, Dropdown } from 'antd';
import {
  FullscreenOutlined,
  FullscreenExitOutlined,
  LinkOutlined,
  DisconnectOutlined,
  CopyOutlined,
  SettingOutlined,
} from '@ant-design/icons';
import type { ConnectionState } from './useDesktopWS';

const RESOLUTIONS = [
  { label: '1280 x 800', value: '1280x800' },
  { label: '1366 x 768', value: '1366x768' },
  { label: '1600 x 900', value: '1600x900' },
  { label: '1920 x 1080', value: '1920x1080' },
  { label: '2560 x 1440', value: '2560x1440' },
];

interface DesktopToolbarProps {
  state: ConnectionState;
  resolution?: string;
  isFullscreen: boolean;
  onResolutionChange: (width: number, height: number) => void;
  onToggleFullscreen: () => void;
  onDisconnect: () => void;
  onClipboardPaste?: () => void;
}

const DesktopToolbar: React.FC<DesktopToolbarProps> = ({
  state,
  resolution,
  isFullscreen,
  onResolutionChange,
  onToggleFullscreen,
  onDisconnect,
  onClipboardPaste,
}) => {
  const stateConfig: Record<ConnectionState, { color: string; text: string }> = {
    disconnected: { color: '#ff4d4f', text: '已断开' },
    connecting: { color: '#faad14', text: '连接中' },
    connected: { color: '#52c41a', text: '已连接' },
  };

  const current = stateConfig[state];

  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '6px 12px',
        background: '#1f1f1f',
        borderBottom: '1px solid #333',
        color: '#ccc',
        fontSize: 13,
        userSelect: 'none',
      }}
    >
      <Space size="middle">
        <Badge color={current.color} text={<span style={{ color: '#ccc' }}>{current.text}</span>} />

        {state === 'connected' && resolution && (
          <span style={{ color: '#888' }}>{resolution}</span>
        )}
      </Space>

      <Space size="small">
        {state === 'connected' && (
          <>
            <Dropdown
              menu={{
                items: RESOLUTIONS.map((r) => ({
                  key: r.value,
                  label: r.label,
                  onClick: () => {
                    const [w, h] = r.value.split('x').map(Number);
                    onResolutionChange(w, h);
                  },
                })),
              }}
              trigger={['click']}
            >
              <Button size="small" type="text" icon={<SettingOutlined />} style={{ color: '#ccc' }}>
                分辨率
              </Button>
            </Dropdown>

            {onClipboardPaste && (
              <Tooltip title="粘贴到远程">
                <Button
                  size="small"
                  type="text"
                  icon={<CopyOutlined />}
                  style={{ color: '#ccc' }}
                  onClick={onClipboardPaste}
                />
              </Tooltip>
            )}

            <Tooltip title={isFullscreen ? '退出全屏' : '全屏'}>
              <Button
                size="small"
                type="text"
                icon={isFullscreen ? <FullscreenExitOutlined /> : <FullscreenOutlined />}
                style={{ color: '#ccc' }}
                onClick={onToggleFullscreen}
              />
            </Tooltip>
          </>
        )}

        <Tooltip title="断开">
          <Button
            size="small"
            type="text"
            danger={state === 'connected'}
            icon={<DisconnectOutlined />}
            onClick={onDisconnect}
            disabled={state === 'disconnected'}
          />
        </Tooltip>
      </Space>
    </div>
  );
};

export default DesktopToolbar;
