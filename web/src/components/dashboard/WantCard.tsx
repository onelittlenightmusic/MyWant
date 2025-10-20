import React, { useState } from 'react';
import { ChevronDown, Users } from 'lucide-react';
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

    // Check if click target is an interactive element (button)
    if (target.closest('button') || target.closest('[role="button"]')) {
      return;
    }

    // Check if target is inside an active menu dropdown
    // Only check immediate parent with group/relative classes that has visible menu
    let element = target as HTMLElement | null;
    while (element && element !== e.currentTarget) {
      if (element.className && element.className.includes('group/menu')) {
        // Check if there's a visible dropdown menu
        const menuDropdown = element.querySelector('[class*="opacity-100"][class*="visible"]');
        if (menuDropdown) {
          return;
        }
      }
      element = element.parentElement;
    }

    onView(want);
  };

  // Handler for child card clicks - passes child directly without closure
  const handleChildCardClick = (child: Want) => (e: React.MouseEvent) => {
    if (e.defaultPrevented) return;
    e.preventDefault();
    e.stopPropagation();

    onView(child);
  };

  // Check if this is a flight want
  const isFlightWant = want.metadata?.type === 'flight';

  return (
    <div
      onClick={handleCardClick}
      className={classNames(
        'hover:shadow-md transition-shadow duration-200 cursor-pointer group relative overflow-hidden rounded-lg shadow-sm border p-6',
        isFlightWant ? 'bg-transparent' : 'bg-white',
        selected ? 'border-blue-500 border-2' : 'border-gray-200',
        className || ''
      )}
    >
      {/* Background decoration for flight wants */}
      {isFlightWant && (
        <div
          className="absolute inset-0 pointer-events-none flex items-center justify-end p-4"
          style={{
            backgroundColor: '#f0f4ff'
          }}
        >
          <svg
            xmlns="http://www.w3.org/2000/svg"
            viewBox="0 0 100 100"
            width="80"
            height="80"
            style={{ opacity: 0.8, color: '#6366f1' }}
          >
            <path
              d="M50 10L60 30L80 40L60 50L50 70L40 50L20 40L40 30Z"
              fill="currentColor"
            />
          </svg>
        </div>
      )}
      {/* Parent want content using reusable component */}
      <div className={classNames('relative z-10', isFlightWant ? 'bg-white rounded' : '')}>
        <WantCardContent
          want={want}
          isChild={false}
          onView={onView}
          onEdit={onEdit}
          onDelete={onDelete}
          onSuspend={onSuspend}
          onResume={onResume}
        />
      </div>

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
              const childId = child.metadata?.id || child.id || '';
              const isChildSelected = selectedWant && (
                (selectedWant.metadata?.id && selectedWant.metadata.id === child.metadata?.id) ||
                (selectedWant.id && selectedWant.id === child.id)
              );
              const isChildFlightWant = isFlightWant || child.metadata?.type === 'flight';
              return (
                <div
                  key={childId || `child-${index}`}
                  className={classNames(
                    "relative overflow-hidden rounded-md border hover:shadow-sm transition-all duration-200 cursor-pointer",
                    isChildSelected ? 'border-blue-500 border-2' : 'border-gray-200 hover:border-gray-300'
                  )}
                  onClick={handleChildCardClick(child)}
                >
                  {/* Background decoration for flight want children */}
                  {isChildFlightWant && (
                    <div
                      className="absolute inset-0 pointer-events-none flex items-center justify-end p-3"
                      style={{
                        backgroundColor: '#f0f4ff'
                      }}
                    >
                      <svg
                        xmlns="http://www.w3.org/2000/svg"
                        viewBox="0 0 100 100"
                        width="50"
                        height="50"
                        style={{ opacity: 0.8, color: '#6366f1' }}
                      >
                        <path
                          d="M50 10L60 30L80 40L60 50L50 70L40 50L20 40L40 30Z"
                          fill="currentColor"
                        />
                      </svg>
                    </div>
                  )}
                  {/* Child want content using reusable component */}
                  <div className={classNames('relative z-10 p-3', isChildFlightWant ? 'bg-transparent' : 'bg-white')}>
                    <WantCardContent
                      want={child}
                      isChild={true}
                      onView={onView}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

    </div>
  );
};