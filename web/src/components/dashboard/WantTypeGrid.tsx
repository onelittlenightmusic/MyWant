import React, { useEffect } from 'react';
import { WantTypeCard } from './WantTypeCard';
import { WantTypeListItem } from '@/types/wantType';

interface WantTypeGridProps {
  wantTypes: WantTypeListItem[];
  selectedWantType: WantTypeListItem | null;
  onViewDetails: (wantType: WantTypeListItem) => void;
  loading?: boolean;
  onGetFilteredWantTypes?: (wantTypes: WantTypeListItem[]) => void;
  onSelectWantType?: (wantType: WantTypeListItem) => void; // For keyboard navigation
}

export const WantTypeGrid: React.FC<WantTypeGridProps> = ({
  wantTypes,
  selectedWantType,
  onSelectWantType,
  onViewDetails,
  loading = false,
  onGetFilteredWantTypes,
}) => {
  // Notify parent of filtered want types for keyboard navigation
  useEffect(() => {
    onGetFilteredWantTypes?.(wantTypes);
  }, [wantTypes, onGetFilteredWantTypes]);
  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-center">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600 mx-auto mb-4"></div>
          <p className="text-gray-600 dark:text-gray-400">Loading want types...</p>
        </div>
      </div>
    );
  }

  if (wantTypes.length === 0) {
    return (
      <div className="flex items-center justify-center h-64 bg-white dark:bg-gray-800 rounded-lg border-2 border-dashed border-gray-300 dark:border-gray-700">
        <div className="text-center">
          <p className="text-lg font-semibold text-gray-900 dark:text-white">No want types found</p>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">Try adjusting your filters</p>
        </div>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-3 gap-4">
      {wantTypes.map((wantType) => (
        <div
          key={wantType.name}
          data-keyboard-nav-selected={selectedWantType?.name === wantType.name}
        >
          <WantTypeCard
            wantType={wantType}
            selected={selectedWantType?.name === wantType.name}
            onView={onViewDetails}
          />
        </div>
      ))}
    </div>
  );
};
