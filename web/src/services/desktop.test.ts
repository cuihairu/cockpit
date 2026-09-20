import { beforeEach, describe, expect, it } from 'vitest'
import {
  deleteDesktopConfig,
  getDesktopConfig,
  getDesktopConfigs,
  getRecentDesktopConfig,
  saveDesktopConfig,
} from './desktop'

// localStorage 持久化的 RDP 连接配置 CRUD（纯前端，无网络）。

const base = { name: 'n1', agentId: 'ag1', host: '192.168.1.10', port: 3389, username: 'u', domain: '', width: 1280, height: 800 }

describe('desktop configs', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('空库返回空数组；按 agentId 过滤', () => {
    expect(getDesktopConfigs()).toEqual([])
    expect(getDesktopConfigs('ag1')).toEqual([])

    saveDesktopConfig({ ...base })
    saveDesktopConfig({ ...base, agentId: 'ag2' })
    expect(getDesktopConfigs()).toHaveLength(2)
    expect(getDesktopConfigs('ag1')).toHaveLength(1)
  })

  it('保存生成 id 与 lastUsed，可按 id 取回', () => {
    const saved = saveDesktopConfig({ ...base })
    expect(saved.id).toBeTruthy()
    expect(saved.lastUsed).toBeGreaterThan(0)
    expect(getDesktopConfig(saved.id)).toEqual(saved)
    expect(getDesktopConfig('nope')).toBeUndefined()
  })

  it('同 agentId+host+port 视为更新而非新增', () => {
    const first = saveDesktopConfig({ ...base })
    const second = saveDesktopConfig({ ...base, username: 'changed' })
    expect(second.id).toBe(first.id)
    expect(getDesktopConfigs()).toHaveLength(1)
    expect(getDesktopConfig(first.id)?.username).toBe('changed')
  })

  it('getRecentDesktopConfig 按 agent+host+port 命中', () => {
    saveDesktopConfig({ ...base })
    expect(getRecentDesktopConfig('ag1', '192.168.1.10', 3389)).toBeDefined()
    expect(getRecentDesktopConfig('ag1', '192.168.1.10', 3390)).toBeUndefined()
  })

  it('删除配置', () => {
    const saved = saveDesktopConfig({ ...base })
    deleteDesktopConfig(saved.id)
    expect(getDesktopConfigs()).toEqual([])
  })

  it('localStorage 损坏数据静默回退空数组', () => {
    localStorage.setItem('desktop_configs', '{broken json')
    expect(getDesktopConfigs()).toEqual([])
  })
})
