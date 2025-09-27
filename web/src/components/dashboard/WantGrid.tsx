import React, { useMemo } from 'react';
import { Want, WantExecutionStatus } from '@/types/want';
import { WantCard } from './WantCard';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';

interface WantWithChildren extends Want {
  children?: Want[];
}

interface WantGridProps {
  wants: Want[];
  loading: boolean;
  searchQuery: string;
  statusFilters: WantExecutionStatus[];
  selectedWant: Want | null;
  onViewWant: (want: Want) => void;
  onEditWant: (want: Want) => void;
  onDeleteWant: (want: Want) => void;
  onSuspendWant?: (want: Want) => void;
  onResumeWant?: (want: Want) => void;
}

export const WantGrid: React.FC<WantGridProps> = ({
  wants,
  loading,
  searchQuery,
  statusFilters,
  selectedWant,
  onViewWant,
  onEditWant,
  onDeleteWant,
  onSuspendWant,
  onResumeWant
}) => {
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
    const childWantsByParent = new Map<string, Want[]>();

    wants.forEach(want => {
      const hasOwnerReferences = want.metadata?.ownerReferences && want.metadata.ownerReferences.length > 0;

      if (!hasOwnerReferences) {
        // This is a top-level want
        topLevelWants.push({ ...want, children: [] });
      } else {
        // This is a child want - group by parent
        const parentName = want.metadata?.ownerReferences?.[0]?.name;
        if (parentName) {
          if (!childWantsByParent.has(parentName)) {
            childWantsByParent.set(parentName, []);
          }
          childWantsByParent.get(parentName)!.push(want);
        }
      }
    });

    // Attach children to their parents
    topLevelWants.forEach(parentWant => {
      if (parentWant.metadata?.name) {
        const children = childWantsByParent.get(parentWant.metadata.name) || [];
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

  if (loading && wants.length === 0) {
    return (
      <div className="flex items-center justify-center py-16">
        <LoadingSpinner size="lg" />
        <span className="ml-3 text-gray-600">Loading wants...</span>
      </div>
    );
  }

  if (wants.length === 0) {
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
              d="M9 5H7a2 2 0 00-2 2v10a2 2 0 002 2h8a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"
            />
          </svg>
        </div>
        <h3 className="text-lg font-medium text-gray-900 mb-2">No wants yet</h3>
        <p className="text-gray-600 mb-4">
          Get started by creating your first want configuration.
        </p>
      </div>
    );
  }

  if (filteredWants.length === 0) {
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
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6">
      {filteredWants.map((want, index) => (
        <WantCard
          key={want.metadata?.id || `want-${index}`}
          want={want}
          children={want.children}
          selected={selectedWant?.metadata?.id === want.metadata?.id}
          onView={onViewWant}
          onEdit={onEditWant}
          onDelete={onDeleteWant}
          onSuspend={onSuspendWant}
          onResume={onResumeWant}
        />
      ))}
    </div>
  );
};