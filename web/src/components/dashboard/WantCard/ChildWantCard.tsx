import React, { useState } from 'react';
import { CheckSquare, Square, Plus } from 'lucide-react';
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

interface ChildWantCardProps {
  child: Want;
  selectedWant?: Want | null;
  correlationRate?: number;
  isSelectMode: boolean;
  selectedWantIds?: Set<string>;
  isBeingProcessed: boolean;
  isTarget: boolean;
  onView: (want: Want) => void;
  onViewAgents?: (want: Want) => void;
  onViewResults?: (want: Want) => void;
  onWantDropped?: (draggedId: string, targetId: string) => void;
  onLabelDropped?: (wantId: string) => void;
}

export const ChildWantCard: React.FC<ChildWantCardProps> = ({
  child,
  selectedWant,
  correlationRate,
  isSelectMode,
  selectedWantIds,
  isBeingProcessed,
  isTarget,
  onView,
  onViewAgents,
  onViewResults,
  onWantDropped,
  onLabelDropped,
}) => {
  const childId = child.metadata?.id || child.id || '';
  const { draggingWant, setDraggingWant, setIsOverTarget, highlightedLabel, blinkingWantId } = useWantStore();

  const [isDragOver, setIsDragOver] = useState(false);
  const [isDragOverWant, setIsDragOverWant] = useState(false);

  const isChildSelected = isSelectMode
    ? selectedWantIds?.has(childId)
    : selectedWant && (
        (selectedWant.metadata?.id && selectedWant.metadata.id === child.metadata?.id) ||
        (selectedWant.id && selectedWant.id === child.id)
      );

  const isHighlighted = highlightedLabel &&
    child.metadata?.labels &&
    child.metadata.labels[highlightedLabel.key] === highlightedLabel.value;

  const isBlinking = blinkingWantId === childId;
  const childBackgroundStyle = getBackgroundStyle(child.metadata?.type, false);
  const childAchievingPercentage = (child.state?.current?.achieving_percentage as number) ?? 0;
  const childStackCount = Math.min((child.metadata?.version ?? 1) - 1, 3);
  const childVersion = child.metadata?.version ?? 1;

  const handleClick = (e: React.MouseEvent) => {
    if (isBeingProcessed) return;
    if (e.defaultPrevented) return;
    e.preventDefault();
    e.stopPropagation();
    onView(child);
  };

  const handleDragStart = (e: React.DragEvent) => {
    e.stopPropagation();
    suppressDragImage(e);
    const id = child.metadata?.id || child.id;
    if (!id) return;
    setDraggingWant(id);
    e.dataTransfer.setData('application/mywant-id', id);
    e.dataTransfer.setData('application/mywant-name', child.metadata?.name || '');
    e.dataTransfer.effectAllowed = 'move';
  };

  const handleDragOver = (e: React.DragEvent) => {
    if (isBeingProcessed) return;
    const isWantDrag = !!draggingWant || e.dataTransfer.types.includes('application/mywant-id');
    const isTemplateDrag = e.dataTransfer.types.includes('application/mywant-template');

    if (isTemplateDrag) return; 

    if (isWantDrag) {
      e.preventDefault();
      e.stopPropagation();
      setIsDragOverWant(true);
      setIsOverTarget(true);
      e.dataTransfer.dropEffect = 'move';
    } else if (e.dataTransfer.types.includes('application/json')) {
      e.preventDefault();
      e.stopPropagation();
      setIsDragOverWant(false);
      setIsOverTarget(false);
      setIsDragOver(true);
      e.dataTransfer.dropEffect = 'copy';
    }
  };

  const handleDragLeave = () => {
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

    const draggedWantId = e.dataTransfer.getData('application/mywant-id');
    if (draggedWantId && childId && isTarget) {
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
        body: JSON.stringify({ key, value }),
      }).then(response => {
        if (response.ok && onLabelDropped) onLabelDropped(childId);
      }).catch(error => console.error('Error dropping label on child:', error));
    } catch (error) {
      console.error('Error dropping label on child:', error);
    }
  };

  return (
    <div className="relative" style={{ isolation: 'isolate' }}>
      <StackLayers stackCount={childStackCount} isChild />
      <div
        data-keyboard-nav-selected={isChildSelected}
        data-keyboard-nav-id={childId}
        tabIndex={isBeingProcessed ? -1 : 0}
        draggable={!isBeingProcessed}
        onDragStart={handleDragStart}
        onDragEnd={(e) => { e.stopPropagation(); setDraggingWant(null); }}
        className={classNames(
          'relative overflow-hidden rounded-md border hover:shadow-sm transition-all duration-300 cursor-pointer focus:outline-none focus:ring-2 focus:ring-blue-400 dark:focus:ring-blue-500 focus:ring-inset min-h-[7rem]',
          isChildSelected ? 'border-blue-500 border-2' : 'border-gray-200 dark:border-gray-700 hover:border-gray-300',
          (isDragOverWant || isDragOver) && !isBeingProcessed && 'border-blue-600 border-2 bg-blue-100 dark:bg-blue-900/30',
          isHighlighted && styles.highlighted,
          isBlinking && styles.minimapBlink,
          isBeingProcessed && 'opacity-50 pointer-events-none cursor-not-allowed',
          childBackgroundStyle.className,
        )}
        style={{ ...childBackgroundStyle.style }}
        onClick={handleClick}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
      >
        <VersionBadge version={childVersion} />
        <ProgressBars achievingPercentage={childAchievingPercentage} />
        <CorrelationOverlay rate={correlationRate} />

        {/* Drop target overlay */}
        <div className={classNames(
          'absolute inset-0 z-30 flex items-center justify-center bg-blue-700 dark:bg-blue-900 transition-all duration-400 ease-out pointer-events-none',
          isDragOverWant && isTarget && !isBeingProcessed ? 'bg-opacity-60 opacity-100' : 'bg-opacity-0 opacity-0',
        )}>
          <div className={classNames(
            'bg-white dark:bg-gray-800 p-2 rounded-full shadow-xl border-2 border-blue-600 dark:border-blue-500 transform transition-all duration-400 ease-out',
            isDragOverWant && isTarget && !isBeingProcessed ? 'scale-100 opacity-100' : 'scale-[2.5] opacity-0',
          )}>
            <Plus className="w-6 h-6 text-blue-700 dark:text-blue-400" />
          </div>
        </div>

        {isSelectMode && (
          <div className="absolute top-2 right-2 z-20 pointer-events-none">
            {isChildSelected
              ? <CheckSquare className="w-5 h-5 text-blue-600 bg-white rounded-md" />
              : <Square className="w-5 h-5 text-gray-400 bg-white rounded-md opacity-50" />}
          </div>
        )}

        <div className="p-2 sm:p-4 w-full h-full relative z-10">
          <WantCardContent
            want={child}
            isChild={true}
            onView={onView}
            onViewAgents={onViewAgents}
            onViewResults={onViewResults}
          />
        </div>
      </div>
    </div>
  );
};
