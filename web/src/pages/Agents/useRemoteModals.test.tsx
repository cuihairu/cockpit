import { act, renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { useRemoteModals } from './useRemoteModals'

// useRemoteModals：协议分流开关——RDP/VNC/SSH 走 GuacamoleModal（三协议统一
// 第三方栈，docs/remote-access-integration-design.md D3），telnet 走 TerminalModal

describe('useRemoteModals', () => {
  it('rdp 分流 Guacamole，标题带 RDP 前缀', () => {
    const { result } = renderHook(() => useRemoteModals())
    act(() => result.current.open('rdp', 'ag1', '10.0.0.1', 3389))
    expect(result.current.guacVisible).toBe(true)
    expect(result.current.guacConfig).toEqual({
      agentId: 'ag1',
      host: '10.0.0.1',
      port: 3389,
      protocol: 'rdp',
      title: 'RDP - 10.0.0.1:3389',
    })
    expect(result.current.terminalConfig).toBeNull()
  })

  it('vnc 分流 Guacamole，标题带 VNC 前缀', () => {
    const { result } = renderHook(() => useRemoteModals())
    act(() => result.current.open('vnc', 'ag1', '10.0.0.2', 5900))
    expect(result.current.guacVisible).toBe(true)
    expect(result.current.guacConfig).toMatchObject({
      protocol: 'vnc',
      port: 5900,
      title: 'VNC - 10.0.0.2:5900',
    })
  })

  it('ssh 分流 Guacamole（三协议统一栈），telnet 走 Terminal', () => {
    const { result } = renderHook(() => useRemoteModals())
    act(() => result.current.open('ssh', 'ag1', '10.0.0.3', 22))
    expect(result.current.guacVisible).toBe(true)
    expect(result.current.guacConfig).toMatchObject({
      protocol: 'ssh',
      port: 22,
      title: 'SSH - 10.0.0.3:22',
    })
    expect(result.current.terminalConfig).toBeNull()
    act(() => result.current.open('telnet', 'ag1', '10.0.0.3', 23))
    expect(result.current.terminalVisible).toBe(true)
    expect(result.current.terminalConfig).toMatchObject({ protocol: 'telnet' })
  })

  it('setVisible 可关闭而不清空目标', () => {
    const { result } = renderHook(() => useRemoteModals())
    act(() => result.current.open('rdp', 'ag1', '10.0.0.1', 3389))
    act(() => result.current.setGuacVisible(false))
    expect(result.current.guacVisible).toBe(false)
    expect(result.current.guacConfig).not.toBeNull()
  })
})
