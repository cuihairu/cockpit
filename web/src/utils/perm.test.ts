import { describe, expect, it } from 'vitest'
import { permAll, permCovers } from './perm'

// 权限判定复刻后端隐含规则（rbac-design.md 笔 6）：write ⊇ read、
// admin ⊇ write；AND 语义；畸形权限点一律 false（fail-closed）。

describe('permCovers', () => {
  it('精确匹配', () => {
    expect(permCovers(['dns:read'], 'dns:read')).toBe(true)
    expect(permCovers(['dns:write'], 'dns:write')).toBe(true)
    expect(permCovers(['dns:read'], 'dns:write')).toBe(false)
  })

  it('隐含规则：granted 的 write/admin 覆盖低 rank 需求', () => {
    expect(permCovers(['dns:write'], 'dns:read')).toBe(true)
    expect(permCovers(['dns:admin'], 'dns:read')).toBe(true)
    expect(permCovers(['dns:admin'], 'dns:write')).toBe(true)
    expect(permCovers(['dns:read'], 'dns:admin')).toBe(false)
  })

  it('resource 不同不覆盖', () => {
    expect(permCovers(['dns:admin'], 'proxy:read')).toBe(false)
    expect(permCovers(['dns:read', 'proxy:read'], 'backup:read')).toBe(false)
  })

  it('granted 清单里的畸形条目跳过，不影响其它条目判定', () => {
    expect(permCovers(['dns', 'dns:read'], 'dns:read')).toBe(true)
    expect(permCovers(['dns:read:extra', 'backup:write'], 'backup:read')).toBe(true)
    expect(permCovers(['dns:whatever'], 'dns:read')).toBe(false)
  })

  it('need 为畸形权限点恒 false', () => {
    expect(permCovers(['dns:read'], 'dns')).toBe(false)
    expect(permCovers(['dns:read'], 'dns:read:extra')).toBe(false)
    expect(permCovers(['dns:read'], 'dns:execute')).toBe(false)
    expect(permCovers(['dns:read'], '')).toBe(false)
  })

  it('空 granted 清单恒 false', () => {
    expect(permCovers([], 'dns:read')).toBe(false)
  })
})

describe('permAll', () => {
  it('单权限点（字符串）', () => {
    expect(permAll(['dns:write'], 'dns:read')).toBe(true)
    expect(permAll(['dns:read'], 'dns:write')).toBe(false)
  })

  it('AND 语义：全部满足才通过', () => {
    const domainBinding = ['dns:write', 'proxy:write']
    expect(permAll(['dns:write', 'proxy:write'], domainBinding)).toBe(true)
    expect(permAll(['dns:admin', 'proxy:read'], domainBinding)).toBe(false)
    expect(permAll(['dns:write'], domainBinding)).toBe(false)
  })

  it('空需求数组恒 true', () => {
    expect(permAll([], [])).toBe(true)
    expect(permAll(['dns:read'], [])).toBe(true)
  })
})
