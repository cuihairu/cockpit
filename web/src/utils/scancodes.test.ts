import { describe, expect, it } from 'vitest'
import { codeToScanCode, getBaseScanCode, isExtendedKey, shouldPreventDefault } from './scancodes'

describe('codeToScanCode', () => {
  it('常用键映射（IBM AT Set 1）', () => {
    expect(codeToScanCode('Enter')).toBe(0x1c)
    expect(codeToScanCode('KeyA')).toBe(0x1e)
    expect(codeToScanCode('Space')).toBe(0x39)
    expect(codeToScanCode('NumpadDivide')).toBe(0x135)
  })

  it('未登记的键返回 0', () => {
    expect(codeToScanCode('MediaPlayPause')).toBe(0)
    expect(codeToScanCode('')).toBe(0)
  })
})

describe('isExtendedKey', () => {
  it('>= 0x100 为扩展键（右 Ctrl/Alt、导航、右 Win）', () => {
    expect(isExtendedKey(0x11d)).toBe(true)
    expect(isExtendedKey(0x14d)).toBe(true)
    expect(isExtendedKey(0x100)).toBe(true)
  })

  it('普通键与 0 不是扩展键', () => {
    expect(isExtendedKey(0x1d)).toBe(false)
    expect(isExtendedKey(0)).toBe(false)
  })
})

describe('getBaseScanCode', () => {
  it('去掉扩展标志位取低 8 位', () => {
    expect(getBaseScanCode(0x11d)).toBe(0x1d)
    expect(getBaseScanCode(0x14d)).toBe(0x4d)
    expect(getBaseScanCode(0x1e)).toBe(0x1e)
  })
})

describe('shouldPreventDefault', () => {
  it('Tab/F 键/退格/空格/修饰键需阻止默认行为', () => {
    expect(shouldPreventDefault('Tab')).toBe(true)
    expect(shouldPreventDefault('F5')).toBe(true)
    expect(shouldPreventDefault('Backspace')).toBe(true)
    expect(shouldPreventDefault('ControlRight')).toBe(true)
  })

  it('普通字符键不阻止', () => {
    expect(shouldPreventDefault('KeyA')).toBe(false)
    expect(shouldPreventDefault('Enter')).toBe(false)
  })
})
