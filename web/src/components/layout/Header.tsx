import React from 'react';
import { Plus, RefreshCw } from 'lucide-react';
import { useWantStore } from '@/stores/wantStore';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';

interface HeaderProps {
  onCreateWant: () => void;
}

export const Header: React.FC<HeaderProps> = ({ onCreateWant }) => {
  const { loading, fetchWants, wants } = useWantStore();

  const handleRefresh = () => {
    fetchWants();
  };

  return (
    <header className="bg-white border-b border-gray-200 px-6 py-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center space-x-4">
          <h1 className="text-2xl font-bold text-gray-900">MyWant Dashboard</h1>
          <div className="text-sm text-gray-500">
            {wants.length} want{wants.length !== 1 ? 's' : ''}
          </div>
        </div>

        <div className="flex items-center space-x-3">
          <button
            onClick={handleRefresh}
            disabled={loading}
            className="inline-flex items-center px-3 py-2 border border-gray-300 shadow-sm text-sm leading-4 font-medium rounded-md text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 disabled:opacity-50"
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
            className="btn-primary"
          >
            <Plus className="h-4 w-4 mr-2" />
            Create Want
          </button>
        </div>
      </div>
    </header>
  );
};