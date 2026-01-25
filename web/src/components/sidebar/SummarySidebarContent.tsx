import React, { useState } from 'react';
import { Download, Upload, ChevronDown, Plus, X } from 'lucide-react';
import { Want } from '@/types/want';
import { StatsOverview } from '@/components/dashboard/StatsOverview';
import { WantFilters } from '@/components/dashboard/WantFilters';
import { classNames } from '@/utils/helpers';
import { addLabelToRegistry } from '@/utils/labelUtils';

interface SummarySidebarContentProps {
  wants: Want[];
  loading: boolean;
  searchQuery: string;
  onSearchChange: (query: string) => void;
  statusFilters: any[];
  onStatusFilter: (filters: any[]) => void;
  allLabels: Map<string, Set<string>>;
  onLabelClick: (key: string, value: string) => void;
  selectedLabel: { key: string; value: string } | null;
  onClearSelectedLabel: () => void;
  labelOwners: Want[];
  labelUsers: Want[];
  onViewWant: (want: Want) => void;
  onExportWants: () => void;
  onImportWants: () => void;
  isExporting: boolean;
  isImporting: boolean;
  fetchLabels: () => Promise<void>;
  fetchWants: () => Promise<void>;
}

export const SummarySidebarContent: React.FC<SummarySidebarContentProps> = ({
  wants,
  loading,
  searchQuery,
  onSearchChange,
  statusFilters,
  onStatusFilter,
  allLabels,
  onLabelClick,
  selectedLabel,
  onClearSelectedLabel,
  labelOwners,
  labelUsers,
  onViewWant,
  onExportWants,
  onImportWants,
  isExporting,
  isImporting,
  fetchLabels,
  fetchWants
}) => {
  const [showAddLabelForm, setShowAddLabelForm] = useState(false);
  const [newLabel, setNewLabel] = useState({ key: '', value: '' });
  const [expandedLabels, setExpandedLabels] = useState(false);

  const handleAddLabel = async () => {
    if (newLabel.key.trim() && newLabel.value.trim()) {
      const success = await addLabelToRegistry(newLabel.key, newLabel.value);
      if (success) {
        await fetchLabels();
        await fetchWants();
        setNewLabel({ key: '', value: '' });
        setShowAddLabelForm(false);
      }
    }
  };

  return (
    <div className="space-y-6">
      {/* Labels Section */}
      <div>
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-lg font-semibold text-gray-900">Labels</h3>
          <button
            onClick={() => setShowAddLabelForm(!showAddLabelForm)}
            className="p-1.5 rounded-md text-gray-400 hover:text-gray-600 hover:bg-gray-100 transition-colors"
            title="Add label"
          >
            <Plus className="w-5 h-5" />
          </button>
        </div>

        {showAddLabelForm && (
          <div className="mb-4 p-3 bg-gray-50 border border-gray-200 rounded-lg">
            <div className="space-y-3">
              <div className="flex gap-2">
                <input
                  type="text"
                  placeholder="Key"
                  value={newLabel.key}
                  onChange={(e) => setNewLabel(prev => ({ ...prev, key: e.target.value }))}
                  className="flex-1 px-3 py-2 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-1 focus:ring-blue-500"
                />
                <input
                  type="text"
                  placeholder="Value"
                  value={newLabel.value}
                  onChange={(e) => setNewLabel(prev => ({ ...prev, value: e.target.value }))}
                  className="flex-1 px-3 py-2 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-1 focus:ring-blue-500"
                />
              </div>
              <div className="flex gap-2">
                <button
                  onClick={() => {
                    setNewLabel({ key: '', value: '' });
                    setShowAddLabelForm(false);
                  }}
                  className="flex-1 px-3 py-2 text-sm text-gray-600 border border-gray-300 rounded-md hover:bg-gray-100 transition-colors"
                >
                  Cancel
                </button>
                <button
                  onClick={handleAddLabel}
                  disabled={!newLabel.key.trim() || !newLabel.value.trim()}
                  className="flex-1 px-3 py-2 text-sm font-medium text-white bg-blue-600 rounded-md hover:bg-blue-700 disabled:bg-gray-400 disabled:cursor-not-allowed transition-colors"
                >
                  Add
                </button>
              </div>
            </div>
          </div>
        )}

        <div className={classNames(
          'overflow-hidden transition-all duration-300',
          expandedLabels ? 'max-h-[1000px]' : 'max-h-48'
        )}>
          <div className="flex flex-wrap gap-2">
            {Array.from(allLabels.entries()).map(([key, values]) => (
              Array.from(values).map(value => (
                <div
                  key={`${key}:${value}`}
                  onClick={() => onLabelClick(key, value)}
                  draggable={true}
                  onDragStart={(e) => {
                    e.dataTransfer.setData('application/json', JSON.stringify({ key, value }));
                    e.dataTransfer.effectAllowed = 'copy';
                    // Add a ghost image or styling if needed, but standard should work
                  }}
                  className={classNames(
                    'px-3 py-1.5 rounded-full text-sm font-medium border transition-all cursor-grab active:cursor-grabbing select-none',
                    selectedLabel?.key === key && selectedLabel?.value === value
                      ? 'bg-blue-500 text-white border-blue-600 shadow-md ring-2 ring-blue-300'
                      : 'bg-blue-100 text-blue-800 border-blue-300 hover:bg-blue-200 hover:shadow-sm'
                  )}
                >
                  {key}: {value}
                </div>
              ))
            ))}
          </div>
        </div>

        {allLabels.size > 0 && (
          <button
            onClick={() => setExpandedLabels(!expandedLabels)}
            className="mt-2 text-xs text-blue-600 hover:text-blue-800 font-medium flex items-center gap-1"
          >
            <ChevronDown className={classNames('w-3 h-3 transition-transform', expandedLabels && 'rotate-180')} />
            {expandedLabels ? 'Show less' : 'Show all labels'}
          </button>
        )}

        {allLabels.size === 0 && (
          <p className="text-sm text-gray-500 italic">No labels registered</p>
        )}
      </div>

      {/* Selected Label Details */}
      {selectedLabel && (
        <div className="bg-blue-50 border border-blue-100 rounded-lg p-4 animate-in fade-in slide-in-from-top-2">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-sm font-bold text-blue-900 uppercase tracking-wider flex items-center gap-2">
              <div className="w-2 h-2 bg-blue-500 rounded-full" />
              {selectedLabel.key}: {selectedLabel.value}
            </h3>
            <button
              onClick={onClearSelectedLabel}
              className="p-1 rounded-md text-blue-400 hover:text-blue-600 hover:bg-blue-100 transition-colors"
            >
              <X className="w-4 h-4" />
            </button>
          </div>

          <div className="space-y-4">
            {labelOwners.length > 0 && (
              <div>
                <h4 className="text-[10px] font-bold text-blue-700 mb-2 uppercase tracking-widest">Provides</h4>
                <div className="grid grid-cols-1 gap-1.5">
                  {labelOwners.map((w) => (
                    <button
                      key={w.metadata?.id || w.id}
                      onClick={() => onViewWant(w)}
                      className="px-3 py-2 bg-white border border-blue-200 rounded-md hover:border-blue-400 transition-all text-left group"
                    >
                      <span className="block text-xs font-semibold text-blue-900 group-hover:text-blue-600 truncate">
                        {w.metadata?.name || w.id}
                      </span>
                    </button>
                  ))}
                </div>
              </div>
            )}

            {labelUsers.length > 0 && (
              <div>
                <h4 className="text-[10px] font-bold text-green-700 mb-2 uppercase tracking-widest">Requires</h4>
                <div className="grid grid-cols-1 gap-1.5">
                  {labelUsers.map((w) => (
                    <button
                      key={w.metadata?.id || w.id}
                      onClick={() => onViewWant(w)}
                      className="px-3 py-2 bg-white border border-green-200 rounded-md hover:border-green-400 transition-all text-left group"
                    >
                      <span className="block text-xs font-semibold text-green-900 group-hover:text-green-600 truncate">
                        {w.metadata?.name || w.id}
                      </span>
                    </button>
                  ))}
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Statistics Section */}
      <div>
        <h3 className="text-lg font-semibold text-gray-900 mb-4">Statistics</h3>
        <StatsOverview wants={wants} loading={loading} layout="vertical" />
        
        <div className="mt-6 flex gap-3">
          <button
            onClick={onExportWants}
            disabled={isExporting || wants.length === 0}
            className="flex-1 flex items-center justify-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:bg-gray-400 transition-colors"
          >
            <Download className="h-4 w-4" />
            <span>{isExporting ? 'Exporting...' : 'Export'}</span>
          </button>
          <button
            onClick={onImportWants}
            disabled={isImporting}
            className="flex-1 flex items-center justify-center gap-2 px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 disabled:bg-gray-400 transition-colors"
          >
            <Upload className="h-4 w-4" />
            <span>{isImporting ? 'Import' : 'Import'}</span>
          </button>
        </div>
      </div>

      {/* Filters Section */}
      <div>
        <h3 className="text-lg font-semibold text-gray-900 mb-4">Filters</h3>
        <WantFilters
          searchQuery={searchQuery}
          onSearchChange={onSearchChange}
          selectedStatuses={statusFilters}
          onStatusFilter={onStatusFilter}
        />
      </div>
    </div>
  );
};
