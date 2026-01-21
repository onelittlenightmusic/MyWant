import React, { useEffect } from 'react';
import { XCircle } from 'lucide-react';
import { WantControlButtons } from './WantControlButtons';

interface WantBatchControlPanelProps {
  selectedCount: number;
  onBatchStart: () => void;
  onBatchStop: () => void;
  onBatchDelete: () => void;
  onBatchCancel: () => void;
  loading?: boolean;
}

export const WantBatchControlPanel: React.FC<WantBatchControlPanelProps> = ({
  selectedCount,
  onBatchStart,
  onBatchStop,
  onBatchDelete,
  onBatchCancel,
  loading = false
}) => {
  // Keyboard shortcuts
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Don't trigger if user is typing in an input/textarea
      const target = e.target as HTMLElement;
      const isInputElement =
        target.tagName === 'INPUT' ||
        target.tagName === 'TEXTAREA' ||
        target.isContentEditable;

      if (isInputElement) return;

      switch (e.key.toLowerCase()) {
        case 'd':
          // Delete
          if (selectedCount > 0 && !loading) {
            e.preventDefault();
            e.stopImmediatePropagation();
            onBatchDelete();
          }
          break;
        case 's':
          // Start
          if (selectedCount > 0 && !loading) {
            e.preventDefault();
            e.stopImmediatePropagation();
            onBatchStart();
          }
          break;
        case 'x':
          // Stop
          if (selectedCount > 0 && !loading) {
            e.preventDefault();
            e.stopImmediatePropagation();
            onBatchStop();
          }
          break;
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [selectedCount, loading, onBatchDelete, onBatchStart, onBatchStop]);

  return (
    <div className="h-full flex flex-col bg-white">
      {/* Header */}
      <div className="px-6 py-4 border-b border-gray-200 bg-gray-50 flex items-center justify-between sticky top-0 z-10">
        <div>
          <h2 className="text-lg font-semibold text-gray-900">Batch Actions</h2>
          <p className="text-sm text-gray-500">{selectedCount} item{selectedCount !== 1 ? 's' : ''} selected</p>
        </div>
        <button
          onClick={onBatchCancel}
          className="p-2 text-gray-400 hover:text-gray-600 rounded-full hover:bg-gray-100 transition-colors"
          title="Exit Select Mode"
        >
          <XCircle className="w-6 h-6" />
        </button>
      </div>

      <div className="flex-1 overflow-y-auto">
        <div className="border-b border-gray-200 px-4 py-2">
          <WantControlButtons
            onStart={onBatchStart}
            onStop={onBatchStop}
            onDelete={onBatchDelete}
            canStart={selectedCount > 0}
            canStop={selectedCount > 0}
            canDelete={selectedCount > 0}
            canSuspend={false} 
            loading={loading}
          />
        </div>
        
        <div className="p-6 text-center text-sm text-gray-500">
          <p>{selectedCount} item{selectedCount !== 1 ? 's' : ''} selected</p>
          <p className="mt-1">Apply actions to all selected wants.</p>
        </div>
      </div>
    </div>
  );
};
