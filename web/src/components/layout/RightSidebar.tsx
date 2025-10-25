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
}

export const RightSidebar: React.FC<RightSidebarProps> = ({
  isOpen,
  onClose,
  title,
  children,
  className,
  backgroundStyle
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
        className={classNames(
          'fixed top-0 right-0 h-full w-[480px] bg-white shadow-xl transform transition-transform duration-300 ease-in-out z-40 border-l border-gray-200 flex flex-col overflow-hidden',
          isOpen ? 'translate-x-0' : 'translate-x-full',
          className || ''
        )}
        style={backgroundStyle}
      >
        {/* Header */}
        <div className="flex-shrink-0 bg-white px-6 py-4 flex items-center justify-between z-10 border-b border-gray-200">
          {title && (
            <h2 className="text-lg font-semibold text-gray-900 truncate">{title}</h2>
          )}
          <button
            onClick={onClose}
            className="p-2 rounded-md text-gray-400 hover:text-gray-600 hover:bg-gray-100 transition-colors"
            title="Close sidebar"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-scroll min-h-0 relative">
          {/* Background overlay for flight wants */}
          {backgroundStyle && (
            <div
              className="absolute inset-0 pointer-events-none"
              style={backgroundStyle}
            />
          )}
          <div className="relative z-10 bg-white bg-opacity-70">
            {children}
          </div>
        </div>
      </div>
    </>
  );
};