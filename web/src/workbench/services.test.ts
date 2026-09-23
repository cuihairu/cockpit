import { describe, expect, it } from 'vitest'
import type { Agent } from '@/types'
import { getRemoteServices, hasRdpClient } from './services'

// workbench getRemoteServices：remote-services 元数据提取——协议白名单/
// 非对象值过滤、运行态与端口校验、host/name 缺省回退

const baseAgent = (over: Partial<Agent> = {}): Agent =>
  ({
    id: 'ag-1',
    hostname: 'h',
    ip: '1.2.3.4',
    region: 'r',
    zone: 'z',
    capabilities: [],
    status: 'online',
    lastSeen: '0',
    ...over,
  }) as Agent

const withMeta = (metadata: Record<string, unknown> | null | undefined): Agent =>
  baseAgent({
    capabilities: [{ type: 'remote-services', metadata }] as Agent['capabilities'],
  })

describe('getRemoteServices', () => {
  it('agent 为空 / 无 capabilities / 无 remote-services 能力返回空数组', () => {
    expect(getRemoteServices(null)).toEqual([])
    expect(getRemoteServices(baseAgent({ capabilities: undefined as never }))).toEqual([])
    expect(getRemoteServices(baseAgent())).toEqual([])
    expect(
      getRemoteServices(baseAgent({ capabilities: [{ type: 'other' }] as Agent['capabilities'] })),
    ).toEqual([])
  })

  it('remote-services 能力缺 metadata（undefined/null）返回空数组', () => {
    expect(getRemoteServices(withMeta(undefined))).toEqual([])
    expect(getRemoteServices(withMeta(null))).toEqual([])
  })

  it('空 metadata 对象返回空数组', () => {
    expect(getRemoteServices(withMeta({}))).toEqual([])
  })

  it('仅提取白名单协议且 running+port 齐全的服务，host/name 缺省回退', () => {
    const agent = withMeta({
      ssh: { host: '10.0.0.1', port: 22, name: 'SSH', running: true },
      rdp: { port: 3389, running: true },
    })
    expect(getRemoteServices(agent)).toEqual([
      { protocol: 'ssh', host: '10.0.0.1', port: 22, name: 'SSH', running: true },
      { protocol: 'rdp', host: '127.0.0.1', port: 3389, name: 'RDP Server', running: true },
    ])
  })

  it('非 running / 无 port 的条目被过滤', () => {
    const agent = withMeta({
      vnc: { port: 5900, running: false },
      telnet: { running: true },
    })
    expect(getRemoteServices(agent)).toEqual([])
  })

  it('非白名单键、非对象值与 null 值被过滤', () => {
    const agent = withMeta({
      http: { port: 80, running: true },
      ftp: 'plain string',
      ssh: null,
    })
    expect(getRemoteServices(agent)).toEqual([])
  })
})

describe('hasRdpClient', () => {
  it('agent 为空 / 无 capabilities / 无 rdp-client 返回 false', () => {
    expect(hasRdpClient(null)).toBe(false)
    expect(hasRdpClient(baseAgent({ capabilities: undefined as never }))).toBe(false)
    expect(hasRdpClient(baseAgent())).toBe(false)
  })

  it('有 rdp-client capability 返回 true', () => {
    const agent = baseAgent({
      capabilities: [{ type: 'rdp-client' }] as Agent['capabilities'],
    })
    expect(hasRdpClient(agent)).toBe(true)
  })
})
