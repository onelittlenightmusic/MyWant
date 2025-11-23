import React from 'react';
import { Plus, RefreshCw } from 'lucide-react';
import { useWantStore } from '@/stores/wantStore';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { ExpandableSearchBar } from '@/components/common/ExpandableSearchBar';

interface HeaderProps {
  onCreateWant: () => void;
  searchQuery?: string;
  onSearchChange?: (query: string) => void;
}

export const Header: React.FC<HeaderProps> = ({
  onCreateWant,
  searchQuery = '',
  onSearchChange
}) => {
  const { loading, fetchWants, wants } = useWantStore();

  const handleRefresh = () => {
    fetchWants();
  };

  return (
    <header className="bg-white border-b border-gray-200 px-6 py-4">
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center space-x-4 min-w-0">
          <h1 className="text-2xl font-bold text-gray-900 whitespace-nowrap">MyWant Dashboard</h1>
          <div className="text-sm text-gray-500 whitespace-nowrap">
            {wants.length} want{wants.length !== 1 ? 's' : ''}
          </div>
        </div>

        {/* Search Bar - centered and expands on interaction */}
        <div className="flex-1 flex justify-center min-w-0">
          {onSearchChange && (
            <ExpandableSearchBar
              placeholder="Search wants by name, type, or labels..."
              value={searchQuery}
              onChange={onSearchChange}
            />
          )}
        </div>

        <div className="flex items-center space-x-3 flex-shrink-0">
          <button
            onClick={handleRefresh}
            disabled={loading}
            className="inline-flex items-center px-3 py-2 border border-gray-300 shadow-sm text-sm leading-4 font-medium rounded-md text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 disabled:opacity-50 whitespace-nowrap"
          >
            {loading ? (
              <LoadingSpinner size="sm" className="mr-2" />
            ) : (
              <RefreshCw className="h-4 w-4 mr-2" />
            )}
            Refresh
          </button>

          <button
            onClick={onCreateWant}
            className="inline-flex items-center px-4 py-2 bg-primary-600 hover:bg-primary-700 focus:ring-primary-500 focus:ring-offset-2 text-white font-medium rounded-md transition duration-150 ease-in-out focus:outline-none focus:ring-2 focus:ring-offset-2 whitespace-nowrap"
          >
            <Plus className="h-4 w-4 mr-2 flex-shrink-0" />
            Create Want
          </button>
        </div>
      </div>
    </header>
  );
};