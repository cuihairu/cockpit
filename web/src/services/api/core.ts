// API 核心上下文：域方法经原型增强挂回 ApiService 时，方法体内通过
// `this.client` 访问共享 axios 实例（api.ts 的类实现本接口）。
import type { AxiosInstance } from 'axios'

export interface ApiCtx {
  client: AxiosInstance
  // 内部辅助：cron 域方法交叉调用（原 ApiService.cronUserQuery，private）
  cronUserQuery(user?: string): string
}
