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
  onViewAgents?: (want: Want) => void;
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
  onViewAgents,
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

  // Check if this is a flight or hotel want
  const isFlightWant = want.metadata?.type === 'flight';
  const isHotelWant = want.metadata?.type === 'hotel';

  // Determine background image based on want type
  const getBackgroundImage = (type?: string) => {
    if (type === 'flight') return '/resources/flight.png';
    if (type === 'hotel') return '/resources/hotel.png';
    if (type === 'restaurant') return '/resources/restaurant.png';
    if (type === 'buffet') return '/resources/buffet.png';
    return undefined;
  };

  const backgroundImage = getBackgroundImage(want.metadata?.type);

  return (
    <div
      onClick={handleCardClick}
      className={classNames(
        'card hover:shadow-md transition-shadow duration-200 cursor-pointer group relative overflow-hidden',
        selected ? 'border-blue-500 border-2' : 'border-gray-200',
        className || ''
      )}
      style={backgroundImage ? {
        backgroundImage: `url(${backgroundImage})`,
        backgroundSize: '100% auto',
        backgroundPosition: 'center center',
        backgroundRepeat: 'no-repeat',
        backgroundAttachment: 'scroll'
      } : undefined}
    >
      {/* Parent want content using reusable component */}
      <div className={backgroundImage ? 'relative z-10 bg-white bg-opacity-70' : ''}>
        <WantCardContent
          want={want}
          isChild={false}
          onView={onView}
          onViewAgents={onViewAgents}
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
              const childBackgroundImage = getBackgroundImage(child.metadata?.type);
              const hasBackgroundImage = backgroundImage || childBackgroundImage;
              return (
                <div
                  key={childId || `child-${index}`}
                  className={classNames(
                    "relative overflow-hidden rounded-md border hover:shadow-sm transition-all duration-200 cursor-pointer",
                    isChildSelected ? 'border-blue-500 border-2' : 'border-gray-200 hover:border-gray-300'
                  )}
                  style={childBackgroundImage ? {
                    backgroundImage: `url(${childBackgroundImage})`,
                    backgroundSize: '100% auto',
                    backgroundPosition: 'center center',
                    backgroundRepeat: 'no-repeat',
                    backgroundAttachment: 'scroll'
                  } : undefined}
                  onClick={handleChildCardClick(child)}
                >
                  {/* Child want content using reusable component */}
                  <div className={classNames('p-3', hasBackgroundImage ? 'bg-white bg-opacity-70 relative z-10' : 'bg-white')}>
                    <WantCardContent
                      want={child}
                      isChild={true}
                      onView={onView}
                      onViewAgents={onViewAgents}
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