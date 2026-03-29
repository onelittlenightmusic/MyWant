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
  const [isMobileSheet, setIsMobileSheet] = useState(() => window.innerWidth < 640);

  useEffect(() => {
    const handler = () => setIsMobileSheet(window.innerWidth < 640);
    window.addEventListener('resize', handler);
    return () => window.removeEventListener('resize', handler);
  }, []);

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

  const backdropStyle: React.CSSProperties = {
    top: isBottom
      ? 'env(safe-area-inset-top, 0px)'
      : 'calc(env(safe-area-inset-top, 0px) + var(--header-height, 0px))',
    left: 0,
    right: 0,
    bottom: 0,
  };

  const sidebarStyle: React.CSSProperties = isMobileSheet
    ? {
        left: 0,
        right: 0,
        ...(isBottom ? { bottom: 0, top: 'auto' } : { top: 0, bottom: 'auto' }),
        height: '70vh',
        transform: isOpen ? 'translateY(0)' : (isBottom ? 'translateY(100%)' : 'translateY(-100%)'),
        transition: 'transform 280ms cubic-bezier(0.32, 0.72, 0, 1)',
        borderRadius: isBottom ? '12px 12px 0 0' : '0 0 12px 12px',
        boxShadow: isBottom ? '0 -4px 20px rgba(0,0,0,0.15)' : '0 4px 20px rgba(0,0,0,0.15)',
      }
    : {
        right: 0,
        top: isBottom
          ? 'env(safe-area-inset-top, 0px)'
          : 'calc(env(safe-area-inset-top, 0px) + var(--header-height, 0px))',
        bottom: isBottom
          ? 'calc(env(safe-area-inset-bottom, 0px) + var(--header-height, 0px))'
          : 'env(safe-area-inset-bottom, 0px)',
        height: 'auto',
        ...(instant
          ? {}
          : { transform: isOpen ? 'translateX(0)' : 'translateX(100%)', transition: 'transform 100ms ease-in-out' }),
        boxShadow: '-4px 0 12px rgba(0,0,0,0.06)',
      };

  const hiddenClass = !isMobileSheet && instant && !isOpen ? 'hidden' : '';

  return (
    <>
      {/* Backdrop */}
      {isOpen && (
        <div
          className={classNames(
            'fixed z-40 lg:hidden bg-gray-600',
            isMobileSheet ? 'bg-opacity-40' : 'bg-opacity-50',
            isAnyDragging ? 'pointer-events-none opacity-0' : ''
          )}
          style={backdropStyle}
          onClick={onClose}
        />
      )}

      {/* Sidebar */}
      <div
        data-sidebar="true"
        className={classNames(
          'fixed bg-white dark:bg-gray-900 z-40 flex flex-col overflow-hidden',
          isMobileSheet ? 'w-full' : 'w-full sm:w-[480px]',
          hiddenClass,
          className || ''
        )}
        style={sidebarStyle}
      >
        {/* Background image */}
        {backgroundStyle && (
          <div
            className="absolute inset-0 w-full pointer-events-none z-0"
            style={{ ...backgroundStyle, backgroundAttachment: 'fixed', minHeight: '100vh' }}
          />
        )}
        {backgroundStyle && (
          <div
            className="absolute inset-0 w-full pointer-events-none bg-white/60 dark:bg-gray-900/70"
            style={{ minHeight: '100vh', zIndex: 1 }}
          />
        )}

        {/* Drag handle — mobile sheet only */}
        {isMobileSheet && isBottom && (
          <div
            className="flex-shrink-0 flex justify-center pt-2 pb-1 cursor-pointer"
            onClick={onClose}
          >
            <div className="w-10 h-1 bg-gray-300 dark:bg-gray-600 rounded-full" />
          </div>
        )}

        {/* Header */}
        <div
          className={classNames(
            'flex-shrink-0 bg-white dark:bg-gray-900 px-4 py-2 flex items-stretch justify-between z-20 gap-4 relative',
            isBottom
              ? 'order-last border-t border-gray-200 dark:border-gray-700'
              : 'border-b border-gray-200 dark:border-gray-700'
          )}
          style={isBottom ? { paddingBottom: 'calc(0.5rem + env(safe-area-inset-bottom))' } : {}}
        >
          <div className="flex items-center gap-3 flex-1 min-w-0">
            {title && (
              <h2 className="text-lg sm:text-xl font-semibold text-gray-900 dark:text-white truncate flex items-center gap-2">
                {TitleIcon && <TitleIcon className={classNames('h-5 w-5 flex-shrink-0', titleIconClassName || '')} />}
                {title}
              </h2>
            )}
          </div>
          <div className="flex items-stretch gap-0 flex-shrink-0">
            {headerActions && <div className="flex items-stretch gap-0">{headerActions}</div>}
            <div className="w-px bg-gray-200 dark:bg-gray-700 self-stretch my-2 mx-2" />
            <button
              onClick={onClose}
              className="flex flex-col items-center justify-center gap-1 px-3 h-full text-gray-500 hover:text-gray-700 hover:bg-gray-100 dark:text-gray-400 dark:hover:text-white dark:hover:bg-gray-800 transition-all duration-150 flex-shrink-0 focus:outline-none"
              title="Close sidebar"
            >
              <X className="h-5 w-5" />
              <span className="text-[10px] font-bold uppercase tracking-tighter hidden sm:block">Close</span>
            </button>
          </div>
        </div>

        {/* Drag handle — mobile sheet only (Bottom handle if top sheet) */}
        {isMobileSheet && !isBottom && (
          <div
            className="flex-shrink-0 flex justify-center pt-1 pb-2 cursor-pointer"
            onClick={onClose}
          >
            <div className="w-10 h-1 bg-gray-300 dark:bg-gray-600 rounded-full" />
          </div>
        )}

        {/* Content */}
        <div className={`flex-1 h-full px-0 py-0 relative z-10 ${overflowHidden ? 'overflow-hidden' : 'overflow-y-auto'}`}>
          {children}
        </div>
      </div>
    </>
  );
};
