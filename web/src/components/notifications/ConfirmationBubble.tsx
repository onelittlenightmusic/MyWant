import React, { useEffect, useState } from 'react';
import { createPortal } from 'react-dom';
import { Bot, Check, X } from 'lucide-react';
import { classNames, truncateText } from '@/utils/helpers';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { useConfirmationDialogKeyboard } from '@/hooks/useConfirmationDialogKeyboard';
import { ConfirmationProps } from './types';

export const ConfirmationBubble: React.FC<ConfirmationProps> = ({
  message,
  isVisible,
  onDismiss,
  onConfirm,
  onCancel,
  loading = false,
  title = 'Please confirm',
  layout = 'bottom-center',
  children
}) => {
  const [isAnimating, setIsAnimating] = useState(false);
  const [displayMessage, setDisplayMessage] = useState<boolean>(false);
  const [isLoading, setIsLoading] = useState(false);
  const [isAnimatingRobot, setIsAnimatingRobot] = useState(false);
  const [isAnimatingBubble, setIsAnimatingBubble] = useState(false);

  useEffect(() => {
    if (isVisible) {
      setDisplayMessage(true);
      setIsAnimating(true);
      setIsAnimatingRobot(false);
      setIsAnimatingBubble(false);
    }
  }, [isVisible]);

  useEffect(() => {
    if (displayMessage && !isAnimatingRobot && !isAnimatingBubble) {
      const timer = setTimeout(() => {
        setIsAnimatingRobot(true);
        setIsAnimatingBubble(true);
      }, 100);

      return () => clearTimeout(timer);
    }
  }, [displayMessage]);

  useEffect(() => {
    if (!isAnimating && displayMessage) {
      const fadeOutTimer = setTimeout(() => {
        setDisplayMessage(false);
        onDismiss();
      }, 300);

      return () => clearTimeout(fadeOutTimer);
    }
  }, [isAnimating, displayMessage, onDismiss]);

  const handleConfirm = async () => {
    if (isLoading) return;
    setIsLoading(true);
    try {
      const result = onConfirm();
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
    if (isLoading) return;
    setIsAnimatingBubble(false);
    setIsAnimatingRobot(false);
    setIsAnimating(false);
    onCancel();
  };

  useEffect(() => {
    if (!isVisible || isLoading) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement;
      if (['INPUT', 'TEXTAREA'].includes(target.tagName) || target.isContentEditable) {
        return;
      }

      if (e.key.toLowerCase() === 'y') {
        e.preventDefault();
        handleConfirm();
      } else if (e.key.toLowerCase() === 'n') {
        e.preventDefault();
        handleCancel();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isVisible, isLoading, handleConfirm, handleCancel]);

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

  const content = (
    <div
      className={classNames(
        'flex items-end gap-3',
        isDashboardRight ? 'fixed right-4 sm:right-[500px] z-[100] h-auto pr-0 sm:pr-6 items-start' : (
          isInlineHeader ? 'relative' : 'fixed left-1/2 transform -translate-x-1/2 z-[100] pointer-events-auto w-[calc(100%-2rem)] sm:w-auto'
        ),
        isAnimating ? 'opacity-100 pointer-events-auto' : 'opacity-0 pointer-events-none',
        !isDashboardRight && !isInlineHeader ? 'transition-opacity duration-300' : ''
      )}
      style={isDashboardRight ? { top: 'calc(env(safe-area-inset-top, 0px) + 6rem)' } : (!isInlineHeader ? { bottom: 'calc(env(safe-area-inset-bottom, 0px) + 2rem)' } : {})}
    >
      {/* Layout 1: Inline Header */}
      {isInlineHeader && (
        <>
          <div className="relative bg-white dark:bg-gray-800 rounded-lg shadow-lg border border-gray-200 dark:border-gray-700 px-3 py-2">
            <div className="flex items-center gap-2 pr-6">
              <div className="min-w-0">
                <p className="text-xs font-semibold text-gray-900 dark:text-white inline">
                  {title}:&nbsp;
                </p>
                <div className="inline text-xs text-gray-700 dark:text-gray-200">
                  {children || (message ? truncateText(message, 40) : '')}
                </div>
              </div>

              <div className="flex gap-1 flex-shrink-0 ml-2">
                <button
                  onClick={handleCancel}
                  disabled={isLoading || loading}
                  className="flex items-center justify-center h-6 w-6 rounded bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200 hover:bg-gray-200 dark:hover:bg-gray-600 disabled:opacity-50 transition-colors"
                  title="Cancel (N)"
                >
                  <X className="h-3 w-3" />
                </button>
                <button
                  onClick={handleConfirm}
                  disabled={isLoading || loading}
                  className="flex items-center justify-center h-6 w-6 rounded bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400 hover:bg-blue-200 dark:hover:bg-blue-900/50 disabled:opacity-50 transition-colors"
                  title="Confirm (Y)"
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

          <div className="flex-shrink-0">
            <div className="flex items-center justify-center h-8 w-8 rounded-full bg-blue-600 shadow-lg">
              <Bot className="h-5 w-5 text-white" />
            </div>
          </div>
        </>
      )}

      {/* Layout 2: Bottom Center (Standard) */}
      {!isInlineHeader && !isDashboardRight && (
        <>
          <div className="flex-shrink-0">
            <div className="flex items-center justify-center h-8 w-8 sm:h-10 sm:w-10 rounded-full bg-blue-600 shadow-lg">
              <Bot className="h-5 w-5 sm:h-6 sm:w-6 text-white" />
            </div>
          </div>

          <div className="relative bg-white dark:bg-gray-800 rounded-lg shadow-xl border border-gray-200 dark:border-gray-700 p-4 w-full max-w-md sm:min-w-[320px]">
            <div className="absolute left-0 bottom-3 transform -translate-x-2">
              <div
                className="w-0 h-0 border-t-8 border-b-8 border-r-8 border-t-transparent border-b-transparent border-r-white"
                style={{ filter: 'drop-shadow(-1px 0 0 rgba(229, 231, 235, 1))' }}
              />
            </div>

            <div className="flex items-start gap-6">
              <div className="flex-1 min-w-0">
                <p className="text-sm font-bold text-gray-900 dark:text-white mb-2">
                  {title}
                </p>
                <div className="text-sm text-gray-600 dark:text-gray-300 leading-relaxed break-words">
                  {children || message}
                </div>
              </div>

              <div className="flex gap-2 flex-shrink-0 pt-1">
                <button
                  onClick={handleCancel}
                  disabled={isLoading || loading}
                  className={classNames(
                    'flex items-center justify-center w-12 h-12 sm:w-14 sm:h-14 aspect-square flex-shrink-0 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700',
                    'bg-gray-50 dark:bg-gray-900 text-gray-500 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800 hover:text-red-600',
                    'focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-gray-300',
                    'disabled:opacity-50 disabled:cursor-not-allowed transition-all duration-200'
                  )}
                  title="Cancel (N or Esc)"
                >
                  <X className="h-6 w-6 sm:h-7 sm:w-7" />
                </button>
                <button
                  onClick={handleConfirm}
                  disabled={isLoading || loading}
                  className={classNames(
                    'flex items-center justify-center w-12 h-12 sm:w-14 sm:h-14 aspect-square flex-shrink-0 rounded-xl shadow-md',
                    'bg-blue-600 text-white hover:bg-blue-700',
                    'focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-blue-500',
                    'disabled:opacity-50 disabled:cursor-not-allowed transition-all duration-200'
                  )}
                  title="Confirm (Y)"
                >
                  {isLoading || loading ? (
                    <LoadingSpinner size="md" color="white" />
                  ) : (
                    <Check className="h-6 w-6 sm:h-7 sm:w-7" />
                  )}
                </button>
              </div>
            </div>
          </div>
        </>
      )}

      {/* Layout 3: Dashboard Right */}
      {isDashboardRight && (
        <div className={classNames(
          'flex flex-col sm:flex-row items-center sm:items-stretch gap-2 sm:gap-0 transition-all duration-300 w-full max-w-[calc(100vw-2rem)] sm:max-w-none',
          (isAnimatingRobot || isAnimatingBubble) ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-4'
        )}>
          <div className="flex items-center justify-center flex-shrink-0 w-12 h-12 sm:w-16 sm:h-16 bg-blue-600 rounded-full shadow-lg">
            <Bot className="h-7 w-7 sm:h-9 sm:w-9 text-white" />
          </div>

          <div className="flex-1 sm:ml-3 bg-white dark:bg-gray-800 rounded-lg shadow-lg border border-gray-200 dark:border-gray-700 px-4 py-3 flex flex-col justify-center min-w-0 sm:min-w-[250px] max-w-full sm:max-w-sm">
            <p className="text-sm font-semibold text-gray-900 dark:text-white truncate">
              {title}
            </p>
            <div className="text-sm text-gray-700 dark:text-gray-200">
              {children || (message ? truncateText(message, 50) : '')}
            </div>
          </div>

          <div className="flex gap-2 sm:ml-3 h-auto sm:h-full">
            <button
              onClick={handleCancel}
              disabled={isLoading || loading}
              className={classNames(
                'flex items-center justify-center w-12 h-12 sm:w-16 sm:h-16 aspect-square flex-shrink-0 rounded-lg shadow-lg',
                'bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-500 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-gray-800 hover:text-red-500',
                'disabled:opacity-50 disabled:cursor-not-allowed transition-all'
              )}
              title="Cancel (N)"
            >
              <X className="h-6 w-6 sm:h-7 sm:w-7" />
            </button>
            <button
              onClick={handleConfirm}
              disabled={isLoading || loading}
              className={classNames(
                'flex items-center justify-center w-12 h-12 sm:w-16 sm:h-16 aspect-square flex-shrink-0 rounded-lg shadow-lg',
                'bg-blue-600 text-white hover:bg-blue-700',
                'disabled:opacity-50 disabled:cursor-not-allowed transition-all'
              )}
              title="Confirm (Y)"
            >
              {isLoading || loading ? (
                <LoadingSpinner size="md" color="white" />
              ) : (
                <Check className="h-6 w-6 sm:h-7 sm:w-7" />
              )}
            </button>
          </div>
        </div>
      )}
    </div>
  );

  if (isInlineHeader) {
    return content;
  }

  return createPortal(content, document.body);
};