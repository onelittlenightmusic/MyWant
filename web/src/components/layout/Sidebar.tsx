import React from 'react';
import { X, Heart, Bot, BookOpen, AlertTriangle } from 'lucide-react';
import { classNames } from '@/utils/helpers';

interface SidebarProps {
  isOpen: boolean;
  onClose: () => void;
}

// Get current path for active state
const getCurrentPath = () => {
  if (typeof window !== 'undefined') {
    return window.location.pathname;
  }
  return '/dashboard';
};

const getMenuItems = () => {
  const currentPath = getCurrentPath();
  return [
    {
      id: 'wants',
      label: 'Wants',
      icon: Heart,
      href: '/dashboard',
      active: currentPath === '/dashboard'
    },
    {
      id: 'errors',
      label: 'Error History',
      icon: AlertTriangle,
      href: '/errors',
      active: currentPath === '/errors'
    },
    {
      id: 'agents',
      label: 'Agents',
      icon: Bot,
      href: '/agents',
      active: currentPath === '/agents'
    }
  ];
};

const getAdvancedItems = () => {
  const currentPath = getCurrentPath();
  return [
    {
      id: 'recipes',
      label: 'Recipes',
      icon: BookOpen,
      href: '/recipes',
      active: currentPath === '/recipes',
      disabled: false
    }
  ];
};

export const Sidebar: React.FC<SidebarProps> = ({
  isOpen,
  onClose
}) => {
  const menuItems = getMenuItems();

  return (
    <>
      {/* Overlay */}
      {isOpen && (
        <div
          className="fixed inset-0 bg-gray-600 bg-opacity-50 z-40 lg:hidden"
          onClick={onClose}
        />
      )}

      {/* Sidebar */}
      <div className={classNames(
        'fixed lg:relative inset-y-0 left-0 z-50 w-64 bg-white border-r border-gray-200 transform transition-transform duration-300 ease-in-out lg:translate-x-0 lg:flex lg:flex-col lg:h-screen',
        isOpen ? 'translate-x-0' : '-translate-x-full'
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
          <div className="hidden lg:block px-4 py-6 border-b border-gray-200">
            <h1 className="text-xl font-bold text-gray-900">MyWant</h1>
            <p className="text-sm text-gray-500 mt-1">Chain Programming</p>
          </div>

          {/* Navigation */}
          <div className="flex-1 px-4 py-6 space-y-8">
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
                      {
                        'bg-primary-100 text-primary-900': item.active,
                        'text-gray-600 hover:bg-gray-100 hover:text-gray-900': !item.active && !item.disabled,
                        'text-gray-400 cursor-not-allowed': item.disabled
                      }
                    )}
                    onClick={(e) => item.disabled && e.preventDefault()}
                  >
                    <Icon className="h-5 w-5 mr-3" />
                    {item.label}
                    {item.disabled && (
                      <span className="ml-auto text-xs text-gray-400">Soon</span>
                    )}
                  </a>
                );
              })}
            </nav>

            {/* Advanced Section */}
            <div>
              <h3 className="px-3 text-xs font-semibold text-gray-500 uppercase tracking-wider mb-3">
                Advanced
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
                        {
                          'bg-primary-100 text-primary-900': item.active,
                          'text-gray-600 hover:bg-gray-100 hover:text-gray-900': !item.active && !item.disabled,
                          'text-gray-400 cursor-not-allowed': item.disabled
                        }
                      )}
                      onClick={(e) => item.disabled && e.preventDefault()}
                    >
                      <Icon className="h-5 w-5 mr-3" />
                      {item.label}
                      {item.disabled && (
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
    </>
  );
};