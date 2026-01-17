import React from 'react';
import { DraftWant } from '@/types/draft';
import { Want } from '@/types/want';
import { WantCardContent } from './WantCardContent';
import { classNames } from '@/utils/helpers';

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
  // Convert DraftWant to a partial Want object for WantCardContent
  const pseudoWant: Want = {
    id: draft.id,
    metadata: {
      id: draft.id,
      name: draft.message,
      type: 'draft',
      labels: { '__draft': 'true' }
    },
    spec: {
      params: {}
    },
    status: draft.isThinking ? 'reaching' : (draft.error ? 'failed' : 'created'),
    state: {
      achieving_percentage: draft.isThinking ? 10 : 100,
      error: draft.error
    }
  };

  // Match WantCard's progress bar styles
  const achievingPercentage = draft.isThinking ? 10 : 100;
  
  const whiteProgressBarStyle = {
    position: 'absolute' as const,
    top: 0,
    left: 0,
    height: '100%',
    width: `${achievingPercentage}%`,
    background: 'rgba(255, 255, 255, 0.5)',
    transition: 'width 0.3s ease-out',
    zIndex: 0,
    pointerEvents: 'none' as const
  };

  const blackOverlayStyle = {
    position: 'absolute' as const,
    top: 0,
    left: `${achievingPercentage}%`,
    height: '100%',
    width: `${100 - achievingPercentage}%`,
    background: 'rgba(0, 0, 0, 0.1)',
    transition: 'width 0.3s ease-out, left 0.3s ease-out',
    zIndex: 0,
    pointerEvents: 'none' as const
  };

  return (
    <div
      onClick={onClick}
      className={classNames(
        'card hover:shadow-md transition-all duration-300 cursor-pointer group relative overflow-hidden h-full min-h-[200px] flex flex-col focus:outline-none focus:ring-2 focus:ring-blue-400 focus:ring-inset',
        selected ? 'border-blue-500 border-2' : 'border-gray-200',
        draft.error ? 'bg-red-50' : 'bg-white'
      )}
    >
      {/* Progress bar effect to match WantCard */}
      <div style={whiteProgressBarStyle}></div>
      <div style={blackOverlayStyle}></div>

      {/* Content using the SAME component as regular WantCard */}
      <div className="relative z-10 flex-1">
        <WantCardContent
          want={pseudoWant}
          isChild={false}
          onView={() => onClick()}
          onDelete={() => onDelete()}
        />
      </div>

      {/* Draft-specific status information at the bottom area - pushing it to the bottom */}
      <div className="relative z-10 mt-auto border-t border-gray-100 pt-4">
          {draft.isThinking ? (
            <div className="flex items-center gap-2">
              <div className="flex gap-1">
                <span className="w-1.5 h-1.5 bg-blue-600 rounded-full animate-bounce" style={{ animationDelay: '0ms' }}></span>
                <span className="w-1.5 h-1.5 bg-blue-600 rounded-full animate-bounce" style={{ animationDelay: '150ms' }}></span>
                <span className="w-1.5 h-1.5 bg-blue-600 rounded-full animate-bounce" style={{ animationDelay: '300ms' }}></span>
              </div>
              <span className="text-xs text-blue-600 font-medium animate-pulse">AI思考中...</span>
            </div>
          ) : draft.error ? (
            <p className="text-xs text-red-600 font-medium">詳細を確認して再試行してください</p>
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
  );
};
