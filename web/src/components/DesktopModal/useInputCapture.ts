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

  // 用 ref 保存最新回调，避免依赖变化导致事件监听器反复重注册
  const sendKeyboardRef = useRef(sendKeyboard);
  sendKeyboardRef.current = sendKeyboard;
  const sendMouseRef = useRef(sendMouse);
  sendMouseRef.current = sendMouse;
  const enabledRef = useRef(enabled);
  enabledRef.current = enabled;

  const setCanvas = useCallback((canvas: HTMLCanvasElement | null) => {
    canvasRef.current = canvas;
  }, []);

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

  useEffect(() => {
    const handleMouseDown = (e: MouseEvent) => {
      if (!enabledRef.current) return;
      e.preventDefault();

      const button = e.button;
      lastButtonsRef.current = mouseEventToButtons(e);
      const { x, y } = getCanvasCoords(e);

      sendMouseRef.current(x, y, button, 0, 'down');
    };

    const handleMouseUp = (e: MouseEvent) => {
      if (!enabledRef.current) return;
      e.preventDefault();

      const button = e.button;
      const { x, y } = getCanvasCoords(e);

      sendMouseRef.current(x, y, button, 0, 'up');
      lastButtonsRef.current = 0;
    };

    const handleMouseMove = (e: MouseEvent) => {
      if (!enabledRef.current) return;

      const { x, y } = getCanvasCoords(e);
      sendMouseRef.current(x, y, 0, 0, 'move');
    };

    const handleWheel = (e: WheelEvent) => {
      if (!enabledRef.current) return;
      e.preventDefault();

      const { x, y } = getCanvasCoords(e);
      const delta = e.deltaY > 0 ? -1 : e.deltaY < 0 ? 1 : 0;
      if (delta !== 0) {
        sendMouseRef.current(x, y, 0, delta, 'move');
      }
    };

    const handleContextMenu = (e: Event) => e.preventDefault();

    const handleKeyDown = (e: KeyboardEvent) => {
      if (!enabledRef.current) return;
      if (shouldPreventDefault(e.code)) {
        e.preventDefault();
      }
      e.stopPropagation();

      const scanCode = codeToScanCode(e.code);
      if (scanCode === 0) return;

      const extended = isExtendedKey(scanCode);
      const baseCode = getBaseScanCode(scanCode);

      sendKeyboardRef.current(baseCode, true, extended);
    };

    const handleKeyUp = (e: KeyboardEvent) => {
      if (!enabledRef.current) return;
      e.stopPropagation();

      const scanCode = codeToScanCode(e.code);
      if (scanCode === 0) return;

      const extended = isExtendedKey(scanCode);
      const baseCode = getBaseScanCode(scanCode);

      sendKeyboardRef.current(baseCode, false, extended);
    };

    const canvas = canvasRef.current;
    if (!canvas) return;

    canvas.addEventListener('mousedown', handleMouseDown);
    canvas.addEventListener('mouseup', handleMouseUp);
    canvas.addEventListener('mousemove', handleMouseMove);
    canvas.addEventListener('wheel', handleWheel, { passive: false });
    canvas.addEventListener('contextmenu', handleContextMenu);

    window.addEventListener('keydown', handleKeyDown);
    window.addEventListener('keyup', handleKeyUp);

    return () => {
      canvas.removeEventListener('mousedown', handleMouseDown);
      canvas.removeEventListener('mouseup', handleMouseUp);
      canvas.removeEventListener('mousemove', handleMouseMove);
      canvas.removeEventListener('wheel', handleWheel);
      canvas.removeEventListener('contextmenu', handleContextMenu);

      window.removeEventListener('keydown', handleKeyDown);
      window.removeEventListener('keyup', handleKeyUp);
    };
  }, [getCanvasCoords]); // 仅依赖稳定的 getCanvasCoords

  return { setCanvas };
}

function mouseEventToButtons(e: MouseEvent): number {
  return e.buttons;
}
