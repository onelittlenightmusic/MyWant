import React, { useState, useEffect, useRef } from 'react';
import { ChevronDown, ChevronUp, CheckSquare, Square, Plus, Heart } from 'lucide-react';
import { Want } from '@/types/want';
import { WantCardContent } from '../WantCardContent';
import { classNames, suppressDragImage } from '@/utils/helpers';
import { getBackgroundStyle } from '@/utils/backgroundStyles';
import { useWantStore } from '@/stores/wantStore';
import styles from '../WantCard.module.css';

import { ProgressBars } from './parts/ProgressBars';
import { CorrelationOverlay } from './parts/CorrelationOverlay';
import { StackLayers } from './parts/StackLayers';
import { VersionBadge } from './parts/VersionBadge';
import { getStatusHexColor } from './parts/StatusColor';
import { ChildWantCard } from './ChildWantCard';
import { QuickActionsOverlay } from './parts/QuickActionsOverlay';

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
  onReorderDragOver?: (index: number, position: 'before' | 'after' | 'inside' | null) => void;
  onReorderDrop?: (draggedId: string, index: number, position: 'before' | 'after') => void;
  index: number;
  isSelectMode?: boolean;
  selectedWantIds?: Set<string>;
  isBeingProcessed?: boolean;
  onCreateWant?: (parentWant?: Want) => void;
  correlationRate?: number;
  correlationHighlights?: Map<string, number>;
  stackCount?: number;
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
  onReorderDragOver,
  onReorderDrop,
  index,
  isSelectMode = false,
  selectedWantIds,
  isBeingProcessed = false,
  onCreateWant,
  correlationRate,
  correlationHighlights,
  stackCount = 0,
}) => {
  const wantId = want.metadata?.id || want.id;
  const { 
    setDraggingWant, 
    setIsOverTarget, 
    highlightedLabel, 
    blinkingWantId, 
    startWant, 
    stopWant,
    quickActionsWantId,
    setQuickActionsWantId
  } = useWantStore();
  
  const showQuickActions = quickActionsWantId === wantId;
  const isExpanded = expandedParents?.has(wantId || '') ?? false;
  const hasChildren = children && children.length > 0;

  const isHighlighted = highlightedLabel &&
    want.metadata?.labels &&
    want.metadata.labels[highlightedLabel.key] === highlightedLabel.value;

  const isBlinking = blinkingWantId === wantId;
  const wantType = want.metadata?.type?.toLowerCase() || '';
  const isTargetWant = wantType.includes('target') ||
    wantType === 'owner' ||
    wantType.includes('approval') ||
    wantType.includes('system') ||
    wantType.includes('travel') ||
    hasChildren;

  const isRecipeBased = want.metadata?.labels?.['recipe-based'] === 'true';

  const [localIsExpanded, setLocalIsExpanded] = useState(false);
  const displayIsExpanded = expandedParents ? isExpanded : localIsExpanded;

  const expandedContainerRef = useRef<HTMLDivElement>(null);
  const [showAnimation, setShowAnimation] = useState(false);

  const [isDragOver, setIsDragOver] = useState(false);
  const [isDragOverWant, setIsDragOverWant] = useState(false);

  // Long-press for overlay
  const longPressTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const longPressPos = useRef<{ x: number; y: number } | null>(null);

  const startLongPress = (x: number, y: number) => {
    if (isBeingProcessed || isSelectMode) return;
    longPressPos.current = { x, y };
    longPressTimer.current = setTimeout(() => {
      if (longPressPos.current) {
        setQuickActionsWantId(wantId || null);
        longPressTimer.current = null;
      }
    }, 600); // 600ms for long press
  };

  const cancelLongPress = () => {
    if (longPressTimer.current) {
      clearTimeout(longPressTimer.current);
      longPressTimer.current = null;
    }
    longPressPos.current = null;
  };

  const handleMouseDown = (e: React.MouseEvent) => {
    // Only left click starts long press
    if (e.button !== 0) return;
    startLongPress(e.clientX, e.clientY);
  };

  const handleMouseMove = (e: React.MouseEvent) => {
    if (longPressPos.current) {
      const dist = Math.sqrt(
        Math.pow(e.clientX - longPressPos.current.x, 2) +
        Math.pow(e.clientY - longPressPos.current.y, 2)
      );
      if (dist > 10) cancelLongPress();
    }
  };

  const handleTouchStart = (e: React.TouchEvent) => {
    if (isBeingProcessed || isSelectMode) return;
    const touch = e.touches[0];
    startLongPress(touch.clientX, touch.clientY);
  };

  const handleTouchMove = (e: React.TouchEvent) => {
    if (longPressPos.current) {
      const touch = e.touches[0];
      const dist = Math.sqrt(
        Math.pow(touch.clientX - longPressPos.current.x, 2) +
        Math.pow(touch.clientY - longPressPos.current.y, 2)
      );
      if (dist > 10) cancelLongPress();
    }
  };

  const handleTouchEnd = () => {
    cancelLongPress();
  };
  const cardRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const wId = want.metadata?.id || want.id;
    const selectedId = selectedWant?.metadata?.id || selectedWant?.id;
    const isNavSelected = selectedId === wId;

    if (isNavSelected && document.activeElement !== cardRef.current) {
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

  useEffect(() => {
    if (displayIsExpanded && expandedContainerRef.current) {
      setShowAnimation(false);
      const timer = requestAnimationFrame(() => {
        setShowAnimation(true);
      });
      return () => cancelAnimationFrame(timer);
    } else {
      setShowAnimation(false);
    }
  }, [displayIsExpanded]);

  const handleContextMenu = (e: React.MouseEvent) => {
    if (isBeingProcessed || isSelectMode) return;
    e.preventDefault();
    setQuickActionsWantId(wantId || null);
    onView(want);
  };

  const handleCardClick = (e: React.MouseEvent) => {
    if (isBeingProcessed) return;
    const target = e.target as HTMLElement;
    if (target.closest('button') || target.closest('[role="button"]')) {
      return;
    }
    let element = target as HTMLElement | null;
    while (element && element !== e.currentTarget) {
      if (element.className && element.className.includes('group/menu')) {
        const menuDropdown = element.querySelector('[class*="opacity-100"][class*="visible"]');
        if (menuDropdown) return;
      }
      element = element.parentElement;
    }
    onView(want);
  };

  const handleDragStart = (e: React.DragEvent) => {
    if (isSelectMode || isBeingProcessed) return;
    suppressDragImage(e);
    const id = want.metadata?.id || want.id;
    if (!id) return;
    setDraggingWant(id);
    e.dataTransfer.setData('application/mywant-id', id);
    e.dataTransfer.setData('application/mywant-name', want.metadata?.name || '');
    e.dataTransfer.effectAllowed = 'move';
  };

  const handleDragOver = (e: React.DragEvent) => {
    if (isBeingProcessed) return;

    const { draggingWant: currentDraggingWant } = useWantStore.getState();
    const isWantDrag = !!currentDraggingWant || e.dataTransfer.types.includes('application/mywant-id');
    const isLabelDrag = e.dataTransfer.types.includes('application/json');
    const isTemplateDrag = e.dataTransfer.types.includes('application/mywant-template');

    if (isTemplateDrag) return;

    if (isWantDrag) {
      e.preventDefault();
      setIsDragOver(false);

      const rect = e.currentTarget.getBoundingClientRect();
      const x = e.clientX - rect.left;
      const edgeThreshold = rect.width * 0.2;

      let position: 'before' | 'after' | 'inside' | null = null;

      if (isTargetWant) {
        if (x < edgeThreshold) {
          position = 'before';
        } else if (x > rect.width - edgeThreshold) {
          position = 'after';
        } else {
          position = 'inside';
        }
      } else {
        position = x < rect.width / 2 ? 'before' : 'after';
      }

      if (onReorderDragOver) {
        onReorderDragOver(index, position);
      }

      if (position === 'inside') {
        e.dataTransfer.dropEffect = 'move';
        setIsOverTarget(true);
        if (!isDragOverWant) setIsDragOverWant(true);
      } else {
        setIsOverTarget(false);
        setIsDragOverWant(false);
        e.dataTransfer.dropEffect = 'move';
      }
    } else if (isLabelDrag) {
      e.preventDefault();
      setIsDragOverWant(false);
      setIsOverTarget(false);
      e.dataTransfer.dropEffect = 'copy';
      setIsDragOver(true);
    }
  };

  const handleDragLeave = () => {
    if (isBeingProcessed) return;
    setIsDragOver(false);
    setIsDragOverWant(false);
    setIsOverTarget(false);
  };

  const handleDrop = (e: React.DragEvent) => {
    if (isBeingProcessed) return;
    if (e.dataTransfer.types.includes('application/mywant-template')) return;

    e.preventDefault();
    e.stopPropagation();
    setIsDragOver(false);
    setIsDragOverWant(false);
    setIsOverTarget(false);
    setDraggingWant(null);

    if (onReorderDragOver) {
      onReorderDragOver(index, null);
    }

    const draggedWantId = e.dataTransfer.getData('application/mywant-id');
    const tWantId = want.metadata?.id || want.id;

    if (!draggedWantId || !tWantId) return;
    if (draggedWantId === tWantId) return;

    const rect = e.currentTarget.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const edgeThreshold = rect.width * 0.2;

    if (x < edgeThreshold) {
      if (onReorderDrop) onReorderDrop(draggedWantId, index, 'before');
      return;
    } else if (x > rect.width - edgeThreshold) {
      if (onReorderDrop) onReorderDrop(draggedWantId, index, 'after');
      return;
    } else if (isTargetWant) {
      if (onWantDropped) onWantDropped(draggedWantId, tWantId);
      return;
    }

    if (onReorderDrop) {
      onReorderDrop(draggedWantId, index, x < rect.width / 2 ? 'before' : 'after');
    }

    try {
      const labelData = e.dataTransfer.getData('application/json');
      if (!labelData) return;
      const { key, value } = JSON.parse(labelData);
      fetch(`/api/v1/wants/${tWantId}/labels`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ key, value })
      }).then(response => {
        if (response.ok && onLabelDropped) onLabelDropped(tWantId);
      }).catch(error => console.error('Error dropping label:', error));
    } catch (error) {
      console.error('Error dropping label:', error);
    }
  };

  const parentBackgroundStyle = getBackgroundStyle(want.metadata?.type, false);
  const achievingPercentage = (want.state?.current?.achieving_percentage as number) ?? 0;
  const replayScreenshotUrl = want.state?.current?.replay_screenshot_url as string | undefined;
  const version = want.metadata?.version ?? 1;

  return (
    <div className="relative h-full" style={{ isolation: 'isolate' }}>
      <StackLayers stackCount={stackCount} />
      <div
        ref={cardRef}
        draggable={!isSelectMode && !isBeingProcessed}
        onDragStart={handleDragStart}
        onDragEnd={() => setDraggingWant(null)}
        onClick={handleCardClick}
        onContextMenu={handleContextMenu}
        onMouseDown={handleMouseDown}
        onMouseMove={handleMouseMove}
        onMouseUp={cancelLongPress}
        onMouseLeave={() => {
          cancelLongPress();
          if (showQuickActions) setQuickActionsWantId(null);
        }}
        onBlur={(e) => {
          const relatedTarget = e.relatedTarget as Node;
          if (showQuickActions && (!relatedTarget || !cardRef.current?.contains(relatedTarget))) {
            setQuickActionsWantId(null);
          }
        }}
        onTouchStart={handleTouchStart}
        onTouchMove={handleTouchMove}
        onTouchEnd={handleTouchEnd}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        tabIndex={isBeingProcessed ? -1 : 0}
        data-keyboard-nav-selected={selected}
        data-keyboard-nav-id={wantId}
        data-is-target={isTargetWant}
        className={classNames(
          'card hover:shadow-md dark:hover:shadow-blue-900/20 transition-all duration-300 group relative overflow-hidden h-full min-h-[8rem] sm:min-h-[12.5rem] flex flex-col focus:outline-none focus:ring-2 focus:ring-blue-400 dark:focus:ring-blue-500 focus:ring-inset',
          selected ? 'border-blue-500 border-2' : 'border-gray-200 dark:border-gray-700',
          (isDragOverWant || isDragOver) && !isBeingProcessed && 'border-blue-600 border-2 bg-blue-100 dark:bg-blue-900/30',
          isHighlighted && styles.highlighted,
          isBlinking && styles.minimapBlink,
          isBeingProcessed && 'opacity-50 pointer-events-none cursor-not-allowed',
          parentBackgroundStyle.className,
          className || ''
        )}
        style={{ ...parentBackgroundStyle.style }}
      >
        <ProgressBars achievingPercentage={achievingPercentage} />
        <CorrelationOverlay rate={correlationRate} />

        {replayScreenshotUrl && (
          <div
            className="absolute inset-0 z-0 pointer-events-none rounded-[inherit]"
            style={{
              backgroundImage: `url(${replayScreenshotUrl})`,
              backgroundSize: 'cover',
              backgroundPosition: 'center top',
              opacity: 0.12,
            }}
          />
        )}

        <div className={classNames(
          'absolute inset-0 z-30 flex items-center justify-center bg-blue-700 dark:bg-blue-900 transition-all duration-400 ease-out pointer-events-none',
          isDragOverWant && isTargetWant && !isBeingProcessed ? 'bg-opacity-60 opacity-100' : 'bg-opacity-0 opacity-0'
        )}>
          <div className={classNames(
            'bg-white dark:bg-gray-800 p-4 rounded-full shadow-2xl border-4 border-blue-600 dark:border-blue-500 transform transition-all duration-400 ease-out',
            isDragOverWant && isTargetWant && !isBeingProcessed ? 'scale-100 opacity-100' : 'scale-[2.5] opacity-0'
          )}>
            <Plus className="w-16 h-12 text-blue-700 dark:text-blue-400" />
          </div>
        </div>

        {isSelectMode && (
          <div className="absolute top-2 right-2 z-20 pointer-events-none">
            {selected ? <CheckSquare className="w-6 h-6 text-blue-600 bg-white rounded-md" /> : <Square className="w-6 h-6 text-gray-400 bg-white rounded-md opacity-50" />}
          </div>
        )}

        <VersionBadge version={version} />

        <div
          className="relative z-10 transition-all duration-150"
          style={showQuickActions ? { filter: 'blur(2px)', opacity: 0.5, pointerEvents: 'none' } : undefined}
        >
          <WantCardContent
            want={want} isChild={false} hasChildren={!!hasChildren}
            onView={onView} onViewAgents={onViewAgents} onViewResults={onViewResults}
            onEdit={onEdit} onDelete={onDelete}
            onSuspend={onSuspend} onResume={onResume}
            onShowReactionConfirmation={onShowReactionConfirmation}
          />
        </div>

        {showQuickActions && (
          <QuickActionsOverlay
            want={want}
            onClose={() => setQuickActionsWantId(null)}
            onView={() => onView(want)}
            onStart={() => startWant(wantId || '')}
            onStop={() => stopWant(wantId || '')}
            onSuspend={() => onSuspend?.(want)}
            onResume={() => onResume?.(want)}
            onRestart={async () => {
              await stopWant(wantId || '');
              setTimeout(() => startWant(wantId || ''), 300);
            }}
            onEdit={() => onEdit(want)}
            onDelete={() => onDelete(want)}
          />
        )}

        {(hasChildren || isRecipeBased) && !displayIsExpanded && (
          <div className="relative z-10 mt-auto border-t border-gray-200 dark:border-gray-700">
            <button
              onClick={(e) => {
                e.stopPropagation();
                if (expandedParents && onToggleExpand && wantId) onToggleExpand(wantId);
                else setLocalIsExpanded(true);
              }}
              className={classNames('flex items-center gap-2 sm:gap-3 w-full pt-3 pb-1 px-1 rounded-b-xl text-sm text-gray-600 dark:text-gray-400 bg-black/10 dark:bg-white/10 hover:bg-black/15 dark:hover:bg-white/20 transition-colors', isBeingProcessed && 'pointer-events-none')}
              disabled={isBeingProcessed}
            >
              <ChevronDown className="h-4 w-4 sm:h-5 sm:w-5 text-blue-500 dark:text-blue-400 flex-shrink-0" strokeWidth={2.5} />
              <span className="text-blue-600 dark:text-blue-400 font-medium text-xs sm:text-sm">{children?.length ?? 0} child want{(children?.length ?? 0) !== 1 ? 's' : ''}</span>
              {children && children.length > 0 && (
                <div className="flex items-center gap-1 sm:gap-1.5" title={`${children.length} child want${children.length !== 1 ? 's' : ''}`}>
                  {children.map((child, idx) => (
                    <div key={idx} className={`w-1.5 h-1.5 sm:w-2 sm:h-2 rounded-full flex-shrink-0 ${(child.status === 'reaching' || child.status === 'waiting_user_action') ? styles.pulseGlow : ''}`} style={{ backgroundColor: getStatusHexColor(child.status) }} title={child.status} />
                  ))}
                </div>
              )}
            </button>
          </div>
        )}

        {(hasChildren || isRecipeBased) && displayIsExpanded && (
          <div ref={expandedContainerRef} className="relative z-10 mt-2 sm:mt-4 border-t border-gray-200 dark:border-gray-700 transition-opacity duration-300 ease-out" style={{ opacity: showAnimation ? 1 : 0 } as React.CSSProperties}>
            <button
              onClick={(e) => {
                e.stopPropagation();
                if (expandedParents && onToggleExpand && wantId) onToggleExpand(wantId);
                else setLocalIsExpanded(false);
              }}
              className={classNames('flex items-center gap-1.5 w-full pt-2 sm:pt-3 pb-2 px-1 mb-1 text-xs sm:text-sm font-medium text-gray-900 dark:text-white bg-black/10 dark:bg-white/10 hover:bg-black/15 dark:hover:bg-white/20 transition-colors', isBeingProcessed && 'pointer-events-none')}
              disabled={isBeingProcessed}
            >
              <ChevronUp className="h-4 w-4 sm:h-5 sm:w-5 text-blue-500 dark:text-blue-400 flex-shrink-0" strokeWidth={2.5} />
              Child Wants ({children?.length ?? 0})
            </button>
            <div className="pt-1 sm:pt-2">
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2 sm:gap-3 transition-all duration-300 ease-out" style={{ opacity: showAnimation ? 1 : 0, transform: showAnimation ? 'translateY(0)' : 'translateY(-8px)' } as React.CSSProperties}>
                {(children ?? []).sort((a, b) => (a.metadata?.id || a.id || '').localeCompare(b.metadata?.id || b.id || '')).map((child) => {
                  const childId = child.metadata?.id || child.id || '';
                  const isChildTarget = child.metadata?.type.toLowerCase().includes('target') ||
                    child.metadata?.type.toLowerCase() === 'owner' ||
                    child.metadata?.type.toLowerCase().includes('approval');
                  const childCorrelationRate = correlationHighlights?.get(childId);

                  return (
                    <ChildWantCard
                      key={childId || child.id}
                      child={child}
                      selectedWant={selectedWant}
                      correlationRate={childCorrelationRate}
                      isSelectMode={isSelectMode}
                      selectedWantIds={selectedWantIds}
                      isBeingProcessed={isBeingProcessed}
                      isTarget={isChildTarget}
                      onView={onView}
                      onViewAgents={onViewAgents}
                      onViewResults={onViewResults}
                      onWantDropped={onWantDropped}
                      onLabelDropped={onLabelDropped}
                    />
                  );
                })}

                {onCreateWant && (
                  <button
                    onClick={(e) => { e.stopPropagation(); onCreateWant(want); }}
                    className="flex flex-col items-center justify-center p-3 rounded-md border-2 border-dashed border-gray-300 dark:border-gray-700 hover:border-blue-500 dark:hover:border-blue-500 transition-colors group min-h-[6rem]"
                  >
                    <span className="relative inline-flex flex-shrink-0 mb-1.5">
                      <Heart className="w-6 h-6 text-gray-400 dark:text-gray-500 group-hover:text-blue-500 dark:group-hover:text-blue-400 transition-colors" />
                      <Plus className="w-3 h-3 absolute -top-1.5 -right-1.5 text-gray-400 dark:text-gray-500 group-hover:text-blue-500 dark:group-hover:text-blue-400 transition-colors" style={{ strokeWidth: 3 }} />
                    </span>
                    <p className="text-xs text-gray-500 dark:text-gray-400 group-hover:text-blue-600 dark:group-hover:text-blue-400 transition-colors font-medium">Add Want</p>
                  </button>
                )}
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Right-click context menu */}
    </div>
  );
};
