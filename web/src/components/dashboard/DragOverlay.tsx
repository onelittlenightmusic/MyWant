import React, { useEffect, useState } from 'react';
import { useWantStore } from '@/stores/wantStore';
import { classNames } from '@/utils/helpers';

export const DragOverlay: React.FC = () => {
  const { wants, draggingWant, isOverTarget } = useWantStore();
  const [mousePos, setMousePos] = useState({ x: 0, y: 0 });

  const want = wants.find(w => (w.metadata?.id === draggingWant) || (w.id === draggingWant));

  useEffect(() => {
    const handleDragOver = (e: DragEvent) => {
      // Use clientX/Y from the dragover event
      setMousePos({ x: e.clientX, y: e.clientY });
    };

    if (draggingWant) {
      window.addEventListener('dragover', handleDragOver);
    }

    return () => {
      window.removeEventListener('dragover', handleDragOver);
    };
  }, [draggingWant]);

  if (!draggingWant || !want) return null;

  return (
    <div
      className="fixed pointer-events-none z-[9999] transition-transform duration-200 ease-out"
      style={{
        left: mousePos.x,
        top: mousePos.y,
        transform: `translate(-50%, -50%) ${isOverTarget ? 'scale(0.6)' : 'scale(1)'}`,
      }}
    >
      <div className={classNames(
        "bg-white rounded-lg shadow-2xl border-2 border-blue-500 p-4 w-48 overflow-hidden transition-all duration-300",
        isOverTarget ? "opacity-90 border-blue-600" : "opacity-100"
      )}>
        <h4 className="text-sm font-bold text-gray-900 truncate">
          {want.metadata?.name || 'Unnamed Want'}
        </h4>
        <p className="text-xs text-gray-500">{want.metadata?.type || 'Unknown Type'}</p>
      </div>
    </div>
  );
};
