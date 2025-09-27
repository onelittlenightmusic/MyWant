import React, { useState } from 'react';
import { ChevronDown, ChevronRight, Users } from 'lucide-react';
import { Want } from '@/types/want';
import { WantCardContent } from './WantCardContent';
import { classNames } from '@/utils/helpers';

interface WantCardProps {
  want: Want;
  children?: Want[];
  selected: boolean;
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
  onView,
  onEdit,
  onDelete,
  onSuspend,
  onResume,
  className
}) => {
  const [isExpanded, setIsExpanded] = useState(false);
  const hasChildren = children && children.length > 0;

  return (
    <div className={classNames(
      'card hover:shadow-md transition-shadow duration-200 cursor-pointer group relative',
      selected ? 'border-blue-500 border-2' : 'border-gray-200',
      className || ''
    )}>
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

      {/* Children indicator in header */}
      {hasChildren && (
        <div className="absolute top-2 right-2 z-20 bg-white rounded-md shadow-sm border border-gray-200 px-2 py-1">
          <div className="flex items-center space-x-1" title={`${children!.length} child want${children!.length !== 1 ? 's' : ''}`}>
            <Users className="h-4 w-4 text-blue-600" />
            <span className="text-xs text-blue-600 font-medium">{children!.length}</span>
            <button
              onClick={(e) => {
                e.stopPropagation();
                setIsExpanded(!isExpanded);
              }}
              className="p-0.5 rounded-sm hover:bg-blue-100 text-blue-600"
              title={isExpanded ? 'Collapse children' : 'Expand children'}
            >
              {isExpanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
            </button>
          </div>
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
            {children!.map((child, index) => (
              <div
                key={child.metadata?.id || `child-${index}`}
                className="p-3 bg-white rounded-md border border-gray-200 hover:border-gray-300 hover:shadow-sm transition-all duration-200"
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
            ))}
          </div>
        </div>
      )}

    </div>
  );
};