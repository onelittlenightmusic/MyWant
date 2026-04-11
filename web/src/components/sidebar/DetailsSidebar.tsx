import React, { useState, useEffect } from 'react';
import { LucideIcon } from 'lucide-react';
import { classNames } from '@/utils/helpers';
import { useConfigStore } from '@/stores/configStore';

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
  headerOverlay?: React.ReactNode;
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
  headerOverlay,
  onTabChange
}) => {
  const config = useConfigStore(state => state.config);
  const isBottom = config?.header_position === 'bottom';
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
    <div className="h-full flex flex-col relative overflow-hidden">
      {/* Header overlay (e.g. ConfirmationBubble) */}
      {headerOverlay}

      {/* Tab navigation - Unified with WantDetailsSidebar style */}
      <div className={classNames(
        'flex border-gray-200 dark:border-gray-700 bg-gray-50/50 dark:bg-gray-900/50',
        isBottom ? 'order-last border-t' : 'border-b'
      )}>
        {tabs.map((tab) => {
          const Icon = tab.icon;
          const isActive = activeTab === tab.id;
          return (
            <button
              key={tab.id}
              onClick={() => handleTabChange(tab.id)}
              className={classNames(
                'flex-1 flex flex-col items-center justify-center py-1 sm:py-2.5 px-1 sm:px-2 transition-all relative min-w-0',
                isActive
                  ? 'text-blue-600 dark:text-blue-400 bg-white dark:bg-gray-800 shadow-sm'
                  : 'text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-white/50 dark:hover:bg-gray-800/30'
              )}
            >
              <Icon className="h-3.5 w-3.5 sm:h-4 sm:w-4 mb-0 sm:mb-1 flex-shrink-0" />
              <span className="text-[9px] sm:text-[10px] font-bold uppercase tracking-tighter truncate w-full text-center">
                {tab.label}
              </span>
              {/* Active indicator bar - adjacent to content */}
              {isActive && (
                <div className={classNames(
                  "absolute h-0.5 bg-blue-600 dark:bg-blue-400",
                  isBottom ? "top-0 left-0 right-0" : "bottom-0 left-0 right-0"
                )} />
              )}
            </button>
          );
        })}
      </div>

      {/* Content */}
      <div className={classNames(
        "flex-1 overflow-y-auto overflow-x-hidden relative",
        isBottom ? "order-first" : ""
      )}>
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
  <div className={classNames('bg-gray-50 dark:bg-gray-800/50 rounded-lg p-3 sm:p-4', className)}>
    <h4 className="text-sm sm:text-base font-medium text-gray-900 dark:text-white mb-2 sm:mb-3">{title}</h4>
    {children}
  </div>
);

export const TabGrid: React.FC<{
  children: React.ReactNode;
  columns?: number;
}> = ({ children, columns = 1 }) => (
  <div className={classNames(
    'space-y-2 sm:space-y-3',
    columns > 1 && 'md:grid md:gap-4 sm:gap-6 md:space-y-0',
    columns === 2 && 'md:grid-cols-2',
    columns === 3 && 'lg:grid-cols-3'
  )}>
    {children}
  </div>
);

export const TabContent: React.FC<{
  children: React.ReactNode;
}> = ({ children }) => (
  <div className="p-3 sm:p-6 space-y-4 sm:space-y-6">
    {children}
  </div>
);

export const EmptyState: React.FC<{
  icon: LucideIcon;
  message: string;
}> = ({ icon: Icon, message }) => (
  <div className="text-center py-6 sm:py-8">
    <Icon className="h-10 w-10 sm:h-12 sm:w-12 text-gray-400 dark:text-gray-600 mx-auto mb-3 sm:mb-4" />
    <p className="text-sm text-gray-500 dark:text-gray-400">{message}</p>
  </div>
);

export const InfoRow: React.FC<{
  label: string;
  value: React.ReactNode;
}> = ({ label, value }) => (
  <div className="flex justify-between items-center text-xs sm:text-sm">
    <dt className="text-gray-600 dark:text-gray-400">{label}:</dt>
    <dd className="font-medium text-gray-900 dark:text-gray-200">{value}</dd>
  </div>
);