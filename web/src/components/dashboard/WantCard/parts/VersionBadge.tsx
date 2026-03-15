import React from 'react';

interface VersionBadgeProps {
  version: number;
}

export const VersionBadge: React.FC<VersionBadgeProps> = ({ version }) => {
  if (version < 2) return null;
  
  return (
    <div className="absolute top-1 left-1 sm:top-2 sm:left-2 z-20 pointer-events-none">
      <span className="inline-flex items-center px-1 py-0.5 sm:px-1.5 sm:py-0.5 rounded text-[10px] sm:text-xs font-semibold bg-gray-700 dark:bg-gray-600 text-white opacity-80">
        v{version}
      </span>
    </div>
  );
};
