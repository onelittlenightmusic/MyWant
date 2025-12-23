import { useState, useEffect } from 'react';
import { useWantTypeStore } from '@/stores/wantTypeStore';
import { WantTypeListItem, ExampleDef, WantConfiguration } from '@/types/wantType';
import { useKeyboardNavigation } from '@/hooks/useKeyboardNavigation';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { WantTypeGrid } from '@/components/dashboard/WantTypeGrid';
import { WantTypeDetailsSidebar } from '@/components/sidebar/WantTypeDetailsSidebar';
import { WantTypeStatsOverview } from '@/components/dashboard/WantTypeStatsOverview';
import { WantTypeFilters } from '@/components/dashboard/WantTypeFilters';
import { RightSidebar } from '@/components/layout/RightSidebar';
import { Layout } from '@/components/layout/Layout';
import { Header } from '@/components/layout/Header';
import { classNames } from '@/utils/helpers';
import { apiClient } from '@/api/client';

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
  const [sidebarMinimized, setSidebarMinimized] = useState(true); // Start minimized
  const [notification, setNotification] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [filteredWantTypes, setFilteredWantTypes] = useState<WantTypeListItem[]>([]);
  const [searchQuery, setSearchQuery] = useState('');
  const [showSummary, setShowSummary] = useState(false);

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

  // Handle deploy example
  const handleDeployExample = async (example: ExampleDef) => {
    try {
      await apiClient.createWant({
        metadata: example.want.metadata,
        spec: example.want.spec
      });
      setNotification({
        message: `Deployed example: ${example.name}`,
        type: 'success'
      });
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Failed to deploy example';
      setNotification({
        message: errorMessage,
        type: 'error'
      });
    }
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
        showSummary={showSummary}
        onSummaryToggle={() => setShowSummary(!showSummary)}
        sidebarMinimized={sidebarMinimized}
      />

      {/* Main content area with sidebar-aware layout */}
      <main className="flex-1 flex overflow-hidden bg-gray-50 mt-16 mr-[480px]">
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
      </main>

      {/* Summary Sidebar */}
      <RightSidebar
        isOpen={showSummary && !selectedWantType}
        onClose={() => setShowSummary(false)}
        title="Summary"
      >
        <div className="space-y-6">
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
      </RightSidebar>

      {/* Right Sidebar for Want Type Details */}
      <RightSidebar
        isOpen={!!selectedWantType}
        onClose={() => setSelectedWantType(null)}
        title={selectedWantType ? selectedWantType.metadata.name : undefined}
      >
        <WantTypeDetailsSidebar
          wantType={selectedWantType}
          onDownload={(wantType) => {
            const filename = `${wantType.metadata.name}.json`;
            const jsonContent = JSON.stringify(wantType, null, 2);

            const element = document.createElement('a');
            element.setAttribute(
              'href',
              `data:application/json;charset=utf-8,${encodeURIComponent(jsonContent)}`
            );
            element.setAttribute('download', filename);
            element.style.display = 'none';

            document.body.appendChild(element);
            element.click();
            document.body.removeChild(element);
          }}
          onDeployExample={handleDeployExample}
        />
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
