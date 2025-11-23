import React from 'react';
import { X } from 'lucide-react';
import { classNames } from '@/utils/helpers';

interface CreateSidebarProps {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  children: React.ReactNode;
  className?: string;
}

export const CreateSidebar: React.FC<CreateSidebarProps> = ({
  isOpen,
  onClose,
  title,
  children,
  className
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
          'fixed top-0 right-0 h-full w-[480px] bg-white shadow-xl transform transition-transform duration-300 ease-in-out z-40 border-l border-gray-200',
          isOpen ? 'translate-x-0' : 'translate-x-full',
          className || ''
        )}
      >
        {/* Header */}
        <div className="sticky top-0 bg-white px-8 py-6 border-b border-gray-200 z-10">
          <div className="flex items-center justify-between">
            <h2 className="text-xl font-semibold text-gray-900">{title}</h2>
            <button
              onClick={onClose}
              className="p-2 rounded-md text-gray-400 hover:text-gray-600 hover:bg-gray-100 transition-colors"
              title="Close sidebar"
            >
              <X className="h-5 w-5" />
            </button>
          </div>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto h-full px-8 py-6">
          {children}
        </div>
      </div>
    </>
  );
};