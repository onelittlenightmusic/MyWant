import React, { useEffect, useState } from 'react';
import { Bot, Check, X } from 'lucide-react';
import { classNames, truncateText } from '@/utils/helpers';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { useConfirmationDialogKeyboard } from '@/hooks/useConfirmationDialogKeyboard';

interface ConfirmationMessageNotificationProps {
  message: string | null;
  isVisible: boolean;
  onDismiss: () => void;
  onConfirm: () => void | Promise<void>;
  onCancel: () => void;
  loading?: boolean;
  title?: string;
  layout?: 'bottom-center' | 'inline-header' | 'dashboard-right'; // 'dashboard-right' = fixed right side, robot left, bubble right
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
  const [isAnimatingRobot, setIsAnimatingRobot] = useState(false);
  const [isAnimatingBubble, setIsAnimatingBubble] = useState(false);

  useEffect(() => {
    if (isVisible && message) {
      // Set message and ensure initial state is rendered
      setDisplayMessage(message);
      setIsAnimating(true);
      setIsAnimatingRobot(false);
      setIsAnimatingBubble(false);
    }
  }, [isVisible, message]);

  // Separate effect to trigger animation after the notification is displayed
  useEffect(() => {
    if (displayMessage && !isAnimatingRobot && !isAnimatingBubble) {
      // Wait for the initial state to be painted, then trigger animation
      const timer = setTimeout(() => {
        setIsAnimatingRobot(true);
        setIsAnimatingBubble(true);
      }, 100);

      return () => clearTimeout(timer);
    }
  }, [displayMessage]);

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
      setIsAnimatingBubble(false);
      setIsAnimatingRobot(false);
      setIsAnimating(false);
    }
  };

  const handleCancel = () => {
    setIsAnimatingBubble(false);
    setIsAnimatingRobot(false);
    setIsAnimating(false);
    onCancel();
  };

  // Keyboard shortcuts for confirmation dialog
  useConfirmationDialogKeyboard({
    isVisible,
    onConfirm: handleConfirm,
    onCancel: handleCancel,
    loading: isLoading || loading,
    enabled: true
  });

  if (!displayMessage) {
    return null;
  }

  const isInlineHeader = layout === 'inline-header';
  const isDashboardRight = layout === 'dashboard-right';

  return (
    <div
      className={classNames(
        'flex items-center gap-0',
        isDashboardRight ? 'fixed top-2 right-[480px] z-50 pointer-events-auto h-16 pr-6' : (
          isInlineHeader ? 'relative' : 'fixed bottom-8 left-1/2 transform -translate-x-1/2 z-50 pointer-events-auto'
        ),
        !isDashboardRight && (isAnimating ? 'opacity-100' : 'opacity-0'),
        !isDashboardRight && !isInlineHeader ? 'pointer-events-auto' : ''
      )}
    >
      {/* Layout 1: Bubble on left, Robot on right (for inline-header) */}
      {isInlineHeader && (
        <>
          {/* Message bubble with buttons - pointing to the right */}
          <div className="relative bg-white rounded-lg shadow-lg border border-gray-200 px-3 py-2">
            {/* Close button */}
            <button
              onClick={handleCancel}
              disabled={isLoading || loading}
              className="absolute top-1 right-1 text-gray-400 hover:text-gray-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
              title="Cancel"
            >
              <X className="h-3 w-3" />
            </button>

            {/* Arrow pointing right */}
            <div className="absolute right-0 top-1/2 transform translate-x-2 -translate-y-1/2">
              <div
                className="w-0 h-0 border-t-5 border-b-5 border-l-5 border-t-transparent border-b-transparent border-l-white"
                style={{
                  filter: 'drop-shadow(1px 0 0 rgba(229, 231, 235, 1))'
                }}
              />
            </div>

            {/* Content - single line */}
            <div className="flex items-center gap-2 pr-6">
              {/* Title and Message on one line */}
              <div className="min-w-0">
                <p className="text-xs font-semibold text-gray-900 inline">
                  {title}:&nbsp;
                </p>
                <p className="text-xs text-gray-700 inline truncate">
                  {message ? truncateText(message, 40) : ''}
                </p>
              </div>

              {/* Action buttons */}
              <div className="flex gap-1 flex-shrink-0 ml-2">
                <button
                  onClick={handleCancel}
                  disabled={isLoading || loading}
                  className={classNames(
                    'flex items-center justify-center px-2 py-1 rounded-md text-xs font-medium',
                    'bg-gray-100 text-gray-700 hover:bg-gray-200 shadow-lg',
                    'disabled:opacity-50 disabled:cursor-not-allowed',
                    'transition-colors'
                  )}
                  title="Cancel"
                >
                  <X className="h-3 w-3" />
                </button>
                <button
                  onClick={handleConfirm}
                  disabled={isLoading || loading}
                  className={classNames(
                    'flex items-center justify-center px-2 py-1 rounded-md text-xs font-medium',
                    'bg-blue-100 text-blue-700 hover:bg-blue-200 shadow-lg',
                    'disabled:opacity-50 disabled:cursor-not-allowed',
                    'transition-colors'
                  )}
                  title="Confirm"
                >
                  {isLoading || loading ? (
                    <LoadingSpinner size="sm" color="white" className="h-3 w-3" />
                  ) : (
                    <Check className="h-3 w-3" />
                  )}
                </button>
              </div>
            </div>
          </div>

          {/* Robot icon on right */}
          <div className="flex-shrink-0">
            <div className="flex items-center justify-center h-8 w-8 rounded-full bg-blue-600 shadow-lg">
              <Bot className="h-5 w-5 text-white" />
            </div>
          </div>
        </>
      )}

      {/* Layout 2: Robot on left, Bubble on right (for bottom-center) */}
      {!isInlineHeader && !isDashboardRight && (
        <>
          {/* Robot icon */}
          <div className="flex-shrink-0">
            <div className="flex items-center justify-center h-10 w-10 rounded-full bg-blue-600 shadow-lg">
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
                    'bg-gray-100 text-gray-700 hover:bg-gray-200 shadow-lg',
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
                    'bg-blue-100 text-blue-700 hover:bg-blue-200 shadow-lg',
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

      {/* Layout 3: Dashboard right - Robot left, Bubble middle, Buttons right (full height) */}
      {isDashboardRight && (
        <div className={classNames(
          'flex items-stretch gap-0 h-16 transition-all duration-300',
          (isAnimatingRobot || isAnimatingBubble) ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-4'
        )}>
          {/* Robot icon - left (header height) */}
          <div className="flex items-center justify-center flex-shrink-0 w-16 bg-blue-600 rounded-full shadow-lg">
            <Bot className="h-9 w-9 text-white" />
          </div>

          {/* Message bubble - middle */}
          <div className="flex-1 ml-3 bg-white rounded-lg shadow-lg border border-gray-200 px-4 flex flex-col justify-center min-w-0">
            {/* Title and Message */}
            <p className="text-sm font-semibold text-gray-900 truncate">
              {title}
            </p>
            <p className="text-sm text-gray-700 truncate">
              {truncateText(message || '', 50)}
            </p>
          </div>

          {/* Action buttons - right (full height) */}
          <div className="flex gap-2 ml-3 h-full">
            <button
              onClick={handleCancel}
              disabled={isLoading || loading}
              className={classNames(
                'flex items-center justify-center rounded-md font-medium h-full px-4 shadow-lg',
                'bg-gray-100 text-gray-700 hover:bg-gray-200',
                'disabled:opacity-50 disabled:cursor-not-allowed',
                'transition-colors'
              )}
              title="Cancel"
            >
              <X className="h-6 w-6" />
            </button>
            <button
              onClick={handleConfirm}
              disabled={isLoading || loading}
              className={classNames(
                'flex items-center justify-center rounded-md font-medium h-full px-4 shadow-lg',
                'bg-blue-100 text-blue-700 hover:bg-blue-200',
                'disabled:opacity-50 disabled:cursor-not-allowed',
                'transition-colors'
              )}
              title="Confirm"
            >
              {isLoading || loading ? (
                <LoadingSpinner size="sm" color="white" className="h-6 w-6" />
              ) : (
                <Check className="h-6 w-6" />
              )}
            </button>
          </div>
        </div>
      )}
    </div>
  );
};
