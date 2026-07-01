import { useEffect, useState } from 'react'
import { message } from 'antd'
import { api } from '@/services/api'
import type { UserInfo } from '@/types'
import { logger } from '@/utils/logger'
import { getApiErrorMessage } from '@/utils/apiError'
import { useSettingsContext } from '@/contexts/useSettingsContext'
import type { GeneralSettingsValues } from './GeneralSettings'

// 统一管理设置页：用户信息、保存设置、TOTP 禁用流程
export const useSettings = () => {
  const { settings, updateSettings } = useSettingsContext()
  const [loading, setLoading] = useState(false)
  const [userInfo, setUserInfo] = useState<UserInfo | null>(null)
  const [totpVerifyCode, setTotpVerifyCode] = useState('')
  const [disablingTOTP, setDisablingTOTP] = useState(false)
  const [showDisableModal, setShowDisableModal] = useState(false)

  // 拉取当前用户信息（含 TOTP 状态），失败时降级到 localStorage
  useEffect(() => {
    const fetchUserInfo = async () => {
      try {
        const info = await api.getCurrentUser()
        setUserInfo(info)
      } catch (err) {
        logger.error('Failed to fetch user info:', err)
        const username = localStorage.getItem('username')
        if (username) {
          setUserInfo({
            id: localStorage.getItem('userId') || '',
            username,
            role: localStorage.getItem('role') || 'user',
            totp_enabled: false,
          })
        }
      }
    }
    fetchUserInfo()
  }, [])

  const saveSettings = async (values: GeneralSettingsValues) => {
    setLoading(true)
    try {
      await api.saveSettings(values)
      updateSettings({
        siteName: values.siteName ?? settings.siteName,
        refreshInterval: values.refreshInterval ?? settings.refreshInterval,
        enableNotifications: values.enableNotifications ?? settings.enableNotifications,
        theme: values.theme ?? settings.theme,
        compactMode: values.compactMode ?? settings.compactMode,
        showResourceCount: values.showResourceCount ?? settings.showResourceCount,
      })
      message.success('设置已保存')
    } catch (err) {
      const errorMsg = getApiErrorMessage(err, '保存设置失败')
      message.error(errorMsg)
    } finally {
      setLoading(false)
    }
  }

  // 提交禁用 TOTP：要求 6 位验证码
  const disableTOTP = async () => {
    if (!totpVerifyCode || totpVerifyCode.length !== 6) {
      message.warning('请输入 6 位验证码')
      return
    }

    setDisablingTOTP(true)
    try {
      await api.disableTOTP(totpVerifyCode)
      message.success('TOTP 已禁用')
      setShowDisableModal(false)
      setTotpVerifyCode('')
      if (userInfo) {
        setUserInfo({ ...userInfo, totp_enabled: false, totp_setup_at: undefined })
      }
    } catch (err) {
      const errorMsg = getApiErrorMessage(err, '操作失败')
      message.error(errorMsg)
    } finally {
      setDisablingTOTP(false)
    }
  }

  return {
    loading,
    settings,
    userInfo,
    totpVerifyCode,
    disablingTOTP,
    showDisableModal,
    setTotpVerifyCode,
    setShowDisableModal,
    saveSettings,
    disableTOTP,
  }
}
