import React from 'react';
import { Search, Filter, X, Bot, Monitor, Zap } from 'lucide-react';
import { classNames } from '@/utils/helpers';

type AgentType = 'do' | 'monitor';

interface AgentFiltersProps {
  searchQuery: string;
  onSearchChange: (query: string) => void;
  selectedTypes: AgentType[];
  onTypeFilter: (types: AgentType[]) => void;
}

const TYPE_OPTIONS: { value: AgentType; label: string; icon: React.ElementType; color: string }[] = [
  { value: 'do', label: 'Do Agent', icon: Zap, color: 'blue' },
  { value: 'monitor', label: 'Monitor Agent', icon: Monitor, color: 'green' }
];

export const AgentFilters: React.FC<AgentFiltersProps> = ({
  searchQuery,
  onSearchChange,
  selectedTypes,
  onTypeFilter
}) => {
  const handleTypeToggle = (type: AgentType) => {
    if (selectedTypes.includes(type)) {
      onTypeFilter(selectedTypes.filter(t => t !== type));
    } else {
      onTypeFilter([...selectedTypes, type]);
    }
  };

  const clearAllFilters = () => {
    onTypeFilter([]);
    onSearchChange('');
  };

  const hasActiveFilters = selectedTypes.length > 0 || searchQuery.trim();

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
              placeholder="Search agents by name, type, capabilities, or dependencies..."
              className="w-full pl-10 pr-4 py-2 border border-gray-300 rounded-md focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
            />
          </div>
        </div>

        {/* Type Filters */}
        <div className="flex items-center gap-2">
          <Filter className="h-4 w-4 text-gray-500" />
          <span className="text-sm font-medium text-gray-700">Type:</span>
          <div className="flex gap-2 flex-wrap">
            {TYPE_OPTIONS.map((typeOption) => {
              const isSelected = selectedTypes.includes(typeOption.value);
              const Icon = typeOption.icon;

              return (
                <button
                  key={typeOption.value}
                  onClick={() => handleTypeToggle(typeOption.value)}
                  className={classNames(
                    'inline-flex items-center px-3 py-1 rounded-full text-xs font-medium border transition-colors',
                    isSelected ? 'bg-primary-100 border-primary-300 text-primary-800' : 'bg-gray-100 border-gray-300 text-gray-700 hover:bg-gray-200'
                  )}
                >
                  <Icon className="w-3 h-3 mr-2" />
                  <span>{typeOption.label}</span>
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
            {selectedTypes.length > 0 && (
              <span className="inline-flex items-center px-2 py-1 rounded bg-green-100 text-green-800">
                Type: {selectedTypes.length} selected
              </span>
            )}
          </div>
        </div>
      )}
    </div>
  );
};