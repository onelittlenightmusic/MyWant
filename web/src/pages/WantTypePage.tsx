import { useState, useEffect } from 'react';
import { Menu, Zap, AlertCircle } from 'lucide-react';
import { useWantTypeStore } from '@/stores/wantTypeStore';
import { WantTypeListItem } from '@/types/wantType';
import { WantTypeGrid } from '@/components/dashboard/WantTypeGrid';
import { WantTypeMenu } from '@/components/dashboard/WantTypeMenu';
import { WantTypeDetailsSidebar } from '@/components/sidebar/WantTypeDetailsSidebar';
import { WantTypeControlPanel } from '@/components/dashboard/WantTypeControlPanel';
import { RightSidebar } from '@/components/layout/RightSidebar';
import { Sidebar } from '@/components/layout/Sidebar';
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
    setFilters,
    clearFilters,
    clearError,
    getCategories,
    getPatterns,
    getFilteredWantTypes,
  } = useWantTypeStore();

  // UI State
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [sidebarMinimized, setSidebarMinimized] = useState(false);
  const [notification, setNotification] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

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

  // Handle want type selection to fetch details
  const handleSelectWantType = (wantType: WantTypeListItem) => {
    getWantType(wantType.name);
  };

  // Handle view details - open right sidebar
  const handleViewDetails = async (wantType: WantTypeListItem) => {
    await getWantType(wantType.name);
  };

  // Handle search
  const handleSearch = (term: string) => {
    setFilters({ searchTerm: term });
  };

  // Handle category filter
  const handleCategoryChange = (category?: string) => {
    setFilters({ category });
  };

  // Handle pattern filter
  const handlePatternChange = (pattern?: string) => {
    setFilters({ pattern });
  };

  const filteredWantTypes = getFilteredWantTypes();
  const categories = getCategories();
  const patterns = getPatterns();

  return (
    <div className="min-h-screen bg-gray-50 flex">
      {/* Mobile sidebar toggle */}
      <div className="lg:hidden fixed top-4 left-4 z-40">
        <button
          onClick={() => setSidebarOpen(true)}
          className="p-2 rounded-md bg-white shadow-md border border-gray-200 text-gray-600 hover:text-gray-900"
        >
          <Menu className="h-5 w-5" />
        </button>
      </div>

      {/* Sidebar */}
      <Sidebar
        isOpen={sidebarOpen}
        isMinimized={sidebarMinimized}
        onClose={() => setSidebarOpen(false)}
        onMinimizeToggle={() => setSidebarMinimized(!sidebarMinimized)}
      />

      {/* Main content */}
      <div className={classNames(
        "flex-1 flex flex-col relative transition-all duration-300 ease-in-out",
        sidebarMinimized ? "lg:ml-20" : "lg:ml-64"
      )}>
        {/* Header */}
        <header className="bg-white border-b border-gray-200 px-6 py-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center space-x-4">
              <h1 className="text-2xl font-bold text-gray-900 flex items-center gap-2">
                <Zap className="h-6 w-6 text-blue-600" />
                Want Types
              </h1>
              <div className="text-sm text-gray-500">
                {wantTypes.length} type{wantTypes.length !== 1 ? 's' : ''}
              </div>
            </div>

            <button
              onClick={() => fetchWantTypes()}
              disabled={loading}
              className="flex items-center space-x-2 px-3 py-2 text-gray-600 hover:text-gray-900 border border-gray-300 rounded-md hover:bg-gray-50 disabled:opacity-50"
            >
              {loading ? (
                <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-gray-600"></div>
              ) : (
                <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                </svg>
              )}
              <span>Refresh</span>
            </button>
          </div>
        </header>

        {/* Main content area */}
        <main className="flex-1 p-6 overflow-y-auto">
          {/* Error Message */}
          {error && (
            <div className="mb-6 p-4 bg-red-50 border border-red-200 rounded-md">
              <div className="flex items-center">
                <AlertCircle className="h-5 w-5 text-red-400 flex-shrink-0" />
                <div className="ml-3">
                  <p className="text-sm text-red-700">{error}</p>
                </div>
                <button
                  onClick={clearError}
                  className="ml-auto text-red-400 hover:text-red-600"
                >
                  <svg className="h-4 w-4" viewBox="0 0 20 20" fill="currentColor">
                    <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
                  </svg>
                </button>
              </div>
            </div>
          )}

          {/* Filter Menu */}
          <div className="mb-6 bg-white p-4 rounded-lg border border-gray-200">
            <WantTypeMenu
              categories={categories}
              patterns={patterns}
              selectedCategory={filters.category}
              selectedPattern={filters.pattern}
              searchTerm={filters.searchTerm}
              onSearchChange={handleSearch}
              onCategoryChange={handleCategoryChange}
              onPatternChange={handlePatternChange}
              onClearFilters={clearFilters}
            />
          </div>

          {/* Want Types Grid */}
          <WantTypeGrid
            wantTypes={filteredWantTypes}
            selectedWantType={filteredWantTypes.find(wt => wt.name === selectedWantType?.metadata.name) || null}
            onSelectWantType={handleSelectWantType}
            onViewDetails={handleViewDetails}
            loading={loading}
          />
        </main>

        {/* Want Type Control Panel */}
        <WantTypeControlPanel
          selectedWantType={selectedWantType}
          onViewDetails={handleViewDetails}
          onDeploySuccess={(message) => setNotification({ message, type: 'success' })}
          onDeployError={(error) => setNotification({ message: error, type: 'error' })}
          loading={loading}
          sidebarMinimized={sidebarMinimized}
        />

        {/* Right Sidebar for Want Type Details */}
        <RightSidebar
          isOpen={!!selectedWantType}
          onClose={() => {
            // Store will maintain the selection, but sidebar will close
            // Next selection will open it again
          }}
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
      </div>
    </div>
  );
}
