import { Card, Tabs } from 'antd'
import { useSettings } from './useSettings'
import { GeneralSettings } from './GeneralSettings'
import { SecuritySettings } from './SecuritySettings'
import { AlertSettings } from './AlertSettings'
import { SystemInfo } from './SystemInfo'
import { DisableTOTPModal } from './DisableTOTPModal'

const Settings = () => {
  const {
    loading,
    userInfo,
    totpVerifyCode,
    disablingTOTP,
    showDisableModal,
    setTotpVerifyCode,
    setShowDisableModal,
    saveSettings,
    disableTOTP,
  } = useSettings()

  return (
    <div className="page-container">
      <Card title="系统设置">
        <Tabs
          items={[
            {
              key: 'general',
              label: '通用设置',
              children: <GeneralSettings loading={loading} onSave={saveSettings} />,
            },
            {
              key: 'security',
              label: '安全设置',
              children: (
                <SecuritySettings
                  userInfo={userInfo}
                  onRequestDisable={() => setShowDisableModal(true)}
                />
              ),
            },
            {
              key: 'alerts',
              label: '告警设置',
              children: <AlertSettings />,
            },
            {
              key: 'system',
              label: '系统信息',
              children: <SystemInfo />,
            },
          ]}
        />
      </Card>

      <DisableTOTPModal
        open={showDisableModal}
        verifyCode={totpVerifyCode}
        loading={disablingTOTP}
        onCodeChange={setTotpVerifyCode}
        onCancel={() => {
          setShowDisableModal(false)
          setTotpVerifyCode('')
        }}
        onConfirm={disableTOTP}
      />
    </div>
  )
}

export default Settings
