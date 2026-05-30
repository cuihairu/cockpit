import axios from 'axios';
import { logger } from '@/utils/logger';

// 远程连接类型
export type RemoteProtocol = 'ssh' | 'rdp' | 'vnc' | 'telnet' | 'ftp';

// 远程连接配置
export interface RemoteConnection {
  id: string;
  name: string;
  agentId: string;
  protocol: RemoteProtocol;
  host: string;
  port: number;
  username?: string;
  description?: string;
  createdAt: string;
}

// 创建远程连接请求
export interface CreateRemoteConnectionRequest {
  name: string;
  agentId: string;
  protocol: RemoteProtocol;
  host: string;
  port: number;
  username?: string;
  description?: string;
}

const remoteClient = axios.create({
  baseURL: '/api/remote',
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json',
  },
});

// 请求拦截器
remoteClient.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('token');
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error) => Promise.reject(error)
);

// 响应拦截器
remoteClient.interceptors.response.use(
  (response) => response.data,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token');
      localStorage.removeItem('username');
      window.location.href = '/login';
    }
    logger.error('Remote API Error:', error);
    return Promise.reject(error);
  }
);

// 获取所有远程连接配置
export async function getRemoteConnections(agentId?: string): Promise<RemoteConnection[]> {
  const params = agentId ? { agent_id: agentId } : {};
  return remoteClient.get<any, RemoteConnection[]>('/connections', { params });
}

// 创建远程连接配置
export async function createRemoteConnection(
  data: CreateRemoteConnectionRequest
): Promise<RemoteConnection> {
  return remoteClient.post<any, RemoteConnection>('/connections', data);
}

// 更新远程连接配置
export async function updateRemoteConnection(
  id: string,
  data: Partial<CreateRemoteConnectionRequest>
): Promise<RemoteConnection> {
  return remoteClient.put<any, RemoteConnection>(`/connections/${id}`, data);
}

// 删除远程连接配置
export async function deleteRemoteConnection(id: string): Promise<void> {
  return remoteClient.delete(`/connections/${id}`);
}

// 启动远程终端会话
export async function startTerminalSession(params: {
  agentId: string;
  host: string;
  port: number;
  protocol: RemoteProtocol;
}): Promise<{ sessionId: string }> {
  return remoteClient.post<any, { sessionId: string }>('/terminal/start', params);
}
