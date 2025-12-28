import React, { useState, useRef, useEffect } from 'react';
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
  const [internalMinimized, setInternalMinimized] = useState(true); // Start minimized
  const sidebarRef = useRef<HTMLDivElement>(null);
  const hoverTimeoutRef = useRef<ReturnType<typeof setTimeout>>();

  const sidebarMinimized = controlledMinimized !== undefined ? controlledMinimized : internalMinimized;

  const handleMinimizeToggle = () => {
    const newValue = !sidebarMinimized;
    if (onSidebarMinimizedChange) {
      onSidebarMinimizedChange(newValue);
    } else {
      setInternalMinimized(newValue);
    }
  };

  // Handle mouse enter - expand sidebar
  const handleSidebarMouseEnter = () => {
    if (hoverTimeoutRef.current) {
      clearTimeout(hoverTimeoutRef.current);
    }
    if (onSidebarMinimizedChange) {
      onSidebarMinimizedChange(false);
    } else {
      setInternalMinimized(false);
    }
  };

  // Handle mouse leave - collapse sidebar after a delay
  const handleSidebarMouseLeave = () => {
    hoverTimeoutRef.current = setTimeout(() => {
      if (onSidebarMinimizedChange) {
        onSidebarMinimizedChange(true);
      } else {
        setInternalMinimized(true);
      }
    }, 300); // 300ms delay before auto-collapse
  };

  useEffect(() => {
    return () => {
      if (hoverTimeoutRef.current) {
        clearTimeout(hoverTimeoutRef.current);
      }
    };
  }, []);

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

      {/* Sidebar wrapper with hover handlers */}
      <div
        ref={sidebarRef}
        onMouseEnter={handleSidebarMouseEnter}
        onMouseLeave={handleSidebarMouseLeave}
      >
        <Sidebar
          isOpen={sidebarOpen}
          isMinimized={sidebarMinimized}
          onClose={() => setSidebarOpen(false)}
          onMinimizeToggle={handleMinimizeToggle}
        />
      </div>

      {/* Main content */}
      <div className={classNames(
        "flex-1 flex flex-col relative transition-all duration-300 ease-in-out min-w-0",
        sidebarMinimized ? "lg:ml-20" : "lg:ml-44"
      )}>
        <div className="flex-1 flex flex-col min-w-0">
          {children}
        </div>
      </div>
    </div>
  );
};