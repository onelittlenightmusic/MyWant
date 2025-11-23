import React from 'react';
import { Search } from 'lucide-react';
import { ExpandableSearchBar } from '@/components/common/ExpandableSearchBar';

interface WantTypeFiltersProps {
  searchQuery: string;
  onSearchChange: (query: string) => void;
}

export const WantTypeFilters: React.FC<WantTypeFiltersProps> = ({
  searchQuery,
  onSearchChange
}) => {
  return (
    <div>
      <ExpandableSearchBar
        placeholder="Search want types by name..."
        value={searchQuery}
        onChange={onSearchChange}
      />
    </div>
  );
};
