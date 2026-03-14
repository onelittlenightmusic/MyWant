import React from 'react';
import { classNames } from '@/utils/helpers';
import { useConfigStore } from '@/stores/configStore';

interface LayoutProps {
  children: React.ReactNode;
  sidebarMinimized?: boolean;
  onSidebarMinimizedChange?: (minimized: boolean) => void;
}

export const Layout: React.FC<LayoutProps> = ({ children }) => {
  const config = useConfigStore(state => state.config);
  const isBottom = config?.header_position === 'bottom';

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-950 flex">
      <div
        className={classNames(
          "flex-1 flex flex-col relative min-w-0",
          isBottom ? "pb-16 sm:pb-20" : "pt-16 sm:pt-20"
        )}
        style={isBottom ? {} : { marginTop: 'env(safe-area-inset-top, 0px)' }}
      >
        <div className={classNames(
          "flex-1 flex flex-col min-w-0",
          isBottom ? "pb-safe" : ""
        )} style={isBottom ? { paddingBottom: 'env(safe-area-inset-bottom)' } : {}}>
          {children}
        </div>
      </div>
    </div>
  );
};
