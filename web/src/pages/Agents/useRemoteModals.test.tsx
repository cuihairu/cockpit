import { act, renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { useRemoteModals } from './useRemoteModals'

// useRemoteModals：三协议分流开关 Terminal/Desktop/VNC

describe('useRemoteModals', () => {
  it('rdp 分流 Desktop，标题带 RDP 前缀', () => {
    const { result } = renderHook(() => useRemoteModals())
    act(() => result.current.open('rdp', 'ag1', '10.0.0.1', 3389))
    expect(result.current.desktopVisible).toBe(true)
    expect(result.current.desktopConfig).toEqual({
      agentId: 'ag1',
      host: '10.0.0.1',
      port: 3389,
      title: 'RDP - 10.0.0.1:3389',
    })
    expect(result.current.terminalConfig).toBeNull()
    expect(result.current.vncConfig).toBeNull()
  })

  it('vnc 分流 VNC', () => {
    const { result } = renderHook(() => useRemoteModals())
    act(() => result.current.open('vnc', 'ag1', '10.0.0.2', 5900))
    expect(result.current.vncVisible).toBe(true)
    expect(result.current.vncConfig).toMatchObject({ port: 5900, title: 'VNC - 10.0.0.2:5900' })
  })

  it('ssh/telnet 走 Terminal，标题大写协议并携带 protocol', () => {
    const { result } = renderHook(() => useRemoteModals())
    act(() => result.current.open('ssh', 'ag1', '10.0.0.3', 22))
    expect(result.current.terminalVisible).toBe(true)
    expect(result.current.terminalConfig).toMatchObject({
      protocol: 'ssh',
      title: 'SSH - 10.0.0.3:22',
    })
    act(() => result.current.open('telnet', 'ag1', '10.0.0.3', 23))
    expect(result.current.terminalConfig).toMatchObject({ protocol: 'telnet' })
  })

  it('setVisible 可关闭而不清空目标', () => {
    const { result } = renderHook(() => useRemoteModals())
    act(() => result.current.open('rdp', 'ag1', '10.0.0.1', 3389))
    act(() => result.current.setDesktopVisible(false))
    expect(result.current.desktopVisible).toBe(false)
    expect(result.current.desktopConfig).not.toBeNull()
  })
})
