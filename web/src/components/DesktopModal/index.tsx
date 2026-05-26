import React, { useState, useCallback, useRef, useEffect } from 'react';
import { Modal, Form, Input, message } from 'antd';
import DesktopToolbar from './DesktopToolbar';
import { useDesktopWS } from './useDesktopWS';
import { useCanvasRenderer } from './useCanvasRenderer';
import { useInputCapture } from './useInputCapture';
import { getRecentDesktopConfig, saveDesktopConfig } from '@/services/desktop';

interface DesktopModalProps {
  visible: boolean;
  onClose: () => void;
  agentId: string;
  host: string;
  port: number;
  title?: string;
}

const DesktopModal: React.FC<DesktopModalProps> = ({
  visible,
  onClose,
  agentId,
  host,
  port,
  title,
}) => {
  const [showCredentials, setShowCredentials] = useState(true);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [resolution, setResolution] = useState('1280x800');
  const containerRef = useRef<HTMLDivElement>(null);
  const [form] = Form.useForm();

  // 打开时自动填充上次使用的凭据
  useEffect(() => {
    if (visible) {
      const saved = getRecentDesktopConfig(agentId, host, port);
      if (saved) {
        form.setFieldsValue({
          username: saved.username,
          domain: saved.domain,
        });
      }
    }
  }, [visible, agentId, host, port, form]);

  const renderer = useCanvasRenderer();

  // 用 ref 保存 renderer 的回调，避免 useDesktopWS 回调中引用不稳定的 renderer
  const rendererRef = useRef(renderer);
  rendererRef.current = renderer;

  const {
    state,
    connect,
    disconnect,
    sendKeyboard,
    sendMouse,
    sendSetResolution,
  } = useDesktopWS({
    onConnected: (w, h) => {
      setResolution(`${w}x${h}`);
      setShowCredentials(false);
      rendererRef.current.initBuffer(w, h);
    },
    onDisconnected: (reason) => {
      message.info(`连接断开: ${reason}`);
    },
    onError: (error) => {
      message.error(`远程桌面错误: ${error}`);
    },
    onScreenUpdate: (update) => {
      rendererRef.current.handleScreenUpdate(update);
    },
    onClipboard: (text) => {
      navigator.clipboard.writeText(text).catch(() => {});
    },
  });

  const { setCanvas } = useInputCapture({
    sendKeyboard,
    sendMouse,
    enabled: state === 'connected' && !showCredentials,
  });

  const canvasRef = useCallback((node: HTMLCanvasElement | null) => {
    renderer.canvasRef.current = node;
    setCanvas(node);
  }, [renderer, setCanvas]);

  const handleConnect = () => {
    form.validateFields().then((values) => {
      const username = values.username || '';
      const password = values.password || '';
      const domain = values.domain || '';

      // 保存连接配置（不含密码）
      saveDesktopConfig({
        name: `${host}:${port}`,
        agentId,
        host,
        port,
        username,
        domain,
        width: 1280,
        height: 800,
      });

      connect({
        agentId,
        host,
        port,
        username,
        password,
        domain,
        width: 1280,
        height: 800,
      });
    });
  };

  const handleDisconnect = () => {
    disconnect();
    setShowCredentials(true);
    if (isFullscreen) {
      document.exitFullscreen?.();
      setIsFullscreen(false);
    }
  };

  const handleClose = () => {
    disconnect();
    setShowCredentials(true);
    setIsFullscreen(false);
    onClose();
  };

  const handleToggleFullscreen = () => {
    if (isFullscreen) {
      document.exitFullscreen?.();
      setIsFullscreen(false);
    } else {
      containerRef.current?.requestFullscreen?.();
      setIsFullscreen(true);
    }
  };

  const handleResolutionChange = (width: number, height: number) => {
    sendSetResolution(width, height);
    setResolution(`${width}x${height}`);
  };

  useEffect(() => {
    const handler = () => {
      setIsFullscreen(!!document.fullscreenElement);
    };
    document.addEventListener('fullscreenchange', handler);
    return () => document.removeEventListener('fullscreenchange', handler);
  }, []);

  // 凭据输入 UI
  if (showCredentials && state === 'disconnected') {
    return (
      <Modal
        title={title || `RDP 远程桌面 - ${host}:${port}`}
        open={visible}
        onCancel={handleClose}
        onOk={handleConnect}
        okText="连接"
        width={420}
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item label="用户名" name="username">
            <Input placeholder="administrator" autoFocus />
          </Form.Item>
          <Form.Item label="密码" name="password">
            <Input.Password placeholder="password" />
          </Form.Item>
          <Form.Item label="域" name="domain">
            <Input placeholder="(可选)" />
          </Form.Item>
        </Form>
      </Modal>
    );
  }

  // 远程桌面 UI
  return (
    <Modal
      title={null}
      open={visible}
      onCancel={handleClose}
      footer={null}
      width={isFullscreen ? '100vw' : 1320}
      style={isFullscreen ? { top: 0, maxWidth: '100vw', paddingBottom: 0 } : undefined}
      styles={{
        body: { padding: 0, background: '#1e1e1e', height: isFullscreen ? '100vh' : 800 },
      }}
      closable={!isFullscreen}
      destroyOnClose
    >
      <div
        ref={containerRef}
        style={{
          display: 'flex',
          flexDirection: 'column',
          height: '100%',
          background: '#000',
        }}
      >
        <DesktopToolbar
          state={state}
          resolution={resolution}
          isFullscreen={isFullscreen}
          onResolutionChange={handleResolutionChange}
          onToggleFullscreen={handleToggleFullscreen}
          onDisconnect={handleDisconnect}
        />

        <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', overflow: 'hidden' }}>
          {state === 'connecting' && (
            <div style={{ color: '#888', fontSize: 16 }}>正在连接到 {host}:{port}...</div>
          )}
          {state === 'connected' && (
            <canvas
              ref={canvasRef}
              style={{
                maxWidth: '100%',
                maxHeight: '100%',
                cursor: 'default',
                imageRendering: 'pixelated',
              }}
              tabIndex={0}
            />
          )}
          {state === 'disconnected' && !showCredentials && (
            <div style={{ color: '#888', fontSize: 16 }}>
              连接已断开
            </div>
          )}
        </div>
      </div>
    </Modal>
  );
};

export default DesktopModal;
