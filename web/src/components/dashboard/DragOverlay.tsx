import React, { useEffect, useState } from 'react';
import { Zap, Package } from 'lucide-react';
import { useWantStore } from '@/stores/wantStore';
import { classNames } from '@/utils/helpers';

export const DragOverlay: React.FC = () => {
  const { wants, draggingWant, draggingTemplate, isOverTarget } = useWantStore();
  const [mousePos, setMousePos] = useState({ x: 0, y: 0 });

  const want = wants.find(w => (w.metadata?.id === draggingWant) || (w.id === draggingWant));
  const isTemplateMode = !!draggingTemplate && !draggingWant;

  useEffect(() => {
    const handleDragOver = (e: DragEvent) => {
      // Use clientX/Y from the dragover event
      setMousePos({ x: e.clientX, y: e.clientY });
    };

    if (draggingWant || draggingTemplate) {
      window.addEventListener('dragover', handleDragOver);
    }

    return () => {
      window.removeEventListener('dragover', handleDragOver);
    };
  }, [draggingWant, draggingTemplate]);

  // Don't show overlay if nothing is being dragged
  if ((!draggingWant && !draggingTemplate) || (draggingWant && !want)) return null;

  // Template drag overlay
  if (isTemplateMode) {
    const icon = draggingTemplate.type === 'want-type'
      ? <Zap className="w-5 h-5 text-blue-500" />
      : <Package className="w-5 h-5 text-green-500" />;

    const borderColor = draggingTemplate.type === 'want-type' ? 'border-blue-500' : 'border-green-500';

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
          `bg-white rounded-lg shadow-2xl border-2 ${borderColor} p-4 w-48 overflow-hidden transition-all duration-300`,
          isOverTarget ? "opacity-90" : "opacity-100"
        )}>
          <div className="flex items-center gap-2 mb-2">
            {icon}
            <h4 className="text-sm font-bold text-gray-900 truncate">
              {draggingTemplate.name}
            </h4>
          </div>
          <p className="text-xs text-gray-500">
            {draggingTemplate.type === 'want-type' ? 'Want Type' : 'Recipe'}
          </p>
          <p className="text-xs text-gray-400 mt-1">Drop to create</p>
        </div>
      </div>
    );
  }

  // Want drag overlay (original behavior)
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
