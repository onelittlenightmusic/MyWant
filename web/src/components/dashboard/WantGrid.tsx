import React, { useMemo } from 'react';
import { Want, WantExecutionStatus } from '@/types/want';
import { DraftWant } from '@/types/draft';
import { WantCard } from './WantCard';
import { DraftWantCard } from './DraftWantCard';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { classNames } from '@/utils/helpers';

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
  onCreateWant?: () => void;
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
  console.log('[DEBUG WantGrid] Received drafts:', drafts);

  const hierarchicalWants = useMemo(() => {
    // First, build a map of all wants by name for efficient lookup
    const wantsByName = new Map<string, Want>();
    wants.forEach(want => {
      if (want.metadata?.name) {
        wantsByName.set(want.metadata.name, want);
      }
    });

    // Separate top-level wants (no owner references) and child wants
    const topLevelWants: WantWithChildren[] = [];
    const childWantsByParentId = new Map<string, Want[]>();

    wants.forEach(want => {
      // Skip internal wants (prefixed with __)
      if (want.metadata?.name?.startsWith('__')) {
        return;
      }

      const hasOwnerReferences = want.metadata?.ownerReferences && want.metadata.ownerReferences.length > 0;

      if (!hasOwnerReferences) {
        // This is a top-level want
        topLevelWants.push({ ...want, children: [] });
      } else {
        // This is a child want - group by parent ID
        const parentId = want.metadata?.ownerReferences?.[0]?.id;
        if (parentId) {
          if (!childWantsByParentId.has(parentId)) {
            childWantsByParentId.set(parentId, []);
          }
          childWantsByParentId.get(parentId)!.push(want);
        }
      }
    });

    // Attach children to their parents
    topLevelWants.forEach(parentWant => {
      const parentId = parentWant.metadata?.id;
      if (parentId) {
        const children = childWantsByParentId.get(parentId) || [];
        parentWant.children = children;
      }
    });

    return topLevelWants;
  }, [wants]);

  const filteredWants = useMemo(() => {
    return hierarchicalWants.filter(want => {
      // Apply search and status filters to both parent and children
      const checkWantMatches = (wantToCheck: Want): boolean => {
        // Search filter
        if (searchQuery) {
          const query = searchQuery.toLowerCase();
          const wantName = wantToCheck.metadata?.name || wantToCheck.metadata?.id || '';
          const wantType = wantToCheck.metadata?.type || '';
          const labels = wantToCheck.metadata?.labels || {};

          const matchesSearch =
            wantName.toLowerCase().includes(query) ||
            wantType.toLowerCase().includes(query) ||
            (wantToCheck.metadata?.id || '').toLowerCase().includes(query) ||
            Object.values(labels).some(value =>
              value.toLowerCase().includes(query)
            );

          if (!matchesSearch) return false;
        }

        // Status filter
        if (statusFilters.length > 0) {
          if (!statusFilters.includes(wantToCheck.status)) return false;
        }

        return true;
      };

      // Check if parent matches
      const parentMatches = checkWantMatches(want);

      // Check if any child matches
      const hasMatchingChild = want.children?.some(child => checkWantMatches(child)) || false;

      // Include if parent matches or has matching children
      return parentMatches || hasMatchingChild;
    }).sort((a, b) => {
      // Sort by ID to ensure consistent ordering
      const idA = a.metadata?.id || '';
      const idB = b.metadata?.id || '';
      return idA.localeCompare(idB);
    });
  }, [hierarchicalWants, searchQuery, statusFilters]);

  // Notify parent component of filtered wants for keyboard navigation
  React.useEffect(() => {
    onGetFilteredWants?.(filteredWants);
  }, [filteredWants, onGetFilteredWants]);

  // Calculate non-internal wants count (exclude internal wants prefixed with __)
  // Include drafts in the check so we show the grid if either exists
  const hasUserWants = hierarchicalWants.length > 0 || drafts.length > 0;

  if (loading && !hasUserWants) {
    return (
      <div className="flex items-center justify-center py-16">
        <LoadingSpinner size="lg" />
        <span className="ml-3 text-gray-600">Loading wants...</span>
      </div>
    );
  }

  if (!hasUserWants) {
    return (
      <div className="flex items-center justify-center py-16">
        <button
          onClick={onCreateWant}
          className="flex flex-col items-center gap-4 p-8 rounded-lg border-2 border-dashed border-gray-300 hover:border-blue-500 transition-colors group"
        >
          <div className="w-24 h-24 bg-gray-100 group-hover:bg-blue-50 rounded-full flex items-center justify-center transition-colors">
            <svg
              className="w-12 h-12 text-gray-400 group-hover:text-blue-500 transition-colors"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M12 4v16m8-8H4"
              />
            </svg>
          </div>
          <div className="text-center">
            <h3 className="text-lg font-medium text-gray-900 mb-1">No wants yet</h3>
            <p className="text-gray-600">
              Click the plus icon to create your first want configuration.
            </p>
          </div>
        </button>
      </div>
    );
  }

  if (filteredWants.length === 0 && drafts.length === 0) {
    return (
      <div className="text-center py-16">
        <div className="mx-auto w-24 h-24 bg-gray-100 rounded-full flex items-center justify-center mb-4">
          <svg
            className="w-12 h-12 text-gray-400"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={1.5}
              d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
            />
          </svg>
        </div>
        <h3 className="text-lg font-medium text-gray-900 mb-2">No matches found</h3>
        <p className="text-gray-600">
          No wants match your current search and filter criteria.
        </p>
      </div>
    );
  }

  return (
    <div
      className="grid grid-cols-3 gap-6"
      id="want-grid-container"
      onDragOver={(e) => {
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
      }}
      onDrop={(e) => {
        // Check if drop happens on the grid gaps
        if (e.target === e.currentTarget) {
          e.preventDefault();
          const draggedWantId = e.dataTransfer.getData('application/mywant-id');
          if (draggedWantId && onWantDropped) {
            onWantDropped(draggedWantId, '');
          }
        }
      }}
    >
      {filteredWants.map((want, index) => {
        const wantId = want.metadata?.id || want.id;
        const isExpanded = expandedParents?.has(wantId || '') ?? false;
        const childCount = want.children?.length || 0;
        
        const isSelected = isSelectMode 
          ? (wantId && selectedWantIds.has(wantId))
          : selectedWant?.metadata?.id === want.metadata?.id;

        return (
          <div
            key={wantId || `want-${index}`}
            data-keyboard-nav-selected={selectedWant?.metadata?.id === want.metadata?.id}
            className={classNames(
              'transition-all duration-300 ease-out h-full',
              isExpanded ? 'col-span-3' : ''
            )}
          >
            <WantCard
              want={want}
              children={want.children}
              selected={!!isSelected}
              selectedWant={selectedWant}
              onView={(w) => {
                if (isSelectMode && onSelectWant) {
                  const id = w.metadata?.id || w.id;
                  if (id) onSelectWant(id);
                } else {
                  onViewWant(w);
                }
              }}
              onViewAgents={onViewAgentsWant}
              onViewResults={onViewResultsWant}
              onEdit={onEditWant}
              onDelete={onDeleteWant}
              onSuspend={onSuspendWant}
              onResume={onResumeWant}
              expandedParents={expandedParents}
              onToggleExpand={onToggleExpand}
              onLabelDropped={onLabelDropped}
              onWantDropped={onWantDropped}
              onShowReactionConfirmation={onShowReactionConfirmation}
              isSelectMode={isSelectMode}
              selectedWantIds={selectedWantIds}
            />
          </div>
        );
      })}

      {/* Draft Want Cards - appear after regular wants */}
      {drafts.map((draft) => (
        <div key={draft.id} className="h-full">
          <DraftWantCard
            draft={draft}
            selected={activeDraftId === draft.id}
            onClick={() => onDraftClick?.(draft)}
            onDelete={() => onDraftDelete?.(draft)}
          />
        </div>
      ))}

      {/* Add Want Card - appears after drafts */}
      <button
        onClick={onCreateWant}
        className="flex flex-col items-center justify-center p-8 rounded-lg border-2 border-dashed border-gray-300 hover:border-blue-500 transition-colors group h-full min-h-[200px]"
      >
        <div className="w-16 h-16 bg-gray-100 group-hover:bg-blue-50 rounded-full flex items-center justify-center transition-colors mb-3">
          <svg
            className="w-8 h-8 text-gray-400 group-hover:text-blue-500 transition-colors"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M12 4v16m8-8H4"
            />
          </svg>
        </div>
        <p className="text-sm text-gray-600 group-hover:text-blue-600 transition-colors font-medium">
          Add Want
        </p>
      </button>
    </div>
  );
};