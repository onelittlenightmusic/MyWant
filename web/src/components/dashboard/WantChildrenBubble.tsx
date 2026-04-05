import React, { useState, useEffect } from 'react';
import { X, ChevronRight, Layers } from 'lucide-react';
import { Want } from '@/types/want';
import { WantCard } from './WantCard/WantCard';
import { StatusBadge } from '@/components/common/StatusBadge';
import { classNames } from '@/utils/helpers';
import { getBackgroundStyle } from '@/utils/backgroundStyles';
import { ProgressBars } from './WantCard/parts/ProgressBars';

interface WantChildrenBubbleProps {
  parentWant: Want;
  childWants: Want[];
  allWants: Want[];
  expandedChain: Want[]; // [nextExpandedChild, deeper...]
  selectedWant: Want | null;
  onChildClick: (want: Want) => void;
  onViewAgents?: (want: Want) => void;
  onViewResults?: (want: Want) => void;
  onViewChat?: (want: Want) => void;
  onEditWant: (want: Want) => void;
  onDeleteWant: (want: Want) => void;
  onSuspendWant?: (want: Want) => void;
  onResumeWant?: (want: Want) => void;
  onShowReactionConfirmation?: (want: Want, action: 'approve' | 'deny') => void;
  onClose: () => void;
  depth?: number;
  parentIndex?: number;
}

