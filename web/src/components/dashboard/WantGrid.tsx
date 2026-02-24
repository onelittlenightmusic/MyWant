import React, { useMemo, useState } from 'react';
import { Plus, Heart } from 'lucide-react';
import { Want, WantExecutionStatus } from '@/types/want';
import { DraftWant } from '@/types/draft';
import { WantCard } from './WantCard';
import { DraftWantCard } from './DraftWantCard';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { classNames } from '@/utils/helpers';
import { useWantStore } from '@/stores/wantStore';
import styles from './WantCard.module.css';

interface WantWithChildren extends Want {
  children?: Want[];
}

interface WantGridProps {
  wants: Want[];
  drafts?: DraftWant[];
  activeDraftId?: string | null;
  onDraftClick?: (draft: DraftWant) => void;
  onDraftDelete?: (draft: DraftWant) => void;
  loading: boolean;
  searchQuery: string;
  statusFilters: WantExecutionStatus[];
  selectedWant: Want | null;
  onViewWant: (want: Want) => void;
  onViewAgentsWant?: (want: Want) => void;
  onViewResultsWant?: (want: Want) => void;
  onEditWant: (want: Want) => void;
  onDeleteWant: (want: Want) => void;
  onSuspendWant?: (want: Want) => void;
  onResumeWant?: (want: Want) => void;
  onShowReactionConfirmation?: (want: Want, action: 'approve' | 'deny') => void;
  onGetFilteredWants?: (wants: Want[]) => void;
  expandedParents?: Set<string>;
  onToggleExpand?: (wantId: string) => void;
  onCreateWant?: (parentWant?: Want) => void;
  onLabelDropped?: (wantId: string) => void;
  onWantDropped?: (draggedWantId: string, targetWantId: string) => void;
  isSelectMode?: boolean;
  selectedWantIds?: Set<string>;
  onSelectWant?: (wantId: string) => void;
}

