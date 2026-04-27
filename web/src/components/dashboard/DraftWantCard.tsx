import React, { useRef, useEffect } from 'react';
import { Want } from '@/types/want';
import { WantCardContent } from './WantCardContent';
import { classNames } from '@/utils/helpers';
import { getBackgroundStyle } from '@/utils/backgroundStyles';
import { FocusTriangle } from './WantCard/parts/FocusTriangle';
import { ProgressBars } from './WantCard/parts/ProgressBars';
import { CARD_BORDER_BASE, CARD_FOCUS_BASE } from './WantCard/hooks/cardStyles';
import { Trash2 } from 'lucide-react';

interface DraftWantCardProps {
  want: Want;
  selected: boolean;
  onClick: () => void;
  onDelete: () => void;
}

export const DraftWantCard: React.FC<DraftWantCardProps> = ({
  want,
  selected,
  onClick,
  onDelete,
}) => {
  const cardRef = useRef<HTMLDivElement>(null);
  const wantId = want.metadata?.id || want.id;

  useEffect(() => {
    if (selected && document.activeElement !== cardRef.current) {
      cardRef.current?.focus();
    }
  }, [selected]);

  const current = want.state?.current || {};
  const phase = (current.phase as string) || '';
  const isThinking = (current.isThinking as boolean) ||
    phase === 'ideating' || phase === 'decomposing' || phase === 're_planning';
  const recommendations = (current.proposed_recommendations as any[]) ||
    (current.recommendations as any[]) || [];
  const hasIdeas = recommendations.length > 0;
  const error = current.error as string | undefined;

  const achievingPercentage = (want.state?.current?.achieving_percentage as number) ??
    (isThinking && !hasIdeas ? 10 : 100);

  const bgStyle = getBackgroundStyle(want.metadata?.type, false);

  return (
    <div className="relative h-full" style={{ isolation: 'isolate' }}>
      <FocusTriangle visible={selected} />
      <div
        ref={cardRef}
        onClick={onClick}
        tabIndex={0}
        data-keyboard-nav-selected={selected}
        data-keyboard-nav-id={wantId}
        className={classNames(
          `card hover:shadow-md dark:hover:shadow-blue-900/20 transition-all duration-300 cursor-pointer group relative overflow-hidden h-full min-h-[6rem] sm:min-h-[10rem] flex flex-col ${CARD_FOCUS_BASE}`,
          CARD_BORDER_BASE,
          'border-dashed border-2',
          selected
            ? 'border-blue-400 shadow-blue-100 dark:shadow-blue-900/20'
            : error
            ? 'border-red-300 dark:border-red-700'
            : '',
          bgStyle.className,
        )}
        style={bgStyle.style}
      >
        <ProgressBars achievingPercentage={achievingPercentage} />

        <div
          className="relative z-10 transition-all duration-150 flex-1"
        >
          <WantCardContent
            want={want}
            isChild={false}
            onView={onClick}
          />
        </div>

        {/* Draft-specific bottom bar */}
        <div className="relative z-10 mt-auto border-t border-gray-100/50 dark:border-gray-700/50 px-3 py-1.5 min-h-[32px] flex items-center justify-between">
          <div className="flex items-center gap-2 flex-1 min-w-0">
            {isThinking ? (
              <>
                <div className="flex gap-1 flex-shrink-0">
                  <span className="w-1.5 h-1.5 bg-blue-500 rounded-full animate-bounce" style={{ animationDelay: '0ms' }} />
                  <span className="w-1.5 h-1.5 bg-blue-500 rounded-full animate-bounce" style={{ animationDelay: '150ms' }} />
                  <span className="w-1.5 h-1.5 bg-blue-500 rounded-full animate-bounce" style={{ animationDelay: '300ms' }} />
                </div>
                <span className="text-[10px] sm:text-xs text-blue-500 font-medium animate-pulse truncate">
                  {hasIdeas ? '選択を待機中...' : 'アイディアを拡張中...'}
                </span>
              </>
            ) : error ? (
              <p className="text-[10px] sm:text-xs text-red-600 font-medium truncate">詳細を確認して再試行してください</p>
            ) : (
              <span className="text-[10px] sm:text-xs text-green-600 font-bold bg-green-50 dark:bg-green-900/20 px-2 py-0.5 rounded-lg">
                結晶化完了
              </span>
            )}
          </div>

          <button
            onClick={(e) => { e.stopPropagation(); onDelete(); }}
            className="flex-shrink-0 p-1 ml-1 text-gray-400 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-900/30 rounded transition-all"
            title="Delete Draft"
          >
            <Trash2 className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>
    </div>
  );
};
