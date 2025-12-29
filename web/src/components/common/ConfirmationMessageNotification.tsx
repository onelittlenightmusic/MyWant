import React, { useEffect, useState } from 'react';
import { Bot, Check, X } from 'lucide-react';
import { classNames } from '@/utils/helpers';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';

interface ConfirmationMessageNotificationProps {
  message: string | null;
  isVisible: boolean;
  onDismiss: () => void;
  onConfirm: () => void | Promise<void>;
  onCancel: () => void;
  loading?: boolean;
  title?: string;
  layout?: 'bottom-center' | 'inline-header'; // 'inline-header' = robot right, bubble left in header
}

export const ConfirmationMessageNotification: React.FC<ConfirmationMessageNotificationProps> = ({
  message,
  isVisible,
  onDismiss,
  onConfirm,
  onCancel,
  loading = false,
  title = 'Please confirm',
  layout = 'bottom-center'
}) => {
  const [isAnimating, setIsAnimating] = useState(false);
  const [displayMessage, setDisplayMessage] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  useEffect(() => {
    if (isVisible && message) {
      // Set message and start fade-in animation
      setDisplayMessage(message);
      setIsAnimating(true);
    }
  }, [isVisible, message]);

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

  const handleConfirm = async () => {
    setIsLoading(true);
    try {
      const result = onConfirm();
      // Handle both sync and async confirmations
      if (result instanceof Promise) {
        await result;
      }
    } catch (error) {
      console.error('Confirmation error:', error);
    } finally {
      setIsLoading(false);
      setIsAnimating(false);
    }
  };

  const handleCancel = () => {
    setIsAnimating(false);
    onCancel();
  };

  if (!displayMessage) {
    return null;
  }

  const isInlineHeader = layout === 'inline-header';

  return (
    <div
      className={classNames(
        'flex items-center gap-3 transition-opacity duration-300',
        isInlineHeader ? 'relative' : 'fixed bottom-8 left-1/2 transform -translate-x-1/2 z-50 pointer-events-auto',
        isAnimating ? 'opacity-100' : 'opacity-0',
        isInlineHeader ? '' : 'pointer-events-auto'
      )}
    >
      {/* Layout 1: Bubble on left, Robot on right (for inline-header) */}
      {isInlineHeader && (
        <>
          {/* Message bubble with buttons - pointing to the right */}
          <div className="relative bg-white rounded-lg shadow-lg border border-gray-200 px-4 py-3 whitespace-nowrap">
            {/* Close button */}
            <button
              onClick={handleCancel}
              disabled={isLoading || loading}
              className="absolute top-2 right-2 text-gray-400 hover:text-gray-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
              title="Cancel"
            >
              <X className="h-4 w-4" />
            </button>

            {/* Arrow pointing right */}
            <div className="absolute right-0 top-1/2 transform translate-x-2 -translate-y-1/2">
              <div
                className="w-0 h-0 border-t-6 border-b-6 border-l-6 border-t-transparent border-b-transparent border-l-white"
                style={{
                  filter: 'drop-shadow(1px 0 0 rgba(229, 231, 235, 1))'
                }}
              />
            </div>

            {/* Content */}
            <div className="space-y-2 pr-6">
              {/* Title */}
              <p className="text-xs font-semibold text-gray-900">
                {title}
              </p>

              {/* Message */}
              <p className="text-xs text-gray-700">
                {message}
              </p>

              {/* Action buttons */}
              <div className="flex gap-2 pt-1">
                <button
                  onClick={handleCancel}
                  disabled={isLoading || loading}
                  className={classNames(
                    'flex items-center gap-1 px-2 py-1 rounded-md text-xs font-medium',
                    'bg-gray-100 text-gray-700 hover:bg-gray-200',
                    'disabled:opacity-50 disabled:cursor-not-allowed',
                    'transition-colors'
                  )}
                >
                  Cancel
                </button>
                <button
                  onClick={handleConfirm}
                  disabled={isLoading || loading}
                  className={classNames(
                    'flex items-center gap-1 px-2 py-1 rounded-md text-xs font-medium',
                    'bg-blue-100 text-blue-700 hover:bg-blue-200',
                    'disabled:opacity-50 disabled:cursor-not-allowed',
                    'transition-colors'
                  )}
                >
                  {isLoading || loading ? (
                    <>
                      <LoadingSpinner size="sm" color="white" className="h-3 w-3" />
                      <span>...</span>
                    </>
                  ) : (
                    <>
                      <Check className="h-3 w-3" />
                      OK
                    </>
                  )}
                </button>
              </div>
            </div>
          </div>

          {/* Robot icon on right */}
          <div className="flex-shrink-0">
            <div className="flex items-center justify-center h-8 w-8 rounded-full bg-blue-600">
              <Bot className="h-5 w-5 text-white" />
            </div>
          </div>
        </>
      )}

      {/* Layout 2: Robot on left, Bubble on right (for bottom-center) */}
      {!isInlineHeader && (
        <>
          {/* Robot icon */}
          <div className="flex-shrink-0">
            <div className="flex items-center justify-center h-10 w-10 rounded-full bg-blue-600">
              <Bot className="h-6 w-6 text-white" />
            </div>
          </div>

          {/* Message bubble with buttons - pointing to the left */}
          <div className="relative bg-white rounded-lg shadow-lg border border-gray-200 px-4 py-3 max-w-sm">
            {/* Close button */}
            <button
              onClick={handleCancel}
              disabled={isLoading || loading}
              className="absolute top-3 right-3 text-gray-400 hover:text-gray-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
              title="Cancel"
            >
              <X className="h-5 w-5" />
            </button>

            {/* Arrow pointing left */}
            <div className="absolute left-0 top-1/2 transform -translate-x-2 -translate-y-1/2">
              <div
                className="w-0 h-0 border-t-8 border-b-8 border-r-8 border-t-transparent border-b-transparent border-r-white"
                style={{
                  filter: 'drop-shadow(-1px 0 0 rgba(229, 231, 235, 1))'
                }}
              />
            </div>

            {/* Content */}
            <div className="space-y-3 pr-8">
              {/* Title */}
              <p className="text-sm font-semibold text-gray-900">
                {title}
              </p>

              {/* Message */}
              <p className="text-sm text-gray-700">
                {message}
              </p>

              {/* Action buttons */}
              <div className="flex gap-2 pt-2">
                <button
                  onClick={handleCancel}
                  disabled={isLoading || loading}
                  className={classNames(
                    'flex items-center gap-1 px-3 py-2 rounded-md text-sm font-medium',
                    'bg-gray-100 text-gray-700 hover:bg-gray-200',
                    'disabled:opacity-50 disabled:cursor-not-allowed',
                    'transition-colors'
                  )}
                >
                  Cancel
                </button>
                <button
                  onClick={handleConfirm}
                  disabled={isLoading || loading}
                  className={classNames(
                    'flex items-center gap-1 px-3 py-2 rounded-md text-sm font-medium',
                    'bg-blue-100 text-blue-700 hover:bg-blue-200',
                    'disabled:opacity-50 disabled:cursor-not-allowed',
                    'transition-colors'
                  )}
                >
                  {isLoading || loading ? (
                    <>
                      <LoadingSpinner size="sm" color="white" className="h-4 w-4" />
                      Processing...
                    </>
                  ) : (
                    <>
                      <Check className="h-4 w-4" />
                      OK
                    </>
                  )}
                </button>
              </div>
            </div>
          </div>
        </>
      )}
    </div>
  );
};
