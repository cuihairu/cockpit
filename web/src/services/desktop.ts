// 桌面会话客户端服务
// 管理已保存的 RDP 连接配置（localStorage 持久化）

export interface SavedDesktopConfig {
  id: string;
  name: string;
  agentId: string;
  host: string;
  port: number;
  username: string;
  domain: string;
  width: number;
  height: number;
  lastUsed: number;
}

const STORAGE_KEY = 'desktop_configs';

function loadAll(): SavedDesktopConfig[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? JSON.parse(raw) : [];
  } catch {
    return [];
  }
}

function saveAll(configs: SavedDesktopConfig[]) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(configs));
}

function genId(): string {
  return Date.now().toString(36) + Math.random().toString(36).slice(2, 6);
}

export function getDesktopConfigs(agentId?: string): SavedDesktopConfig[] {
  const configs = loadAll();
  if (agentId) {
    return configs.filter((c) => c.agentId === agentId);
  }
  return configs;
}

export function getDesktopConfig(id: string): SavedDesktopConfig | undefined {
  return loadAll().find((c) => c.id === id);
}

export function saveDesktopConfig(
  input: Omit<SavedDesktopConfig, 'id' | 'lastUsed'>
): SavedDesktopConfig {
  const configs = loadAll();

  // 同 agentId + host + port 视为更新
  const existing = configs.find(
    (c) => c.agentId === input.agentId && c.host === input.host && c.port === input.port
  );

  if (existing) {
    Object.assign(existing, input, { lastUsed: Date.now() });
    saveAll(configs);
    return existing;
  }

  const config: SavedDesktopConfig = {
    ...input,
    id: genId(),
    lastUsed: Date.now(),
  };
  configs.push(config);
  saveAll(configs);
  return config;
}

export function deleteDesktopConfig(id: string): void {
  const configs = loadAll().filter((c) => c.id !== id);
  saveAll(configs);
}

// 获取最近使用的配置
export function getRecentDesktopConfig(
  agentId: string,
  host: string,
  port: number
): SavedDesktopConfig | undefined {
  return loadAll().find(
    (c) => c.agentId === agentId && c.host === host && c.port === port
  );
}
