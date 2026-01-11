export type NotificationType = 'toast' | 'modal' | 'banner';
export type NotificationSeverity = 'success' | 'info' | 'warning' | 'error';

export interface BaseNotificationProps {
  isVisible: boolean;
  onDismiss: () => void;
  message: string | null;
  title?: string;
  severity?: NotificationSeverity;
}

export interface ToastProps extends BaseNotificationProps {
  duration?: number;
}

export interface ConfirmationProps extends BaseNotificationProps {
  onConfirm: () => void | Promise<void>;
  onCancel: () => void;
  loading?: boolean;
  layout?: 'bottom-center' | 'inline-header' | 'dashboard-right';
}
