import React from 'react';
import { Bot, Trash2 } from 'lucide-react';
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
  // Styles to match WantCardContent exactly
  const titleClass = 'text-lg font-semibold';
  const typeClass = 'text-sm';
  const iconSize = 'h-4 w-4';
  const statusSize = 'sm';
  const textTruncate = 30;

  const progress = draft.isThinking ? 10 : 100;
  
  const whiteProgressBarStyle = {
    position: 'absolute' as const,
    top: 0,
    left: 0,
    height: '100%',
    width: `${progress}%`,
    background: 'rgba(255, 255, 255, 0.5)',
    transition: 'width 0.3s ease-out',
    zIndex: 0,
    pointerEvents: 'none' as const
  };

  const blackOverlayStyle = {
    position: 'absolute' as const,
    top: 0,
    left: `${progress}%`,
    height: '100%',
    width: `${100 - progress}%`,
    background: 'rgba(0, 0, 0, 0.1)',
    transition: 'width 0.3s ease-out, left 0.3s ease-out',
    zIndex: 0,
    pointerEvents: 'none' as const
  };

  const status = draft.isThinking ? 'reaching' : (draft.error ? 'failed' : 'created');

  return (
    <div
      onClick={onClick}
      className={classNames(
        'card hover:shadow-md transition-all duration-300 cursor-pointer group relative overflow-hidden min-h-[200px] focus:outline-none focus:ring-2 focus:ring-blue-400 focus:ring-inset',
        selected ? 'border-blue-500 border-2' : 'border-gray-200',
        draft.error ? 'bg-red-50' : 'bg-white'
      )}
    >
      {/* Progress bar effect to match WantCard */}
      <div style={whiteProgressBarStyle}></div>
      <div style={blackOverlayStyle}></div>

      {/* Main Content Container - Matches WantCardContent structure */}
      <div className="relative z-10 h-full flex flex-col p-6">
        {/* Header - Replicating WantCardContent layout but with an added delete button */}
        <div className="mb-4">
          <div className="flex items-start justify-between">
            <div className="flex-1 min-w-0">
              <h3 className={`${titleClass} text-gray-900 truncate group-hover:text-primary-600 transition-colors`}>
                draft
              </h3>
              <p className={`${typeClass} text-gray-500 mt-1 truncate`}>
                {truncateText(draft.message, textTruncate)}
              </p>
            </div>

            <div className="flex items-center space-x-2 ml-2">
              {/* Agent indicator placeholder */}
              <div className="flex items-center space-x-1 p-1 rounded-md bg-blue-50 transition-colors">
                <Bot className={`${iconSize} text-blue-600`} />
                {draft.isThinking && (
                  <div className={`w-2 h-2 bg-blue-500 rounded-full ${styles.pulseGlow}`} title="AI thinking" />
                )}
              </div>

              {/* Status Badge */}
              <div className="hover:opacity-80 transition-opacity">
                <StatusBadge status={status} size={statusSize} />
              </div>

              {/* Explicit Delete Button - Re-added for Drafts */}
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  onDelete();
                }}
                className="p-1.5 text-gray-400 hover:text-red-600 hover:bg-red-50 rounded-md transition-all"
                title="ドラフトを削除"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            </div>
          </div>
        </div>

        {/* Draft-specific status information at the bottom area - pushing it to the bottom */}
        <div className="mt-auto border-t border-gray-100 pt-4">
          {draft.isThinking ? (
            <div className="flex flex-col items-center justify-center space-y-2">
              <div className="flex items-center gap-2">
                <div className="flex gap-1">
                  <span className="w-1.5 h-1.5 bg-blue-600 rounded-full animate-bounce" style={{ animationDelay: '0ms' }}></span>
                  <span className="w-1.5 h-1.5 bg-blue-600 rounded-full animate-bounce" style={{ animationDelay: '150ms' }}></span>
                  <span className="w-1.5 h-1.5 bg-blue-600 rounded-full animate-bounce" style={{ animationDelay: '300ms' }}></span>
                </div>
                <span className="text-xs text-blue-600 font-medium animate-pulse uppercase tracking-wider">AI Thinking...</span>
              </div>
              <p className="text-[10px] text-gray-400">生成には通常30〜60秒かかります</p>
            </div>
          ) : draft.error ? (
            <div className="p-2 bg-red-100 border border-red-200 rounded-md">
              <p className="text-xs font-bold text-red-800 uppercase tracking-wider mb-1">Error</p>
              <p className="text-xs text-red-600 line-clamp-1">{draft.error}</p>
            </div>
          ) : (
            <div className="flex items-center justify-between">
              <span className="text-xs text-green-600 font-bold bg-green-50 px-2 py-1 rounded">
                {draft.recommendations.length}件の提案あり
              </span>
              <span className="text-[10px] text-gray-400 font-mono">
                {new Date(draft.createdAt).toLocaleTimeString()}
              </span>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
