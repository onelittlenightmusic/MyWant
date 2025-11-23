import React from 'react';
import { Search } from 'lucide-react';
import { ExpandableSearchBar } from '@/components/common/ExpandableSearchBar';

interface RecipeFiltersProps {
  searchQuery: string;
  onSearchChange: (query: string) => void;
}

export const RecipeFilters: React.FC<RecipeFiltersProps> = ({
  searchQuery,
  onSearchChange
}) => {
  return (
    <div>
      <ExpandableSearchBar
        placeholder="Search recipes by name..."
        value={searchQuery}
        onChange={onSearchChange}
      />
    </div>
  );
};
