import React, { useRef } from 'react';
import { useWantStore } from '@/stores/wantStore';

interface UseLongPressOptions {
  disabled?: boolean;
}

export function useLongPress(id: string | null, { disabled = false }: UseLongPressOptions = {}) {
  const { setQuickActionsWantId } = useWantStore();
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const posRef = useRef<{ x: number; y: number } | null>(null);

  const start = (x: number, y: number) => {
    if (disabled) return;
    posRef.current = { x, y };
    timerRef.current = setTimeout(() => {
      if (posRef.current) {
        setQuickActionsWantId(id);
        timerRef.current = null;
      }
    }, 600);
  };

  const cancel = () => {
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    posRef.current = null;
  };

  const checkMove = (x: number, y: number) => {
    if (posRef.current) {
      const dist = Math.sqrt(
        Math.pow(x - posRef.current.x, 2) + Math.pow(y - posRef.current.y, 2)
      );
      if (dist > 10) cancel();
    }
  };

  return {
    onMouseDown: (e: React.MouseEvent) => { if (e.button !== 0) return; start(e.clientX, e.clientY); },
    onMouseMove: (e: React.MouseEvent) => checkMove(e.clientX, e.clientY),
    onMouseUp: cancel,
    onTouchStart: (e: React.TouchEvent) => { const t = e.touches[0]; start(t.clientX, t.clientY); },
    onTouchMove: (e: React.TouchEvent) => { const t = e.touches[0]; checkMove(t.clientX, t.clientY); },
    onTouchEnd: cancel,
    cancel,
  };
}
