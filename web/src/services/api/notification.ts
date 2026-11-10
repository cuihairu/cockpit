import type { NotificationStatus, NotificationSendResult } from '@/types'
import type { ApiCtx } from './core'

// notification 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface NotificationMethods {
    getNotificationStatus(): Promise<NotificationStatus>
    testNotification(): Promise<{ results: NotificationSendResult[] }>
}

export const notificationMethods: NotificationMethods = {
  async getNotificationStatus(this: ApiCtx): Promise<NotificationStatus> {

    return this.client.get<unknown, NotificationStatus>('/notification/status')
  
  },

  async testNotification(this: ApiCtx): Promise<{ results: NotificationSendResult[] }> {

    return this.client.post<unknown, { results: NotificationSendResult[] }>('/notification/test')
  
  }
}
