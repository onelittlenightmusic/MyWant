import React from 'react';
import { X } from 'lucide-react';
import { classNames } from '@/utils/helpers';

interface RightSidebarProps {
  isOpen: boolean;
  onClose: () => void;
  title?: string;
  children: React.ReactNode;
  className?: string;
  backgroundStyle?: React.CSSProperties;
  headerActions?: React.ReactNode;
  overflowHidden?: boolean;
}

export const RightSidebar: React.FC<RightSidebarProps> = ({
  isOpen,
  onClose,
  title,
  children,
  className,
  backgroundStyle,
  headerActions,
  overflowHidden = false
}) => {
  return (
    <>
      {/* Backdrop */}
      {isOpen && (
        <div
          className="fixed inset-0 bg-gray-600 bg-opacity-50 transition-opacity z-40 lg:hidden"
          onClick={onClose}
        />
      )}

      {/* Sidebar */}
      <div
        data-sidebar="true"
        className={classNames(
          'fixed top-0 right-0 h-full w-full sm:w-[480px] bg-white shadow-xl transform transition-transform duration-300 ease-in-out z-40 border-l border-gray-200 flex flex-col overflow-hidden',
          isOpen ? 'translate-x-0' : 'translate-x-full',
          className || ''
        )}
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

        {/* White semi-transparent overlay on background image */}
        {backgroundStyle && (
          <div
            className="absolute inset-0 w-full pointer-events-none"
            style={{
              backgroundColor: 'rgba(255, 255, 255, 0.6)',
              minHeight: '100vh',
              zIndex: 1
            }}
          />
        )}

        {/* Header */}
        <div className="flex-shrink-0 bg-white px-4 sm:px-8 py-3 sm:py-6 flex items-center justify-between z-20 border-b border-gray-200 gap-4 relative">
          <div className="flex items-center gap-3 flex-1 min-w-0">
            {title && (
              <h2 className="text-lg sm:text-xl font-semibold text-gray-900 truncate">{title}</h2>
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
              className="p-2 rounded-md text-gray-400 hover:text-gray-600 hover:bg-gray-100 transition-colors flex-shrink-0"
              title="Close sidebar"
            >
              <X className="h-5 w-5" />
            </button>
          </div>
        </div>

        {/* Content */}
        <div className={`flex-1 h-full px-4 sm:px-8 py-4 sm:py-6 relative z-10 ${overflowHidden ? 'overflow-hidden' : 'overflow-y-auto'}`}>
          {children}
        </div>
      </div>
    </>
  );
};