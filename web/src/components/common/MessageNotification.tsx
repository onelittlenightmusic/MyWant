import React, { useEffect, useState } from 'react';
import { Bot } from 'lucide-react';
import { classNames } from '@/utils/helpers';

interface MessageNotificationProps {
  message: string | null;
  isVisible: boolean;
  onDismiss: () => void;
  duration?: number; // in milliseconds, default 10000 (10 seconds)
}

export const MessageNotification: React.FC<MessageNotificationProps> = ({
  message,
  isVisible,
  onDismiss,
  duration = 10000
}) => {
  const [isAnimating, setIsAnimating] = useState(false);
  const [displayMessage, setDisplayMessage] = useState<string | null>(null);

  useEffect(() => {
    if (isVisible && message) {
      // Set message and start fade-in animation
      setDisplayMessage(message);
      setIsAnimating(true);

      // Auto-dismiss after duration
      const timer = setTimeout(() => {
        // Start fade-out animation
        setIsAnimating(false);
      }, duration);

      return () => clearTimeout(timer);
    }
  }, [isVisible, message, duration]);

  // Handle fade-out completion
  useEffect(() => {
    if (!isAnimating && displayMessage) {
      // Wait for fade-out animation to complete
      const fadeOutTimer = setTimeout(() => {
        setDisplayMessage(null);
        onDismiss();
      }, 300); // Match CSS transition duration

      return () => clearTimeout(fadeOutTimer);
    }
  }, [isAnimating, displayMessage, onDismiss]);

  if (!displayMessage) {
    return null;
  }

  // Truncate message to 30 characters
  const truncatedMessage = message.length > 30 ? message.substring(0, 27) + '...' : message;

  return (
    <div
      className={classNames(
        'fixed flex items-center gap-3 transition-opacity duration-300',
        'bottom-8 left-1/2 transform -translate-x-1/2',
        isAnimating ? 'opacity-100' : 'opacity-0',
        'z-50 pointer-events-auto'
      )}
    >
      {/* Robot icon */}
      <div className="flex-shrink-0">
        <div className="flex items-center justify-center h-10 w-10 rounded-full bg-blue-600 shadow-lg">
          <Bot className="h-6 w-6 text-white" />
        </div>
      </div>

      {/* Message bubble - pointing to the left */}
      <div className="relative bg-white rounded-lg shadow-lg border border-gray-200 px-4 py-3 max-w-xs">
        {/* Arrow pointing left */}
        <div className="absolute left-0 top-1/2 transform -translate-x-2 -translate-y-1/2">
          <div className="w-0 h-0 border-t-8 border-b-8 border-r-8 border-t-transparent border-b-transparent border-r-white"
               style={{
                 filter: 'drop-shadow(-1px 0 0 rgba(229, 231, 235, 1))'
               }}
          />
        </div>

        {/* Message text */}
        <p className="text-sm font-medium text-gray-900">
          {truncatedMessage}
        </p>
      </div>
    </div>
  );
};
