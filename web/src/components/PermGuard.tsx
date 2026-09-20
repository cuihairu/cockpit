import type { ReactNode } from 'react'
import { usePerm } from '@/hooks/usePerm'

interface PermGuardProps {
  /** 所需权限点；数组为 AND 语义 */
  perm: string | string[]
  /** 无权限时的替代渲染（默认不渲染——「看不到入口」验收） */
  fallback?: ReactNode
  children: ReactNode
}

// PermGuard 按权限裁剪 UI（v-perm 的 React 等价物）：写按钮/危险操作
// 包一层，无权限直接不渲染
export const PermGuard = ({ perm, fallback = null, children }: PermGuardProps) => {
  const allowed = usePerm(perm)
  return <>{allowed ? children : fallback}</>
}
