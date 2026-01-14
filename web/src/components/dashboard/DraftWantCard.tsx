import React from 'react';
import { Bot, Trash2 } from 'lucide-react';
import { DraftWant } from '@/types/draft';
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
  return (
    <div
      className={classNames(
        'relative rounded-lg transition-all duration-200 cursor-pointer',
        'border-2 bg-white hover:shadow-lg',
        draft.error ? 'border-red-300 bg-red-50' : (
          selected
            ? 'border-blue-500 shadow-lg ring-2 ring-blue-200'
            : 'border-blue-200 hover:border-blue-300'
        )
      )}
      onClick={onClick}
    >
      {/* Header */}
      <div className={classNames(
        'p-4 border-b rounded-t-lg',
        draft.error ? 'border-red-100 bg-red-100' : 'border-blue-100 bg-blue-50'
      )}>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3 flex-1 min-w-0">
            <div className={classNames(
              "flex items-center justify-center h-10 w-10 rounded-full shadow-md flex-shrink-0",
              draft.error ? "bg-red-600" : "bg-blue-600"
            )}>
              <Bot className="h-6 w-6 text-white" />
            </div>
            <div className="flex-1 min-w-0">
              <h3 className="text-lg font-semibold text-gray-900 truncate">
                {draft.message}
              </h3>
              <p className={classNames(
                "text-sm mt-0.5",
                draft.error ? "text-red-700 font-medium" : "text-gray-600"
              )}>
                {draft.isThinking ? "ドラフトを作成中..." : (draft.error ? "エラーが発生しました" : "レコメンデーション準備完了")}
              </p>
            </div>
          </div>

          {/* Delete Button */}
          <button
            onClick={(e) => {
              e.stopPropagation();
              onDelete();
            }}
            className="p-2 text-red-600 hover:text-red-700 hover:bg-red-50 rounded-lg transition-colors flex-shrink-0"
            title="削除"
          >
            <Trash2 className="h-5 w-5" />
          </button>
        </div>
      </div>

      {/* Body - Thinking / Error / Success State */}
      <div className="p-6">
        {draft.isThinking ? (
          <div className="flex items-center justify-center gap-3">
            <div className="flex items-center justify-center h-8 w-8 rounded-full bg-blue-600">
              <Bot className="h-5 w-5 text-white" />
            </div>
            <div className="relative flex items-center gap-2 px-4 py-2 bg-blue-50 rounded-2xl border-2 border-blue-400">
              <span className="text-gray-700 text-sm font-medium animate-pulse">
                Thinking<span className="inline-block w-3">...</span>
              </span>
            </div>
          </div>
        ) : draft.error ? (
          <div className="text-center">
            <p className="text-sm text-red-600 line-clamp-2">
              {draft.error}
            </p>
            <button
              onClick={(e) => {
                e.stopPropagation();
                onClick();
              }}
              className="mt-2 text-xs font-bold text-red-700 hover:underline"
            >
              再試行または詳細を確認
            </button>
          </div>
        ) : draft.recommendations.length > 0 ? (
          <div className="text-center text-gray-600">
            <p className="text-sm font-medium">
              {draft.recommendations.length}件の推奨案があります
            </p>
            <p className="text-xs text-gray-500 mt-1">
              クリックして内容を確認・デプロイ
            </p>
          </div>
        ) : (
          <div className="text-center text-gray-500">
            <p className="text-sm">推奨案が見つかりませんでした</p>
          </div>
        )}
      </div>

      {/* Selected Indicator */}
      {selected && !draft.error && (
        <div className="absolute top-2 right-2 w-3 h-3 bg-blue-500 rounded-full shadow-md"></div>
      )}
    </div>
  );
};
