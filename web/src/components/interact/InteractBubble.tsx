import React, { useState, useRef, useEffect } from 'react';
import { Bot, Send } from 'lucide-react';
import { classNames } from '@/utils/helpers';

interface InteractBubbleProps {
  onSubmit: (message: string) => void;
  isThinking: boolean;
  disabled?: boolean;
  onRobotClick?: () => void;
  autoFocus?: boolean;
}

export const InteractBubble: React.FC<InteractBubbleProps> = ({
  onSubmit,
  isThinking,
  disabled = false,
  onRobotClick,
  autoFocus = false
}) => {
  const [message, setMessage] = useState('');
  const [isComposing, setIsComposing] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const placeholders = [
    'リマインダが欲しい',
    'ホテルを予約したい',
    '毎朝メールをチェックしたい',
    'キューシステムが必要'
  ];

  const [placeholderIndex, setPlaceholderIndex] = useState(0);

  // Auto-focus input when it becomes visible on mobile or requested
  useEffect(() => {
    if (autoFocus || (window.innerWidth < 1024 && inputRef.current)) {
      const timer = setTimeout(() => {
        inputRef.current?.focus();
      }, 300); // Wait for slide-in animation
      return () => clearTimeout(timer);
    }
  }, [autoFocus]);
  useEffect(() => {
    const interval = setInterval(() => {
      setPlaceholderIndex((prev) => (prev + 1) % placeholders.length);
    }, 3000);
    return () => clearInterval(interval);
  }, []);

  const handleSubmit = () => {
    // Allow submission even when thinking (for parallel draft creation)
    if (message.trim() && !disabled) {
      onSubmit(message.trim());
      setMessage('');
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    // Handle Escape key to unfocus input
    if (e.key === 'Escape') {
      e.preventDefault();
      console.log('[InteractBubble] Escape pressed, blurring input');
      inputRef.current?.blur();
      return;
    }

    // Ignore Enter key press during IME composition (Japanese input)
    if (e.key === 'Enter' && !e.shiftKey && !isComposing) {
      e.preventDefault();
      handleSubmit();
    }
  };

  const handleCompositionStart = () => {
    setIsComposing(true);
  };

  const handleCompositionEnd = () => {
    setIsComposing(false);
  };

  return (
    <div className="inline-flex items-center gap-2">
      {/* Robot Icon */}
      <div
        className="flex items-center justify-center h-10 w-10 rounded-full bg-blue-600 shadow-lg flex-shrink-0 cursor-pointer hover:bg-blue-700 transition-colors"
        onClick={onRobotClick}
      >
        <Bot className="h-6 w-6 text-white" />
      </div>

      {/* Speech Bubble */}
      <div className={classNames(
        'relative flex items-center gap-2 px-4 py-2 bg-white dark:bg-gray-800 rounded-2xl shadow-lg',
        'border-2 transition-all duration-200',
        isThinking ? 'border-blue-400 bg-blue-50 dark:bg-blue-900/20' : 'border-blue-500 dark:border-blue-600'
      )}>
        {/* Triangle pointer (left side) */}
        <div className="absolute left-0 top-1/2 -translate-y-1/2 -translate-x-[7px]">
          <div className="w-0 h-0 border-t-[6px] border-t-transparent border-b-[6px] border-b-transparent border-r-[8px] border-r-blue-500" />
          <div className="absolute top-[1px] left-[2px] w-0 h-0 border-t-[5px] border-t-transparent border-b-[5px] border-b-transparent border-r-[6px] border-r-white dark:border-r-gray-800" />
        </div>

        {/* Always show input field for parallel draft creation */}
        <>
          {isThinking && (
            <span className="text-gray-600 dark:text-gray-400 text-xs animate-pulse mr-2">
              処理中...
            </span>
          )}
          <input
            ref={inputRef}
            data-interact-input
            type="text"
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            onKeyDown={handleKeyDown}
            onCompositionStart={handleCompositionStart}
            onCompositionEnd={handleCompositionEnd}
            placeholder={placeholders[placeholderIndex]}
            disabled={disabled}
            className={classNames(
              'bg-transparent border-none outline-none',
              'text-base text-gray-900 dark:text-gray-100 placeholder-gray-400',
              'w-64 focus:ring-0'
            )}
          />
          {message && (
            <button
              onClick={handleSubmit}
              disabled={disabled}
              className="text-blue-600 hover:text-blue-700 disabled:opacity-50"
            >
              <Send className="h-4 w-4" />
            </button>
          )}
        </>
      </div>
    </div>
  );
};
