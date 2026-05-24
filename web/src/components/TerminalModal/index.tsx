import React, { useEffect, useRef, useState } from 'react';
import { Modal, Button, Space, message } from 'antd';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebLinksAddon } from '@xterm/addon-web-links';
import '@xterm/xterm/css/xterm.css';

interface TerminalModalProps {
  visible: boolean;
  onClose: () => void;
  agentId: string;
  host: string;
  port: number;
  protocol: 'ssh' | 'telnet' | 'vnc';
  title?: string;
}

const TerminalModal: React.FC<TerminalModalProps> = ({
  visible,
  onClose,
  agentId,
  host,
  port,
  protocol,
  title,
}) => {
  const terminalRef = useRef<HTMLDivElement>(null);
  const terminalInstanceRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    if (visible && terminalRef.current && !terminalInstanceRef.current) {
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
      terminal.writeln(`正在连接到 ${host}:${port}...\r\n`);

      // 连接 WebSocket
      connectWebSocket(terminal);

      // 终端输入处理
      terminal.onData((data) => {
        if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
          wsRef.current.send(JSON.stringify({ type: 'input', data }));
        }
      });

      // 窗口大小变化时适配
      const resizeObserver = new ResizeObserver(() => {
        fitAddon.fit();
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
  }, [visible]);

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

  const connectWebSocket = (terminal: Terminal) => {
    const token = localStorage.getItem('token');
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${wsProtocol}//${window.location.host}/api/remote/terminal?agent_id=${agentId}&host=${host}&port=${port}&protocol=${protocol}&token=${token}`;

    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      setConnected(true);
      terminal.writeln('\x1b[1;32m连接成功！\x1b[0m\r\n');
    };

    ws.onmessage = (event) => {
      const msg = JSON.parse(event.data);

      switch (msg.type) {
        case 'data':
          terminal.write(msg.data);
          break;
        case 'resize':
          // 处理终端大小调整
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
    };

    ws.onerror = (error) => {
      terminal.writeln('\r\n\x1b[1;31m连接错误\x1b[0m\r\n');
      setConnected(false);
    };

    ws.onclose = () => {
      setConnected(false);
    };
  };

  const handleReconnect = () => {
    if (terminalInstanceRef.current) {
      terminalInstanceRef.current.reset();
      terminalInstanceRef.current.writeln('\r\n正在重连...\r\n');
      connectWebSocket(terminalInstanceRef.current);
    }
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
    </Modal>
  );
};

export default TerminalModal;
