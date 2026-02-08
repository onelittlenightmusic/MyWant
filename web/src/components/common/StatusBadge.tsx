import React from 'react';
import { WantExecutionStatus, WantPhase } from '@/types/want';
import { getStatusColor, getStatusIconComponent, classNames } from '@/utils/helpers';
import styles from '@/components/dashboard/WantCard.module.css';

interface StatusBadgeProps {
  status: WantExecutionStatus | WantPhase;
  showIcon?: boolean;
  size?: 'xs' | 'sm' | 'md' | 'lg';
  className?: string;
}

export const StatusBadge: React.FC<StatusBadgeProps> = ({
  status,
  showIcon = true,
  size = 'md',
  className
}) => {
  const color = getStatusColor(status);
  const icon = getStatusIconComponent(status);

  const sizeClasses = {
    xs: 'px-1.5 py-0.5 text-xs',
    sm: 'px-2 py-1 text-xs',
    md: 'px-2.5 py-1.5 text-sm',
    lg: 'px-3 py-2 text-base'
  };

  const colorClasses = {
    gray: 'bg-gray-100 text-gray-800 border-gray-200',
    blue: 'bg-blue-100 text-blue-800 border-blue-200',
    green: 'bg-green-100 text-green-800 border-green-200',
    red: 'bg-red-100 text-red-800 border-red-200',
    yellow: 'bg-yellow-100 text-yellow-800 border-yellow-200'
  };

  return (
    <span
      className={classNames(
        'inline-flex items-center gap-1 font-medium rounded-full border group/status relative',
        sizeClasses[size],
        colorClasses[color as keyof typeof colorClasses],
        (status === 'reaching' || status === 'waiting_user_action') && styles.pulseGlow,
        className
      )}
      title={status}
    >
      {showIcon && <span>{icon}</span>}
      <span className="capitalize hidden group-hover/status:inline">{status}</span>
    </span>
  );
};