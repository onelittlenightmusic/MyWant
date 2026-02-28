import React, { useState, useEffect, useRef } from 'react';
import { ChevronDown, ChevronUp, CheckSquare, Square, Plus, Heart } from 'lucide-react';
import { Want, WantExecutionStatus } from '@/types/want';
import { WantCardContent } from './WantCardContent';
import { classNames, suppressDragImage } from '@/utils/helpers';
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
    case 'module_error':
      return '#ef4444'; // Red
    case 'config_error':
    case 'stopped':
    case 'waiting_user_action':
      return '#f59e0b'; // Amber/Yellow
    default:
      return '#d1d5db'; // Gray for created, initializing, suspended
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
  onReorderDragOver?: (index: number, position: 'before' | 'after' | 'inside' | null) => void;
  onReorderDrop?: (draggedId: string, index: number, position: 'before' | 'after') => void;
  index: number;
  isSelectMode?: boolean;
  selectedWantIds?: Set<string>;
  isBeingProcessed?: boolean; // New prop for initializing/deleting states
  onCreateWant?: (parentWant?: Want) => void;
  correlationRate?: number; // When set (radar mode active), highlight this card with blue intensity based on rate
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
}) => {
  const wantId = want.metadata?.id || want.id;
  const { setDraggingWant, setIsOverTarget, highlightedLabel, blinkingWantId } = useWantStore();
  const isExpanded = expandedParents?.has(wantId || '') ?? false;
  const hasChildren = children && children.length > 0;

  // Check if this want is highlighted by label selection
  const isHighlighted = highlightedLabel &&
                        want.metadata?.labels &&
                        want.metadata.labels[highlightedLabel.key] === highlightedLabel.value;

  // Check if this want is blinking from minimap click
  const isBlinking = blinkingWantId === wantId;

  // Correlation radar highlight: blue emboss based on rate (1=light, 2=medium, 3+=strong)
  // backgroundColor is intentionally NOT in inline style — it's handled by the overlay div animation.
  const isCorrelated = correlationRate !== undefined && correlationRate > 0;
  const correlationStyle = isCorrelated ? (() => {
    const rate = correlationRate!;
    const alpha = Math.min(0.3 + rate * 0.15, 0.75);
    const glowAlpha = Math.min(0.15 + rate * 0.1, 0.45);
    return {
      boxShadow: `inset 0 1px 2px rgba(255,255,255,0.35), 0 0 0 2px rgba(59,130,246,${alpha}), 0 4px 16px rgba(59,130,246,${glowAlpha})`,
    };
  })() : undefined;
  // CSS custom property for the overlay animation peak opacity (rate-dependent)
  const correlationOverlayVars = isCorrelated
    ? ({ '--corr-peak': String(Math.min(0.12 + correlationRate! * 0.1, 0.45)) } as React.CSSProperties)
    : undefined;

  // Identify if this want is a Target (can have children)
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
  const [draggedOverChildId, setDraggedOverChildId] = useState<string | null>(null);
  const cardRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const wantId = want.metadata?.id || want.id;
    const selectedId = selectedWant?.metadata?.id || selectedWant?.id;
    const isNavSelected = selectedId === wantId;

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

  const handleChildCardClick = (child: Want) => (e: React.MouseEvent) => {
    if (isBeingProcessed) return;
    if (e.defaultPrevented) return;
    e.preventDefault();
    e.stopPropagation();
    onView(child);
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

  const handleDragEnd = () => {
    setDraggingWant(null);
  };

  const handleDragOver = (e: React.DragEvent) => {
    if (isBeingProcessed) return;

    const { draggingWant } = useWantStore.getState();
    const isWantDrag = !!draggingWant || e.dataTransfer.types.includes('application/mywant-id');
    const isLabelDrag = e.dataTransfer.types.includes('application/json');
    const isTemplateDrag = e.dataTransfer.types.includes('application/mywant-template');
    
    // テンプレートのドラッグ時は、カード側では処理せずバブリングさせて Dashboard で処理する
    if (isTemplateDrag) {
      return; 
    }

    if (isWantDrag) {
      e.preventDefault();
      setIsDragOver(false);

      // Determine position within the card for reordering vs child-drop
      const rect = e.currentTarget.getBoundingClientRect();
      const x = e.clientX - rect.left;
      const edgeThreshold = rect.width * 0.2; // 20% from edges
      
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
        // For non-target wants, always show reorder position based on which side we're closer to
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

  const handleDragLeave = (e: React.DragEvent) => {
    if (isBeingProcessed) return;
    setIsDragOver(false);
    setIsDragOverWant(false);
    setIsOverTarget(false);
    setDraggedOverChildId(null);
    // Note: We don't call onReorderDragOver(index, null) here to prevent flickering 
    // when moving between cards or into gaps. The Grid container handles clearing.
  };

  const handleDrop = (e: React.DragEvent) => {
    if (isBeingProcessed) return;

    // テンプレートのドロップ時は何もしない（バブリングさせて Dashboard で処理する）
    if (e.dataTransfer.types.includes('application/mywant-template')) {
      return;
    }

    e.preventDefault();
    e.stopPropagation();
    setIsDragOver(false);
    setIsDragOverWant(false);
    setIsOverTarget(false);
    setDraggedOverChildId(null);
    setDraggingWant(null);
    
    if (onReorderDragOver) {
      onReorderDragOver(index, null);
    }

    const draggedWantId = e.dataTransfer.getData('application/mywant-id');
    const targetWantId = want.metadata?.id || want.id;

    if (!draggedWantId || !targetWantId) return;
    if (draggedWantId === targetWantId) return;

    // Determine position for drop
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
      if (onWantDropped) onWantDropped(draggedWantId, targetWantId);
      return;
    }
    
    // Default to reorder if not a target
    if (onReorderDrop) {
      onReorderDrop(draggedWantId, index, x < rect.width / 2 ? 'before' : 'after');
    }

    try {
      const labelData = e.dataTransfer.getData('application/json');
      if (!labelData) return;
      const { key, value } = JSON.parse(labelData);
      const wantId = want.metadata?.id || want.id;
      if (!wantId) return;

      fetch(`/api/v1/wants/${wantId}/labels`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ key, value })
      }).then(response => {
        if (response.ok && onLabelDropped) onLabelDropped(wantId);
      }).catch(error => console.error('Error dropping label:', error));
    } catch (error) {
      console.error('Error dropping label:', error);
    }
  };

  const parentBackgroundStyle = getBackgroundStyle(want.metadata?.type, false);
  const achievingPercentage = (want.state?.achieving_percentage as number) ?? 0;
  const replayScreenshotUrl = want.state?.replay_screenshot_url as string | undefined;

  const whiteProgressBarStyle = {
    position: 'absolute' as const,
    top: 0, left: 0, height: '100%', width: `${achievingPercentage}%`,
    background: 'var(--progress-bg-light)',
    transition: 'width 0.3s ease-out',
    zIndex: 0, pointerEvents: 'none' as const
  };

  const blackOverlayStyle = {
    position: 'absolute' as const,
    top: 0, left: `${achievingPercentage}%`, height: '100%', width: `${100 - achievingPercentage}%`,
    background: 'var(--progress-bg-dark)',
    transition: 'width 0.3s ease-out, left 0.3s ease-out',
    zIndex: 0, pointerEvents: 'none' as const
  };

  return (
    <div
      ref={cardRef}
      draggable={!isSelectMode && !isBeingProcessed}
      onDragStart={handleDragStart}
      onDragEnd={handleDragEnd}
      onClick={handleCardClick}
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
      style={{ ...parentBackgroundStyle.style, ...correlationStyle }}
    >
      <div style={whiteProgressBarStyle}></div>
      <div style={blackOverlayStyle}></div>
      {/* Correlation radar overlay: pulses blue transparent over the whole card */}
      {isCorrelated && (
        <div
          className={styles.correlationOverlay}
          style={correlationOverlayVars}
        />
      )}

      {/* Replay screenshot as subtle card background */}
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
        "absolute inset-0 z-30 flex items-center justify-center bg-blue-700 dark:bg-blue-900 transition-all duration-400 ease-out pointer-events-none",
        isDragOverWant && isTargetWant && !draggedOverChildId && !isBeingProcessed ? "bg-opacity-60 opacity-100" : "bg-opacity-0 opacity-0"
      )}>
        <div className={classNames(
          "bg-white dark:bg-gray-800 p-4 rounded-full shadow-2xl border-4 border-blue-600 dark:border-blue-500 transform transition-all duration-400 ease-out",
          isDragOverWant && isTargetWant && !draggedOverChildId && !isBeingProcessed ? "scale-100 opacity-100" : "scale-[2.5] opacity-0"
        )}>
          <Plus className="w-16 h-12 text-blue-700 dark:text-blue-400" />
        </div>
      </div>

      {isSelectMode && (
        <div className="absolute top-2 right-2 z-20 pointer-events-none">
          {selected ? <CheckSquare className="w-6 h-6 text-blue-600 bg-white rounded-md" /> : <Square className="w-6 h-6 text-gray-400 bg-white rounded-md opacity-50" />}
        </div>
      )}

      <div className="relative z-10">
        <WantCardContent
          want={want} isChild={false} hasChildren={!!hasChildren}
          onView={onView} onViewAgents={onViewAgents} onViewResults={onViewResults}
          onEdit={onEdit} onDelete={onDelete}
          onSuspend={onSuspend} onResume={onResume}
          onShowReactionConfirmation={onShowReactionConfirmation}
        />
      </div>

      {(hasChildren || isRecipeBased) && !displayIsExpanded && (
        <div className="relative z-10 mt-auto border-t border-gray-200 dark:border-gray-700">
          <button
            onClick={(e) => {
              e.stopPropagation();
              if (expandedParents && onToggleExpand && wantId) onToggleExpand(wantId);
              else setLocalIsExpanded(true);
            }}
            className={classNames("flex items-center gap-2 sm:gap-3 w-full pt-3 pb-1 px-1 rounded-b-xl text-sm text-gray-600 dark:text-gray-400 bg-black/10 dark:bg-white/10 hover:bg-black/15 dark:hover:bg-white/20 transition-colors", isBeingProcessed && 'pointer-events-none')}
            disabled={isBeingProcessed}
          >
            <ChevronDown className="h-4 w-4 sm:h-5 sm:w-5 text-blue-500 dark:text-blue-400 flex-shrink-0" strokeWidth={2.5} />
            <span className="text-blue-600 dark:text-blue-400 font-medium text-xs sm:text-sm">{children?.length ?? 0} child want{(children?.length ?? 0) !== 1 ? 's' : ''}</span>
            {children && children.length > 0 && (
              <div className="flex items-center gap-1 sm:gap-1.5" title={`${children.length} child want${children.length !== 1 ? 's' : ''}`}>
                {children.map((child, idx) => (
                  <div key={idx} className={`w-1.5 h-1.5 sm:w-2 sm:h-2 rounded-full flex-shrink-0 ${(child.status === 'reaching' || child.status === 'waiting_user_action') ? styles.pulseGlow : ''}`} style={{ backgroundColor: getStatusColor(child.status) }} title={child.status} />
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
            className={classNames("flex items-center gap-1.5 w-full pt-2 sm:pt-3 pb-2 px-1 mb-1 text-xs sm:text-sm font-medium text-gray-900 dark:text-white bg-black/10 dark:bg-white/10 hover:bg-black/15 dark:hover:bg-white/20 transition-colors", isBeingProcessed && 'pointer-events-none')}
            disabled={isBeingProcessed}
          >
            <ChevronUp className="h-4 w-4 sm:h-5 sm:w-5 text-blue-500 dark:text-blue-400 flex-shrink-0" strokeWidth={2.5} />
            Child Wants ({children?.length ?? 0})
          </button>
          <div className="pt-1 sm:pt-2">
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2 sm:gap-3 transition-all duration-300 ease-out" style={{ opacity: showAnimation ? 1 : 0, transform: showAnimation ? 'translateY(0)' : 'translateY(-8px)' } as React.CSSProperties}>
            {(children ?? []).sort((a, b) => (a.metadata?.id || a.id || '').localeCompare(b.metadata?.id || b.id || '')).map((child, index) => {
              const childId = child.metadata?.id || child.id || '';
              const isChildSelected = isSelectMode ? (selectedWantIds?.has(childId)) : (selectedWant && ((selectedWant.metadata?.id && selectedWant.metadata.id === child.metadata?.id) || (selectedWant.id && selectedWant.id === child.id)));
              const childBackgroundStyle = getBackgroundStyle(child.metadata?.type, false);
              const childAchievingPercentage = (child.state?.achieving_percentage as number) ?? 0;
              const isChildTarget = child.metadata?.type.toLowerCase().includes('target') || child.metadata?.type.toLowerCase() === 'owner' || child.metadata?.type.toLowerCase().includes('approval');

              const handleChildDragOver = (e: React.DragEvent) => {
                if (isBeingProcessed) return;
                const { draggingWant } = useWantStore.getState();
                const isWantDrag = !!draggingWant || e.dataTransfer.types.includes('application/mywant-id');
                const isTemplateDrag = e.dataTransfer.types.includes('application/mywant-template');
                
                if (isTemplateDrag) return; // Bubble to Dashboard

                if (isWantDrag) {
                  e.preventDefault();
                  if (isChildTarget) {
                    e.dataTransfer.dropEffect = 'move';
                    setIsDragOverWant(true); setIsOverTarget(true); setDraggedOverChildId(childId);
                  } else {
                    e.dataTransfer.dropEffect = 'move';
                    setIsDragOverWant(true); setIsOverTarget(true); setDraggedOverChildId(null);
                  }
                } else if (e.dataTransfer.types.includes('application/json')) {
                  e.preventDefault();
                  setIsDragOverWant(false); setIsOverTarget(false);
                  e.dataTransfer.dropEffect = 'copy'; setIsDragOver(true);
                }
              };

              const handleChildDrop = (e: React.DragEvent) => {
                if (isBeingProcessed) return;
                if (e.dataTransfer.types.includes('application/mywant-template')) return; // Bubble to Dashboard

                e.preventDefault(); e.stopPropagation();
                setIsDragOver(false); setIsDragOverWant(false); setIsOverTarget(false);
                setDraggedOverChildId(null); setDraggingWant(null);

                const draggedWantId = e.dataTransfer.getData('application/mywant-id');
                if (draggedWantId && childId && isChildTarget) {
                  if (draggedWantId === childId) return;
                  if (onWantDropped) onWantDropped(draggedWantId, childId);
                  return;
                }

                try {
                  const labelData = e.dataTransfer.getData('application/json');
                  if (!labelData) return;
                  const { key, value } = JSON.parse(labelData);
                  fetch(`/api/v1/wants/${childId}/labels`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ key, value })
                  }).then(response => {
                    if (response.ok && onLabelDropped) onLabelDropped(childId);
                  });
                } catch (error) { console.error('Error dropping label on child:', error); }
              };

              return (
                <div
                  key={childId || `child-${index}`}
                  data-keyboard-nav-selected={isChildSelected}
                  data-keyboard-nav-id={childId}
                  tabIndex={isBeingProcessed ? -1 : 0}
                  draggable={true && !isBeingProcessed}
                  onDragStart={(e) => {
                    e.stopPropagation();
                    suppressDragImage(e);
                    const id = child.metadata?.id || child.id;
                    if (!id) return;
                    setDraggingWant(id);
                    e.dataTransfer.setData('application/mywant-id', id);
                    e.dataTransfer.setData('application/mywant-name', child.metadata?.name || '');
                    e.dataTransfer.effectAllowed = 'move';
                  }}
                  onDragEnd={(e) => { e.stopPropagation(); setDraggingWant(null); }}
                  className={classNames(
                    "relative overflow-hidden rounded-md border hover:shadow-sm transition-all duration-300 cursor-pointer focus:outline-none focus:ring-2 focus:ring-blue-400 focus:ring-inset",
                    isChildSelected ? 'border-blue-500 border-2' : 'border-gray-200 hover:border-gray-300',
                    (isDragOverWant || isDragOver) && !isBeingProcessed && 'border-blue-600 border-2 bg-blue-100',
                    isBeingProcessed && 'opacity-50 pointer-events-none cursor-not-allowed',
                    childBackgroundStyle.className
                  )}
                  style={childBackgroundStyle.style}
                  onClick={handleChildCardClick(child)}
                  onDragOver={handleChildDragOver}
                  onDragLeave={() => setDraggedOverChildId(null)}
                  onDrop={handleChildDrop}
                >
                  <div style={{ position: 'absolute', top: 0, left: 0, height: '100%', width: `${childAchievingPercentage}%`, background: 'var(--progress-bg-light)', transition: 'width 0.3s ease-out', zIndex: 0, pointerEvents: 'none' }}></div>
                  <div style={{ position: 'absolute', top: 0, left: `${childAchievingPercentage}%`, height: '100%', width: `${100 - childAchievingPercentage}%`, background: 'var(--progress-bg-dark)', transition: 'width 0.3s ease-out, left 0.3s ease-out', zIndex: 0, pointerEvents: 'none' }}></div>
                  
                  <div className={classNames("absolute inset-0 z-30 flex items-center justify-center bg-blue-700 transition-all duration-400 ease-out pointer-events-none", isDragOverWant && draggedOverChildId === childId && isChildTarget && !isBeingProcessed ? "bg-opacity-60 opacity-100" : "bg-opacity-0 opacity-0")}>
                    <div className={classNames("bg-white p-2 rounded-full shadow-xl border-2 border-blue-600 transform transition-all duration-400 ease-out", isDragOverWant && draggedOverChildId === childId && isChildTarget && !isBeingProcessed ? "scale-100 opacity-100" : "scale-[2.5] opacity-0")}>
                      <Plus className="w-6 h-6 text-blue-700" />
                    </div>
                  </div>

                  <div className="p-2 sm:p-4 w-full h-full relative z-10">
                    <WantCardContent want={child} isChild={true} onView={onView} onViewAgents={onViewAgents} onViewResults={onViewResults} />
                  </div>
                </div>
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
  );
};