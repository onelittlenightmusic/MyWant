import React from 'react';
import { WantTypeCard } from './WantTypeCard';
import { WantTypeListItem } from '@/types/wantType';

interface WantTypeGridProps {
  wantTypes: WantTypeListItem[];
  selectedWantType: WantTypeListItem | null;
  onSelectWantType: (wantType: WantTypeListItem) => void;
  onViewDetails: (wantType: WantTypeListItem) => void;
  loading?: boolean;
}

export const WantTypeGrid: React.FC<WantTypeGridProps> = ({
  wantTypes,
  selectedWantType,
  onSelectWantType,
  onViewDetails,
  loading = false,
}) => {
  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-center">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600 mx-auto mb-4"></div>
          <p className="text-gray-600">Loading want types...</p>
        </div>
      </div>
    );
  }

  if (wantTypes.length === 0) {
    return (
      <div className="flex items-center justify-center h-64 bg-white rounded-lg border-2 border-dashed border-gray-300">
        <div className="text-center">
          <p className="text-lg font-semibold text-gray-900">No want types found</p>
          <p className="text-sm text-gray-500 mt-1">Try adjusting your filters</p>
        </div>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
      {wantTypes.map((wantType) => (
        <WantTypeCard
          key={wantType.name}
          wantType={wantType}
          selected={selectedWantType?.name === wantType.name}
          onSelect={onSelectWantType}
          onView={onViewDetails}
        />
      ))}
    </div>
  );
};
