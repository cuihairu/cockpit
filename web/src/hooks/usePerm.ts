import { permAll } from '@/utils/perm'
import { useUser } from '@/contexts/useUser'

// usePerm 权限判定 hook（rbac-design.md P1 笔 6）：need 为单权限点或
// AND 数组（如 ['dns:write','proxy:write']）。permissions 未加载
// （undefined）时恒 false——fail-closed，与后端 RBAC 语义一致。
export const usePerm = (need: string | string[]): boolean => {
  const { user } = useUser()
  const granted = user?.permissions
  if (!granted) return false
  return permAll(granted, need)
}
