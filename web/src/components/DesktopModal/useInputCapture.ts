import { useRef, useCallback, useEffect } from 'react';
import { codeToScanCode, isExtendedKey, getBaseScanCode, shouldPreventDefault } from '../../utils/scancodes';

interface InputCaptureOptions {
  sendKeyboard: (scanCode: number, keyDown: boolean, extended: boolean) => void;
  sendMouse: (x: number, y: number, buttons: number, wheelDelta: number, action: string) => void;
  enabled: boolean;
}

export function useInputCapture({ sendKeyboard, sendMouse, enabled }: InputCaptureOptions) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const lastButtonsRef = useRef(0);

  const setCanvas = useCallback((canvas: HTMLCanvasElement | null) => {
    canvasRef.current = canvas;
  }, []);

  // 获取鼠标在 Canvas 上的相对坐标
  const getCanvasCoords = useCallback((e: MouseEvent): { x: number; y: number } => {
    const canvas = canvasRef.current;
    if (!canvas) return { x: 0, y: 0 };

    const rect = canvas.getBoundingClientRect();
    const scaleX = canvas.width / rect.width;
    const scaleY = canvas.height / rect.height;

    return {
      x: Math.floor((e.clientX - rect.left) * scaleX),
      y: Math.floor((e.clientY - rect.top) * scaleY),
    };
  }, []);

  // 鼠标事件处理
  const handleMouseDown = useCallback((e: MouseEvent) => {
    if (!enabled) return;
    e.preventDefault();

    const buttons = mouseEventToButtons(e);
    lastButtonsRef.current = buttons;
    const { x, y } = getCanvasCoords(e);

    // 发送单个按钮的 down 事件
    const button = mouseEventToGrdpButton(e);
    sendMouse(x, y, button, 0, 'down');
  }, [enabled, getCanvasCoords, sendMouse]);

  const handleMouseUp = useCallback((e: MouseEvent) => {
    if (!enabled) return;
    e.preventDefault();

    const button = mouseEventToGrdpButton(e);
    const { x, y } = getCanvasCoords(e);

    sendMouse(x, y, button, 0, 'up');
    lastButtonsRef.current = 0;
  }, [enabled, getCanvasCoords, sendMouse]);

  const handleMouseMove = useCallback((e: MouseEvent) => {
    if (!enabled) return;

    const { x, y } = getCanvasCoords(e);
    sendMouse(x, y, 0, 0, 'move');
  }, [enabled, getCanvasCoords, sendMouse]);

  const handleWheel = useCallback((e: WheelEvent) => {
    if (!enabled) return;
    e.preventDefault();

    const { x, y } = getCanvasCoords(e);
    // RDP 使用 WHEEL_DELTA=120 每格，browser deltaY 通常 ±100
    const delta = e.deltaY > 0 ? -1 : e.deltaY < 0 ? 1 : 0;
    if (delta !== 0) {
      sendMouse(x, y, 0, delta, 'move');
    }
  }, [enabled, getCanvasCoords, sendMouse]);

  // 键盘事件处理
  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    if (!enabled) return;
    if (shouldPreventDefault(e.code)) {
      e.preventDefault();
    }
    e.stopPropagation();

    const scanCode = codeToScanCode(e.code);
    if (scanCode === 0) return;

    const extended = isExtendedKey(scanCode);
    const baseCode = getBaseScanCode(scanCode);

    sendKeyboard(baseCode, true, extended);
  }, [enabled, sendKeyboard]);

  const handleKeyUp = useCallback((e: KeyboardEvent) => {
    if (!enabled) return;
    e.stopPropagation();

    const scanCode = codeToScanCode(e.code);
    if (scanCode === 0) return;

    const extended = isExtendedKey(scanCode);
    const baseCode = getBaseScanCode(scanCode);

    sendKeyboard(baseCode, false, extended);
  }, [enabled, sendKeyboard]);

  // 注册/注销事件监听
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || !enabled) return;

    canvas.addEventListener('mousedown', handleMouseDown);
    canvas.addEventListener('mouseup', handleMouseUp);
    canvas.addEventListener('mousemove', handleMouseMove);
    canvas.addEventListener('wheel', handleWheel, { passive: false });
    canvas.addEventListener('contextmenu', (e) => e.preventDefault());

    // 键盘事件在 window 上捕获（canvas 不可聚焦）
    window.addEventListener('keydown', handleKeyDown);
    window.addEventListener('keyup', handleKeyUp);

    return () => {
      canvas.removeEventListener('mousedown', handleMouseDown);
      canvas.removeEventListener('mouseup', handleMouseUp);
      canvas.removeEventListener('mousemove', handleMouseMove);
      canvas.removeEventListener('wheel', handleWheel);
      window.removeEventListener('keydown', handleKeyDown);
      window.removeEventListener('keyup', handleKeyUp);
    };
  }, [enabled, handleKeyDown, handleKeyUp, handleMouseDown, handleMouseUp, handleMouseMove, handleWheel]);

  return { setCanvas };
}

// MouseEvent.button -> grdp button 参数
// 0=left, 1=middle, 2=right
function mouseEventToGrdpButton(e: MouseEvent): number {
  return e.button;
}

// MouseEvent -> buttons 位标志
// 1=left, 2=right, 4=middle
function mouseEventToButtons(e: MouseEvent): number {
  return e.buttons;
}
