import React from 'react';
import { Play, Pause, Square, CheckCircle, AlertCircle, Clock, RotateCw } from 'lucide-react';
import { WantExecutionStatus, WantPhase } from '@/types/want';

export const formatDate = (dateString?: string): string => {
  if (!dateString) return 'N/A';

  try {
    return new Date(dateString).toLocaleString();
  } catch {
    return dateString;
  }
};

export const formatDuration = (startTime?: string, endTime?: string): string => {
  if (!startTime) return 'N/A';

  const start = new Date(startTime);
  const end = endTime ? new Date(endTime) : new Date();

  const diffMs = end.getTime() - start.getTime();
  const diffSecs = Math.floor(diffMs / 1000);
  const diffMins = Math.floor(diffSecs / 60);
  const diffHours = Math.floor(diffMins / 60);

  if (diffHours > 0) {
    return `${diffHours}h ${diffMins % 60}m`;
  } else if (diffMins > 0) {
    return `${diffMins}m ${diffSecs % 60}s`;
  } else {
    return `${diffSecs}s`;
  }
};

export const getStatusColor = (status: WantExecutionStatus | WantPhase): string => {
  switch (status) {
    case 'created':
    case 'pending':
      return 'gray';
    case 'initializing':
    case 'reaching':
      return 'blue';
    case 'suspended':
      return 'yellow';
    case 'achieved':
      return 'green';
    case 'failed':
      return 'red';
    case 'stopped':
      return 'yellow';
    default:
      return 'gray';
  }
};

export const getStatusIcon = (status: WantExecutionStatus | WantPhase): string => {
  switch (status) {
    case 'created':
    case 'pending':
      return '⏳';
    case 'initializing':
      return '🔄';
    case 'reaching':
      return '▶️';
    case 'suspended':
      return '⏸️';
    case 'achieved':
      return '✅';
    case 'failed':
      return '❌';
    case 'stopped':
      return '⏹️';
    default:
      return '❓';
  }
};

export const getStatusIconComponent = (status: WantExecutionStatus | WantPhase): React.ReactNode => {
  const iconProps = { className: 'h-4 w-4' };

  switch (status) {
    case 'created':
    case 'pending':
      return React.createElement(Clock, iconProps);
    case 'initializing':
      return React.createElement(RotateCw, iconProps);
    case 'reaching':
      return React.createElement(Play, iconProps);
    case 'suspended':
      return React.createElement(Pause, iconProps);
    case 'achieved':
      return React.createElement(CheckCircle, iconProps);
    case 'failed':
      return React.createElement(AlertCircle, iconProps);
    case 'stopped':
      return React.createElement(Square, iconProps);
    default:
      return React.createElement(AlertCircle, iconProps);
  }
};

export const truncateText = (text: string, maxLength: number): string => {
  if (text.length <= maxLength) return text;
  return text.slice(0, maxLength) + '...';
};

export const generateId = (): string => {
  return Math.random().toString(36).substring(2, 15) + Math.random().toString(36).substring(2, 15);
};

export const debounce = <T extends (...args: unknown[]) => void>(
  func: T,
  wait: number
): ((...args: Parameters<T>) => void) => {
  let timeout: ReturnType<typeof setTimeout>;

  return (...args: Parameters<T>) => {
    clearTimeout(timeout);
    timeout = setTimeout(() => func(...args), wait);
  };
};

export const classNames = (...classes: (string | undefined | null | false)[]): string => {
  return classes.filter(Boolean).join(' ');
};