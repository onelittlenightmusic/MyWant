import React, { useState, useEffect } from 'react';
import { LucideIcon } from 'lucide-react';
import { classNames } from '@/utils/helpers';

export interface TabConfig {
  id: string;
  label: string;
  icon: LucideIcon;
}

interface DetailsSidebarProps {
  title: string;
  subtitle?: string;
  badge?: React.ReactNode;
  tabs: TabConfig[];
  defaultTab?: string;
  children: React.ReactNode;
  headerContent?: React.ReactNode;
  onTabChange?: (tabId: string) => void;
}

export const DetailsSidebar: React.FC<DetailsSidebarProps> = ({
  title,
  subtitle,
  badge,
  tabs,
  defaultTab,
  children,
  headerContent,
  onTabChange
}) => {
  const [activeTab, setActiveTab] = useState(defaultTab || tabs[0]?.id || '');

  useEffect(() => {
    if (defaultTab) {
      setActiveTab(defaultTab);
    }
  }, [defaultTab]);

  const handleTabChange = (tabId: string) => {
    setActiveTab(tabId);
    onTabChange?.(tabId);
  };

  return (
    <div className="h-full flex flex-col">
      {/* Header */}
      <div className="border-b border-gray-200 px-6 py-4">
        {/* Badge and Title */}
        <div className="flex items-center space-x-3 mb-4">
          {badge}
        </div>
        <div>
          <h3 className="text-lg font-semibold text-gray-900 truncate">
            {title}
          </h3>
          {subtitle && (
            <p className="text-sm text-gray-500 mt-1">{subtitle}</p>
          )}
        </div>

        {/* Additional header content */}
        {headerContent && (
          <div className="mt-4">
            {headerContent}
          </div>
        )}

        {/* Tab navigation */}
        <div className="mt-4">
          <div className="flex space-x-1 bg-gray-100 rounded-lg p-1">
            {tabs.map((tab) => {
              const Icon = tab.icon;
              return (
                <button
                  key={tab.id}
                  onClick={() => handleTabChange(tab.id)}
                  className={classNames(
                    'flex-1 flex items-center justify-center space-x-1 px-2 py-2 text-sm font-medium rounded-md transition-colors min-w-0',
                    activeTab === tab.id
                      ? 'bg-white text-blue-600 shadow-sm'
                      : 'text-gray-600 hover:text-gray-900'
                  )}
                >
                  <Icon className="h-4 w-4 flex-shrink-0" />
                  <span className="truncate text-xs">{tab.label}</span>
                </button>
              );
            })}
          </div>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto">
        {children}
      </div>
    </div>
  );
};

// Common layout components for tabs
export const TabSection: React.FC<{
  title: string;
  children: React.ReactNode;
  className?: string;
}> = ({ title, children, className }) => (
  <div className={classNames('bg-gray-50 rounded-lg p-4', className)}>
    <h4 className="font-medium text-gray-900 mb-3">{title}</h4>
    {children}
  </div>
);

export const TabGrid: React.FC<{
  children: React.ReactNode;
  columns?: number;
}> = ({ children, columns = 1 }) => (
  <div className={classNames(
    'space-y-3',
    columns > 1 && 'md:grid md:gap-6 md:space-y-0',
    columns === 2 && 'md:grid-cols-2',
    columns === 3 && 'lg:grid-cols-3'
  )}>
    {children}
  </div>
);

export const TabContent: React.FC<{
  children: React.ReactNode;
}> = ({ children }) => (
  <div className="p-6 space-y-6">
    {children}
  </div>
);

export const EmptyState: React.FC<{
  icon: LucideIcon;
  message: string;
}> = ({ icon: Icon, message }) => (
  <div className="text-center py-8">
    <Icon className="h-12 w-12 text-gray-400 mx-auto mb-4" />
    <p className="text-gray-500">{message}</p>
  </div>
);

export const InfoRow: React.FC<{
  label: string;
  value: React.ReactNode;
}> = ({ label, value }) => (
  <div className="flex justify-between">
    <dt className="text-sm text-gray-600">{label}:</dt>
    <dd className="text-sm font-medium text-gray-900">{value}</dd>
  </div>
);