import React from 'react';
import { Plus, BarChart3, CheckSquare } from 'lucide-react';
import { classNames } from '@/utils/helpers';

interface HeaderProps {
  onCreateWant: () => void;
  title?: string;
  createButtonLabel?: string;
  itemCount?: number;
  itemLabel?: string;
  showSummary?: boolean;
  onSummaryToggle?: () => void;
  sidebarMinimized?: boolean;
  hideCreateButton?: boolean;
  showSelectMode?: boolean;
  onToggleSelectMode?: () => void;
}

export const Header: React.FC<HeaderProps> = ({
  onCreateWant,
  title = 'MyWant Dashboard',
  createButtonLabel = 'Add Want',
  itemCount,
  itemLabel,
  showSummary = false,
  onSummaryToggle,
  sidebarMinimized = false,
  hideCreateButton = false,
  showSelectMode = false,
  onToggleSelectMode
}) => {
  return (
    <header className={classNames(
      "bg-white border-b border-gray-200 px-6 py-4 fixed top-0 right-0 z-40 transition-all duration-300 ease-in-out",
      sidebarMinimized ? "lg:left-20" : "lg:left-44"
    )}>
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center space-x-4 min-w-0">
          <h1 className="text-2xl font-bold text-gray-900 whitespace-nowrap">{title}</h1>
          {itemLabel && (
            <div className="text-sm text-gray-500 whitespace-nowrap">
              {itemCount} {itemLabel}{itemCount !== 1 ? 's' : ''}
            </div>
          )}
        </div>

        <div className="flex items-center space-x-3 flex-shrink-0">
          {onToggleSelectMode && (
            <button
              onClick={onToggleSelectMode}
              className={`inline-flex items-center px-4 py-2 font-medium rounded-full transition duration-150 ease-in-out focus:outline-none focus:ring-2 focus:ring-offset-2 whitespace-nowrap ${
                showSelectMode
                  ? 'bg-blue-100 text-blue-700 hover:bg-blue-200 focus:ring-blue-500'
                  : 'border border-gray-300 text-gray-700 bg-white hover:bg-gray-50 focus:ring-primary-500'
              }`}
              title={showSelectMode ? 'Exit Select Mode' : 'Enter Select Mode'}
            >
              <CheckSquare className="h-4 w-4 mr-2 flex-shrink-0" />
              Select
            </button>
          )}

          {onSummaryToggle && (
            <button
              onClick={onSummaryToggle}
              className={`inline-flex items-center px-4 py-2 font-medium rounded-full transition duration-150 ease-in-out focus:outline-none focus:ring-2 focus:ring-offset-2 whitespace-nowrap ${
                showSummary
                  ? 'bg-blue-100 text-blue-700 hover:bg-blue-200 focus:ring-blue-500'
                  : 'border border-gray-300 text-gray-700 bg-white hover:bg-gray-50 focus:ring-primary-500'
              }`}
              title={showSummary ? 'Hide summary' : 'Show summary'}
            >
              <BarChart3 className="h-4 w-4 mr-2 flex-shrink-0" />
              Summary
            </button>
          )}

          {!hideCreateButton && (
            <button
              onClick={onCreateWant}
              className="inline-flex items-center px-4 py-2 bg-primary-600 hover:bg-primary-700 focus:ring-primary-500 focus:ring-offset-2 text-white font-medium rounded-full transition duration-150 ease-in-out focus:outline-none focus:ring-2 focus:ring-offset-2 whitespace-nowrap"
            >
              <Plus className="h-4 w-4 mr-2 flex-shrink-0" />
              {createButtonLabel}
            </button>
          )}
        </div>
      </div>
    </header>
  );
};