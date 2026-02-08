import React from 'react';
import { Search, Filter, X } from 'lucide-react';
import { WantExecutionStatus } from '@/types/want';
import { getStatusColor, classNames } from '@/utils/helpers';

interface WantFiltersProps {
  searchQuery: string;
  onSearchChange: (query: string) => void;
  selectedStatuses: WantExecutionStatus[];
  onStatusFilter: (statuses: WantExecutionStatus[]) => void;
}

const STATUS_OPTIONS: WantExecutionStatus[] = [
  'created',
  'reaching',
  'waiting_user_action',
  'achieved',
  'failed',
  'module_error',
  'config_error',
  'stopped'
];

export const WantFilters: React.FC<WantFiltersProps> = ({
  searchQuery,
  onSearchChange,
  selectedStatuses,
  onStatusFilter
}) => {
  const handleStatusToggle = (status: WantExecutionStatus) => {
    if (selectedStatuses.includes(status)) {
      onStatusFilter(selectedStatuses.filter(s => s !== status));
    } else {
      onStatusFilter([...selectedStatuses, status]);
    }
  };

  const clearAllFilters = () => {
    onStatusFilter([]);
    onSearchChange('');
  };

  const hasActiveFilters = selectedStatuses.length > 0 || searchQuery.trim();

  return (
    <div className="bg-white rounded-lg shadow border border-gray-200 p-4 mb-6">
      <div className="flex items-center gap-4 flex-wrap">
        {/* Search */}
        <div className="flex-1 min-w-[280px]">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-gray-400" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => onSearchChange(e.target.value)}
              placeholder="Search wants by name, type, or labels..."
              className="w-full pl-10 pr-4 py-2 border border-gray-300 rounded-md focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
            />
          </div>
        </div>

        {/* Status Filters */}
        <div className="flex items-center gap-2">
          <Filter className="h-4 w-4 text-gray-500" />
          <span className="text-sm font-medium text-gray-700">Status:</span>
          <div className="flex gap-2 flex-wrap">
            {STATUS_OPTIONS.map((status) => {
              const color = getStatusColor(status);
              const isSelected = selectedStatuses.includes(status);

              return (
                <button
                  key={status}
                  onClick={() => handleStatusToggle(status)}
                  className={classNames(
                    'inline-flex items-center px-3 py-1 rounded-full text-xs font-medium border transition-colors',
                    isSelected ? 'bg-primary-100 border-primary-300 text-primary-800' : 'bg-gray-100 border-gray-300 text-gray-700 hover:bg-gray-200'
                  )}
                >
                  <div className={classNames(
                    'w-2 h-2 rounded-full mr-2',
                    color === 'gray' ? 'bg-gray-400' :
                    color === 'blue' ? 'bg-blue-400' :
                    color === 'green' ? 'bg-green-400' :
                    color === 'red' ? 'bg-red-400' :
                    color === 'yellow' ? 'bg-yellow-400' : ''
                  )} />
                  <span className="capitalize">{status}</span>
                </button>
              );
            })}
          </div>
        </div>

        {/* Clear Filters */}
        {hasActiveFilters && (
          <button
            onClick={clearAllFilters}
            className="inline-flex items-center px-3 py-1 text-sm text-gray-600 hover:text-gray-800 border border-gray-300 rounded-md hover:bg-gray-50"
          >
            <X className="h-3 w-3 mr-1" />
            Clear filters
          </button>
        )}
      </div>

      {/* Active Filters Summary */}
      {hasActiveFilters && (
        <div className="mt-3 pt-3 border-t border-gray-200">
          <div className="flex items-center gap-2 text-sm text-gray-600">
            <span>Active filters:</span>
            {searchQuery && (
              <span className="inline-flex items-center px-2 py-1 rounded bg-blue-100 text-blue-800">
                Search: "{searchQuery}"
              </span>
            )}
            {selectedStatuses.length > 0 && (
              <span className="inline-flex items-center px-2 py-1 rounded bg-green-100 text-green-800">
                Status: {selectedStatuses.length} selected
              </span>
            )}
          </div>
        </div>
      )}
    </div>
  );
};