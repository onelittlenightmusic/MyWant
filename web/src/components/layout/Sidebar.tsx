import React from 'react';
import { Filter, X } from 'lucide-react';
import { WantExecutionStatus } from '@/types/want';
import { getStatusColor, classNames } from '@/utils/helpers';

interface SidebarProps {
  isOpen: boolean;
  onClose: () => void;
  selectedStatuses: WantExecutionStatus[];
  onStatusFilter: (statuses: WantExecutionStatus[]) => void;
  searchQuery: string;
  onSearchChange: (query: string) => void;
}

const STATUS_OPTIONS: WantExecutionStatus[] = [
  'created',
  'running',
  'completed',
  'failed',
  'stopped'
];

export const Sidebar: React.FC<SidebarProps> = ({
  isOpen,
  onClose,
  selectedStatuses,
  onStatusFilter,
  searchQuery,
  onSearchChange
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

  return (
    <>
      {/* Overlay */}
      {isOpen && (
        <div
          className="fixed inset-0 bg-gray-600 bg-opacity-50 z-40 lg:hidden"
          onClick={onClose}
        />
      )}

      {/* Sidebar */}
      <div className={classNames(
        'fixed lg:relative inset-y-0 left-0 z-50 w-64 bg-white border-r border-gray-200 transform transition-transform duration-300 ease-in-out lg:translate-x-0 lg:flex lg:flex-col lg:h-screen',
        isOpen ? 'translate-x-0' : '-translate-x-full'
      )}>
        <div className="flex flex-col h-full">
          {/* Header */}
          <div className="flex items-center justify-between px-4 py-4 border-b border-gray-200 lg:hidden">
            <h2 className="text-lg font-semibold text-gray-900">Filters</h2>
            <button
              onClick={onClose}
              className="text-gray-400 hover:text-gray-600"
            >
              <X className="h-5 w-5" />
            </button>
          </div>

          <div className="hidden lg:block px-4 py-6 border-b border-gray-200">
            <h2 className="text-lg font-semibold text-gray-900 flex items-center">
              <Filter className="h-5 w-5 mr-2" />
              Filters
            </h2>
          </div>

          {/* Content */}
          <div className="flex-1 px-4 py-4 space-y-6">
            {/* Search */}
            <div>
              <label className="label">Search</label>
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => onSearchChange(e.target.value)}
                placeholder="Search wants..."
                className="input"
              />
            </div>

            {/* Status Filter */}
            <div>
              <label className="label">Status</label>
              <div className="space-y-2">
                {STATUS_OPTIONS.map((status) => {
                  const color = getStatusColor(status);
                  const isSelected = selectedStatuses.includes(status);

                  return (
                    <label
                      key={status}
                      className="flex items-center cursor-pointer"
                    >
                      <input
                        type="checkbox"
                        checked={isSelected}
                        onChange={() => handleStatusToggle(status)}
                        className="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                      />
                      <span className="ml-2 text-sm text-gray-700 capitalize">
                        {status}
                      </span>
                      <div className={classNames(
                        'ml-auto w-3 h-3 rounded-full',
                        {
                          'bg-gray-400': color === 'gray',
                          'bg-blue-400': color === 'blue',
                          'bg-green-400': color === 'green',
                          'bg-red-400': color === 'red',
                          'bg-yellow-400': color === 'yellow'
                        }
                      )} />
                    </label>
                  );
                })}
              </div>
            </div>

            {/* Clear Filters */}
            {(selectedStatuses.length > 0 || searchQuery) && (
              <div>
                <button
                  onClick={clearAllFilters}
                  className="text-sm text-primary-600 hover:text-primary-800"
                >
                  Clear all filters
                </button>
              </div>
            )}
          </div>
        </div>
      </div>
    </>
  );
};