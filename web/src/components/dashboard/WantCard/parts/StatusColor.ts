import { WantExecutionStatus, WantPhase } from '@/types/want';

/**
 * Get color for a want status (Hex code)
 */
export const getStatusHexColor = (status: WantExecutionStatus | WantPhase): string => {
  switch (status) {
    case 'achieved':
      return '#10b981'; // Green
    case 'reaching':
    case 'terminated':
      return '#9333ea'; // Purple
    case 'failed':
    case 'module_error':
      return '#ef4444'; // Red
    case 'config_error':
    case 'stopped':
    case 'waiting_user_action':
      return '#f59e0b'; // Amber/Yellow
    case 'cancelled':
      return '#9ca3af'; // Gray for cancelled (superseded by rebook)
    default:
      return '#d1d5db'; // Gray for created, initializing, suspended
  }
};