export const WantGrid: React.FC<WantGridProps> = ({
  wants,
  drafts = [],
  activeDraftId,
  onDraftClick,
  onDraftDelete,
  loading,
  searchQuery,
  statusFilters,
  selectedWant,
  onViewWant,
  onViewAgentsWant,
  onViewResultsWant,
  onEditWant,
  onDeleteWant,
  onSuspendWant,
  onResumeWant,
  onGetFilteredWants,
  expandedParents,
  onToggleExpand,
  onCreateWant,
  onLabelDropped,
  onWantDropped,
  onShowReactionConfirmation,
  isSelectMode = false,
  selectedWantIds = new Set(),
  onSelectWant
}) => {
  const { reorderWant, draggingWant } = useWantStore();
  const [dragOverGap, setDragOverGap] = useState<number | null>(null);

  // Clear drag indicator when dragging stops
  React.useEffect(() => {
    if (!draggingWant) {
      setDragOverGap(null);
    }
  }, [draggingWant]);

  const handleReorderDragOver = (index: number, position: 'before' | 'after' | 'inside' | null) => {
    if (position === 'before') {
      setDragOverGap(index);
    } else if (position === 'after') {
      setDragOverGap(index + 1);
    } else {
      setDragOverGap(null);
    }
  };

  const handleGridDragOver = (e: React.DragEvent) => {
    const isWantDrag = !!draggingWant || e.dataTransfer.types.includes('application/mywant-id');
    if (!isWantDrag) return;

    e.preventDefault();
    
    // If we are over a card, it's already handled by the card's onReorderDragOver
    if ((e.target as HTMLElement).closest('[data-want-id]')) return;

    // We are in a gap or over the grid background
    // Find the closest card and determine the gap
    const container = e.currentTarget as HTMLElement;
    const cardElements = Array.from(container.querySelectorAll('[data-want-id]')) as HTMLElement[];
    if (cardElements.length === 0) return;

    let closestIndex = -1;
    let minDistance = Infinity;
    let isAfter = false;

    cardElements.forEach((el, idx) => {
      const rect = el.getBoundingClientRect();
      const centerX = rect.left + rect.width / 2;
      const centerY = rect.top + rect.height / 2;
      const dist = Math.sqrt(Math.pow(e.clientX - centerX, 2) + Math.pow(e.clientY - centerY, 2));
      
      if (dist < minDistance) {
        minDistance = dist;
        closestIndex = idx;
        isAfter = e.clientX > centerX;
      }
    });

    if (closestIndex !== -1) {
      setDragOverGap(isAfter ? closestIndex + 1 : closestIndex);
    }
  };

  const handleGridDragLeave = (e: React.DragEvent) => {
    // Only clear if we are actually leaving the grid container (not just moving to a child)
    const rect = e.currentTarget.getBoundingClientRect();
    const x = e.clientX;
    const y = e.clientY;
    
    if (x <= rect.left || x >= rect.right || y <= rect.top || y >= rect.bottom) {
      setDragOverGap(null);
    }
  };

  const handleReorderDrop = async (draggedId: string, index: number, position: 'before' | 'after') => {
    const targetGap = position === 'before' ? index : index + 1;
    
    let previousWantId: string | undefined;
    let nextWantId: string | undefined;

    if (targetGap === 0) {
      // First position
      nextWantId = filteredWants[0].metadata?.id || filteredWants[0].id;
    } else if (targetGap >= filteredWants.length) {
      // Last position
      previousWantId = filteredWants[filteredWants.length - 1].metadata?.id || filteredWants[filteredWants.length - 1].id;
    } else {
      // Between items
      const prevWant = filteredWants[targetGap - 1];
      const nextWant = filteredWants[targetGap];
      previousWantId = prevWant.metadata?.id || prevWant.id;
      nextWantId = nextWant.metadata?.id || nextWant.id;
    }

    try {
      await reorderWant(draggedId, previousWantId, nextWantId);
    } catch (error) {
      console.error('Failed to reorder want:', error);
    }
  };

  const hierarchicalWants = useMemo(() => {
    const wantsByName = new Map<string, Want>();
    wants.forEach(want => {
      if (want.metadata?.name) wantsByName.set(want.metadata.name, want);
    });

    const topLevelWants: WantWithChildren[] = [];
    const childWantsByParentId = new Map<string, Want[]>();

    wants.forEach(want => {
      if (want.metadata?.name?.startsWith('__')) return;
      const hasOwnerReferences = want.metadata?.ownerReferences && want.metadata.ownerReferences.length > 0;
      if (!hasOwnerReferences) {
        topLevelWants.push({ ...want, children: [] });
      } else {
        const parentId = want.metadata?.ownerReferences?.[0]?.id;
        if (parentId) {
          if (!childWantsByParentId.has(parentId)) childWantsByParentId.set(parentId, []);
          childWantsByParentId.get(parentId)!.push(want);
        }
      }
    });

    topLevelWants.forEach(parentWant => {
      const parentId = parentWant.metadata?.id;
      if (parentId) parentWant.children = childWantsByParentId.get(parentId) || [];
    });

    return topLevelWants;
  }, [wants]);

  const filteredWants = useMemo(() => {
    return hierarchicalWants.filter(want => {
      const checkWantMatches = (wantToCheck: Want): boolean => {
        if (searchQuery) {
          const query = searchQuery.toLowerCase();
          const wantName = wantToCheck.metadata?.name || wantToCheck.metadata?.id || '';
          const wantType = wantToCheck.metadata?.type || '';
          const labels = wantToCheck.metadata?.labels || {};
          const matchesSearch = wantName.toLowerCase().includes(query) || wantType.toLowerCase().includes(query) || (wantToCheck.metadata?.id || '').toLowerCase().includes(query) || Object.values(labels).some(value => value.toLowerCase().includes(query));
          if (!matchesSearch) return false;
        }
        if (statusFilters.length > 0 && !statusFilters.includes(wantToCheck.status)) return false;
        return true;
      };
      return checkWantMatches(want) || want.children?.some(child => checkWantMatches(child)) || false;
    }).sort((a, b) => {
      // Sort by orderKey if available, otherwise fall back to ID
      const keyA = a.metadata?.orderKey || a.metadata?.id || '';
      const keyB = b.metadata?.orderKey || b.metadata?.id || '';
      return keyA.localeCompare(keyB);
    });
  }, [hierarchicalWants, searchQuery, statusFilters]);

  React.useEffect(() => {
    onGetFilteredWants?.(filteredWants);
  }, [filteredWants, onGetFilteredWants]);

  const hasUserWants = hierarchicalWants.length > 0 || drafts.length > 0;

  if (loading && !hasUserWants) {
    return (
      <div className="flex items-center justify-center py-16">
        <LoadingSpinner size="lg" />
        <span className="ml-3 text-gray-600 dark:text-gray-400">Loading wants...</span>
      </div>
    );
  }

  if (!hasUserWants) {
    return (
      <div className="flex items-center justify-center py-16">
        <button onClick={() => onCreateWant?.()} className="flex flex-col items-center gap-4 p-8 rounded-lg border-2 border-dashed border-gray-300 dark:border-gray-700 hover:border-blue-500 dark:hover:border-blue-500 transition-colors group">
          <div className="w-24 h-24 bg-gray-100 dark:bg-gray-800 group-hover:bg-blue-50 dark:group-hover:bg-blue-900/20 rounded-full flex items-center justify-center transition-colors">
            <span className="relative inline-flex flex-shrink-0">
              <Heart className="w-12 h-12 text-gray-400 dark:text-gray-500 group-hover:text-blue-500 dark:group-hover:text-blue-400 transition-colors" />
              <Plus className="w-5 h-5 absolute -top-2 -right-2 text-gray-400 dark:text-gray-500 group-hover:text-blue-500 dark:group-hover:text-blue-400 transition-colors" style={{ strokeWidth: 3 }} />
            </span>
          </div>
          <div className="text-center">
            <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-1">No wants yet</h3>
            <p className="text-gray-600 dark:text-gray-400">Click the plus icon to create your first want configuration.</p>
          </div>
        </button>
      </div>
    );
  }

  if (filteredWants.length === 0 && drafts.length === 0) {
    return (
      <div className="text-center py-16">
        <div className="mx-auto w-24 h-24 bg-gray-100 dark:bg-gray-800 rounded-full flex items-center justify-center mb-4">
          <svg className="w-12 h-12 text-gray-400 dark:text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
        </div>
        <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-2">No matches found</h3>
        <p className="text-gray-600 dark:text-gray-400">No wants match your current search and filter criteria.</p>
      </div>
    );
  }

  return (
    <div 
      className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 sm:gap-4 lg:gap-6" 
      id="want-grid-container"
      onDragOver={handleGridDragOver}
      onDragLeave={handleGridDragLeave}
    >
      {filteredWants.map((want, index) => {
        const wantId = want.metadata?.id || want.id;
        const isExpanded = expandedParents?.has(wantId || '') ?? false;
        const isSelected = isSelectMode ? (wantId && selectedWantIds.has(wantId)) : selectedWant?.metadata?.id === want.metadata?.id;

        return (
          <div
            key={wantId || `want-${index}`}
            data-want-id={wantId}
            data-keyboard-nav-selected={selectedWant?.metadata?.id === want.metadata?.id}
            className={classNames('transition-all duration-300 ease-out h-full relative', isExpanded ? 'sm:col-span-2 lg:col-span-3' : '')}
          >
            {/* Drop Indicator Before */}
            {dragOverGap === index && (
              <div className={classNames(styles.dropIndicator, styles.dropIndicatorVertical, "-left-[14px]")}>
                <div className={styles.plusIconContainer}>
                  <Plus size={14} />
                </div>
              </div>
            )}

            <WantCard
              want={want} children={want.children} selected={!!isSelected} selectedWant={selectedWant}
              onView={(w) => isSelectMode && onSelectWant ? onSelectWant(w.metadata?.id || w.id || '') : onViewWant(w)}
              onViewAgents={onViewAgentsWant} onViewResults={onViewResultsWant} onEdit={onEditWant} onDelete={onDeleteWant}
              onSuspend={onSuspendWant} onResume={onResumeWant} expandedParents={expandedParents} onToggleExpand={onToggleExpand}
              onLabelDropped={onLabelDropped} onWantDropped={onWantDropped} onShowReactionConfirmation={onShowReactionConfirmation}
              onReorderDragOver={handleReorderDragOver} onReorderDrop={handleReorderDrop} index={index}
              isSelectMode={isSelectMode} selectedWantIds={selectedWantIds} isBeingProcessed={want.status === 'deleting' || want.status === 'initializing'}
              onCreateWant={onCreateWant}
            />

            {/* Drop Indicator After (only for the last item) */}
            {index === filteredWants.length - 1 && dragOverGap === index + 1 && (
              <div className={classNames(styles.dropIndicator, styles.dropIndicatorVertical, "-right-[14px]")}>
                <div className={styles.plusIconContainer}>
                  <Plus size={14} />
                </div>
              </div>
            )}
          </div>
        );
      })}

      {drafts.map((draft) => (
        <div
          key={draft.id}
          data-draft-id={draft.id}
          className="h-full"
        >
          <DraftWantCard draft={draft} selected={activeDraftId === draft.id} onClick={() => onDraftClick?.(draft)} onDelete={() => onDraftDelete?.(draft)} />
        </div>
      ))}

      <button
        onClick={() => onCreateWant?.()}
        onDragOver={(e) => {
          const { draggingWant } = useWantStore.getState();
          if (draggingWant || e.dataTransfer.types.includes('application/mywant-id')) {
            e.preventDefault();
            setDragOverGap(filteredWants.length);
          }
        }}
        onDragLeave={() => setDragOverGap(null)}
        onDrop={(e) => {
          e.preventDefault();
          setDragOverGap(null);
          const draggedId = e.dataTransfer.getData('application/mywant-id');
          if (draggedId) {
            handleReorderDrop(draggedId, filteredWants.length - 1, 'after');
          }
        }}
        className="flex flex-col items-center justify-center p-3 sm:p-8 rounded-lg border-2 border-dashed border-gray-300 dark:border-gray-700 hover:border-blue-500 dark:hover:border-blue-500 transition-colors group h-full min-h-[8rem] sm:min-h-[12.5rem]"
      >
        <div className="w-12 h-12 sm:w-16 sm:h-16 bg-gray-100 dark:bg-gray-800 group-hover:bg-blue-50 dark:group-hover:bg-blue-900/20 rounded-full flex items-center justify-center transition-colors mb-2 sm:mb-3">
          <span className="relative inline-flex flex-shrink-0">
            <Heart className="w-6 h-6 sm:w-8 sm:h-8 text-gray-400 dark:text-gray-500 group-hover:text-blue-500 dark:group-hover:text-blue-400 transition-colors" />
            <Plus className="w-3 h-3 sm:w-4 sm:h-4 absolute -top-1.5 -right-1.5 sm:-top-2 sm:-right-2 text-gray-400 dark:text-gray-500 group-hover:text-blue-500 dark:group-hover:text-blue-400 transition-colors" style={{ strokeWidth: 3 }} />
          </span>
        </div>
        <p className="text-xs sm:text-sm text-gray-600 dark:text-gray-400 group-hover:text-blue-600 dark:group-hover:text-blue-400 transition-colors font-medium">Add Want</p>
      </button>
    </div>
  );
};