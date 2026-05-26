import { useRef, useCallback, useEffect } from 'react';
import type { ScreenUpdate, BitmapRect } from './useDesktopWS';

export function useCanvasRenderer() {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const bufferRef = useRef<ImageData | null>(null);
  const rafRef = useRef<number>(0);
  const pendingRectsRef = useRef<BitmapRect[]>([]);
  const desktopSizeRef = useRef({ width: 0, height: 0 });

  const initBuffer = useCallback((width: number, height: number) => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    canvas.width = width;
    canvas.height = height;
    bufferRef.current = new ImageData(width, height);
    desktopSizeRef.current = { width, height };

    const ctx = canvas.getContext('2d');
    if (ctx) {
      ctx.fillStyle = '#000';
      ctx.fillRect(0, 0, width, height);
    }
  }, []);

  const handleScreenUpdate = useCallback((update: ScreenUpdate) => {
    const { width, height, rects } = update;

    if (!bufferRef.current ||
        bufferRef.current.width !== width ||
        bufferRef.current.height !== height) {
      initBuffer(width, height);
    }

    pendingRectsRef.current.push(...rects);

    if (!rafRef.current) {
      rafRef.current = requestAnimationFrame(flushFrame);
    }
  }, [initBuffer]);

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
      const pixels = decodeBase64(rect.data);
      if (!pixels) continue;

      const expectedSize = rect.width * rect.height * 4;
      if (pixels.length !== expectedSize) continue;

      // 写入帧缓冲并直接用解码像素绘制
      for (let y = 0; y < rect.height; y++) {
        const srcOffset = y * rect.width * 4;
        const dstY = rect.y + y;
        if (dstY >= buffer.height) break;

        const dstOffset = (dstY * buffer.width + rect.x) * 4;
        const copyWidth = Math.min(rect.width, buffer.width - rect.x) * 4;
        if (copyWidth <= 0) continue;

        // 写帧缓冲
        buffer.data.set(
          pixels.subarray(srcOffset, srcOffset + copyWidth),
          dstOffset
        );
      }

      // 直接用解码后的像素创建 ImageData 绘制（零拷贝）
      const region = new ImageData(
        new Uint8ClampedArray(pixels.buffer, pixels.byteOffset, pixels.length),
        rect.width,
        rect.height
      );
      ctx.putImageData(region, rect.x, rect.y);
    }
  }, []);

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
