import React, { useState, useEffect } from 'react';
import { LucideIcon, X } from 'lucide-react';
import { classNames } from '@/utils/helpers';
import { useConfigStore } from '@/stores/configStore';

interface RightSidebarProps {
  isOpen: boolean;
  onClose: () => void;
  title?: string;
  titleIcon?: LucideIcon;
  titleIconClassName?: string;
  children: React.ReactNode;
  className?: string;
  backgroundStyle?: React.CSSProperties;
  headerActions?: React.ReactNode;
  overflowHidden?: boolean;
  instant?: boolean;
}

export const RightSidebar: React.FC<RightSidebarProps> = ({
  isOpen,
  onClose,
  title,
  titleIcon: TitleIcon,
  titleIconClassName,
  children,
  className,
  backgroundStyle,
  headerActions,
  overflowHidden = false,
  instant = false
}) => {
  const config = useConfigStore(state => state.config);
  const isBottom = config?.header_position === 'bottom';
  const [isAnyDragging, setIsAnyDragging] = useState(false);

  useEffect(() => {
    const handleDragStart = () => setIsAnyDragging(true);
    const handleDragEnd = () => setIsAnyDragging(false);

    window.addEventListener('dragstart', handleDragStart);
    window.addEventListener('dragend', handleDragEnd);

    return () => {
      window.removeEventListener('dragstart', handleDragStart);
      window.removeEventListener('dragend', handleDragEnd);
    };
  }, []);

  return (
    <>
      {/* Backdrop */}
      {isOpen && (
        <div
          className={classNames(
            "fixed inset-x-0 bottom-0 bg-gray-600 bg-opacity-50 transition-opacity z-40 lg:hidden",
            isAnyDragging ? "pointer-events-none opacity-0" : "bg-opacity-50"
          )}
          style={{
            top: isBottom ? 'env(safe-area-inset-top, 0px)' : 'calc(env(safe-area-inset-top, 0px) + var(--header-height, 0px))',
          }}
          onClick={onClose}
        />
      )}

      {/* Sidebar */}
      <div
        data-sidebar="true"
        className={classNames(
          'fixed right-0 w-full sm:w-[480px] bg-white dark:bg-gray-900 transform z-40 flex flex-col overflow-hidden',
          instant ? '' : 'transition-transform duration-100 ease-in-out',
          isOpen ? 'translate-x-0' : 'translate-x-full',
          className || ''
        )}
        style={{
          top: isBottom ? 'env(safe-area-inset-top, 0px)' : 'calc(env(safe-area-inset-top, 0px) + var(--header-height, 0px))',
          bottom: isBottom ? 'calc(env(safe-area-inset-bottom, 0px) + var(--header-height, 0px))' : 'env(safe-area-inset-bottom, 0px)',
          height: 'auto',
          boxShadow: '-4px 0 12px rgba(0,0,0,0.06)'
        }}
      >
        {/* Background image - covers entire sidebar */}
        {backgroundStyle && (
          <div
            className="absolute inset-0 w-full pointer-events-none z-0"
            style={{
              ...backgroundStyle,
              backgroundAttachment: 'fixed',
              minHeight: '100vh'
            }}
          />
        )}

        {/* Semi-transparent overlay on background image */}
        {backgroundStyle && (
          <div
            className="absolute inset-0 w-full pointer-events-none bg-white/60 dark:bg-gray-900/70"
            style={{
              minHeight: '100vh',
              zIndex: 1
            }}
          />
        )}

        {/* Header */}
        <div className={classNames(
          "flex-shrink-0 bg-white dark:bg-gray-900 px-4 py-3 flex items-center justify-between z-20 gap-4 relative",
          isBottom ? "order-last border-t border-gray-200 dark:border-gray-700" : "border-b border-gray-200 dark:border-gray-700"
        )} style={isBottom ? { paddingBottom: 'calc(0.75rem + env(safe-area-inset-bottom))' } : {}}>
          <div className="flex items-center gap-3 flex-1 min-w-0">
            {title && (
              <h2 className="text-lg sm:text-xl font-semibold text-gray-900 dark:text-white truncate flex items-center gap-2">
                {TitleIcon && <TitleIcon className={classNames("h-5 w-5 flex-shrink-0", titleIconClassName || "")} />}
                {title}
              </h2>
            )}
          </div>
          <div className="flex items-center gap-2 flex-shrink-0">
            {headerActions && (
              <div className="flex items-center gap-2">
                {headerActions}
              </div>
            )}
            <button
              onClick={onClose}
              className="p-2 rounded-md text-gray-400 hover:text-gray-600 hover:bg-gray-100 dark:hover:text-gray-300 dark:hover:bg-gray-800 transition-colors flex-shrink-0"
              title="Close sidebar"
            >
              <X className="h-5 w-5" />
            </button>
          </div>
        </div>

        {/* Content */}
        <div className={`flex-1 h-full px-0 py-0 relative z-10 ${overflowHidden ? 'overflow-hidden' : 'overflow-y-auto'}`}>
          {children}
        </div>
      </div>
    </>
  );
};