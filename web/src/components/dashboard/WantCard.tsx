import React, { useState, useEffect, useRef } from 'react';
import { ChevronDown, Users } from 'lucide-react';
import { Want } from '@/types/want';
import { WantCardContent } from './WantCardContent';
import { classNames } from '@/utils/helpers';
import { getBackgroundStyle, getBackgroundOverlayClass, getBackgroundContentContainerClass } from '@/utils/backgroundStyles';

interface WantCardProps {
  want: Want;
  children?: Want[];
  selected: boolean;
  selectedWant?: Want | null;
  onView: (want: Want) => void;
  onViewAgents?: (want: Want) => void;
  onViewResults?: (want: Want) => void;
  onEdit: (want: Want) => void;
  onDelete: (want: Want) => void;
  onSuspend?: (want: Want) => void;
  onResume?: (want: Want) => void;
  className?: string;
  expandedParents?: Set<string>;
  onToggleExpand?: (wantId: string) => void;
  onLabelDropped?: (wantId: string) => void;
}

export const WantCard: React.FC<WantCardProps> = ({
  want,
  children,
  selected,
  selectedWant,
  onView,
  onViewAgents,
  onViewResults,
  onEdit,
  onDelete,
  onSuspend,
  onResume,
  className,
  expandedParents,
  onToggleExpand,
  onLabelDropped
}) => {
  const wantId = want.metadata?.id || want.id;
  // Use expandedParents from keyboard navigation if provided, otherwise use local state
  const isExpanded = expandedParents?.has(wantId || '') ?? false;
  const hasChildren = children && children.length > 0;

  // Local state for managing expansion (fallback if expandedParents not provided)
  const [localIsExpanded, setLocalIsExpanded] = useState(false);
  const displayIsExpanded = expandedParents ? isExpanded : localIsExpanded;

  // Ref for animation container
  const expandedContainerRef = useRef<HTMLDivElement>(null);
  const [showAnimation, setShowAnimation] = useState(false);

  // State for drag and drop
  const [isDragOver, setIsDragOver] = useState(false);

  // Trigger animation when expanded children section mounts
  useEffect(() => {
    if (displayIsExpanded && expandedContainerRef.current) {
      // Force reflow to ensure animation triggers
      setShowAnimation(false);
      const timer = requestAnimationFrame(() => {
        setShowAnimation(true);
      });
      return () => cancelAnimationFrame(timer);
    } else {
      setShowAnimation(false);
    }
  }, [displayIsExpanded]);

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

    // Smooth scroll the card into view after selection
    requestAnimationFrame(() => {
      setTimeout(() => {
        const selectedElement = document.querySelector('[data-keyboard-nav-selected="true"]');
        if (selectedElement && selectedElement instanceof HTMLElement) {
          selectedElement.scrollIntoView({ behavior: 'smooth', block: 'center' });
        }
      }, 0);
    });
  };

  // Handler for child card clicks - passes child directly without closure
  const handleChildCardClick = (child: Want) => (e: React.MouseEvent) => {
    if (e.defaultPrevented) return;
    e.preventDefault();
    e.stopPropagation();

    onView(child);

    // Smooth scroll the card into view after selection
    requestAnimationFrame(() => {
      setTimeout(() => {
        const selectedElement = document.querySelector('[data-keyboard-nav-selected="true"]');
        if (selectedElement && selectedElement instanceof HTMLElement) {
          selectedElement.scrollIntoView({ behavior: 'smooth', block: 'center' });
        }
      }, 0);
    });
  };

  // Handle drag over
  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    e.dataTransfer.dropEffect = 'copy';
    setIsDragOver(true);
  };

  // Handle drag leave
  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragOver(false);
  };

  // Handle drop
  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragOver(false);

    try {
      const labelData = e.dataTransfer.getData('application/json');
      if (!labelData) return;

      const { key, value } = JSON.parse(labelData);
      const wantId = want.metadata?.id || want.id;

      if (!wantId) {
        console.error('No want ID found');
        return;
      }

      // Add label via POST /api/v1/wants/{id}/labels
      fetch(`http://localhost:8080/api/v1/wants/${wantId}/labels`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ key, value })
      })
        .then(response => {
          if (response.ok && onLabelDropped) {
            // Notify parent to refresh want details
            onLabelDropped(wantId);
          }
        })
        .catch(error => {
          console.error('Error dropping label:', error);
        });
    } catch (error) {
      console.error('Error dropping label:', error);
    }
  };

  // Check if this is a flight or hotel want
  const isFlightWant = want.metadata?.type === 'flight';
  const isHotelWant = want.metadata?.type === 'hotel';

  // Get background style for parent want (isParentWant = true)
  const parentBackgroundStyle = getBackgroundStyle(want.metadata?.type, true);

  // Get achieving percentage for progress bar
  const achievingPercentage = (want.state?.achieving_percentage as number) ?? 0;

  // Create white progress bar style - animates from left to right
  const whiteProgressBarStyle = {
    position: 'absolute' as const,
    top: 0,
    left: 0,
    height: '100%',
    width: `${achievingPercentage}%`,
    background: 'rgba(255, 255, 255, 0.5)',
    transition: 'width 0.3s ease-out',
    zIndex: 0,
    pointerEvents: 'none' as const
  };

  // Create black overlay style - covers the remaining right portion
  const blackOverlayStyle = {
    position: 'absolute' as const,
    top: 0,
    left: `${achievingPercentage}%`,
    height: '100%',
    width: `${100 - achievingPercentage}%`,
    background: 'rgba(0, 0, 0, 0.2)',
    transition: 'width 0.3s ease-out, left 0.3s ease-out',
    zIndex: 0,
    pointerEvents: 'none' as const
  };

  return (
    <div
      onClick={handleCardClick}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
      data-keyboard-nav-selected={selected}
      className={classNames(
        'card hover:shadow-md transition-shadow duration-200 cursor-pointer group relative overflow-hidden min-h-[200px]',
        selected ? 'border-blue-500 border-2' : 'border-gray-200',
        isDragOver && 'border-green-500 border-2 bg-green-50',
        parentBackgroundStyle.className,
        className || ''
      )}
      style={parentBackgroundStyle.style}
    >
      {/* White progress bar - animates from left to right */}
      <div style={whiteProgressBarStyle}></div>

      {/* Black overlay - covers remaining right portion */}
      <div style={blackOverlayStyle}></div>

      {/* Parent want content using reusable component */}
      <div className="relative z-10">
        <WantCardContent
          want={want}
          isChild={false}
          onView={onView}
          onViewAgents={onViewAgents}
          onViewResults={onViewResults}
          onEdit={onEdit}
          onDelete={onDelete}
          onSuspend={onSuspend}
          onResume={onResume}
        />
      </div>

      {/* Children indicator at bottom */}
      {hasChildren && !displayIsExpanded && (
        <div className="relative z-10 mt-4 pt-3 border-t border-gray-200">
          <button
            onClick={(e) => {
              e.stopPropagation();
              if (expandedParents && onToggleExpand && wantId) {
                // Use parent callback to update expandedParents
                onToggleExpand(wantId);
              } else {
                // Fallback to local state if no callback provided
                setLocalIsExpanded(true);
              }
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
      {hasChildren && displayIsExpanded && (
        <div
          ref={expandedContainerRef}
          className="relative z-10 mt-4 pt-4 border-t border-gray-200 transition-opacity duration-300 ease-out"
          style={{
            opacity: showAnimation ? 1 : 0
          } as React.CSSProperties}
        >
          <div className="flex items-center justify-between mb-3">
            <h4 className="text-sm font-medium text-gray-900">Child Wants ({children!.length})</h4>
            <button
              onClick={(e) => {
                e.stopPropagation();
                if (expandedParents && onToggleExpand && wantId) {
                  // Use parent callback to update expandedParents
                  onToggleExpand(wantId);
                } else {
                  // Fallback to local state if no callback provided
                  setLocalIsExpanded(false);
                }
              }}
              className="text-xs text-gray-500 hover:text-gray-700"
            >
              Collapse
            </button>
          </div>
          {/* Grid layout for child wants - 3 columns, wraps to new rows */}
          <div
            className="grid grid-cols-3 gap-3 transition-all duration-300 ease-out"
            style={{
              opacity: showAnimation ? 1 : 0,
              transform: showAnimation ? 'translateY(0)' : 'translateY(-8px)'
            } as React.CSSProperties}
          >
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
              // Get background style for child want (isParentWant = false)
              const childBackgroundStyle = getBackgroundStyle(child.metadata?.type, false);

              // Get achieving percentage for child card progress bar
              const childAchievingPercentage = (child.state?.achieving_percentage as number) ?? 0;

              // Create white progress bar style for child - animates from left to right
              const childWhiteProgressBarStyle = {
                position: 'absolute' as const,
                top: 0,
                left: 0,
                height: '100%',
                width: `${childAchievingPercentage}%`,
                background: 'rgba(255, 255, 255, 0.5)',
                transition: 'width 0.3s ease-out',
                zIndex: 0,
                pointerEvents: 'none' as const
              };

              // Create black overlay style for child - covers remaining right portion
              const childBlackOverlayStyle = {
                position: 'absolute' as const,
                top: 0,
                left: `${childAchievingPercentage}%`,
                height: '100%',
                width: `${100 - childAchievingPercentage}%`,
                background: 'rgba(0, 0, 0, 0.2)',
                transition: 'width 0.3s ease-out, left 0.3s ease-out',
                zIndex: 0,
                pointerEvents: 'none' as const
              };

              // Handle drag over for child card
              const handleChildDragOver = (e: React.DragEvent) => {
                e.preventDefault();
                e.stopPropagation();
                e.dataTransfer.dropEffect = 'copy';
                setIsDragOver(true);
              };

              // Handle drag leave for child card
              const handleChildDragLeave = (e: React.DragEvent) => {
                e.preventDefault();
                e.stopPropagation();
                setIsDragOver(false);
              };

              // Handle drop for child card
              const handleChildDrop = (e: React.DragEvent) => {
                e.preventDefault();
                e.stopPropagation();
                setIsDragOver(false);

                try {
                  const labelData = e.dataTransfer.getData('application/json');
                  if (!labelData) return;

                  const { key, value } = JSON.parse(labelData);
                  const childWantId = child.metadata?.id || child.id;

                  if (!childWantId) {
                    console.error('No child want ID found');
                    return;
                  }

                  // Add label via POST /api/v1/wants/{id}/labels
                  fetch(`http://localhost:8080/api/v1/wants/${childWantId}/labels`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ key, value })
                  })
                    .then(response => {
                      if (response.ok && onLabelDropped) {
                        // Notify parent to refresh want details
                        onLabelDropped(childWantId);
                      }
                    })
                    .catch(error => {
                      console.error('Error dropping label on child:', error);
                    });
                } catch (error) {
                  console.error('Error dropping label on child:', error);
                }
              };

              return (
                <div
                  key={childId || `child-${index}`}
                  data-keyboard-nav-selected={isChildSelected}
                  className={classNames(
                    "relative overflow-hidden rounded-md border hover:shadow-sm transition-all duration-200 cursor-pointer",
                    isChildSelected ? 'border-blue-500 border-2' : 'border-gray-200 hover:border-gray-300',
                    isDragOver && 'border-green-500 border-2 bg-green-50',
                    childBackgroundStyle.className
                  )}
                  style={childBackgroundStyle.style}
                  onClick={handleChildCardClick(child)}
                  onDragOver={handleChildDragOver}
                  onDragLeave={handleChildDragLeave}
                  onDrop={handleChildDrop}
                >
                  {/* White progress bar for child - animates from left to right */}
                  <div style={childWhiteProgressBarStyle}></div>

                  {/* Black overlay for child - covers remaining right portion */}
                  <div style={childBlackOverlayStyle}></div>

                  {/* Child want content using reusable component */}
                  <div className="p-4 w-full h-full relative z-10">
                    <WantCardContent
                      want={child}
                      isChild={true}
                      onView={onView}
                      onViewAgents={onViewAgents}
                      onViewResults={onViewResults}
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