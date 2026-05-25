import { useRef, useCallback, useState, useEffect } from 'react';

export interface DesktopWSOptions {
  onConnected?: (width: number, height: number) => void;
  onDisconnected?: (reason: string) => void;
  onError?: (error: string) => void;
  onScreenUpdate?: (update: ScreenUpdate) => void;
  onClipboard?: (text: string) => void;
}

export interface ScreenUpdate {
  width: number;
  height: number;
  rects: BitmapRect[];
}

export interface BitmapRect {
  x: number;
  y: number;
  width: number;
  height: number;
  data: string; // base64 RGBA
}

export type ConnectionState = 'disconnected' | 'connecting' | 'connected';

export function useDesktopWS(options: DesktopWSOptions) {
  const wsRef = useRef<WebSocket | null>(null);
  const [state, setState] = useState<ConnectionState>('disconnected');
  const optionsRef = useRef(options);
  optionsRef.current = options;

  const connect = useCallback((params: {
    agentId: string;
    host: string;
    port: number;
    username: string;
    password: string;
    domain?: string;
    width?: number;
    height?: number;
  }) => {
    const token = localStorage.getItem('token');
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';

    const q = new URLSearchParams({
      agent_id: params.agentId,
      host: params.host,
      port: String(params.port),
      token: token || '',
      username: params.username,
      password: params.password,
      domain: params.domain || '',
      width: String(params.width || 1280),
      height: String(params.height || 800),
    });

    const url = `${wsProtocol}//${window.location.host}/api/remote/desktop?${q}`;
    const ws = new WebSocket(url);
    wsRef.current = ws;
    setState('connecting');

    ws.binaryType = 'arraybuffer';

    ws.onopen = () => {
      setState('connecting');
    };

    ws.onmessage = (event) => {
      const msg = JSON.parse(event.data);
      handleMessage(msg);
    };

    ws.onerror = () => {
      setState('disconnected');
      optionsRef.current.onError?.('Connection error');
    };

    ws.onclose = () => {
      setState('disconnected');
    };
  }, []);

  const handleMessage = useCallback((msg: Record<string, unknown>) => {
    const type = msg.type as string;

    switch (type) {
      case 'connecting':
        break;

      case 'connected':
        setState('connected');
        optionsRef.current.onConnected?.(
          (msg.width as number) || 1280,
          (msg.height as number) || 800
        );
        break;

      case 'screen_update':
        optionsRef.current.onScreenUpdate?.({
          width: msg.width as number,
          height: msg.height as number,
          rects: msg.rects as BitmapRect[],
        });
        break;

      case 'disconnected':
        setState('disconnected');
        optionsRef.current.onDisconnected?.((msg.reason as string) || 'Disconnected');
        break;

      case 'error':
        optionsRef.current.onError?.((msg.error as string) || 'Unknown error');
        break;

      case 'clipboard_data':
        optionsRef.current.onClipboard?.(msg.text as string);
        break;

      case 'ping':
        break;
    }
  }, []);

  const sendKeyboard = useCallback((scanCode: number, keyDown: boolean, extended: boolean) => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;

    ws.send(JSON.stringify({
      type: 'keyboard',
      scanCode,
      keyDown,
      extended,
    }));
  }, []);

  const sendMouse = useCallback((x: number, y: number, buttons: number, wheelDelta: number, action: string) => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;

    ws.send(JSON.stringify({
      type: 'mouse',
      x, y, buttons, wheelDelta, action,
    }));
  }, []);

  const sendClipboard = useCallback((text: string) => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;

    ws.send(JSON.stringify({
      type: 'clipboard',
      text,
    }));
  }, []);

  const sendSetResolution = useCallback((width: number, height: number) => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;

    ws.send(JSON.stringify({
      type: 'set_resolution',
      width, height,
    }));
  }, []);

  const disconnect = useCallback(() => {
    wsRef.current?.close();
    wsRef.current = null;
    setState('disconnected');
  }, []);

  // 组件卸载时断开
  useEffect(() => {
    return () => {
      wsRef.current?.close();
    };
  }, []);

  return {
    state,
    connect,
    disconnect,
    sendKeyboard,
    sendMouse,
    sendClipboard,
    sendSetResolution,
  };
}
