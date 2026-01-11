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
  layout = 'bottom-center'
}) => {
  const [isAnimating, setIsAnimating] = useState(false);
  const [displayMessage, setDisplayMessage] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [isAnimatingRobot, setIsAnimatingRobot] = useState(false);
  const [isAnimatingBubble, setIsAnimatingBubble] = useState(false);

  useEffect(() => {
    if (isVisible && message) {
      setDisplayMessage(message);
      setIsAnimating(true);
      setIsAnimatingRobot(false);
      setIsAnimatingBubble(false);
    }
  }, [isVisible, message]);

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
        setDisplayMessage(null);
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
        isDashboardRight ? 'fixed top-24 right-[500px] z-[100] h-auto pr-6 items-start' : (
          isInlineHeader ? 'relative' : 'fixed bottom-8 left-1/2 transform -translate-x-1/2 z-[100] pointer-events-auto'
        ),
        isAnimating ? 'opacity-100 pointer-events-auto' : 'opacity-0 pointer-events-none',
        !isDashboardRight && !isInlineHeader ? 'transition-opacity duration-300' : ''
      )}
    >
      {/* Layout 1: Inline Header */}
      {isInlineHeader && (
        <>
          <div className="relative bg-white rounded-lg shadow-lg border border-gray-200 px-3 py-2">
            <div className="flex items-center gap-2 pr-6">
              <div className="min-w-0">
                <p className="text-xs font-semibold text-gray-900 inline">
                  {title}:&nbsp;
                </p>
                <p className="text-xs text-gray-700 inline truncate">
                  {message ? truncateText(message, 40) : ''}
                </p>
              </div>

              <div className="flex gap-1 flex-shrink-0 ml-2">
                <button
                  onClick={handleCancel}
                  disabled={isLoading || loading}
                  className="flex items-center justify-center h-6 w-6 rounded bg-gray-100 text-gray-700 hover:bg-gray-200 disabled:opacity-50 transition-colors"
                  title="Cancel (N)"
                >
                  <X className="h-3 w-3" />
                </button>
                <button
                  onClick={handleConfirm}
                  disabled={isLoading || loading}
                  className="flex items-center justify-center h-6 w-6 rounded bg-blue-100 text-blue-700 hover:bg-blue-200 disabled:opacity-50 transition-colors"
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
            <div className="flex items-center justify-center h-10 w-10 rounded-full bg-blue-600 shadow-lg">
              <Bot className="h-6 w-6 text-white" />
            </div>
          </div>

          <div className="relative bg-white rounded-lg shadow-xl border border-gray-200 p-4 max-w-md">
            <div className="absolute left-0 bottom-3 transform -translate-x-2">
              <div
                className="w-0 h-0 border-t-8 border-b-8 border-r-8 border-t-transparent border-b-transparent border-r-white"
                style={{ filter: 'drop-shadow(-1px 0 0 rgba(229, 231, 235, 1))' }}
              />
            </div>

            <div className="flex items-center gap-6">
              <div className="flex-1 min-w-0">
                <p className="text-sm font-bold text-gray-900 mb-1">
                  {title}
                </p>
                <p className="text-sm text-gray-600 leading-relaxed break-words">
                  {message}
                </p>
              </div>

              <div className="flex gap-2 flex-shrink-0">
                <button
                  onClick={handleCancel}
                  disabled={isLoading || loading}
                  className={classNames(
                    'flex items-center justify-center w-14 h-14 aspect-square flex-shrink-0 rounded-xl shadow-sm border border-gray-200',
                    'bg-gray-50 text-gray-500 hover:bg-gray-100 hover:text-red-600',
                    'focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-gray-300',
                    'disabled:opacity-50 disabled:cursor-not-allowed transition-all duration-200'
                  )}
                  title="Cancel (N or Esc)"
                >
                  <X className="h-7 w-7" />
                </button>
                <button
                  onClick={handleConfirm}
                  disabled={isLoading || loading}
                  className={classNames(
                    'flex items-center justify-center w-14 h-14 aspect-square flex-shrink-0 rounded-xl shadow-md',
                    'bg-blue-600 text-white hover:bg-blue-700',
                    'focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-blue-500',
                    'disabled:opacity-50 disabled:cursor-not-allowed transition-all duration-200'
                  )}
                  title="Confirm (Y)"
                >
                  {isLoading || loading ? (
                    <LoadingSpinner size="md" color="white" />
                  ) : (
                    <Check className="h-7 w-7" />
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
          'flex items-stretch gap-0 transition-all duration-300',
          (isAnimatingRobot || isAnimatingBubble) ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-4'
        )}>
          <div className="flex items-center justify-center flex-shrink-0 w-16 bg-blue-600 rounded-full shadow-lg">
            <Bot className="h-9 w-9 text-white" />
          </div>

          <div className="flex-1 ml-3 bg-white rounded-lg shadow-lg border border-gray-200 px-4 py-3 flex flex-col justify-center min-w-0">
            <p className="text-sm font-semibold text-gray-900 truncate">
              {title}
            </p>
            <p className="text-sm text-gray-700 truncate">
              {truncateText(message || '', 50)}
            </p>
          </div>

          <div className="flex gap-2 ml-3 h-full">
            <button
              onClick={handleCancel}
              disabled={isLoading || loading}
              className={classNames(
                'flex items-center justify-center w-16 h-16 aspect-square flex-shrink-0 rounded-lg shadow-lg',
                'bg-white border border-gray-200 text-gray-500 hover:bg-gray-50 hover:text-red-500',
                'disabled:opacity-50 disabled:cursor-not-allowed transition-all'
              )}
              title="Cancel (N)"
            >
              <X className="h-7 w-7" />
            </button>
            <button
              onClick={handleConfirm}
              disabled={isLoading || loading}
              className={classNames(
                'flex items-center justify-center w-16 h-16 aspect-square flex-shrink-0 rounded-lg shadow-lg',
                'bg-blue-600 text-white hover:bg-blue-700',
                'disabled:opacity-50 disabled:cursor-not-allowed transition-all'
              )}
              title="Confirm (Y)"
            >
              {isLoading || loading ? (
                <LoadingSpinner size="md" color="white" />
              ) : (
                <Check className="h-7 w-7" />
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
