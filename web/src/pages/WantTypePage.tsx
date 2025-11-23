import { useState, useEffect } from 'react';
import { useWantTypeStore } from '@/stores/wantTypeStore';
import { WantTypeListItem } from '@/types/wantType';
import { useKeyboardNavigation } from '@/hooks/useKeyboardNavigation';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { WantTypeGrid } from '@/components/dashboard/WantTypeGrid';
import { WantTypeDetailsSidebar } from '@/components/sidebar/WantTypeDetailsSidebar';
import { WantTypeControlPanel } from '@/components/dashboard/WantTypeControlPanel';
import { WantTypeStatsOverview } from '@/components/dashboard/WantTypeStatsOverview';
import { WantTypeFilters } from '@/components/dashboard/WantTypeFilters';
import { RightSidebar } from '@/components/layout/RightSidebar';
import { Layout } from '@/components/layout/Layout';
import { Header } from '@/components/layout/Header';
import { classNames } from '@/utils/helpers';

export default function WantTypePage() {
  const {
    wantTypes,
    selectedWantType,
    loading,
    error,
    filters,
    fetchWantTypes,
    getWantType,
    setSelectedWantType,
    setFilters,
    clearFilters,
    clearError,
    getCategories,
    getPatterns,
    getFilteredWantTypes,
  } = useWantTypeStore();

  // UI State
  const [sidebarMinimized, setSidebarMinimized] = useState(true); // Auto-collapse on mouse leave
  const [notification, setNotification] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [filteredWantTypes, setFilteredWantTypes] = useState<WantTypeListItem[]>([]);
  const [searchQuery, setSearchQuery] = useState('');

  // Auto-dismiss notifications after 5 seconds
  useEffect(() => {
    if (notification) {
      const timer = setTimeout(() => {
        setNotification(null);
      }, 5000);
      return () => clearTimeout(timer);
    }
  }, [notification]);

  // Initial load
  useEffect(() => {
    fetchWantTypes();
  }, [fetchWantTypes]);

  // Handle view details
  const handleViewDetails = async (wantType: WantTypeListItem) => {
    await getWantType(wantType.name);
  };

  // Handle search
  const handleSearch = (term: string) => {
    setSearchQuery(term);
    setFilters({ searchTerm: term });
  };

  const allFilteredWantTypes = getFilteredWantTypes();

  // Sync local state with all filtered want types
  useEffect(() => {
    setFilteredWantTypes(allFilteredWantTypes);
  }, [allFilteredWantTypes]);

  // Keyboard navigation
  const currentWantTypeIndex = selectedWantType
    ? filteredWantTypes.findIndex(wt => wt.name === selectedWantType.metadata.name)
    : -1;

  const handleKeyboardNavigate = (index: number) => {
    if (index >= 0 && index < filteredWantTypes.length) {
      const wantType = filteredWantTypes[index];
      handleViewDetails(wantType);
    }
  };

  useKeyboardNavigation({
    itemCount: filteredWantTypes.length,
    currentIndex: currentWantTypeIndex,
    onNavigate: handleKeyboardNavigate,
    enabled: filteredWantTypes.length > 0
  });

  // Handle ESC key to close details sidebar and deselect
  const handleEscapeKey = () => {
    if (selectedWantType) {
      setSelectedWantType(null);
    }
  };

  useEscapeKey({
    onEscape: handleEscapeKey,
    enabled: !!selectedWantType
  });

  return (
    <Layout
      sidebarMinimized={sidebarMinimized}
      onSidebarMinimizedChange={setSidebarMinimized}
    >
      {/* Header */}
      <Header
        onCreateWant={() => {}}
        title="Want Types"
        itemCount={wantTypes.length}
        itemLabel="type"
        searchPlaceholder="Search want types by name..."
        onRefresh={() => fetchWantTypes()}
        loading={loading}
      />

      {/* Main content area with sidebar-aware layout */}
      <main className="flex-1 flex overflow-hidden bg-gray-50">
        {/* Left content area - main dashboard */}
        <div className="flex-1 overflow-y-auto">
          <div className="p-6 pb-24">
            {/* Error Message */}
            {error && (
              <div className="mb-6 p-4 bg-red-50 border border-red-200 rounded-md">
                <div className="flex items-center">
                  <div className="flex-shrink-0">
                    <svg
                      className="h-5 w-5 text-red-400"
                      viewBox="0 0 20 20"
                      fill="currentColor"
                    >
                      <path
                        fillRule="evenodd"
                        d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z"
                        clipRule="evenodd"
                      />
                    </svg>
                  </div>
                  <div className="ml-3">
                    <p className="text-sm text-red-700">{error}</p>
                  </div>
                  <div className="ml-auto">
                    <button
                      onClick={clearError}
                      className="text-red-400 hover:text-red-600"
                    >
                      <svg className="h-4 w-4" viewBox="0 0 20 20" fill="currentColor">
                        <path
                          fillRule="evenodd"
                          d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
                          clipRule="evenodd"
                        />
                      </svg>
                    </button>
                  </div>
                </div>
              </div>
            )}

            {/* Want Types Grid */}
            <WantTypeGrid
              wantTypes={filteredWantTypes}
              selectedWantType={filteredWantTypes.find(wt => wt.name === selectedWantType?.metadata.name) || null}
              onViewDetails={handleViewDetails}
              loading={loading}
              onGetFilteredWantTypes={setFilteredWantTypes}
            />
          </div>
        </div>

        {/* Right sidebar area - reserved for statistics (hidden when sidebar is open) */}
        <div className={`w-[480px] bg-white border-l border-gray-200 overflow-y-auto transition-opacity duration-300 ease-in-out ${selectedWantType ? 'opacity-0 pointer-events-none' : 'opacity-100'}`}>
          <div className="p-6 space-y-6">
            <div>
              <h3 className="text-lg font-semibold text-gray-900 mb-4">Statistics</h3>
              <div>
                <WantTypeStatsOverview wantTypes={wantTypes} loading={loading} />
              </div>
            </div>

            {/* Search section */}
            <div>
              <h3 className="text-lg font-semibold text-gray-900 mb-4">Search</h3>
              <WantTypeFilters
                searchQuery={searchQuery}
                onSearchChange={handleSearch}
              />
            </div>
          </div>
        </div>
      </main>

      {/* Want Type Control Panel */}
      <WantTypeControlPanel
        selectedWantType={selectedWantType}
        onViewDetails={(wantType) => {
          const listItem = filteredWantTypes.find(wt => wt.name === wantType.metadata.name);
          if (listItem) handleViewDetails(listItem);
        }}
        onDeploySuccess={(message) => setNotification({ message, type: 'success' })}
        onDeployError={(error) => setNotification({ message: error, type: 'error' })}
        loading={loading}
        sidebarMinimized={sidebarMinimized}
      />

      {/* Right Sidebar for Want Type Details */}
      <RightSidebar
        isOpen={!!selectedWantType}
        onClose={() => setSelectedWantType(null)}
        title={selectedWantType ? selectedWantType.metadata.name : undefined}
      >
        <WantTypeDetailsSidebar wantType={selectedWantType} />
      </RightSidebar>

      {/* Notification Toast */}
      {notification && (
        <div className={classNames(
          'fixed top-4 right-4 px-4 py-3 rounded-md shadow-lg flex items-center space-x-2 z-50 animate-fade-in',
          notification.type === 'success'
            ? 'bg-green-50 text-green-800 border border-green-200'
            : 'bg-red-50 text-red-800 border border-red-200'
        )}>
          {notification.type === 'success' ? (
            <svg className="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
            </svg>
          ) : (
            <svg className="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
            </svg>
          )}
          <span className="text-sm font-medium">{notification.message}</span>
        </div>
      )}
    </Layout>
  );
}
