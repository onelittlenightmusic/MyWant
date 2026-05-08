import React, { useState, useRef, useEffect } from 'react';
import { createPortal } from 'react-dom';

interface TooltipProps {
  label: string;
  shortcut?: string;
  /** Place the tooltip below the element instead of above. */
  below?: boolean;
  /** Programmatically force the tooltip visible (e.g. keyboard/gamepad focus). */
  forceVisible?: boolean;
  children: React.ReactElement;
}

export const Tooltip: React.FC<TooltipProps> = ({ label, shortcut, below = false, forceVisible = false, children }) => {
  const [hovered, setHovered] = useState(false);
  const [pos, setPos] = useState({ top: 0, left: 0 });
  const ref = useRef<HTMLDivElement>(null);

  const calcPos = () => {
    if (!ref.current) return;
    const rect = ref.current.getBoundingClientRect();
    setPos({
      top: below
        ? rect.bottom + window.scrollY + 8
        : rect.top   + window.scrollY - 8,
      left: rect.left + window.scrollX + rect.width / 2,
    });
  };

  const show = () => { calcPos(); setHovered(true); };
  const hide = () => setHovered(false);

  // Recalculate position whenever forceVisible flips on.
  useEffect(() => { if (forceVisible) calcPos(); }, [forceVisible]);

  const visible = hovered || forceVisible;

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
              transform: below ? 'translate(-50%, 0%)' : 'translate(-50%, -100%)',
              pointerEvents: 'none',
              zIndex: 9999,
            }}
          >
            {!below && (
              <div className="bg-gray-800 dark:bg-gray-100 text-white dark:text-gray-900 text-xs font-medium px-2 py-1 rounded shadow-lg whitespace-nowrap inline-flex items-center gap-1.5">
                {label}
                {shortcut && (
                  <kbd className="bg-gray-600 dark:bg-gray-300 text-gray-200 dark:text-gray-700 text-xs px-1 py-0.5 rounded font-mono leading-none">
                    {shortcut}
                  </kbd>
                )}
              </div>
            )}
            {!below && (
              <div className="w-0 h-0 mx-auto border-l-4 border-r-4 border-t-4 border-l-transparent border-r-transparent border-t-gray-800 dark:border-t-gray-100" />
            )}
            {below && (
              <div className="w-0 h-0 mx-auto border-l-4 border-r-4 border-b-4 border-l-transparent border-r-transparent border-b-gray-800 dark:border-b-gray-100" />
            )}
            {below && (
              <div className="bg-gray-800 dark:bg-gray-100 text-white dark:text-gray-900 text-xs font-medium px-2 py-1 rounded shadow-lg whitespace-nowrap inline-flex items-center gap-1.5">
                {label}
                {shortcut && (
                  <kbd className="bg-gray-600 dark:bg-gray-300 text-gray-200 dark:text-gray-700 text-xs px-1 py-0.5 rounded font-mono leading-none">
                    {shortcut}
                  </kbd>
                )}
              </div>
            )}
          </div>,
          document.body
        )}
    </div>
  );
};
