import React from 'react';
import { Bot, Trash2, Clock } from 'lucide-react';
import { DraftWant } from '@/types/draft';
import { StatusBadge } from '@/components/common/StatusBadge';
import { classNames, truncateText } from '@/utils/helpers';
import styles from './WantCard.module.css';

interface DraftWantCardProps {
  draft: DraftWant;
  selected: boolean;
  onClick: () => void;
  onDelete: () => void;
}

export const DraftWantCard: React.FC<DraftWantCardProps> = ({
  draft,
  selected,
  onClick,
  onDelete
}) => {
  // Constants to match WantCard styling
  const titleClass = 'text-lg font-semibold';
  const typeClass = 'text-sm';
  const iconSize = 'h-4 w-4';
  const statusSize = 'sm';
  const textTruncate = 30;

  // Background style for draft (distinctive but matching)
  const backgroundStyle = {
    backgroundColor: '#ffffff',
    borderStyle: 'dashed',
    borderWidth: '2px'
  };

  // Progress bar logic - drafts are always "thinking" or "waiting for user"
  // We'll use 10% for thinking and 100% for success/error to match WantCard visual
  const progress = draft.isThinking ? 10 : 100;

  const progressBarStyle = {
    position: 'absolute' as const,
    top: 0,
    left: 0,
    height: '100%',
    width: `${progress}%`,
    background: draft.error ? 'rgba(239, 68, 68, 0.1)' : 'rgba(59, 130, 246, 0.1)',
    transition: 'width 0.3s ease-out',
    zIndex: 0,
    pointerEvents: 'none' as const
  };

  const overlayStyle = {
    position: 'absolute' as const,
    top: 0,
    left: `${progress}%`,
    height: '100%',
    width: `${100 - progress}%`,
    background: 'rgba(0, 0, 0, 0.02)',
    transition: 'width 0.3s ease-out, left 0.3s ease-out',
    zIndex: 0,
    pointerEvents: 'none' as const
  };

  return (
    <div
      onClick={onClick}
      className={classNames(
        'card hover:shadow-md transition-all duration-300 cursor-pointer group relative overflow-hidden min-h-[200px] focus:outline-none focus:ring-2 focus:ring-blue-400 focus:ring-inset',
        selected ? 'border-blue-500 border-2' : (draft.error ? 'border-red-300' : 'border-blue-200'),
        draft.error ? 'bg-red-50' : 'bg-white'
      )}
      style={backgroundStyle}
    >
      {/* Progress bar effect to match WantCard */}
      <div style={progressBarStyle}></div>
      <div style={overlayStyle}></div>

      {/* Main Content Container */}
      <div className="relative z-10 p-6">
        {/* Header */}
        <div className="mb-4">
          <div className="flex items-start justify-between">
            <div className="flex-1 min-w-0">
              <h3 className={`${titleClass} text-gray-900 truncate group-hover:text-blue-600 transition-colors`}>
                draft
              </h3>
              <p className={`${typeClass} text-gray-500 mt-1 truncate`}>
                {truncateText(draft.message, textTruncate)}
              </p>
            </div>

            <div className="flex items-center space-x-2 ml-2">
              <div className="flex items-center space-x-1 p-1 rounded-md bg-blue-50 transition-colors">
                <Bot className={`${iconSize} text-blue-600`} />
                {draft.isThinking && (
                  <div className={`w-2 h-2 bg-blue-500 rounded-full ${styles.pulseGlow}`} title="AI thinking" />
                )}
              </div>

              {/* Status Badge */}
              <div className="hover:opacity-80 transition-opacity">
                <StatusBadge 
                  status={draft.isThinking ? 'reaching' : (draft.error ? 'failed' : 'created')} 
                  size={statusSize} 
                />
              </div>

              {/* Delete Button */}
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  onDelete();
                }}
                className="p-1.5 text-gray-400 hover:text-red-600 hover:bg-red-50 rounded-md transition-all"
                title="削除"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            </div>
          </div>
        </div>

        {/* Body Content */}
        <div className="mt-4">
          {draft.isThinking ? (
            <div className="flex flex-col items-center justify-center py-4 space-y-3">
              <div className="flex items-center gap-2 px-4 py-2 bg-blue-50 rounded-full border border-blue-200">
                <span className="text-blue-700 text-xs font-bold uppercase tracking-wider animate-pulse">
                  AI Thinking
                </span>
                <span className="flex gap-1">
                  <span className="w-1 h-1 bg-blue-600 rounded-full animate-bounce" style={{ animationDelay: '0ms' }}></span>
                  <span className="w-1 h-1 bg-blue-600 rounded-full animate-bounce" style={{ animationDelay: '150ms' }}></span>
                  <span className="w-1 h-1 bg-blue-600 rounded-full animate-bounce" style={{ animationDelay: '300ms' }}></span>
                </span>
              </div>
              <p className="text-xs text-gray-500 text-center">生成には通常30〜60秒かかります</p>
            </div>
          ) : draft.error ? (
            <div className="p-3 bg-red-100 border border-red-200 rounded-md">
              <p className="text-xs font-bold text-red-800 uppercase tracking-wider mb-1">Error</p>
              <p className="text-xs text-red-600 line-clamp-2">{draft.error}</p>
              <button className="text-[10px] text-red-700 font-bold hover:underline mt-2">
                RETRY / VIEW DETAILS →
              </button>
            </div>
          ) : draft.recommendations.length > 0 ? (
            <div className="flex flex-col items-center justify-center py-2 space-y-2">
              <div className="flex items-center gap-2 px-4 py-2 bg-green-50 rounded-full border border-green-200 shadow-sm">
                <span className="text-green-700 text-sm font-bold">
                  {draft.recommendations.length} Recommendations
                </span>
              </div>
              <p className="text-xs text-gray-500">クリックして推奨案を確認</p>
            </div>
          ) : (
            <div className="flex flex-col items-center justify-center py-4 text-gray-400">
              <Clock className="h-8 w-8 mb-2 opacity-20" />
              <p className="text-xs">No recommendations found</p>
            </div>
          )}
        </div>

        {/* Created At - bottom aligned if space permits */}
        <div className="mt-6 flex justify-end">
           <span className="text-[10px] text-gray-400 font-mono">
             {new Date(draft.createdAt).toLocaleTimeString()}
           </span>
        </div>
      </div>
    </div>
  );
};