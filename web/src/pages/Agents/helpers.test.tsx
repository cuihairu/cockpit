import { describe, expect, it } from 'vitest'
import type { Agent } from '@/types'
import { getVirtDisplay, getRemoteServices, virtTypeConfig, statusConfig } from './helpers'

// helpers：虚拟化显示映射与远程服务提取（纯逻辑）

const baseAgent = (over: Partial<Agent>): Agent =>
  ({
    id: 'ag-1',
    hostname: 'h',
    ip: '1.2.3.4',
    region: 'r', zone: 'z',
    capabilities: [],
    status: 'online',
    lastSeen: '0',
    ...over,
  }) as Agent

describe('getVirtDisplay', () => {
  it('宿主机角色或 none 类型显示物理机', () => {
    expect(getVirtDisplay(baseAgent({ virtRole: 'host', virtType: 'kvm' })).label).toBe('物理机')
    expect(getVirtDisplay(baseAgent({ virtType: 'none' })).label).toBe('物理机')
  })

  it('已知虚拟化类型走映射表', () => {
    expect(getVirtDisplay(baseAgent({ virtType: 'kvm', virtRole: 'guest' }))).toMatchObject({
      label: 'KVM',
      color: 'blue',
    })
    expect(getVirtDisplay(baseAgent({ virtType: 'docker' })).label).toBe('Docker')
  })

  it('未知类型兜底「未知」', () => {
    expect(getVirtDisplay(baseAgent({ virtType: 'hyper-x' }))).toMatchObject({
      label: '未知',
      color: 'default',
    })
    expect(getVirtDisplay(baseAgent({})).label).toBe('未知')
  })
})

describe('getRemoteServices', () => {
  it('空 agent / 无能力 / 无 metadata 返回空数组', () => {
    expect(getRemoteServices(null)).toEqual([])
    expect(getRemoteServices(baseAgent({}))).toEqual([])
    expect(
      getRemoteServices(baseAgent({ capabilities: [{ type: 'remote-services' }] })),
    ).toEqual([])
    expect(
      getRemoteServices(baseAgent({ capabilities: [{ type: 'other' }] })),
    ).toEqual([])
  })

  it('提取 running 的远程服务；非 running 与未知协议被过滤', () => {
    const agent = baseAgent({
      capabilities: [
        {
          type: 'remote-services',
          metadata: {
            ssh: { host: '10.0.0.1', port: 22, name: 'SSH', running: true },
            rdp: { host: '10.0.0.1', port: 3389, name: 'RDP', running: false },
            telnet: { host: '10.0.0.1', port: 23, name: 'Tel', running: true },
            weird: { host: 'x', port: 1, name: 'w', running: true },
          },
        },
      ],
    })
    expect(getRemoteServices(agent).map((s) => s.protocol)).toEqual(['ssh', 'telnet'])
  })

  it('host 与 name 缺省回退', () => {
    const agent = baseAgent({
      capabilities: [
        {
          type: 'remote-services',
          metadata: { vnc: { port: 5900, running: true } },
        },
      ],
    })
    expect(getRemoteServices(agent)).toEqual([
      { protocol: 'vnc', host: '127.0.0.1', port: 5900, name: 'VNC Server', running: true },
    ])
  })

  it('metadata 值非对象/null/缺 running 字段均跳过', () => {
    const agent = baseAgent({
      capabilities: [
        {
          type: 'remote-services',
          metadata: {
            ssh: 'plain-string',
            rdp: null,
            vnc: { host: 'h', port: 5900 },
            telnet: { host: '10.0.0.1', port: 23, name: 'Tel', running: true },
          },
        },
      ],
    })
    expect(getRemoteServices(agent)).toEqual([
      { protocol: 'telnet', host: '10.0.0.1', port: 23, name: 'Tel', running: true },
    ])
  })
})

describe('配置表完整性', () => {
  it('virtTypeConfig 覆盖常用虚拟化；statusConfig 三态', () => {
    expect(Object.keys(virtTypeConfig)).toEqual(
      expect.arrayContaining(['kvm', 'vmware', 'docker', 'none']),
    )
    expect(statusConfig.online.text).toBe('在线')
    expect(statusConfig.offline.text).toBe('离线')
    expect(statusConfig.error.text).toBe('异常')
  })
})
