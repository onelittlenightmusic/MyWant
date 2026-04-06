import React, { useState, useEffect, useRef } from 'react';
import { CheckSquare, Square, Plus, Heart } from 'lucide-react';
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

import { QuickActionsOverlay } from './parts/QuickActionsOverlay';
import { DeleteConfirmOverlay } from './parts/DeleteConfirmOverlay';
import { FocusTriangle } from './parts/FocusTriangle';
import { useLongPress } from './hooks/useLongPress';
import { useCardOverlay } from './hooks/useCardOverlay';
import { dropLabelOnWant } from './hooks/labelDrop';
import { CARD_BORDER_BASE, CARD_FOCUS_BASE } from './hooks/cardStyles';

interface WantCardProps {
  want: Want;
  children?: Want[];
  selected: boolean;
  selectedWant?: Want | null;
  onView: (want: Want) => void;
  onViewAgents?: (want: Want) => void;
  onViewResults?: (want: Want) => void;
  onViewChat?: (want: Want) => void;
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
  isBubbleOpen?: boolean;
}

export const WantCard: React.FC<WantCardProps> = ({
  want,
  children,
  selected,
  selectedWant,
  onView,
  onViewAgents,
  onViewResults,
  onViewChat,
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
  isBubbleOpen = false,
}) => {
  const wantId = want.metadata?.id || want.id;
  const { setDraggingWant, setIsOverTarget, highlightedLabel, blinkingWantId, startWant, stopWant } = useWantStore();

  const longPress = useLongPress(wantId || null, { disabled: isBeingProcessed || isSelectMode });
  const overlay = useCardOverlay(wantId || null);

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


  const [isDragOver, setIsDragOver] = useState(false);
  const [isDragOverWant, setIsDragOverWant] = useState(false);

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


  const handleContextMenu = (e: React.MouseEvent) => {
    if (isBeingProcessed || isSelectMode) return;
    e.preventDefault();
    overlay.setQuickActionsWantId(wantId || null);
    onView(want);
  };

  const handleCardClick = (e: React.MouseEvent) => {
    if (isBeingProcessed) return;
    const target = e.target as HTMLElement;
    if (target.closest('button') || target.closest('[role="button"]')) return;
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

      if (onReorderDragOver) onReorderDragOver(index, position);

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

    if (onReorderDragOver) onReorderDragOver(index, null);

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

    dropLabelOnWant(tWantId, e, onLabelDropped);
  };

  const parentBackgroundStyle = getBackgroundStyle(want.metadata?.type, false);
  const achievingPercentage = (want.state?.current?.achieving_percentage as number) ?? 0;
  const replayScreenshotUrl = want.state?.current?.replay_screenshot_url as string | undefined;
  const version = want.metadata?.version ?? 1;

  return (
    <div className="relative h-full" style={{ isolation: 'isolate' }}>
      <FocusTriangle visible={selected} />
      <StackLayers stackCount={stackCount} />
      <div
        ref={cardRef}
        draggable={!isSelectMode && !isBeingProcessed}
        onDragStart={handleDragStart}
        onDragEnd={() => setDraggingWant(null)}
        onClick={handleCardClick}
        onContextMenu={handleContextMenu}
        onMouseDown={longPress.onMouseDown}
        onMouseMove={longPress.onMouseMove}
        onMouseUp={longPress.onMouseUp}
        onMouseLeave={() => {
          longPress.cancel();
          if (overlay.showQuickActions) overlay.closeQuickActions();
        }}
        onBlur={(e) => {
          const relatedTarget = e.relatedTarget as Node;
          if (overlay.showQuickActions && (!relatedTarget || !cardRef.current?.contains(relatedTarget))) {
            overlay.closeQuickActions();
          }
        }}
        onTouchStart={longPress.onTouchStart}
        onTouchMove={longPress.onTouchMove}
        onTouchEnd={longPress.onTouchEnd}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        tabIndex={isBeingProcessed ? -1 : 0}
        data-keyboard-nav-selected={selected}
        data-keyboard-nav-id={wantId}
        data-is-target={isTargetWant}
        className={classNames(
          `card hover:shadow-md dark:hover:shadow-blue-900/20 transition-all duration-300 group relative overflow-hidden h-full min-h-[6rem] sm:min-h-[10rem] flex flex-col ${CARD_FOCUS_BASE}`,
          CARD_BORDER_BASE,
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
          style={overlay.showQuickActions ? { filter: 'blur(2px)', opacity: 0.5, pointerEvents: 'none' } : undefined}
        >
          <WantCardContent
            want={want} isChild={false} hasChildren={!!hasChildren} isFocused={selected} isSelectMode={isSelectMode}
            onView={onView} onViewAgents={onViewAgents} onViewResults={onViewResults} onViewChat={onViewChat}
            onEdit={onEdit} onDelete={onDelete}
            onSuspend={onSuspend} onResume={onResume}
            onShowReactionConfirmation={onShowReactionConfirmation}
          />
        </div>

        {overlay.showQuickActions && !overlay.showDeleteOverlay && (
          <QuickActionsOverlay
            want={want}
            onClose={overlay.closeQuickActions}
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
            onDelete={overlay.openDeleteConfirm}
          />
        )}

        {overlay.showDeleteOverlay && (
          <DeleteConfirmOverlay
            onConfirm={() => { overlay.confirmDelete(); onDelete(want); }}
            onCancel={overlay.closeDeleteConfirm}
          />
        )}

        {/* Child want count + status dots */}
        {hasChildren && (
          <div className={classNames(
            "absolute bottom-0 left-0 right-0 z-10 border-t border-gray-200/50 dark:border-gray-700/50 px-3 py-1.5 flex items-center gap-2",
            isBubbleOpen && "invisible"
          )}>
            <span className="text-blue-600 dark:text-blue-400 font-medium text-xs">{children!.length}</span>
            <div className="flex items-center gap-1">
              {children!.map((child, idx) => (
                <div
                  key={idx}
                  className={`w-2 h-2 rounded-full flex-shrink-0 ${(child.status === 'reaching' || child.status === 'waiting_user_action') ? styles.pulseGlow : ''}`}
                  style={{ backgroundColor: getStatusHexColor(child.status) }}
                  title={child.status}
                />
              ))}
            </div>
          </div>
        )}

      </div>

      {/* Right-click context menu */}
    </div>
  );
};
