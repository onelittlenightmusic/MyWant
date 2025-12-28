import React from 'react';
import { X, Heart, Bot, BookOpen, AlertTriangle, Activity, Zap, ChevronsLeft, ChevronsRight } from 'lucide-react';
import { classNames } from '@/utils/helpers';

interface SidebarProps {
  isOpen: boolean;
  isMinimized: boolean;
  onClose: () => void;
  onMinimizeToggle: () => void;
}

interface MenuItem {
  id: string;
  label: string;
  icon: React.ComponentType<any>;
  href: string;
  active: boolean;
  disabled?: boolean;
}

// Get current path for active state
const getCurrentPath = () => {
  if (typeof window !== 'undefined') {
    return window.location.pathname;
  }
  return '/dashboard';
};

const getMenuItems = (): MenuItem[] => {
  const currentPath = getCurrentPath();
  return [
    {
      id: 'wants',
      label: 'Wants',
      icon: Heart,
      href: '/dashboard',
      active: currentPath === '/dashboard',
      disabled: false
    },
    {
      id: 'agents',
      label: 'Agents',
      icon: Bot,
      href: '/agents',
      active: currentPath === '/agents',
      disabled: false
    }
  ];
};

const getAdvancedItems = (): MenuItem[] => {
  const currentPath = getCurrentPath();
  return [
    {
      id: 'wantTypes',
      label: 'Want Types',
      icon: Zap,
      href: '/want-types',
      active: currentPath === '/want-types',
      disabled: false
    },
    {
      id: 'recipes',
      label: 'Recipes',
      icon: BookOpen,
      href: '/recipes',
      active: currentPath === '/recipes',
      disabled: false
    },
    {
      id: 'logs',
      label: 'Logs',
      icon: Activity,
      href: '/logs',
      active: currentPath === '/logs'
    }
  ];
};

export const Sidebar: React.FC<SidebarProps> = ({
  isOpen,
  isMinimized,
  onClose,
  onMinimizeToggle
}) => {
  const menuItems = getMenuItems();

  return (
    <>
      {/* Overlay */}
      {isOpen && !isMinimized && (
        <div
          className="fixed inset-0 bg-gray-600 bg-opacity-50 z-40 lg:hidden"
          onClick={onClose}
        />
      )}

      {/* Sidebar */}
      <div className={classNames(
        'fixed inset-y-0 left-0 z-50 bg-white border-r border-gray-200 transform transition-all duration-300 ease-in-out lg:flex lg:flex-col lg:h-screen',
        isOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0',
        isMinimized ? 'w-20' : 'w-64'
      )}>
        <div className="flex flex-col h-full">
          {/* Mobile Header */}
          <div className="flex items-center justify-between px-4 py-4 border-b border-gray-200 lg:hidden">
            <h2 className="text-lg font-semibold text-gray-900">Menu</h2>
            <button
              onClick={onClose}
              className="text-gray-400 hover:text-gray-600"
            >
              <X className="h-5 w-5" />
            </button>
          </div>

          {/* Logo/Brand */}
          <div className={classNames(
            "hidden lg:block px-4 py-6 border-b border-gray-200",
            isMinimized && "px-2 text-center"
          )}>
            <h1 className={classNames(
              "text-xl font-bold text-gray-900",
              isMinimized && "text-lg"
            )}>
              {isMinimized ? "MW" : "MyWant"}
            </h1>
          </div>

          {/* Navigation */}
          <div className="flex-1 px-4 py-6 space-y-8 overflow-y-auto">
            {/* Main Menu */}
            <nav className="space-y-2">
              {menuItems.map((item) => {
                const Icon = item.icon;
                return (
                  <a
                    key={item.id}
                    href={item.href}
                    className={classNames(
                      'flex items-center px-3 py-2 rounded-md text-sm font-medium transition-colors',
                      item.active ? 'bg-primary-100 text-primary-900' : (!item.disabled ? 'text-gray-600 hover:bg-gray-100 hover:text-gray-900' : 'text-gray-400 cursor-not-allowed'),
                      isMinimized && "justify-center"
                    )}
                    onClick={(e) => item.disabled && e.preventDefault()}
                    title={isMinimized ? item.label : undefined}
                  >
                    <Icon className={classNames("h-5 w-5", !isMinimized && "mr-3")} />
                    {!isMinimized && item.label}
                    {!isMinimized && item.disabled && (
                      <span className="ml-auto text-xs text-gray-400">Soon</span>
                    )}
                  </a>
                );
              })}
            </nav>

            {/* Advanced Section */}
            <div>
              <h3 className={classNames(
                "px-3 text-xs font-semibold text-gray-500 uppercase tracking-wider mb-3",
                isMinimized && "text-center"
              )}>
                {isMinimized ? "Adv" : "Advanced"}
              </h3>
              <nav className="space-y-2">
                {getAdvancedItems().map((item) => {
                  const Icon = item.icon;
                  return (
                    <a
                      key={item.id}
                      href={item.href}
                      className={classNames(
                        'flex items-center px-3 py-2 rounded-md text-sm font-medium transition-colors',
                        item.active ? 'bg-primary-100 text-primary-900' : (!item.disabled ? 'text-gray-600 hover:bg-gray-100 hover:text-gray-900' : 'text-gray-400 cursor-not-allowed'),
                        isMinimized && "justify-center"
                      )}
                      onClick={(e) => item.disabled && e.preventDefault()}
                      title={isMinimized ? item.label : undefined}
                    >
                      <Icon className={classNames("h-5 w-5", !isMinimized && "mr-3")} />
                      {!isMinimized && item.label}
                      {!isMinimized && item.disabled && (
                        <span className="ml-auto text-xs text-gray-400">Soon</span>
                      )}
                    </a>
                  );
                })}
              </nav>
            </div>
          </div>
        </div>
      </div>

      {/* Minimize Toggle */}
      <div className="hidden lg:block fixed z-50 bottom-4 left-4">
        <button
          onClick={onMinimizeToggle}
          className="flex items-center justify-center w-12 h-12 p-2 rounded-full bg-white border border-gray-200 text-gray-500 hover:bg-gray-100 hover:text-gray-700 transition-colors shadow-md"
          title={isMinimized ? "Expand sidebar" : "Collapse sidebar"}
        >
          {isMinimized ? <ChevronsRight className="h-5 w-5" /> : <ChevronsLeft className="h-5 w-5" />}
        </button>
      </div>
    </>
  );
};