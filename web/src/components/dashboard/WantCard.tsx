import React, { useState } from 'react';
import { ChevronDown, ChevronRight, Users } from 'lucide-react';
import { Want } from '@/types/want';
import { WantCardContent } from './WantCardContent';
import { classNames } from '@/utils/helpers';

interface WantCardProps {
  want: Want;
  children?: Want[];
  selected: boolean;
  selectedWant?: Want | null;
  onView: (want: Want) => void;
  onEdit: (want: Want) => void;
  onDelete: (want: Want) => void;
  onSuspend?: (want: Want) => void;
  onResume?: (want: Want) => void;
  className?: string;
}

export const WantCard: React.FC<WantCardProps> = ({
  want,
  children,
  selected,
  selectedWant,
  onView,
  onEdit,
  onDelete,
  onSuspend,
  onResume,
  className
}) => {
  const [isExpanded, setIsExpanded] = useState(false);
  const hasChildren = children && children.length > 0;

  const handleCardClick = (e: React.MouseEvent) => {
    // Don't trigger view if clicking on interactive elements (buttons, menu, etc.)
    const target = e.target as HTMLElement;
    if (
      target.closest('button') ||
      target.closest('[role="button"]') ||
      target.closest('.relative.group/menu')
    ) {
      return;
    }
    onView(want);
  };

  return (
    <div
      onClick={handleCardClick}
      className={classNames(
        'card hover:shadow-md transition-shadow duration-200 cursor-pointer group relative',
        selected ? 'border-blue-500 border-2' : 'border-gray-200',
        className || ''
      )}
    >
      {/* Parent want content using reusable component */}
      <WantCardContent
        want={want}
        isChild={false}
        onView={onView}
        onEdit={onEdit}
        onDelete={onDelete}
        onSuspend={onSuspend}
        onResume={onResume}
      />

      {/* Children indicator at bottom */}
      {hasChildren && !isExpanded && (
        <div className="mt-4 pt-3 border-t border-gray-200">
          <button
            onClick={(e) => {
              e.stopPropagation();
              setIsExpanded(true);
            }}
            className="flex items-center justify-between w-full text-sm text-gray-600 hover:text-gray-900 transition-colors"
          >
            <div className="flex items-center space-x-2">
              <Users className="h-4 w-4 text-blue-600" />
              <span className="text-blue-600 font-medium">{children!.length} child want{children!.length !== 1 ? 's' : ''}</span>
            </div>
            <ChevronDown className="h-4 w-4 text-gray-400" />
          </button>
        </div>
      )}

      {/* Expanded children section */}
      {hasChildren && isExpanded && (
        <div className="mt-4 pt-4 border-t border-gray-200">
          <div className="flex items-center justify-between mb-3">
            <h4 className="text-sm font-medium text-gray-900">Child Wants ({children!.length})</h4>
            <button
              onClick={(e) => {
                e.stopPropagation();
                setIsExpanded(false);
              }}
              className="text-xs text-gray-500 hover:text-gray-700"
            >
              Collapse
            </button>
          </div>
          <div className="space-y-2">
            {children!.sort((a, b) => {
              const idA = a.metadata?.id || a.id || '';
              const idB = b.metadata?.id || b.id || '';
              return idA.localeCompare(idB);
            }).map((child, index) => {
              const isChildSelected = selectedWant && (
                (selectedWant.metadata?.id && selectedWant.metadata.id === child.metadata?.id) ||
                (selectedWant.id && selectedWant.id === child.id)
              );
              return (
                <div
                  key={child.metadata?.id || `child-${index}`}
                  className={classNames(
                    "p-3 bg-white rounded-md border hover:shadow-sm transition-all duration-200",
                    isChildSelected ? 'border-blue-500 border-2' : 'border-gray-200 hover:border-gray-300'
                  )}
                  onClick={(e) => {
                    e.stopPropagation();
                    onView(child);
                  }}
                >
                {/* Child want content using reusable component */}
                <WantCardContent
                  want={child}
                  isChild={true}
                  onView={onView}
                />
                </div>
              );
            })}
          </div>
        </div>
      )}

    </div>
  );
};