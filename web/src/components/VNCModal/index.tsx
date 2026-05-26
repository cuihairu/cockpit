import React, { useState, useRef, useCallback, useEffect } from 'react';
import { Modal, Form, Input, message } from 'antd';
import RemoteToolbar, { type ConnectionState } from '../RemoteToolbar';
import RFB from '@novnc/novnc';
import { useConnectionTimeout } from '@/hooks/useConnectionTimeout';

const CONNECTION_TIMEOUT = 30000; // 30秒超时

interface VNCModalProps {
  visible: boolean;
  onClose: () => void;
  agentId: string;
  host: string;
  port: number;
  title?: string;
}

type VNCState = 'disconnected' | 'connecting' | 'connected';

const VNCModal: React.FC<VNCModalProps> = ({
  visible,
  onClose,
  agentId,
  host,
  port,
  title,
}) => {
  const [showCredentials, setShowCredentials] = useState(true);
  const [vncState, setVncState] = useState<VNCState>('disconnected');
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [desktopName, setDesktopName] = useState('');
  const containerRef = useRef<HTMLDivElement>(null);
  const vncContainerRef = useRef<HTMLDivElement>(null);
  const rfbRef = useRef<RFB | null>(null);
  const [form] = Form.useForm();

  const cleanup = useCallback(() => {
    if (rfbRef.current) {
      rfbRef.current.disconnect();
      rfbRef.current = null;
    }
    // 安全清理 noVNC 创建的 canvas
    if (vncContainerRef.current) {
      while (vncContainerRef.current.firstChild) {
        vncContainerRef.current.removeChild(vncContainerRef.current.firstChild);
      }
    }
  }, []);

  const handleTimeout = useCallback(() => {
    if (vncState === 'connecting') {
      cleanup();
      setVncState('disconnected');
      message.error('连接超时，请检查网络或重试');
    }
  }, [vncState, cleanup]);

  const { start: startTimeout, clear: clearTimeout } = useConnectionTimeout({
    timeout: CONNECTION_TIMEOUT,
    onTimeout: handleTimeout,
    enabled: vncState === 'connecting',
  });

  const handleConnect = useCallback(() => {
    form.validateFields().then((values) => {
      if (!vncContainerRef.current) return;

      cleanup();

      const token = localStorage.getItem('token');
      const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';

      const q = new URLSearchParams({
        agent_id: agentId,
        host,
        port: String(port),
        token: token || '',
        password: values.password || '',
      });

      const url = `${wsProtocol}//${window.location.host}/api/remote/vnc?${q}`;

      setVncState('connecting');
      startTimeout();

      try {
        const rfb = new RFB(vncContainerRef.current, url, {
          credentials: { password: values.password || '' },
        });

        rfb.addEventListener('connect', () => {
          clearTimeout();
          setVncState('connected');
          setShowCredentials(false);
        });

        rfb.addEventListener('disconnect', (e: Event) => {
          clearTimeout();
          const detail = (e as CustomEvent).detail;
          setVncState('disconnected');
          if (!detail?.clean) {
            message.error('VNC 连接异常断开');
          }
          rfbRef.current = null;
        });

        rfb.addEventListener('credentialsrequired', () => {
          const pwd = values.password || prompt('VNC 密码:');
          if (pwd) {
            rfb.sendCredentials({ password: pwd });
          }
        });

        rfb.addEventListener('desktopname', (e: Event) => {
          const detail = (e as CustomEvent).detail;
          setDesktopName(detail?.name || '');
        });

        rfb.scaleViewport = true;
        rfb.resizeSession = false;

        rfbRef.current = rfb;
      } catch (err) {
        clearTimeout();
        message.error(`VNC 连接失败: ${err}`);
        setVncState('disconnected');
      }
    });
  }, [agentId, host, port, form, cleanup, startTimeout, clearTimeout]);

  const handleDisconnect = useCallback(() => {
    cleanup();
    setVncState('disconnected');
    setShowCredentials(true);
    if (isFullscreen) {
      document.exitFullscreen?.();
      setIsFullscreen(false);
    }
  }, [cleanup, isFullscreen]);

  const handleClose = useCallback(() => {
    cleanup();
    setShowCredentials(true);
    setVncState('disconnected');
    setIsFullscreen(false);
    onClose();
  }, [cleanup, onClose]);

  const handleToggleFullscreen = useCallback(() => {
    if (isFullscreen) {
      document.exitFullscreen?.();
      setIsFullscreen(false);
    } else {
      containerRef.current?.requestFullscreen?.();
      setIsFullscreen(true);
    }
  }, [isFullscreen]);

  const sendCtrlAltDel = useCallback(() => {
    rfbRef.current?.sendCtrlAltDel();
  }, []);

  useEffect(() => {
    const handler = () => {
      setIsFullscreen(!!document.fullscreenElement);
    };
    document.addEventListener('fullscreenchange', handler);
    return () => document.removeEventListener('fullscreenchange', handler);
  }, []);

  useEffect(() => {
    return () => {
      cleanup();
    };
  }, [cleanup]);

  // 凭据输入
  if (showCredentials && vncState === 'disconnected') {
    return (
      <Modal
        title={title || `VNC 远程桌面 - ${host}:${port}`}
        open={visible}
        onCancel={handleClose}
        onOk={handleConnect}
        okText="连接"
        width={420}
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item label="密码" name="password">
            <Input.Password placeholder="(可选) VNC 密码" autoFocus />
          </Form.Item>
        </Form>
      </Modal>
    );
  }

  // VNC 桌面 UI
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
        <RemoteToolbar
          state={vncState as ConnectionState}
          isFullscreen={isFullscreen}
          showResolution={false}
          onToggleFullscreen={handleToggleFullscreen}
          onDisconnect={handleDisconnect}
          extraActions={[
            {
              key: 'cad',
              label: 'CAD',
              onClick: sendCtrlAltDel,
            },
          ]}
        >
          {desktopName && <span style={{ color: '#888' }}>{desktopName}</span>}
        </RemoteToolbar>

        {/* VNC 渲染区域 */}
        <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', overflow: 'hidden' }}>
          {vncState === 'connecting' && (
            <div style={{ color: '#888', fontSize: 16 }}>正在连接到 {host}:{port}...</div>
          )}
          <div
            ref={vncContainerRef}
            style={{
              width: '100%',
              height: '100%',
              display: vncState === 'connected' ? 'block' : 'none',
            }}
          />
          {vncState === 'disconnected' && !showCredentials && (
            <div style={{ color: '#888', fontSize: 16 }}>连接已断开</div>
          )}
        </div>
      </div>
    </Modal>
  );
};

export default VNCModal;
