import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { message } from 'antd'
import { useSettings } from './useSettings'
import type { UserInfo } from '@/types'

// useSettings：用户信息拉取与 localStorage 降级（缺省字段/无用户名）、
// 保存设置（null 剔除、缺省回退 context 现值、失败透出）、禁用 TOTP 全分支
// （位数拦截、成功翻转、userInfo 为空跳过翻转、失败透出）

const apiMock = vi.hoisted(() => ({
  getCurrentUser: vi.fn(),
  saveSettings: vi.fn(),
  disableTOTP: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

const updateSettings = vi.hoisted(() => vi.fn())
const settingsBase = vi.hoisted(() => ({
  siteName: 'Cockpit',
  refreshInterval: 30,
  enableNotifications: true,
  theme: 'light' as const,
  compactMode: false,
  showResourceCount: true,
}))
vi.mock('@/contexts/useSettingsContext', () => ({
  useSettingsContext: () => ({ settings: settingsBase, updateSettings }),
}))

const msgSuccess = vi.spyOn(message, 'success')
const msgError = vi.spyOn(message, 'error')
const msgWarning = vi.spyOn(message, 'warning')

const me = (over: Partial<UserInfo> = {}): UserInfo => ({
  id: 'u1',
  username: 'ops',
  role: 'admin',
  permissions: [],
  totp_enabled: true,
  totp_setup_at: '2026-01-01T00:00:00Z',
  ...over,
})

describe('useSettings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
  })

  it('拉取用户信息成功写入 userInfo', async () => {
    apiMock.getCurrentUser.mockResolvedValue(me())
    const { result } = renderHook(() => useSettings())
    await waitFor(() => expect(result.current.userInfo).not.toBeNull())
    expect(result.current.userInfo).toMatchObject({ id: 'u1', username: 'ops', totp_enabled: true })
  })

  it('拉取失败降级 localStorage：缺 userId/role 用默认值', async () => {
    apiMock.getCurrentUser.mockRejectedValue(new Error('net down'))
    localStorage.setItem('username', 'fallback-user')
    const { result } = renderHook(() => useSettings())
    await waitFor(() => expect(result.current.userInfo).not.toBeNull())
    expect(result.current.userInfo).toEqual({
      id: '',
      username: 'fallback-user',
      role: 'user',
      permissions: [],
      totp_enabled: false,
    })
  })

  it('拉取失败降级 localStorage：userId/role 存在则取用', async () => {
    apiMock.getCurrentUser.mockRejectedValue(new Error('net down'))
    localStorage.setItem('username', 'fallback-user')
    localStorage.setItem('userId', 'u9')
    localStorage.setItem('role', 'admin')
    const { result } = renderHook(() => useSettings())
    await waitFor(() => expect(result.current.userInfo).not.toBeNull())
    expect(result.current.userInfo).toMatchObject({ id: 'u9', role: 'admin' })
  })

  it('拉取失败且 localStorage 无用户名时 userInfo 保持 null', async () => {
    apiMock.getCurrentUser.mockRejectedValue(new Error('net down'))
    const { result } = renderHook(() => useSettings())
    await waitFor(() => expect(apiMock.getCurrentUser).toHaveBeenCalled())
    expect(result.current.userInfo).toBeNull()
  })

  it('保存设置：refreshInterval 为 null 剔除，缺省字段回退 context 现值', async () => {
    apiMock.saveSettings.mockResolvedValue({})
    const { result } = renderHook(() => useSettings())
    await act(async () => {
      await result.current.saveSettings({ refreshInterval: null })
    })
    expect(apiMock.saveSettings).toHaveBeenCalledWith({ refreshInterval: undefined })
    expect(updateSettings).toHaveBeenCalledWith({
      siteName: 'Cockpit',
      refreshInterval: 30,
      enableNotifications: true,
      theme: 'light',
      compactMode: false,
      showResourceCount: true,
    })
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('设置已保存'))
    expect(result.current.loading).toBe(false)
  })

  it('保存设置：全量字段直传并联动 context', async () => {
    apiMock.saveSettings.mockResolvedValue({})
    const { result } = renderHook(() => useSettings())
    await act(async () => {
      await result.current.saveSettings({
        siteName: 'My Panel',
        refreshInterval: 45,
        enableNotifications: false,
        theme: 'dark',
        compactMode: true,
        showResourceCount: false,
      })
    })
    expect(apiMock.saveSettings).toHaveBeenCalledWith({
      siteName: 'My Panel',
      refreshInterval: 45,
      enableNotifications: false,
      theme: 'dark',
      compactMode: true,
      showResourceCount: false,
    })
    expect(updateSettings).toHaveBeenCalledWith({
      siteName: 'My Panel',
      refreshInterval: 45,
      enableNotifications: false,
      theme: 'dark',
      compactMode: true,
      showResourceCount: false,
    })
  })

  it('保存失败透出后端 error 文案并复位 loading', async () => {
    apiMock.saveSettings.mockRejectedValue({ response: { data: { error: '配额不足' } } })
    const { result } = renderHook(() => useSettings())
    await act(async () => {
      await result.current.saveSettings({ siteName: 'X' })
    })
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('配额不足'))
    expect(result.current.loading).toBe(false)
  })

  it('禁用 TOTP：空码与不足 6 位均被拦截', async () => {
    apiMock.getCurrentUser.mockResolvedValue(me())
    const { result } = renderHook(() => useSettings())
    await act(async () => {
      await result.current.disableTOTP()
    })
    await waitFor(() => expect(msgWarning).toHaveBeenCalledWith('请输入 6 位验证码'))
    act(() => {
      result.current.setTotpVerifyCode('123')
    })
    await act(async () => {
      await result.current.disableTOTP()
    })
    await waitFor(() => expect(msgWarning).toHaveBeenCalledTimes(2))
    expect(apiMock.disableTOTP).not.toHaveBeenCalled()
  })

  it('禁用 TOTP 成功：关 Modal、清空验证码并翻转 userInfo', async () => {
    apiMock.getCurrentUser.mockResolvedValue(me())
    apiMock.disableTOTP.mockResolvedValue({})
    const { result } = renderHook(() => useSettings())
    await waitFor(() => expect(result.current.userInfo).not.toBeNull())
    act(() => {
      result.current.setShowDisableModal(true)
      result.current.setTotpVerifyCode('124567')
    })
    await act(async () => {
      await result.current.disableTOTP()
    })
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('TOTP 已禁用'))
    expect(apiMock.disableTOTP).toHaveBeenCalledWith('124567')
    expect(result.current.showDisableModal).toBe(false)
    expect(result.current.totpVerifyCode).toBe('')
    expect(result.current.userInfo).toMatchObject({ totp_enabled: false, totp_setup_at: undefined })
    expect(result.current.disablingTOTP).toBe(false)
  })

  it('禁用 TOTP 成功但 userInfo 为空时跳过翻转', async () => {
    apiMock.getCurrentUser.mockRejectedValue(new Error('net down'))
    apiMock.disableTOTP.mockResolvedValue({})
    const { result } = renderHook(() => useSettings())
    await waitFor(() => expect(apiMock.getCurrentUser).toHaveBeenCalled())
    expect(result.current.userInfo).toBeNull()
    act(() => {
      result.current.setTotpVerifyCode('124567')
    })
    await act(async () => {
      await result.current.disableTOTP()
    })
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('TOTP 已禁用'))
    expect(result.current.userInfo).toBeNull()
    expect(result.current.disablingTOTP).toBe(false)
  })

  it('禁用 TOTP 失败透出错误文案并复位 disablingTOTP', async () => {
    apiMock.getCurrentUser.mockResolvedValue(me())
    apiMock.disableTOTP.mockRejectedValue({ response: { data: { error: '验证码错误' } } })
    const { result } = renderHook(() => useSettings())
    await waitFor(() => expect(result.current.userInfo).not.toBeNull())
    act(() => {
      result.current.setShowDisableModal(true)
      result.current.setTotpVerifyCode('000000')
    })
    await act(async () => {
      await result.current.disableTOTP()
    })
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('验证码错误'))
    expect(result.current.showDisableModal).toBe(true)
    expect(result.current.disablingTOTP).toBe(false)
  })
})
