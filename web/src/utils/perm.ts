// perm.ts 前端权限判定（rbac-design.md P1 笔 6）：复刻后端 roleCovers
// 隐含规则——同 resource 下 write ⊇ read、admin ⊇ write。判定一律基于
// /api/me 返回的 permissions（D7 语义：动态拉取，角色变更刷新即生效），
// 不再用 role 字符串推断（幽灵角色在后端即 fail-closed 空清单）。

const actionRank: Record<string, number> = {
  read: 1,
  write: 2,
  admin: 3,
}

// permCovers 单个权限点判定：granted 清单是否覆盖 need
export const permCovers = (granted: string[], need: string): boolean => {
  const needParts = need.split(':')
  if (needParts.length !== 2) return false
  const [needRes, needAct] = needParts
  const needRank = actionRank[needAct]
  if (!needRank) return false
  return granted.some((p) => {
    const parts = p.split(':')
    if (parts.length !== 2) return false
    const [res, act] = parts
    return res === needRes && actionRank[act] >= needRank
  })
}

// permAll AND 语义（全部满足才算有权限，如域名绑定写 = dns:write+proxy:write）
export const permAll = (granted: string[], need: string | string[]): boolean => {
  const needs = Array.isArray(need) ? need : [need]
  return needs.every((n) => permCovers(granted, n))
}
