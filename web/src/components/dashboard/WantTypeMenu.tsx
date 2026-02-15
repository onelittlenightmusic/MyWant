import React, { useState } from 'react';
import { Search, X, Filter } from 'lucide-react';
import { classNames } from '@/utils/helpers';

interface WantTypeMenuProps {
  categories: string[];
  patterns: string[];
  selectedCategory?: string;
  selectedPattern?: string;
  searchTerm?: string;
  onSearchChange: (term: string) => void;
  onCategoryChange: (category?: string) => void;
  onPatternChange: (pattern?: string) => void;
  onClearFilters: () => void;
}

export const WantTypeMenu: React.FC<WantTypeMenuProps> = ({
  categories,
  patterns,
  selectedCategory,
  selectedPattern,
  searchTerm = '',
  onSearchChange,
  onCategoryChange,
  onPatternChange,
  onClearFilters,
}) => {
  const [showFilters, setShowFilters] = useState(false);

  const hasActiveFilters = selectedCategory || selectedPattern || searchTerm;

  return (
    <div className="space-y-4">
      {/* Search Bar */}
      <div className="relative">
        <Search className="absolute left-3 top-3 h-5 w-5 text-gray-400 dark:text-gray-500" />
        <input
          type="text"
          placeholder="Search want types by name or title..."
          value={searchTerm}
          onChange={(e) => onSearchChange(e.target.value)}
          className="w-full pl-10 pr-4 py-2 border border-gray-300 dark:border-gray-600 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 bg-white dark:bg-gray-800 text-gray-900 dark:text-white"
        />
      </div>

      {/* Filter Toggle Button */}
      <button
        onClick={() => setShowFilters(!showFilters)}
        className={classNames(
          'w-full flex items-center justify-between px-4 py-2 rounded-md border transition-colors',
          showFilters
            ? 'bg-blue-50 dark:bg-blue-900/20 border-blue-300 text-blue-700 dark:text-blue-400'
            : 'bg-white dark:bg-gray-800 border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-200 hover:bg-gray-50 dark:hover:bg-gray-800'
        )}
      >
        <div className="flex items-center gap-2">
          <Filter className="h-5 w-5" />
          <span className="font-medium">Filters</span>
          {hasActiveFilters && (
            <span className="ml-2 px-2 py-1 bg-blue-600 text-white text-xs font-semibold rounded-full">
              {[selectedCategory, selectedPattern, searchTerm ? 'search' : ''].filter(Boolean).length}
            </span>
          )}
        </div>
        <svg
          className={classNames(
            'h-5 w-5 transition-transform',
            showFilters ? 'rotate-180' : ''
          )}
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={2}
            d="M19 14l-7 7m0 0l-7-7m7 7V3"
          />
        </svg>
      </button>

      {/* Filter Panel */}
      {showFilters && (
        <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-md p-4 space-y-4">
          {/* Category Filter */}
          <div>
            <h3 className="text-sm font-semibold text-gray-900 dark:text-white mb-3">Category</h3>
            <div className="space-y-2">
              <label className="flex items-center">
                <input
                  type="radio"
                  checked={!selectedCategory}
                  onChange={() => onCategoryChange(undefined)}
                  className="w-4 h-4 text-blue-600 cursor-pointer"
                />
                <span className="ml-2 text-sm text-gray-700 dark:text-gray-200">All Categories</span>
              </label>
              {categories.map((category) => (
                <label key={category} className="flex items-center">
                  <input
                    type="radio"
                    checked={selectedCategory === category}
                    onChange={() => onCategoryChange(category)}
                    className="w-4 h-4 text-blue-600 cursor-pointer"
                  />
                  <span className="ml-2 text-sm text-gray-700 dark:text-gray-200 capitalize">{category}</span>
                </label>
              ))}
            </div>
          </div>

          {/* Pattern Filter */}
          <div>
            <h3 className="text-sm font-semibold text-gray-900 dark:text-white mb-3">Pattern</h3>
            <div className="space-y-2">
              <label className="flex items-center">
                <input
                  type="radio"
                  checked={!selectedPattern}
                  onChange={() => onPatternChange(undefined)}
                  className="w-4 h-4 text-blue-600 cursor-pointer"
                />
                <span className="ml-2 text-sm text-gray-700 dark:text-gray-200">All Patterns</span>
              </label>
              {patterns.map((pattern) => (
                <label key={pattern} className="flex items-center">
                  <input
                    type="radio"
                    checked={selectedPattern === pattern}
                    onChange={() => onPatternChange(pattern)}
                    className="w-4 h-4 text-blue-600 cursor-pointer"
                  />
                  <span className="ml-2 text-sm text-gray-700 dark:text-gray-200 capitalize">{pattern}</span>
                </label>
              ))}
            </div>
          </div>

          {/* Clear Filters Button */}
          {hasActiveFilters && (
            <button
              onClick={onClearFilters}
              className="w-full px-4 py-2 border border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-200 text-sm font-medium rounded hover:bg-gray-50 dark:hover:bg-gray-800 transition-colors flex items-center justify-center gap-2"
            >
              <X className="h-4 w-4" />
              Clear Filters
            </button>
          )}
        </div>
      )}
    </div>
  );
};
