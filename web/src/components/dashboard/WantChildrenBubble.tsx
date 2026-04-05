import React from 'react';
import { X, ChevronRight, Layers } from 'lucide-react';
import { Want } from '@/types/want';
import { WantCard } from './WantCard/WantCard';
import { StatusBadge } from '@/components/common/StatusBadge';
import { classNames } from '@/utils/helpers';

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
}) => {
  const parentName = parentWant.metadata?.name || parentWant.metadata?.type || 'Want';
  const parentType = parentWant.metadata?.type || 'unknown';
  const parentId = parentWant.metadata?.id || parentWant.id;

  // Params from the parent want
  const params = parentWant.spec?.params as Record<string, any> | undefined;
  const paramEntries = params ? Object.entries(params).filter(([, v]) => v !== undefined && v !== '') : [];

  // State from the parent want
  const currentState = parentWant.state?.current;
  const stateEntries = currentState ? Object.entries(currentState).filter(([k]) => !k.startsWith('__')).slice(0, 4) : [];

  const nextExpandedId = expandedChain[0]?.metadata?.id || expandedChain[0]?.id;

  return (
    <div
      className={classNames(
        'col-span-full',
        'relative mt-1 mb-2',
      )}
    >
      {/* Speech bubble caret pointing up */}
      <div
        className="absolute -top-2.5 left-6 w-5 h-5 rotate-45 bg-blue-50 dark:bg-blue-950/60 border-l border-t border-blue-200 dark:border-blue-700"
        style={{ zIndex: 1 }}
      />

      {/* Bubble container */}
      <div
        className={classNames(
          'relative rounded-xl border border-blue-200 dark:border-blue-700',
          'bg-blue-50 dark:bg-blue-950/60',
          'shadow-lg overflow-hidden',
          depth === 0 ? 'animate-slide-down' : ''
        )}
        style={{ zIndex: 0 }}
      >
        {/* Bubble header: parent want info */}
        <div className="flex items-start gap-3 px-4 py-3 bg-white/60 dark:bg-gray-900/60 border-b border-blue-100 dark:border-blue-800">
          <div className="flex-1 min-w-0">
            {/* Name + type row */}
            <div className="flex items-center gap-2 flex-wrap">
              <Layers className="h-4 w-4 text-blue-500 flex-shrink-0" />
              <span className="font-semibold text-gray-900 dark:text-white text-sm truncate max-w-[200px]">
                {parentName}
              </span>
              <span className="text-xs text-gray-500 dark:text-gray-400 bg-gray-100 dark:bg-gray-800 px-1.5 py-0.5 rounded font-mono">
                {parentType}
              </span>
              <StatusBadge status={parentWant.status} size="sm" />
              <ChevronRight className="h-3 w-3 text-blue-400 flex-shrink-0" />
              <span className="text-xs text-blue-600 dark:text-blue-400 font-medium">
                {childWants.length} {childWants.length === 1 ? 'child want' : 'child wants'}
              </span>
            </div>

            {/* Params preview */}
            {paramEntries.length > 0 && (
              <div className="mt-1.5 flex flex-wrap gap-x-4 gap-y-0.5">
                {paramEntries.slice(0, 3).map(([key, val]) => (
                  <span key={key} className="text-xs text-gray-500 dark:text-gray-400">
                    <span className="font-medium text-gray-700 dark:text-gray-300">{key}:</span>{' '}
                    <span className="truncate">{String(val).slice(0, 60)}{String(val).length > 60 ? '…' : ''}</span>
                  </span>
                ))}
              </div>
            )}

            {/* State preview */}
            {stateEntries.length > 0 && (
              <div className="mt-1 flex flex-wrap gap-x-4 gap-y-0.5">
                {stateEntries.map(([key, val]) => (
                  <span key={key} className="text-xs text-gray-500 dark:text-gray-400">
                    <span className="font-medium text-gray-700 dark:text-gray-300">{key}:</span>{' '}
                    <span className="font-mono text-xs">{String(val).slice(0, 40)}{String(val).length > 40 ? '…' : ''}</span>
                  </span>
                ))}
              </div>
            )}
          </div>

          <button
            onClick={onClose}
            className="p-1 rounded-md hover:bg-blue-100 dark:hover:bg-blue-900/50 text-blue-400 hover:text-blue-600 dark:hover:text-blue-300 transition-colors flex-shrink-0"
            title="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Child want cards grid */}
        <div className="p-3">
          {childWants.length === 0 ? (
            <div className="flex items-center justify-center h-16 text-sm text-gray-400 dark:text-gray-500">
              No child wants yet
            </div>
          ) : (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
              {childWants.map((child, index) => {
                const childId = child.metadata?.id || child.id;
                const isSelected = selectedWant?.metadata?.id === childId || selectedWant?.id === childId;
                const childChildren = allWants.filter(w =>
                  w.metadata?.ownerReferences?.some(ref => ref.id === childId)
                );
                const isExpanded = nextExpandedId === childId;

                return (
                  <React.Fragment key={childId}>
                    <div className={classNames(isExpanded ? 'ring-2 ring-blue-400 rounded-lg' : '')}>
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
