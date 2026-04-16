import React from 'react';
import { WantExecutionStatus, WantPhase } from '@/types/want';
import { classNames } from '@/utils/helpers';
import { getStatusHexColor } from '@/components/dashboard/WantCard/parts/StatusColor';

interface StatusBadgeProps {
  status: WantExecutionStatus | WantPhase;
  size?: 'xs' | 'sm' | 'md' | 'lg';
  showLabel?: boolean;
  className?: string;
}

export const StatusBadge: React.FC<StatusBadgeProps> = ({
  status,
  size = 'md',
  showLabel = false,
  className
}) => {
  const hexColor = getStatusHexColor(status);

  const dotSizeClasses = {
    xs: 'w-1.5 h-1.5',
    sm: 'w-2 h-2',
    md: 'w-2.5 h-2.5',
    lg: 'w-3 h-3'
  };

  const textSizeClasses = {
    xs: 'text-[9px]',
    sm: 'text-xs',
    md: 'text-sm',
    lg: 'text-base'
  };

  return (
    <div className={classNames('inline-flex items-center gap-1.5', className)} title={status}>
      <span
        className={classNames(
          'inline-flex items-center justify-center rounded-full border border-white/20 dark:border-black/20 shadow-none flex-shrink-0',
          dotSizeClasses[size]
        )}
        style={{ backgroundColor: hexColor }}
      />
      {showLabel && (
        <span className={classNames('font-medium capitalize text-gray-700 dark:text-gray-300', textSizeClasses[size])}>
          {status}
        </span>
      )}
    </div>
  );
};
