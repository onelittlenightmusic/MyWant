import React, { useState } from 'react';
import { Menu } from 'lucide-react';
import { classNames } from '@/utils/helpers';
import { Sidebar } from './Sidebar';

interface LayoutProps {
  children: React.ReactNode;
  sidebarMinimized?: boolean;
  onSidebarMinimizedChange?: (minimized: boolean) => void;
}

export const Layout: React.FC<LayoutProps> = ({
  children,
  sidebarMinimized: controlledMinimized,
  onSidebarMinimizedChange
}) => {
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [internalMinimized, setInternalMinimized] = useState(false);

  const sidebarMinimized = controlledMinimized !== undefined ? controlledMinimized : internalMinimized;

  const handleMinimizeToggle = () => {
    const newValue = !sidebarMinimized;
    if (onSidebarMinimizedChange) {
      onSidebarMinimizedChange(newValue);
    } else {
      setInternalMinimized(newValue);
    }
  };

  return (
    <div className="min-h-screen bg-gray-50 flex">
      {/* Mobile sidebar toggle */}
      <div className="lg:hidden fixed top-4 left-4 z-40">
        <button
          onClick={() => setSidebarOpen(true)}
          className="p-2 rounded-md bg-white shadow-md border border-gray-200 text-gray-600 hover:text-gray-900"
        >
          <Menu className="h-5 w-5" />
        </button>
      </div>

      {/* Sidebar */}
      <Sidebar
        isOpen={sidebarOpen}
        isMinimized={sidebarMinimized}
        onClose={() => setSidebarOpen(false)}
        onMinimizeToggle={handleMinimizeToggle}
      />

      {/* Main content */}
      <div className={classNames(
        "flex-1 flex flex-col relative transition-all duration-300 ease-in-out",
        sidebarMinimized ? "lg:ml-20" : "lg:ml-64"
      )}>
        {children}
      </div>
    </div>
  );
};