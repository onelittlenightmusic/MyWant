import React, { useEffect, useState } from 'react';
import { Bot, AlertCircle, CheckCircle, Info, AlertTriangle } from 'lucide-react';
import { classNames } from '@/utils/helpers';
import { ToastProps, NotificationSeverity } from './types';

export const Toast: React.FC<ToastProps> = ({
  message,
  isVisible,
  onDismiss,
  duration = 10000,
  severity = 'info'
}) => {
  const [isAnimating, setIsAnimating] = useState(false);
  const [displayMessage, setDisplayMessage] = useState<string | null>(null);

  useEffect(() => {
    if (isVisible && message) {
      setDisplayMessage(message);
      setIsAnimating(true);

      const timer = setTimeout(() => {
        setIsAnimating(false);
      }, duration);

      return () => clearTimeout(timer);
    }
  }, [isVisible, message, duration]);

  useEffect(() => {
    if (!isAnimating && displayMessage) {
      const fadeOutTimer = setTimeout(() => {
        setDisplayMessage(null);
        onDismiss();
      }, 300);

      return () => clearTimeout(fadeOutTimer);
    }
  }, [isAnimating, displayMessage, onDismiss]);

  if (!displayMessage) {
    return null;
  }

  const truncatedMessage = message && message.length > 30 ? message.substring(0, 27) + '...' : message;

  const getIcon = (severity: NotificationSeverity) => {
    switch (severity) {
      case 'success': return <CheckCircle className="h-6 w-6 text-white" />;
      case 'error': return <AlertCircle className="h-6 w-6 text-white" />;
      case 'warning': return <AlertTriangle className="h-6 w-6 text-white" />;
      default: return <Bot className="h-6 w-6 text-white" />;
    }
  };

  const getBgColor = (severity: NotificationSeverity) => {
    switch (severity) {
      case 'success': return 'bg-green-600';
      case 'error': return 'bg-red-600';
      case 'warning': return 'bg-yellow-500';
      default: return 'bg-blue-600';
    }
  };

  return (
    <div
      className={classNames(
        'fixed flex items-center gap-3 transition-opacity duration-300',
        'bottom-8 left-1/2 transform -translate-x-1/2',
        isAnimating ? 'opacity-100' : 'opacity-0',
        'z-50 pointer-events-auto'
      )}
    >
      <div className="flex-shrink-0">
        <div className={classNames("flex items-center justify-center h-10 w-10 rounded-full shadow-lg", getBgColor(severity))}>
          {getIcon(severity)}
        </div>
      </div>

      <div className="relative bg-white rounded-lg shadow-lg border border-gray-200 px-4 py-3 max-w-xs">
        <div className="absolute left-0 top-1/2 transform -translate-x-2 -translate-y-1/2">
          <div className="w-0 h-0 border-t-8 border-b-8 border-r-8 border-t-transparent border-b-transparent border-r-white"
               style={{
                 filter: 'drop-shadow(-1px 0 0 rgba(229, 231, 235, 1))'
               }}
          />
        </div>

        <p className="text-sm font-medium text-gray-900">
          {truncatedMessage}
        </p>
      </div>
    </div>
  );
};
