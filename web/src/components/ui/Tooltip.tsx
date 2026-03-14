import React, { useState, useRef } from 'react';
import { createPortal } from 'react-dom';

interface TooltipProps {
  label: string;
  shortcut?: string;
  children: React.ReactElement;
}

export const Tooltip: React.FC<TooltipProps> = ({ label, shortcut, children }) => {
  const [visible, setVisible] = useState(false);
  const [pos, setPos] = useState({ top: 0, left: 0 });
  const ref = useRef<HTMLDivElement>(null);

  const show = () => {
    if (ref.current) {
      const rect = ref.current.getBoundingClientRect();
      setPos({
        top: rect.top + window.scrollY - 8,
        left: rect.left + window.scrollX + rect.width / 2,
      });
    }
    setVisible(true);
  };

  const hide = () => setVisible(false);

  return (
    <div
      ref={ref}
      className="relative inline-flex"
      onMouseEnter={show}
      onMouseLeave={hide}
      onFocus={show}
      onBlur={hide}
    >
      {children}
      {visible &&
        createPortal(
          <div
            style={{
              position: 'absolute',
              top: pos.top,
              left: pos.left,
              transform: 'translate(-50%, -100%)',
              pointerEvents: 'none',
              zIndex: 9999,
            }}
          >
            <div className="bg-gray-800 dark:bg-gray-100 text-white dark:text-gray-900 text-xs font-medium px-2 py-1 rounded shadow-lg whitespace-nowrap inline-flex items-center gap-1.5">
              {label}
              {shortcut && (
                <kbd className="bg-gray-600 dark:bg-gray-300 text-gray-200 dark:text-gray-700 text-xs px-1 py-0.5 rounded font-mono leading-none">
                  {shortcut}
                </kbd>
              )}
            </div>
            <div
              className="w-0 h-0 mx-auto border-l-4 border-r-4 border-t-4 border-l-transparent border-r-transparent border-t-gray-800 dark:border-t-gray-100"
            />
          </div>,
          document.body
        )}
    </div>
  );
};
