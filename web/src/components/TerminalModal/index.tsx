import React, { useCallback, useEffect, useRef, useState } from 'react';
import { Modal, Button, Space, Form, Input, Alert } from 'antd';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebLinksAddon } from '@xterm/addon-web-links';
import { createRemoteTicket, type RemoteProtocol } from '@/services/remote';
import '@xterm/xterm/css/xterm.css';

const CONNECTION_TIMEOUT = 30000; // 30秒超时

interface TerminalModalProps {
  visible: boolean;
  onClose: () => void;
  agentId: string;
  host: string;
  port: number;
  protocol: RemoteProtocol;
  title?: string;
  /** SSH 登录用户名（调用方已知时传入，跳过凭据表单） */
  username?: string;
  /** SSH 登录口令（仅经加密 WS 通道转发，不落盘） */
  password?: string;
}

const TerminalModal: React.FC<TerminalModalProps> = ({
  visible,
  onClose,
  agentId,
  host,
  port,
  protocol,
  title,
  username,
  password,
}) => {
  const terminalRef = useRef<HTMLDivElement>(null);
  const terminalInstanceRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);

  // SSH 需要登录凭据；调用方未提供时先出表单（telnet 等裸协议免认证）
  const needsCredentials = protocol === 'ssh' && !username;
  const [authPending, setAuthPending] = useState(needsCredentials);
  const [authError, setAuthError] = useState<string | null>(null);
  const [credentials, setCredentials] = useState({ username: username || '', password: password || '' });

  // 用 ref 保存连接参数，避免 useEffect 依赖变化导致重建
  const paramsRef = useRef({ agentId, host, port, protocol, username, password });

  useEffect(() => {
    paramsRef.current = { agentId, host, port, protocol, username: credentials.username, password: credentials.password };
  }, [agentId, host, port, protocol, credentials.username, credentials.password]);

  const connectWebSocket = useCallback(async (terminal: Terminal) => {
    const params = paramsRef.current;
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    let ticket: string;

    try {
      // 空凭据不入参：保持无认证协议（telnet 等）的调用形状不变
      const result = await createRemoteTicket({
        agentId: params.agentId,
        host: params.host,
        port: params.port,
        protocol: params.protocol,
        ...(params.username ? { username: params.username } : {}),
        ...(params.password ? { password: params.password } : {}),
      });
      ticket = result.ticket;
    } catch {
      terminal.writeln('\r\n\x1b[1;31m创建连接票据失败\x1b[0m\r\n');
      setConnected(false);
      return;
    }

    const wsUrl = `${wsProtocol}//${window.location.host}/api/remote/terminal`;
    const ws = new WebSocket(wsUrl, [ticket]);
    wsRef.current = ws;

    // 连接超时处理
    const timeoutTimer = setTimeout(() => {
      if (ws.readyState === WebSocket.CONNECTING) {
        ws.close();
        terminal.writeln('\r\n\x1b[1;31m连接超时，请检查网络或重试\x1b[0m\r\n');
      }
    }, CONNECTION_TIMEOUT);

    ws.onopen = () => {
      clearTimeout(timeoutTimer);
      setConnected(true);
      terminal.writeln('\x1b[1;32m连接成功！\x1b[0m\r\n');
    };

    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);

        switch (msg.type) {
          case 'data':
            terminal.write(msg.data);
            break;
          case 'resize':
            if (fitAddonRef.current) {
              fitAddonRef.current.fit();
            }
            break;
          case 'error':
            terminal.writeln(`\r\n\x1b[1;31m错误: ${msg.message}\x1b[0m\r\n`);
            break;
          case 'close':
            terminal.writeln('\r\n\x1b[1;33m连接已关闭\x1b[0m\r\n');
            setConnected(false);
            break;
        }
      } catch {
        terminal.writeln('\r\n\x1b[1;31m消息解析错误\x1b[0m\r\n');
      }
    };

    ws.onerror = () => {
      clearTimeout(timeoutTimer);
      terminal.writeln('\r\n\x1b[1;31m连接错误\x1b[0m\r\n');
      setConnected(false);
    };

    ws.onclose = () => {
      clearTimeout(timeoutTimer);
      setConnected(false);
    };
  }, []);

  useEffect(() => {
    if (visible && !authPending && terminalRef.current && !terminalInstanceRef.current) {
      // 创建终端实例
      const terminal = new Terminal({
        cursorBlink: true,
        fontSize: 14,
        fontFamily: 'Menlo, Monaco, "Courier New", monospace',
        theme: {
          background: '#1e1e1e',
          foreground: '#d4d4d4',
          cursor: '#ffffff',
          black: '#000000',
          red: '#cd3131',
          green: '#0dbc79',
          yellow: '#e5e510',
          blue: '#2472c8',
          magenta: '#bc3fbc',
          cyan: '#11a8cd',
          white: '#e5e5e5',
          brightBlack: '#666666',
          brightRed: '#f14c4c',
          brightGreen: '#23d18b',
          brightYellow: '#f5f543',
          brightBlue: '#3b8eea',
          brightMagenta: '#d670d6',
          brightCyan: '#29b8db',
          brightWhite: '#ffffff',
        },
      });

      const fitAddon = new FitAddon();
      const webLinksAddon = new WebLinksAddon();

      terminal.loadAddon(fitAddon);
      terminal.loadAddon(webLinksAddon);

      terminal.open(terminalRef.current);
      fitAddon.fit();

      terminalInstanceRef.current = terminal;
      fitAddonRef.current = fitAddon;

      // 欢迎消息
      terminal.writeln('\x1b[1;32mCockpit 远程终端\x1b[0m');
      terminal.writeln(`正在连接到 ${paramsRef.current.host}:${paramsRef.current.port}...\r\n`);

      // 连接 WebSocket
      void connectWebSocket(terminal);

      // 终端输入处理
      terminal.onData((data) => {
        if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
          wsRef.current.send(JSON.stringify({ type: 'input', data }));
        }
      });

      // 窗口大小变化时适配本地渲染，并同步 PTY 尺寸到远端（agent 调 WindowChange）
      const resizeObserver = new ResizeObserver(() => {
        fitAddon.fit();
        const ws = wsRef.current;
        if (ws && ws.readyState === WebSocket.OPEN) {
          ws.send(JSON.stringify({ type: 'resize', rows: terminal.rows, cols: terminal.cols }));
        }
      });
      resizeObserver.observe(terminalRef.current);

      return () => {
        resizeObserver.disconnect();
        if (wsRef.current) {
          wsRef.current.close();
        }
        terminal.dispose();
        terminalInstanceRef.current = null;
      };
    }
  }, [connectWebSocket, visible, authPending]);

  useEffect(() => {
    if (!visible && terminalInstanceRef.current) {
      // 关闭时清理
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
      terminalInstanceRef.current.reset();
      setConnected(false);
    }
  }, [visible]);

  const handleReconnect = () => {
    if (terminalInstanceRef.current) {
      terminalInstanceRef.current.reset();
      terminalInstanceRef.current.writeln('\r\n正在重连...\r\n');
      void connectWebSocket(terminalInstanceRef.current);
    }
  };

  const handleAuthSubmit = () => {
    if (!credentials.username.trim()) {
      setAuthError('请输入用户名');
      return;
    }
    setAuthError(null);
    setAuthPending(false);
  };

  return (
    <Modal
      title={title || `${protocol.toUpperCase()} - ${host}:${port}`}
      open={visible}
      onCancel={onClose}
      width={800}
      footer={null}
      styles={{ body: { padding: 0, background: '#1e1e1e' } }}
    >
      {authPending ? (
        <div style={{ padding: 24, background: '#1e1e1e', color: '#d4d4d4' }}>
          <Alert
            type="info"
            showIcon
            style={{ marginBottom: 16 }}
            message={`登录 ${host}:${port}`}
            description="SSH 需要认证凭据；口令仅经加密通道转发到目标主机，不落盘、不进日志。"
          />
          <Form layout="vertical" onFinish={(e) => { e.preventDefault?.(); handleAuthSubmit(); }}>
            <Form.Item label="用户名" required>
              <Input
                autoFocus
                value={credentials.username}
                onChange={(e) => setCredentials((c) => ({ ...c, username: e.target.value }))}
                placeholder="root"
              />
            </Form.Item>
            <Form.Item label="口令">
              <Input.Password
                value={credentials.password}
                onChange={(e) => setCredentials((c) => ({ ...c, password: e.target.value }))}
                placeholder="登录口令"
              />
            </Form.Item>
            {authError && <Alert type="error" showIcon message={authError} style={{ marginBottom: 16 }} />}
            <Space>
              <Button type="primary" onClick={handleAuthSubmit}>
                连接
              </Button>
              <Button onClick={onClose}>取消</Button>
            </Space>
          </Form>
        </div>
      ) : (
        <div style={{ height: 500, background: '#1e1e1e' }}>
          <div
            ref={terminalRef}
            style={{
              height: '100%',
              padding: '8px',
            }}
          />
          <div
            style={{
              position: 'absolute',
              top: 60,
              right: 40,
              zIndex: 10,
            }}
          >
            <Space>
              <Button
                size="small"
                type="primary"
                ghost={!connected}
                disabled={connected}
                onClick={handleReconnect}
              >
                {connected ? '已连接' : '重连'}
              </Button>
            </Space>
          </div>
        </div>
      )}
    </Modal>
  );
};

export default TerminalModal;
