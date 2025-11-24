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
    case 'running':
      return 'blue';
    case 'suspended':
      return 'yellow';
    case 'completed':
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
    case 'running':
      return '▶️';
    case 'suspended':
      return '⏸️';
    case 'completed':
      return '✅';
    case 'failed':
      return '❌';
    case 'stopped':
      return '⏹️';
    default:
      return '❓';
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
  let timeout: NodeJS.Timeout;

  return (...args: Parameters<T>) => {
    clearTimeout(timeout);
    timeout = setTimeout(() => func(...args), wait);
  };
};

export const classNames = (...classes: (string | undefined | null | false)[]): string => {
  return classes.filter(Boolean).join(' ');
};