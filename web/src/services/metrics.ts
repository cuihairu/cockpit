import axios from 'axios';
import { logger } from '@/utils/logger';

// 系统信息快照
export interface SystemInfoSnapshot {
  id: number;
  agentId: string;
  cpuUsage: number;
  cpuCores: number;
  cpuFreqMhz: number;
  memTotal: number;
  memUsed: number;
  memAvailable: number;
  memUsagePercent: number;
  diskTotal: number;
  diskUsed: number;
  diskFree: number;
  diskUsagePercent: number;
  netBytesSent: number;
  netBytesRecv: number;
  osName: string;
  osVersion: string;
  arch: string;
  uptime: number;
  hostname: string;
  load1: number;
  load5: number;
  load15: number;
  updatedAt: string;
}

// 系统指标历史记录
export interface SystemMetric {
  id: number;
  agentId: string;
  timestamp: string;
  cpuUsage: number;
  cpuCores: number;
  cpuFreqMhz: number;
  memTotal: number;
  memUsed: number;
  memAvailable: number;
  memUsagePercent: number;
  diskTotal: number;
  diskUsed: number;
  diskFree: number;
  diskUsagePercent: number;
  netBytesSent: number;
  netBytesRecv: number;
  osName: string;
  osVersion: string;
  arch: string;
  uptime: number;
  load1: number;
  load5: number;
  load15: number;
  createdAt: string;
}

// 历史指标响应
export interface MetricsHistoryResponse {
  data: SystemMetric[];
  start: number;
  end: number;
  count: number;
}

// 创建 axios 实例
const metricsClient = axios.create({
  baseURL: '/api',
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json',
  },
});

// 请求拦截器 - 添加 token
metricsClient.interceptors.request.use(
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
metricsClient.interceptors.response.use(
  (response) => response.data,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token');
      localStorage.removeItem('username');
      window.location.href = '/login';
    }
    logger.error('Metrics API Error:', error);
    return Promise.reject(error);
  }
);

// 获取所有系统信息快照
export async function getSystemSnapshots(): Promise<SystemInfoSnapshot[]> {
  const response = await metricsClient.get<unknown, { data: SystemInfoSnapshot[] }>('/metrics/snapshots');
  return response?.data || [];
}

// 获取单个 Agent 的系统信息
export async function getSystemSnapshot(agentId: string): Promise<SystemInfoSnapshot> {
  return metricsClient.get<unknown, SystemInfoSnapshot>(`/metrics/snapshot?agent_id=${agentId}`);
}

// 获取历史指标
export async function getMetricsHistory(
  agentId: string,
  params?: {
    start?: number;
    end?: number;
    limit?: number;
  },
): Promise<MetricsHistoryResponse> {
  const response = await metricsClient.get<unknown, MetricsHistoryResponse>(
    `/metrics/history?agent_id=${agentId}`,
    { params }
  );
  return {
    data: response?.data || [],
    start: response?.start || 0,
    end: response?.end || 0,
    count: response?.count || 0,
  };
}

// 格式化字节数

// 格式化百分比

// 格式化运行时间
