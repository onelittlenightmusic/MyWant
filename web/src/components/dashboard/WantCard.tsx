import React, { useState, useEffect, useRef } from 'react';
import { ChevronDown, CheckSquare, Square, Plus } from 'lucide-react';
import { Want, WantExecutionStatus } from '@/types/want';
import { WantCardContent } from './WantCardContent';
import { classNames } from '@/utils/helpers';
import { getBackgroundStyle } from '@/utils/backgroundStyles';
import { useWantStore } from '@/stores/wantStore';
import styles from './WantCard.module.css';

/**
 * Get color for a want status
 */
const getStatusColor = (status: WantExecutionStatus): string => {
  switch (status) {
    case 'achieved':
      return '#10b981'; // Green
    case 'reaching':
    case 'terminated':
      return '#9333ea'; // Purple
    case 'failed':
      return '#ef4444'; // Red
    default:
      return '#d1d5db'; // Gray for created, suspended, stopped
  }
};

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
  onShowReactionConfirmation?: (want: Want, action: 'approve' | 'deny') => void;
  className?: string;
  expandedParents?: Set<string>;
  onToggleExpand?: (wantId: string) => void;
  onLabelDropped?: (wantId: string) => void;
  onWantDropped?: (draggedWantId: string, targetWantId: string) => void;
  isSelectMode?: boolean;
  selectedWantIds?: Set<string>;
  isBeingProcessed?: boolean; // New prop for initializing/deleting states
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
  onShowReactionConfirmation,
  className,
  expandedParents,
  onToggleExpand,
  onLabelDropped,
  onWantDropped,
  isSelectMode = false,
  selectedWantIds,
  isBeingProcessed = false // Default to false
}) => {
  const wantId = want.metadata?.id || want.id;
  const { setDraggingWant, setIsOverTarget } = useWantStore();
  // Use expandedParents from keyboard navigation if provided, otherwise use local state
  const isExpanded = expandedParents?.has(wantId || '') ?? false;
  const hasChildren = children && children.length > 0;

  // Identify if this want is a Target (can have children)
  const wantType = want.metadata?.type?.toLowerCase() || '';
  const isTargetWant = wantType.includes('target') || 
                       wantType === 'owner' ||
                       wantType.includes('approval') ||
                       wantType.includes('system') ||
                       wantType.includes('travel') ||
                       hasChildren;

  // Local state for managing expansion (fallback if expandedParents not provided)
  const [localIsExpanded, setLocalIsExpanded] = useState(false);
  const displayIsExpanded = expandedParents ? isExpanded : localIsExpanded;

  // Ref for animation container
  const expandedContainerRef = useRef<HTMLDivElement>(null);
  const [showAnimation, setShowAnimation] = useState(false);

  // State for drag and drop
  const [isDragOver, setIsDragOver] = useState(false);
  const [isDragOverWant, setIsDragOverWant] = useState(false);
  const [draggedOverChildId, setDraggedOverChildId] = useState<string | null>(null);
  const cardRef = useRef<HTMLDivElement>(null);

  // Focus the card when it's targeted by keyboard navigation
  useEffect(() => {
    const wantId = want.metadata?.id || want.id;
    const selectedId = selectedWant?.metadata?.id || selectedWant?.id;
    const isNavSelected = selectedId === wantId;

    if (isNavSelected && document.activeElement !== cardRef.current) {
      // Don't steal focus if user is already interacting with an input or the sidebar
      const target = document.activeElement as HTMLElement;
      const isFocusInInput =
        target?.tagName === 'INPUT' ||
        target?.tagName === 'TEXTAREA' ||
        target?.isContentEditable;

      const isFocusInSidebar = !!target?.closest('[role="complementary"]') ||
                              !!target?.closest('.right-sidebar');

      if (!isFocusInInput && !isFocusInSidebar) {
        cardRef.current?.focus();
      }
    }
  }, [selectedWant?.metadata?.id, selectedWant?.id, want.metadata?.id, want.id]);

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
    if (isBeingProcessed) return; // Prevent clicks when processing

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
    if (isBeingProcessed) return; // Prevent clicks when processing

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

  // Handle drag start for the card itself
  const handleDragStart = (e: React.DragEvent) => {
    if (isSelectMode || isBeingProcessed) return; // Prevent dragging when processing
    
    const id = want.metadata?.id || want.id;
    if (!id) return;

    // Set dragging state in global store for target-side feedback
    setDraggingWant(id);

    e.dataTransfer.setData('application/mywant-id', id);
    e.dataTransfer.setData('application/mywant-name', want.metadata?.name || '');
    e.dataTransfer.effectAllowed = 'move';
  };

  const handleDragEnd = () => {
    setDraggingWant(null);
  };

  // Handle drag over
  const handleDragOver = (e: React.DragEvent) => {
    if (isBeingProcessed) return; // Prevent drag over when processing

    e.preventDefault();
    e.stopPropagation();
    
    const { draggingWant } = useWantStore.getState();
    const isWantDrag = !!draggingWant || e.dataTransfer.types.includes('application/mywant-id');
    
    if (isWantDrag) {
      setIsDragOver(false);
      if (isTargetWant) {
        e.dataTransfer.dropEffect = 'move';
        setIsOverTarget(true); // Always set true while over a target
        if (!isDragOverWant) {
          setIsDragOverWant(true);
        }
      } else {
        // If we move from a target to a non-target, but still within the card group
        // setIsOverTarget(false); // This might flicker, handled by individual card leave
      }
    } else {
      setIsDragOverWant(false);
      setIsOverTarget(false);
      e.dataTransfer.dropEffect = 'copy';
      setIsDragOver(true);
    }
  };

  // Handle drag leave
  const handleDragLeave = (e: React.DragEvent) => {
    if (isBeingProcessed) return; // Prevent drag leave when processing
    
    e.preventDefault();
    e.stopPropagation();
    setIsDragOver(false);
    setIsDragOverWant(false);
    setIsOverTarget(false);
    setDraggedOverChildId(null);
  };

  // Handle drop
  const handleDrop = (e: React.DragEvent) => {
    if (isBeingProcessed) return; // Prevent drop when processing

    e.preventDefault();
    e.stopPropagation();
    setIsDragOver(false);
    setIsDragOverWant(false);
    setIsOverTarget(false);
    setDraggedOverChildId(null);
    setDraggingWant(null); // Reset dragging state

    const draggedWantId = e.dataTransfer.getData('application/mywant-id');
    const targetWantId = want.metadata?.id || want.id;

    if (draggedWantId && targetWantId && isTargetWant) {
      // Don't drop on yourself
      if (draggedWantId === targetWantId) return;
      
      if (onWantDropped) {
        onWantDropped(draggedWantId, targetWantId);
      }
      return;
    }

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

  // Get background style for parent want card (plain white, no background images)
  const parentBackgroundStyle = getBackgroundStyle(want.metadata?.type, false);

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
      ref={cardRef}
      draggable={!isSelectMode && !isTargetWant && !isBeingProcessed} // Disable dragging when processing
      onDragStart={handleDragStart}
      onDragEnd={handleDragEnd}
      onClick={handleCardClick}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
      tabIndex={isBeingProcessed ? -1 : 0} // Disable focus when processing
      data-keyboard-nav-selected={selected}
      data-keyboard-nav-id={wantId}
      data-is-target={isTargetWant}
      className={classNames(
        'card hover:shadow-md transition-all duration-300 group relative overflow-hidden h-full min-h-[200px] flex flex-col focus:outline-none focus:ring-2 focus:ring-blue-400 focus:ring-inset',
        selected ? 'border-blue-500 border-2' : 'border-gray-200',
        (isDragOverWant || isDragOver) && !isBeingProcessed && 'border-blue-600 border-2 bg-blue-100', // Apply drag styles only if not processing
        isBeingProcessed && 'opacity-50 pointer-events-none cursor-not-allowed', // Visual feedback and disable interaction
        parentBackgroundStyle.className,
        className || ''
      )}
      style={parentBackgroundStyle.style}
    >

      {/* White progress bar - animates from left to right */}
      <div style={whiteProgressBarStyle}></div>

      {/* Black overlay - covers remaining right portion */}
      <div style={blackOverlayStyle}></div>

      {/* Want Drag Over Overlay (+ mark) - only for parent when not dragging over child */}
      <div 
        className={classNames(
          "absolute inset-0 z-30 flex items-center justify-center bg-blue-700 transition-all duration-400 ease-out pointer-events-none",
          isDragOverWant && isTargetWant && !draggedOverChildId && !isBeingProcessed ? "bg-opacity-60 opacity-100" : "bg-opacity-0 opacity-0"
        )}
      >
        <div className={classNames(
          "bg-white p-4 rounded-full shadow-2xl border-4 border-blue-600 transform transition-all duration-400 ease-out",
          isDragOverWant && isTargetWant && !draggedOverChildId && !isBeingProcessed ? "scale-100 opacity-100" : "scale-[2.5] opacity-0"
        )}>
          <Plus className="w-16 h-12 text-blue-700" />
        </div>
      </div>

      {/* Selection Checkbox Overlay */}
      {isSelectMode && (
        <div className="absolute top-2 right-2 z-20 pointer-events-none">
          {selected ? (
            <CheckSquare className="w-6 h-6 text-blue-600 bg-white rounded-md" />
          ) : (
            <Square className="w-6 h-6 text-gray-400 bg-white rounded-md opacity-50" />
          )}
        </div>
      )}

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
          onShowReactionConfirmation={onShowReactionConfirmation}
        />
      </div>

      {/* Children indicator at bottom */}
      {hasChildren && !displayIsExpanded && (
        <div className="relative z-10 mt-auto pt-3 border-t border-gray-200">
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
            className={classNames(
              "flex items-center justify-between w-full text-sm text-gray-600 hover:text-gray-900 transition-colors",
              isBeingProcessed && 'pointer-events-none' // Disable clicks when processing
            )}
            disabled={isBeingProcessed} // Disable button when processing
          >
            <div className="flex items-center gap-3">
              {/* Status dots for each child want */}
              <div className="flex items-center gap-1.5" title={`${children!.length} child want${children!.length !== 1 ? 's' : ''}`}>
                {children!.map((child, idx) => (
                  <div
                    key={idx}
                    className={`w-2 h-2 rounded-full flex-shrink-0 ${child.status === 'reaching' ? styles.pulseGlow : ''}`}
                    style={{ backgroundColor: getStatusColor(child.status) }}
                    title={child.status}
                  />
                ))}
              </div>
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
              className={classNames(
                "text-xs text-gray-500 hover:text-gray-700",
                isBeingProcessed && 'pointer-events-none' // Disable clicks when processing
              )}
              disabled={isBeingProcessed} // Disable button when processing
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
              const isChildSelected = isSelectMode 
                ? (selectedWantIds?.has(childId))
                : (selectedWant && (
                  (selectedWant.metadata?.id && selectedWant.metadata.id === child.metadata?.id) ||
                  (selectedWant.id && selectedWant.id === child.id)
                ));
              
              // Get background style for child want card
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

              // Identify if this child is also a Target
              const isChildTarget = child.metadata?.type.toLowerCase().includes('target') || 
                                    child.metadata?.type.toLowerCase() === 'owner' ||
                                    child.metadata?.type.toLowerCase().includes('approval');

                  // Handle drag over for child card
                  const handleChildDragOver = (e: React.DragEvent) => {
                    if (isBeingProcessed) return; // Prevent drag over when processing

                    e.preventDefault();
                    e.stopPropagation();
                    
                    const { draggingWant } = useWantStore.getState();
                    const isWantDrag = !!draggingWant || e.dataTransfer.types.includes('application/mywant-id');
                    
                    if (isWantDrag) {
                      setIsDragOver(false);
                      if (isChildTarget) {
                        e.dataTransfer.dropEffect = 'move';
                        setIsDragOverWant(true);
                        setIsOverTarget(true);
                        setDraggedOverChildId(childId);
                      } else {
                        // If child is not a target, allow drop on the parent target
                        e.dataTransfer.dropEffect = 'move';
                        setIsDragOverWant(true);
                        setIsOverTarget(true);
                        setDraggedOverChildId(null); // Parent handles it
                      }
                    } else {
                      // Label drag
                      setIsDragOverWant(false);
                      setIsOverTarget(false);
                      e.dataTransfer.dropEffect = 'copy';
                      setIsDragOver(true);
                    }
                  };

                  // Handle drag leave for child card
                  const handleChildDragLeave = (e: React.DragEvent) => {
                    if (isBeingProcessed) return; // Prevent drag leave when processing

                    e.preventDefault();
                    e.stopPropagation();
                    // Don't reset everything, just child ID
                    setDraggedOverChildId(null);
                  };

                  // Handle drop for child card
                  const handleChildDrop = (e: React.DragEvent) => {
                    if (isBeingProcessed) return; // Prevent drop when processing

                    e.preventDefault();
                    e.stopPropagation();
                    setIsDragOver(false);
                    setIsDragOverWant(false);
                    setIsOverTarget(false);
                    setDraggedOverChildId(null);
                    setDraggingWant(null); // Reset dragging state

                const draggedWantId = e.dataTransfer.getData('application/mywant-id');
                const targetWantId = child.metadata?.id || child.id;

                if (draggedWantId && targetWantId && isChildTarget) {
                  if (draggedWantId === targetWantId) return;
                  if (onWantDropped) {
                    onWantDropped(draggedWantId, targetWantId);
                  }
                  return;
                }

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
                  data-keyboard-nav-id={childId}
                  tabIndex={isBeingProcessed ? -1 : 0} // Disable focus when processing
                  draggable={true && !isBeingProcessed} // Disable dragging when processing
                  onDragStart={(e) => {
                    e.stopPropagation();
                    const id = child.metadata?.id || child.id;
                    if (!id) return;
                    setDraggingWant(id);
                    e.dataTransfer.setData('application/mywant-id', id);
                    e.dataTransfer.setData('application/mywant-name', child.metadata?.name || '');
                    e.dataTransfer.effectAllowed = 'move';
                  }}
                  onDragEnd={(e) => {
                    e.stopPropagation();
                    setDraggingWant(null);
                  }}
                  className={classNames(
                    "relative overflow-hidden rounded-md border hover:shadow-sm transition-all duration-300 cursor-pointer focus:outline-none focus:ring-2 focus:ring-blue-400 focus:ring-inset",
                    isChildSelected ? 'border-blue-500 border-2' : 'border-gray-200 hover:border-gray-300',
                    (isDragOverWant || isDragOver) && !isBeingProcessed && 'border-blue-600 border-2 bg-blue-100', // Apply drag styles only if not processing
                    isBeingProcessed && 'opacity-50 pointer-events-none cursor-not-allowed', // Visual feedback and disable interaction
                    childBackgroundStyle.className
                  )}
                  style={childBackgroundStyle.style}
                  onClick={handleChildCardClick(child)}
                  onDragOver={handleChildDragOver}
                  onDragLeave={handleChildDragLeave}
                  onDrop={handleChildDrop}
                >
                  {/* Selection Checkbox Overlay for Child */}
                  {isSelectMode && (
                    <div className="absolute top-2 right-2 z-20 pointer-events-none">
                      {isChildSelected ? (
                        <CheckSquare className="w-5 h-5 text-blue-600 bg-white rounded-md" />
                      ) : (
                        <Square className="w-5 h-5 text-gray-400 bg-white rounded-md opacity-50" />
                      )}
                    </div>
                  )}

                  {/* White progress bar for child - animates from left to right */}
                  <div style={childWhiteProgressBarStyle}></div>

                  {/* Black overlay for child - covers remaining right portion */}
                  <div style={childBlackOverlayStyle}></div>

                  {/* Want Drag Over Overlay for Child (+ mark) */}
                  <div 
                    className={classNames(
                      "absolute inset-0 z-30 flex items-center justify-center bg-blue-700 transition-all duration-400 ease-out pointer-events-none",
                      isDragOverWant && draggedOverChildId === childId && isChildTarget && !isBeingProcessed ? "bg-opacity-60 opacity-100" : "bg-opacity-0 opacity-0"
                    )}
                  >
                    <div className={classNames(
                      "bg-white p-2 rounded-full shadow-xl border-2 border-blue-600 transform transition-all duration-400 ease-out",
                      isDragOverWant && draggedOverChildId === childId && isChildTarget && !isBeingProcessed ? "scale-100 opacity-100" : "scale-[2.5] opacity-0"
                    )}>
                      <Plus className="w-6 h-6 text-blue-700" />
                    </div>
                  </div>

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