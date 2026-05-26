import { useRef, useCallback, useEffect } from 'react';

interface UseConnectionTimeoutOptions {
  timeout: number; // 超时时间（毫秒）
  onTimeout: () => void;
  enabled: boolean;
}

export function useConnectionTimeout({
  timeout,
  onTimeout,
  enabled,
}: UseConnectionTimeoutOptions) {
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const start = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current);
    }
    timerRef.current = setTimeout(() => {
      onTimeout();
      timerRef.current = null;
    }, timeout);
  }, [timeout, onTimeout]);

  const clear = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  const reset = useCallback(() => {
    clear();
    if (enabled) {
      start();
    }
  }, [clear, start, enabled]);

  useEffect(() => {
    return () => {
      clear();
    };
  }, [clear]);

  return { start, clear, reset };
}
