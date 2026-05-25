import { useRef, useCallback, useEffect } from 'react';
import type { ScreenUpdate, BitmapRect } from './useDesktopWS';

export function useCanvasRenderer() {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const bufferRef = useRef<ImageData | null>(null);
  const rafRef = useRef<number>(0);
  const pendingRectsRef = useRef<BitmapRect[]>([]);
  const desktopSizeRef = useRef({ width: 0, height: 0 });

  // 初始化帧缓冲
  const initBuffer = useCallback((width: number, height: number) => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    canvas.width = width;
    canvas.height = height;
    bufferRef.current = new ImageData(width, height);
    desktopSizeRef.current = { width, height };

    // 清空画布
    const ctx = canvas.getContext('2d');
    if (ctx) {
      ctx.fillStyle = '#000';
      ctx.fillRect(0, 0, width, height);
    }
  }, []);

  // 处理屏幕更新
  const handleScreenUpdate = useCallback((update: ScreenUpdate) => {
    const { width, height, rects } = update;

    // 分辨率变更时重建缓冲
    if (!bufferRef.current ||
        bufferRef.current.width !== width ||
        bufferRef.current.height !== height) {
      initBuffer(width, height);
    }

    // 追加脏矩形
    pendingRectsRef.current.push(...rects);

    // 去重 RAF
    if (!rafRef.current) {
      rafRef.current = requestAnimationFrame(flushFrame);
    }
  }, [initBuffer]);

  // 刷新帧：解码 + 绘制
  const flushFrame = useCallback(() => {
    rafRef.current = 0;

    const buffer = bufferRef.current;
    const canvas = canvasRef.current;
    if (!buffer || !canvas) return;

    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const rects = pendingRectsRef.current;
    pendingRectsRef.current = [];

    for (const rect of rects) {
      // base64 解码 RGBA 像素
      const pixels = decodeBase64(rect.data);
      if (!pixels) continue;

      const expectedSize = rect.width * rect.height * 4;
      if (pixels.length !== expectedSize) continue;

      // 写入帧缓冲
      for (let y = 0; y < rect.height; y++) {
        for (let x = 0; x < rect.width; x++) {
          const srcIdx = (y * rect.width + x) * 4;
          const dstX = rect.x + x;
          const dstY = rect.y + y;

          if (dstX < buffer.width && dstY < buffer.height) {
            const dstIdx = (dstY * buffer.width + dstX) * 4;
            buffer.data[dstIdx] = pixels[srcIdx];
            buffer.data[dstIdx + 1] = pixels[srcIdx + 1];
            buffer.data[dstIdx + 2] = pixels[srcIdx + 2];
            buffer.data[dstIdx + 3] = pixels[srcIdx + 3];
          }
        }
      }

      // 使用 putImageData 绘制脏矩形区域
      const region = new ImageData(
        buffer.data.slice(
          rect.y * buffer.width * 4 + rect.x * 4,
          (rect.y + rect.height) * buffer.width * 4 + (rect.x + rect.width) * 4
        ),
        rect.width,
        rect.height
      );
      ctx.putImageData(region, rect.x, rect.y);
    }
  }, []);

  // base64 解码（使用 atob + Uint8Array）
  function decodeBase64(data: string): Uint8Array | null {
    try {
      const binary = atob(data);
      const bytes = new Uint8Array(binary.length);
      for (let i = 0; i < binary.length; i++) {
        bytes[i] = binary.charCodeAt(i);
      }
      return bytes;
    } catch {
      return null;
    }
  }

  // 清理
  useEffect(() => {
    return () => {
      if (rafRef.current) {
        cancelAnimationFrame(rafRef.current);
      }
    };
  }, []);

  return {
    canvasRef,
    initBuffer,
    handleScreenUpdate,
    desktopSize: desktopSizeRef,
  };
}
