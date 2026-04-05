import React from 'react';
import { X, ChevronRight, Layers } from 'lucide-react';
import { Want } from '@/types/want';
import { WantCard } from './WantCard/WantCard';
import { classNames } from '@/utils/helpers';

interface ChildWantsBarProps {
  parentWant: Want;
  childWants: Want[];
  selectedWant: Want | null;
  onViewWant: (want: Want) => void;
  onViewAgentsWant?: (want: Want) => void;
  onViewResultsWant?: (want: Want) => void;
  onViewChatWant?: (want: Want) => void;
  onEditWant: (want: Want) => void;
  onDeleteWant: (want: Want) => void;
  onSuspendWant?: (want: Want) => void;
  onResumeWant?: (want: Want) => void;
  onShowReactionConfirmation?: (want: Want, action: 'approve' | 'deny') => void;
  onClose: () => void;
}

export const ChildWantsBar: React.FC<ChildWantsBarProps> = ({
  parentWant,
  childWants,
  selectedWant,
  onViewWant,
  onViewAgentsWant,
  onViewResultsWant,
  onViewChatWant,
  onEditWant,
  onDeleteWant,
  onSuspendWant,
  onResumeWant,
  onShowReactionConfirmation,
  onClose,
}) => {
  const parentName = parentWant.metadata?.name || parentWant.metadata?.type || 'Want';
  const childCount = childWants.length;

  return (
    <div
      className={classNames(
        'absolute bottom-0 left-0 right-0 z-30',
        'bg-white dark:bg-gray-900 border-t-2 border-blue-200 dark:border-blue-800',
        'shadow-[0_-4px_24px_rgba(0,0,0,0.12)]',
        'flex flex-col',
        'animate-slide-up'
      )}
      style={{ maxHeight: '45%' }}
    >
      {/* Header */}
      <div className="flex items-center gap-2 px-4 py-2 border-b border-gray-100 dark:border-gray-800 flex-shrink-0 bg-blue-50 dark:bg-blue-950/40">
        <Layers className="h-4 w-4 text-blue-500 flex-shrink-0" />
        <span className="text-xs font-semibold text-blue-700 dark:text-blue-300 truncate max-w-[200px]">
          {parentName}
        </span>
        <ChevronRight className="h-3 w-3 text-blue-400 flex-shrink-0" />
        <span className="text-xs text-blue-500 dark:text-blue-400">
          {childCount} {childCount === 1 ? 'child want' : 'child wants'}
        </span>
        <button
          onClick={onClose}
          className="ml-auto p-1 rounded-md hover:bg-blue-100 dark:hover:bg-blue-900/50 text-blue-400 hover:text-blue-600 dark:hover:text-blue-300 transition-colors"
          title="Close"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto p-3">
        {childCount === 0 ? (
          <div className="flex items-center justify-center h-16 text-sm text-gray-400 dark:text-gray-500">
            No child wants yet
          </div>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
            {childWants.map((child, index) => {
              const childId = child.metadata?.id || child.id;
              const isSelected = selectedWant?.metadata?.id === childId || selectedWant?.id === childId;
              return (
                <WantCard
                  key={childId}
                  index={index}
                  want={child}
                  selected={isSelected}
                  selectedWant={selectedWant}
                  onView={onViewWant}
                  onViewAgents={onViewAgentsWant || (() => {})}
                  onViewResults={onViewResultsWant || (() => {})}
                  onViewChat={onViewChatWant || (() => {})}
                  onEdit={onEditWant}
                  onDelete={onDeleteWant}
                  onSuspend={onSuspendWant}
                  onResume={onResumeWant}
                  onShowReactionConfirmation={onShowReactionConfirmation}
                />
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
};
