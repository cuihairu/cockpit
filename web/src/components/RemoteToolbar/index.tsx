import React from 'react';
import { Space, Button, Tooltip, Badge, Dropdown } from 'antd';
import {
  FullscreenOutlined,
  FullscreenExitOutlined,
  DisconnectOutlined,
  CopyOutlined,
  SettingOutlined,
} from '@ant-design/icons';

export type ConnectionState = 'disconnected' | 'connecting' | 'connected';

export interface ToolbarAction {
  key: string;
  label: string;
  icon?: React.ReactNode;
  onClick: () => void;
  disabled?: boolean;
  danger?: boolean;
}

export interface ResolutionOption {
  label: string;
  value: string;
  width: number;
  height: number;
}

export interface RemoteToolbarProps {
  state: ConnectionState;
  resolution?: string;
  isFullscreen: boolean;
  showResolution?: boolean;
  onToggleFullscreen: () => void;
  onDisconnect: () => void;
  onClipboardPaste?: () => void;
  onResolutionChange?: (width: number, height: number) => void;
  resolutions?: ResolutionOption[];
  extraActions?: ToolbarAction[];
  children?: React.ReactNode;
}

const DEFAULT_RESOLUTIONS: ResolutionOption[] = [
  { label: '1280 x 800', value: '1280x800', width: 1280, height: 800 },
  { label: '1366 x 768', value: '1366x768', width: 1366, height: 768 },
  { label: '1600 x 900', value: '1600x900', width: 1600, height: 900 },
  { label: '1920 x 1080', value: '1920x1080', width: 1920, height: 1080 },
  { label: '2560 x 1440', value: '2560x1440', width: 2560, height: 1440 },
];

const STATE_CONFIG: Record<ConnectionState, { color: string; text: string }> = {
  disconnected: { color: '#ff4d4f', text: '已断开' },
  connecting: { color: '#faad14', text: '连接中' },
  connected: { color: '#52c41a', text: '已连接' },
};

const RemoteToolbar: React.FC<RemoteToolbarProps> = ({
  state,
  resolution,
  isFullscreen,
  showResolution = true,
  onToggleFullscreen,
  onDisconnect,
  onClipboardPaste,
  onResolutionChange,
  resolutions = DEFAULT_RESOLUTIONS,
  extraActions = [],
  children,
}) => {
  const current = STATE_CONFIG[state];
  const isConnected = state === 'connected';

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
        {showResolution && isConnected && resolution && (
          <span style={{ color: '#888' }}>{resolution}</span>
        )}
        {children}
      </Space>

      <Space size="small">
        {isConnected && (
          <>
            {onResolutionChange && showResolution && (
              <Dropdown
                menu={{
                  items: resolutions.map((r) => ({
                    key: r.value,
                    label: r.label,
                    onClick: () => onResolutionChange(r.width, r.height),
                  })),
                }}
                trigger={['click']}
              >
                <Button size="small" type="text" icon={<SettingOutlined />} style={{ color: '#ccc' }}>
                  分辨率
                </Button>
              </Dropdown>
            )}

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

        {extraActions.map((action) => (
          <Tooltip key={action.key} title={action.label}>
            <Button
              size="small"
              type="text"
              icon={action.icon}
              danger={action.danger}
              disabled={action.disabled}
              style={{ color: '#ccc' }}
              onClick={action.onClick}
            >
              {!action.icon && action.label}
            </Button>
          </Tooltip>
        ))}

        <Tooltip title="断开">
          <Button
            size="small"
            type="text"
            danger={isConnected}
            icon={<DisconnectOutlined />}
            onClick={onDisconnect}
            disabled={state === 'disconnected'}
          />
        </Tooltip>
      </Space>
    </div>
  );
};

export default RemoteToolbar;
