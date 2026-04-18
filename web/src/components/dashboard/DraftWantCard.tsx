import React, { useState } from 'react';
import { DraftWant } from '@/types/draft';
import { Want } from '@/types/want';
import { WantCardContent } from './WantCardContent';
import { classNames } from '@/utils/helpers';
import { apiClient } from '@/api/client';

import { Trash2, Sparkles, Send } from 'lucide-react';

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
  const [isSelecting, setIsSelecting] = useState<string | null>(null);

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
      current: {
        achieving_percentage: draft.isThinking ? 10 : 100,
        error: draft.error
      }
    }
  };

  const handleSelectRecommendation = async (recId: string) => {
    setIsSelecting(recId);
    try {
      // Use specialized updateDraftWant which merges into state.current correctly
      await apiClient.updateDraftWant(draft.id, {
        selected_recommendation_id: recId
      });
    } catch (error) {
      console.error('Failed to select recommendation:', error);
    } finally {
      setIsSelecting(null);
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
    background: 'rgba(255, 255, 255, 0.4)',
    transition: 'width 0.3s ease-out',
    zIndex: 0,
    pointerEvents: 'none' as const
  };

  return (
    <div
      onClick={onClick}
      className={classNames(
        'card hover:shadow-lg transition-all duration-500 cursor-pointer group relative overflow-hidden h-full min-h-[220px] flex flex-col focus:outline-none focus:ring-2 focus:ring-blue-400 focus:ring-inset',
        'rounded-[2rem] border-dashed border-2', // Cloudy style
        selected ? 'border-blue-400 shadow-blue-100 dark:shadow-blue-900/20 bg-blue-50/30 dark:bg-blue-900/10' : 'border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800',
        draft.error ? 'bg-red-50 dark:bg-red-900/10 border-red-200' : ''
      )}
    >
      {/* Progress bar effect to match WantCard */}
      <div style={whiteProgressBarStyle}></div>

      {/* Background Decor (Cloud/Sparkle) */}
      <div className="absolute top-0 right-0 p-4 opacity-10 group-hover:opacity-30 transition-opacity">
        <Sparkles className="w-12 h-12 text-blue-400" />
      </div>

      {/* Content */}
      <div className="relative z-10 flex-1">
        <WantCardContent
          want={pseudoWant}
          isChild={false}
          onView={() => onClick()}
        />
        
        {/* Recommendations / Ideas Seeds */}
        {draft.recommendations.length > 0 && !draft.error && (
          <div className="px-4 pb-4 animate-in fade-in slide-in-from-bottom-2 duration-700">
            <p className="text-[10px] font-medium text-gray-400 dark:text-gray-500 mb-2 flex items-center gap-1">
              <Sparkles className="w-3 h-3" />
              アイディアの種を選んで具体化する
            </p>
            <div className="flex flex-wrap gap-2">
              {draft.recommendations.map((rec) => (
                <button
                  key={rec.id}
                  onClick={(e) => {
                    e.stopPropagation();
                    handleSelectRecommendation(rec.id);
                  }}
                  disabled={isSelecting !== null}
                  className={classNames(
                    'text-left px-3 py-2 rounded-xl text-xs transition-all border shadow-sm flex items-center justify-between group/seed',
                    isSelecting === rec.id 
                      ? 'bg-blue-100 border-blue-300 text-blue-700' 
                      : 'bg-white dark:bg-gray-700 border-gray-100 dark:border-gray-600 text-gray-700 dark:text-gray-200 hover:border-blue-300 hover:bg-blue-50 dark:hover:bg-blue-900/20'
                  )}
                >
                  <div className="flex-1">
                    <div className="font-semibold">{rec.title}</div>
                    {rec.description && (
                      <div className="text-[9px] opacity-60 line-clamp-1">{rec.description}</div>
                    )}
                  </div>
                  <Send className={classNames(
                    'w-3 h-3 ml-2 transition-transform',
                    isSelecting === rec.id ? 'translate-x-1 text-blue-500' : 'opacity-0 group-hover/seed:opacity-100'
                  )} />
                </button>
              ))}
            </div>
          </div>
        )}

        {/* Draft delete button */}
        <button
          onClick={(e) => {
            e.stopPropagation();
            onDelete();
          }}
          className="absolute top-2 right-2 p-1.5 text-gray-400 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-900/30 rounded-md transition-all z-20"
          title="Delete Draft"
        >
          <Trash2 className="w-4 h-4" />
        </button>
      </div>

      {/* Status Bar */}
      <div className="relative z-10 mt-auto border-t border-gray-100/50 dark:border-gray-700/50 px-3 sm:px-6 py-4">
          {draft.isThinking ? (
            <div className="flex items-center gap-2">
              <div className="flex gap-1">
                <span className="w-1.5 h-1.5 bg-blue-500 rounded-full animate-bounce" style={{ animationDelay: '0ms' }}></span>
                <span className="w-1.5 h-1.5 bg-blue-500 rounded-full animate-bounce" style={{ animationDelay: '150ms' }}></span>
                <span className="w-1.5 h-1.5 bg-blue-500 rounded-full animate-bounce" style={{ animationDelay: '300ms' }}></span>
              </div>
              <span className="text-xs text-blue-500 font-medium animate-pulse">
                {draft.recommendations.length > 0 ? '選択を待機中...' : 'アイディアを拡張中...'}
              </span>
            </div>
          ) : draft.error ? (
            <p className="text-xs text-red-600 font-medium">詳細を確認して再試行してください</p>
          ) : (
            <div className="flex items-center justify-between">
              <span className="text-xs text-green-600 font-bold bg-green-50 dark:bg-green-900/20 px-2 py-1 rounded-lg">
                結晶化完了
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
