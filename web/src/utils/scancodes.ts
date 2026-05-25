// JavaScript KeyboardEvent.code -> RDP 扫描码映射
// 参照 noVNC 和 Apache Guacamole 实现
// RDP 使用 IBM AT 扫描码 (Set 1)

const SCANCODE_MAP: Record<string, number> = {
  // 功能键
  'Backspace': 0x0E,
  'Tab': 0x0F,
  'Enter': 0x1C,
  'Escape': 0x01,
  'Space': 0x39,

  // 修饰键
  'ShiftLeft': 0x2A,
  'ShiftRight': 0x36,
  'ControlLeft': 0x1D,
  'ControlRight': 0x11D,
  'AltLeft': 0x38,
  'AltRight': 0x138,
  'MetaLeft': 0x15B,
  'MetaRight': 0x15C,
  'ContextMenu': 0x15D,
  'CapsLock': 0x3A,

  // 字母键 (QWERTY 布局)
  'KeyA': 0x1E, 'KeyB': 0x30, 'KeyC': 0x2E, 'KeyD': 0x20,
  'KeyE': 0x12, 'KeyF': 0x21, 'KeyG': 0x22, 'KeyH': 0x23,
  'KeyI': 0x17, 'KeyJ': 0x24, 'KeyK': 0x25, 'KeyL': 0x26,
  'KeyM': 0x32, 'KeyN': 0x31, 'KeyO': 0x18, 'KeyP': 0x19,
  'KeyQ': 0x10, 'KeyR': 0x13, 'KeyS': 0x1F, 'KeyT': 0x14,
  'KeyU': 0x16, 'KeyV': 0x2F, 'KeyW': 0x11, 'KeyX': 0x2D,
  'KeyY': 0x15, 'KeyZ': 0x2C,

  // 数字键（主键盘）
  'Digit0': 0x0B, 'Digit1': 0x02, 'Digit2': 0x03, 'Digit3': 0x04,
  'Digit4': 0x05, 'Digit5': 0x06, 'Digit6': 0x07, 'Digit7': 0x08,
  'Digit8': 0x09, 'Digit9': 0x0A,

  // 符号键
  'Minus': 0x0C, 'Equal': 0x0D,
  'BracketLeft': 0x1A, 'BracketRight': 0x1B,
  'Backslash': 0x2B,
  'Semicolon': 0x27, 'Quote': 0x28,
  'Backquote': 0x29,
  'Comma': 0x33, 'Period': 0x34, 'Slash': 0x35,

  // 小键盘
  'Numpad0': 0x52, 'Numpad1': 0x4F, 'Numpad2': 0x50, 'Numpad3': 0x51,
  'Numpad4': 0x4B, 'Numpad5': 0x4C, 'Numpad6': 0x4D,
  'Numpad7': 0x47, 'Numpad8': 0x48, 'Numpad9': 0x49,
  'NumpadMultiply': 0x37, 'NumpadAdd': 0x4E,
  'NumpadSubtract': 0x4A, 'NumpadDecimal': 0x53,
  'NumpadDivide': 0x135, 'NumpadEnter': 0x11C,
  'NumLock': 0x45,

  // 导航键
  'Insert': 0x152, 'Delete': 0x153,
  'Home': 0x147, 'End': 0x14F,
  'PageUp': 0x149, 'PageDown': 0x151,

  // 方向键
  'ArrowUp': 0x148, 'ArrowDown': 0x150,
  'ArrowLeft': 0x14B, 'ArrowRight': 0x14D,

  // F 键
  'F1': 0x3B, 'F2': 0x3C, 'F3': 0x3D, 'F4': 0x3E,
  'F5': 0x3F, 'F6': 0x40, 'F7': 0x41, 'F8': 0x42,
  'F9': 0x43, 'F10': 0x44, 'F11': 0x57, 'F12': 0x58,

  // 其他
  'PrintScreen': 0x137, 'ScrollLock': 0x46,
  'Pause': 0x145,
};

export function codeToScanCode(code: string): number {
  return SCANCODE_MAP[code] ?? 0;
}

// 判断是否为扩展键（扫描码 >= 0x100）
export function isExtendedKey(scanCode: number): boolean {
  return scanCode >= 0x100;
}

// 获取实际扫描码（去掉扩展标志位）
export function getBaseScanCode(scanCode: number): number {
  return scanCode & 0xFF;
}

// 需要被阻止默认行为的键（避免浏览器快捷键干扰）
export function shouldPreventDefault(code: string): boolean {
  return [
    'Tab', 'F1', 'F2', 'F3', 'F4', 'F5', 'F6',
    'F7', 'F8', 'F9', 'F10', 'F11', 'F12',
    'Backspace', 'Space',
    'ControlLeft', 'ControlRight', 'AltLeft', 'AltRight',
  ].includes(code);
}
