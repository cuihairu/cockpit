import { useState, useEffect } from 'react'
import { Badge, Dropdown, List, Avatar, Tag, Empty, Spin, Tooltip } from 'antd'
import {
  BellOutlined,
  CheckCircleOutlined,
  WarningOutlined,
  CloseCircleOutlined,
  InfoCircleOutlined,
} from '@ant-design/icons'
import type { MenuProps } from 'antd'
import { api } from '@/services/api'
import { logger } from '@/utils/logger'

export interface Alert {
  id: string
  type: 'info' | 'warning' | 'error' | 'success'
  title: string
  message: string
  resource_id?: string
  resource_type?: string
  created_at: string
  read: boolean
}

const NotificationDropdown = () => {
  const [alerts, setAlerts] = useState<Alert[]>([])
  const [loading, setLoading] = useState(false)
  const [open, setOpen] = useState(false)

  const fetchAlerts = async () => {
    setLoading(true)
    try {
      const res = await api.getAlerts()
      setAlerts(res.data || [])
    } catch (error) {
      logger.error('Failed to fetch alerts:', error)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (open) {
      fetchAlerts()
    }
  }, [open])

  const unreadCount = alerts.filter((a) => !a.read).length

  const handleMarkAsRead = async (alertId: string) => {
    try {
      await api.markAlertRead(alertId)
      setAlerts(alerts.map((a) => (a.id === alertId ? { ...a, read: true } : a)))
    } catch (error) {
      logger.error('Failed to mark alert as read:', error)
    }
  }

  const handleMarkAllAsRead = async () => {
    try {
      await api.markAllAlertsRead()
      setAlerts(alerts.map((a) => ({ ...a, read: true })))
    } catch (error) {
      logger.error('Failed to mark all alerts as read:', error)
    }
  }

  const handleClearAll = () => {
    setAlerts([])
  }

  const getIcon = (type: Alert['type']) => {
    switch (type) {
      case 'success':
        return <CheckCircleOutlined style={{ color: '#52c41a' }} />
      case 'warning':
        return <WarningOutlined style={{ color: '#faad14' }} />
      case 'error':
        return <CloseCircleOutlined style={{ color: '#ff4d4f' }} />
      default:
        return <InfoCircleOutlined style={{ color: '#1890ff' }} />
    }
  }

  const getTagColor = (type: Alert['type']) => {
    switch (type) {
      case 'success':
        return 'success'
      case 'warning':
        return 'warning'
      case 'error':
        return 'error'
      default:
        return 'default'
    }
  }

  const menuItems: MenuProps['items'] = [
    {
      key: 'header',
      label: (
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '8px 0' }}>
          <span style={{ fontWeight: 600 }}>通知中心</span>
          <div>
            {unreadCount > 0 && (
              <a onClick={handleMarkAllAsRead} style={{ fontSize: 12, marginRight: 8 }}>
                全部已读
              </a>
            )}
            {alerts.length > 0 && (
              <a onClick={handleClearAll} style={{ fontSize: 12 }}>
                清空
              </a>
            )}
          </div>
        </div>
      ),
    },
    {
      type: 'divider',
    },
    {
      key: 'content',
      label: (
        <div style={{ width: 360, maxHeight: 400, overflow: 'auto' }}>
          {loading ? (
            <div style={{ textAlign: 'center', padding: 20 }}>
              <Spin />
            </div>
          ) : alerts.length === 0 ? (
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description="暂无通知"
              style={{ padding: 20 }}
            />
          ) : (
            <List
              dataSource={alerts}
              renderItem={(alert) => (
                <List.Item
                  key={alert.id}
                  style={{
                    padding: '12px 8px',
                    backgroundColor: alert.read ? 'transparent' : 'rgba(24, 144, 255, 0.05)',
                    cursor: 'pointer',
                  }}
                  onClick={() => !alert.read && handleMarkAsRead(alert.id)}
                >
                  <List.Item.Meta
                    avatar={<Avatar icon={getIcon(alert.type)} size="small" />}
                    title={
                      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                        <span style={{ fontSize: 14 }}>{alert.title}</span>
                        <Tag color={getTagColor(alert.type)} style={{ fontSize: 10, margin: 0 }}>
                          {alert.type}
                        </Tag>
                        {!alert.read && <Badge status="processing" />}
                      </div>
                    }
                    description={
                      <div>
                        <div style={{ fontSize: 12, color: '#666', marginBottom: 4 }}>
                          {alert.message}
                        </div>
                        <div style={{ fontSize: 11, color: '#999' }}>
                          {new Date(alert.created_at).toLocaleString('zh-CN')}
                        </div>
                      </div>
                    }
                  />
                </List.Item>
              )}
            />
          )}
        </div>
      ),
    },
  ]

  return (
    <Dropdown menu={{ items: menuItems }} trigger={['click']} open={open} onOpenChange={setOpen}>
      <div style={{ cursor: 'pointer' }}>
        <Tooltip title="通知">
          <Badge count={unreadCount} size="small" offset={[-5, 5]}>
            <BellOutlined style={{ fontSize: 18, color: '#666' }} />
          </Badge>
        </Tooltip>
      </div>
    </Dropdown>
  )
}

export default NotificationDropdown
