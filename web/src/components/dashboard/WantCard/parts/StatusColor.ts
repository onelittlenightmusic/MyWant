import { WantExecutionStatus, WantPhase } from '@/types/want';

/**
 * Get color for a want status (Hex code)
 * Used by StatusBadge, Minimap, and children dots for consistency.
 */
export const getStatusHexColor = (status: WantExecutionStatus | WantPhase): string => {
  switch (status) {
    case 'achieved':
    case 'achieved_with_warning':
      return '#10b981'; // Green

    case 'reaching':
    case 'reaching_with_warning':
    case 'initializing':
      return '#9333ea'; // Purple (Active)
    
    case 'failed':
    case 'module_error':
    case 'deleting':
      return '#ef4444'; // Red (Error/Destruction)
    
    case 'config_error':
    case 'stopped':
    case 'waiting_user_action':
    case 'suspended':
      return '#f59e0b'; // Amber/Yellow (Warning/Paused/User)
    
    case 'terminated':
    case 'cancelled':
      return '#9ca3af'; // Gray (End state but not success)
    
    case 'created':
    case 'pending':
    default:
      return '#d1d5db'; // Light Gray (Initial)
  }
};