export const WantChildrenBubble: React.FC<WantChildrenBubbleProps> = ({
  parentWant,
  childWants,
  allWants,
  expandedChain,
  selectedWant,
  onChildClick,
  onViewAgents,
  onViewResults,
  onViewChat,
  onEditWant,
  onDeleteWant,
  onSuspendWant,
  onResumeWant,
  onShowReactionConfirmation,
  onClose,
  depth = 0,
  parentIndex = 0,
}) => {
  const parentName = parentWant.metadata?.name || parentWant.metadata?.type || 'Want';
  const parentType = parentWant.metadata?.type || 'unknown';

  // State from the parent want
  const currentState = parentWant.state?.current;
  const achievingPercentage = (currentState?.achieving_percentage as number) ?? 0;
  const replayScreenshotUrl = currentState?.replay_screenshot_url as string | undefined;

  // Simplified header info
  const wantText = (parentWant.spec?.params?.want as string) || '';
  const goalText = (currentState?.goal_text as string) || '';

  const nextExpandedId = expandedChain[0]?.metadata?.id || expandedChain[0]?.id;

  const backgroundStyle = getBackgroundStyle(parentWant.metadata?.type, false);

  // Detect current grid column count based on breakpoints (1 / 2 / 3)
  const getColumns = () => {
    if (typeof window === 'undefined') return 1;
    if (window.matchMedia('(min-width: 1024px)').matches) return 3;
    if (window.matchMedia('(min-width: 640px)').matches) return 2;
    return 1;
  };
  const [columns, setColumns] = useState(getColumns);
  useEffect(() => {
    const update = () => setColumns(getColumns());
    window.addEventListener('resize', update);
    return () => window.removeEventListener('resize', update);
  }, []);

  // Caret left = center of parent card's column
  const col = parentIndex % columns;
  const caretLeft = `calc(${(col / columns) * 100}% + ${(1 / columns / 2) * 100}% - 10px)`;

  return (
    <div
      className={classNames(
        'col-span-full',
        'relative mt-1 mb-4',
      )}
    >
      {/* Speech bubble caret pointing up — overflow-hidden clips to top triangle only */}
      <div
        className="absolute overflow-hidden z-10 transition-[left] duration-300"
        style={{ left: `calc(${caretLeft} - 4px)`, top: '-14px', width: '28px', height: '14px' }}
      >
        <div
          className="absolute w-5 h-5 rotate-45 border-l border-t border-blue-200 dark:border-blue-700 bg-white/40 dark:bg-gray-900/40 backdrop-blur-sm"
          style={{ left: '4px', top: '4px' }}
        />
      </div>

      {/* Bubble container */}
      <div
        className={classNames(
          'relative rounded-xl border border-blue-200 dark:border-blue-700',
          'shadow-lg overflow-hidden transition-all duration-300',
          depth === 0 ? 'animate-slide-down' : '',
          backgroundStyle.className
        )}
        style={{ ...backgroundStyle.style, zIndex: 0 }}
      >
        <ProgressBars achievingPercentage={achievingPercentage} />

        {replayScreenshotUrl && (
          <div
            className="absolute inset-0 z-0 pointer-events-none"
            style={{
              backgroundImage: `url(${replayScreenshotUrl})`,
              backgroundSize: 'cover',
              backgroundPosition: 'center top',
              opacity: 0.12,
            }}
          />
        )}

        {/* Bubble header: parent want info */}
        <div className="relative z-10 flex items-start gap-3 px-4 py-3 bg-white/40 dark:bg-gray-900/40 border-b border-blue-100/50 dark:border-blue-800/50 backdrop-blur-sm">
          <div className="flex-1 min-w-0">
            {/* Principal info: want and goal_text */}
            <div className="flex flex-col gap-1">
              {wantText ? (
                <div className="flex items-start justify-between gap-3">
                  <div className="flex items-start gap-2 min-w-0">
                    <Layers className="h-4 w-4 text-blue-500 mt-0.5 flex-shrink-0" />
                    <p className="text-sm font-semibold text-gray-900 dark:text-white leading-tight">
                      {wantText}
                    </p>
                  </div>
                  <StatusBadge status={parentWant.status} size="sm" className="flex-shrink-0" />
                </div>
              ) : (
                <div className="flex items-center justify-between gap-3">
                  <div className="flex items-center gap-2 min-w-0">
                    <Layers className="h-4 w-4 text-blue-500 flex-shrink-0" />
                    <span className="font-semibold text-gray-900 dark:text-white text-sm truncate">
                      {parentName}
                    </span>
                  </div>
                  <StatusBadge status={parentWant.status} size="sm" className="flex-shrink-0" />
                </div>
              )}
              
              {goalText && goalText !== wantText && (
                <p className="text-xs text-gray-600 dark:text-gray-400 italic pl-6 line-clamp-2">
                  {goalText}
                </p>
              )}
            </div>
          </div>

          <button
            onClick={onClose}
            className="p-1.5 rounded-full hover:bg-white/50 dark:hover:bg-gray-800/50 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 transition-colors flex-shrink-0 border border-transparent hover:border-gray-200 dark:hover:border-gray-700"
            title="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Child want cards grid */}
        <div className="relative z-10 p-3 bg-white/10 dark:bg-black/5">
          {childWants.length === 0 ? (
            <div className="flex items-center justify-center h-24 text-sm text-gray-400 dark:text-gray-500 bg-white/20 dark:bg-gray-900/20 rounded-lg border border-dashed border-gray-300 dark:border-gray-700">
              No child wants yet
            </div>
          ) : (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
              {childWants.map((child, index) => {
                const childId = child.metadata?.id || child.id;
                const isSelected = selectedWant?.metadata?.id === childId || selectedWant?.id === childId;
                const childChildren = allWants.filter(w =>
                  w.metadata?.ownerReferences?.some(ref => ref.id === childId)
                );
                const isExpanded = nextExpandedId === childId;

                return (
                  <React.Fragment key={childId}>
                    <div className={classNames(isExpanded ? 'ring-2 ring-blue-400 ring-offset-2 dark:ring-offset-gray-900 rounded-lg transition-all duration-300' : 'transition-all duration-300')}>
                      <WantCard
                        index={index}
                        want={child}
                        children={childChildren}
                        selected={isSelected}
                        selectedWant={selectedWant}
                        onView={onChildClick}
                        onViewAgents={onViewAgents || (() => {})}
                        onViewResults={onViewResults || (() => {})}
                        onViewChat={onViewChat || (() => {})}
                        onEdit={onEditWant}
                        onDelete={onDeleteWant}
                        onSuspend={onSuspendWant}
                        onResume={onResumeWant}
                        onShowReactionConfirmation={onShowReactionConfirmation}
                      />
                    </div>

                    {/* Cascading bubble for this child if it's expanded */}
                    {isExpanded && childChildren.length > 0 && (
                      <WantChildrenBubble
                        parentWant={child}
                        childWants={childChildren}
                        allWants={allWants}
                        expandedChain={expandedChain.slice(1)}
                        selectedWant={selectedWant}
                        onChildClick={onChildClick}
                        onViewAgents={onViewAgents}
                        onViewResults={onViewResults}
                        onViewChat={onViewChat}
                        onEditWant={onEditWant}
                        onDeleteWant={onDeleteWant}
                        onSuspendWant={onSuspendWant}
                        onResumeWant={onResumeWant}
                        onShowReactionConfirmation={onShowReactionConfirmation}
                        onClose={onClose}
                        depth={depth + 1}
                        parentIndex={index}
                      />
                    )}
                  </React.Fragment>
                );
              })}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
