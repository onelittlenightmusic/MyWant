import React, { useState, useEffect } from 'react';
import { RefreshCw } from 'lucide-react';
import { WantExecutionStatus, Want } from '@/types/want';
import { useWantStore } from '@/stores/wantStore';
import { usePolling } from '@/hooks/usePolling';
import { useHierarchicalKeyboardNavigation } from '@/hooks/useHierarchicalKeyboardNavigation';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { StatusBadge } from '@/components/common/StatusBadge';
import { classNames } from '@/utils/helpers';

// Components
import { Layout } from '@/components/layout/Layout';
import { Header } from '@/components/layout/Header';
import { RightSidebar } from '@/components/layout/RightSidebar';
import { StatsOverview } from '@/components/dashboard/StatsOverview';
import { WantFilters } from '@/components/dashboard/WantFilters';
import { WantGrid } from '@/components/dashboard/WantGrid';
import { WantForm } from '@/components/forms/WantForm';
import { WantDetailsSidebar } from '@/components/sidebar/WantDetailsSidebar';
import { ConfirmDeleteModal } from '@/components/modals/ConfirmDeleteModal';

export const Dashboard: React.FC = () => {
  const {
    wants,
    loading,
    error,
    fetchWants,
    deleteWant,
    suspendWant,
    resumeWant,
    stopWant,
    startWant,
    clearError
  } = useWantStore();

  // UI State
  const [showCreateForm, setShowCreateForm] = useState(false);
  const [editingWant, setEditingWant] = useState<Want | null>(null);
  const [selectedWantId, setSelectedWantId] = useState<string | null>(null);
  const [lastSelectedWantId, setLastSelectedWantId] = useState<string | null>(null);
  const [deleteWantState, setDeleteWantState] = useState<Want | null>(null);
  const [sidebarMinimized, setSidebarMinimized] = useState(false); // Start expanded, auto-collapse on mouse leave
  const [sidebarInitialTab, setSidebarInitialTab] = useState<'settings' | 'results' | 'logs' | 'agents'>('settings');
  const [expandedParents, setExpandedParents] = useState<Set<string>>(new Set());

  // Derive selectedWant from wants array using selectedWantId
  // This ensures selectedWant always reflects the current data from polling
  const selectedWant = selectedWantId
    ? wants.find(w => (w.metadata?.id === selectedWantId) || (w.id === selectedWantId)) || null
    : null;

  // Filters
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilters, setStatusFilters] = useState<WantExecutionStatus[]>([]);

  // Keyboard navigation
  const [filteredWants, setFilteredWants] = useState<Want[]>([]);

  // Flatten hierarchical wants to a single array while preserving parent-child relationships
  // This allows proper sibling navigation within child wants
  const flattenedWants = filteredWants.flatMap((parentWant: any) => {
    const items = [parentWant];
    if (parentWant.children && Array.isArray(parentWant.children)) {
      items.push(...parentWant.children);
    }
    return items;
  });

  // Convert wants to hierarchical format for keyboard navigation
  const hierarchicalWants = flattenedWants.map(want => ({
    id: want.metadata?.id || want.id || '',
    parentId: want.metadata?.ownerReferences?.[0]?.id
  }));

  const currentHierarchicalWant = selectedWant ? {
    id: selectedWant.metadata?.id || selectedWant.id || '',
    parentId: selectedWant.metadata?.ownerReferences?.[0]?.id
  } : null;

  // Header state for sidebar
  const [headerState, setHeaderState] = useState<{ autoRefresh: boolean; loading: boolean; status: WantExecutionStatus } | null>(null);

  // Load initial data
  useEffect(() => {
    fetchWants();
  }, [fetchWants]);

  // Auto-refresh wants every 5 seconds
  usePolling(
    () => {
      if (wants.length > 0) {
        fetchWants();
      }
    },
    {
      interval: 5000,
      enabled: true,
      immediate: false
    }
  );

  // Clear selection if selected want was deleted
  useEffect(() => {
    if (selectedWantId) {
      const stillExists = wants.some(w =>
        (w.metadata?.id === selectedWantId) || (w.id === selectedWantId)
      );

      // Only clear selection if the want was actually deleted
      if (!stillExists) {
        setSelectedWantId(null);
      }
    }
  }, [wants, selectedWantId]);

  // Clear errors after 5 seconds
  useEffect(() => {
    if (error) {
      const timer = setTimeout(() => {
        clearError();
      }, 5000);
      return () => clearTimeout(timer);
    }
  }, [error, clearError]);

  // Handlers
  const handleCreateWant = () => {
    setEditingWant(null);
    setShowCreateForm(true);
  };

  const handleEditWant = (want: Want) => {
    setEditingWant(want);
    setShowCreateForm(true);
  };

  const handleViewWant = (want: Want) => {
    const wantId = want.metadata?.id || want.id;
    setSelectedWantId(wantId || null);
    setSidebarInitialTab('settings');
  };

  const handleViewAgents = (want: Want) => {
    const wantId = want.metadata?.id || want.id;
    setSelectedWantId(wantId || null);
    setSidebarInitialTab('agents');
  };

  const handleDeleteWantConfirm = async () => {
    if (deleteWantState) {
      try {
        const wantId = deleteWantState.metadata?.id || deleteWantState.id;
        if (!wantId) {
          console.error('No want ID found for deletion');
          return;
        }
        await deleteWant(wantId);
        setDeleteWantState(null);

        // Close the details sidebar if the deleted want is currently selected
        if (selectedWant && (selectedWant.metadata?.id === wantId || selectedWant.id === wantId)) {
          setSelectedWantId(null);
        }
      } catch (error) {
        console.error('Failed to delete want:', error);
      }
    }
  };

  const handleSuspendWant = async (want: Want) => {
    const wantId = want.metadata?.id || want.id;
    if (!wantId) return;
    try {
      await suspendWant(wantId);
    } catch (error) {
      console.error('Failed to suspend want:', error);
    }
  };

  const handleResumeWant = async (want: Want) => {
    const wantId = want.metadata?.id || want.id;
    if (!wantId) return;
    try {
      await resumeWant(wantId);
    } catch (error) {
      console.error('Failed to resume want:', error);
    }
  };

  const handleStopWant = async (want: Want) => {
    const wantId = want.metadata?.id || want.id;
    if (!wantId) return;
    try {
      await stopWant(wantId);
    } catch (error) {
      console.error('Failed to stop want:', error);
    }
  };

  const handleStartWant = async (want: Want) => {
    const wantId = want.metadata?.id || want.id;
    if (!wantId) return;
    try {
      await startWant(wantId);
    } catch (error) {
      console.error('Failed to start want:', error);
    }
  };

  const handleCloseModals = () => {
    setShowCreateForm(false);
    setEditingWant(null);
    setDeleteWantState(null);
  };

  // Keyboard navigation handler
  const handleHierarchicalNavigate = (hierarchicalItem: { id: string; parentId?: string } | null) => {
    if (!hierarchicalItem) return;

    // Find the corresponding want in flattenedWants
    const targetWant = flattenedWants.find(w =>
      (w.metadata?.id === hierarchicalItem.id) || (w.id === hierarchicalItem.id)
    );

    if (targetWant) {
      handleViewWant(targetWant);
    }
  };

  // Handler to toggle expand/collapse of a parent want
  const handleToggleExpand = (wantId: string) => {
    setExpandedParents(prev => {
      const updated = new Set(prev);
      if (updated.has(wantId)) {
        updated.delete(wantId);
      } else {
        updated.add(wantId);
      }
      return updated;
    });
  };

  // Use hierarchical keyboard navigation hook
  useHierarchicalKeyboardNavigation({
    items: hierarchicalWants,
    currentItem: currentHierarchicalWant,
    onNavigate: handleHierarchicalNavigate,
    onToggleExpand: handleToggleExpand,
    expandedItems: expandedParents,
    lastSelectedItemId: lastSelectedWantId,
    enabled: !showCreateForm && filteredWants.length > 0 // Disable when form is open
  });

  // Handle ESC key to close details sidebar and deselect
  const handleEscapeKey = () => {
    if (selectedWant) {
      // Remember the last selected want before deselecting
      const wantId = selectedWant.metadata?.id || selectedWant.id;
      if (wantId) {
        setLastSelectedWantId(wantId);
      }
      setSelectedWantId(null);
    }
  };

  useEscapeKey({
    onEscape: handleEscapeKey,
    enabled: !!selectedWant
  });

  // Determine background style for flight, hotel, restaurant, and buffet wants
  const getWantBackgroundImage = (type?: string) => {
    if (type === 'flight') return '/resources/flight.png';
    if (type === 'hotel') return '/resources/hotel.png';
    if (type === 'restaurant') return '/resources/restaurant.png';
    if (type === 'buffet') return '/resources/buffet.png';
    return undefined;
  };

  const wantBackgroundImage = getWantBackgroundImage(selectedWant?.metadata?.type);
  const sidebarBackgroundStyle = wantBackgroundImage ? {
    backgroundImage: `url(${wantBackgroundImage})`,
    backgroundSize: 'cover',
    backgroundPosition: 'center',
    backgroundAttachment: 'fixed'
  } : undefined;

  // Create header actions from header state
  const headerActions = headerState ? (
    <div className="flex items-center gap-2">
      <StatusBadge status={headerState.status} size="sm" />
      <button
        onClick={() => {/* auto refresh toggle will be handled in sidebar */}}
        className={`p-2 rounded-md transition-colors ${
          headerState.autoRefresh
            ? 'bg-blue-100 text-blue-600 hover:bg-blue-200'
            : 'text-gray-400 hover:text-gray-600 hover:bg-gray-100'
        }`}
        title={headerState.autoRefresh ? 'Disable auto refresh' : 'Enable auto refresh'}
      >
        <RefreshCw className="h-4 w-4" />
      </button>
      <button
        disabled={headerState.loading}
        className="p-2 text-gray-400 hover:text-gray-600 hover:bg-gray-100 rounded-md transition-colors"
        title="Refresh"
      >
        <RefreshCw className={classNames('h-4 w-4', headerState.loading && 'animate-spin')} />
      </button>
    </div>
  ) : null;

  return (
    <Layout
      sidebarMinimized={sidebarMinimized}
      onSidebarMinimizedChange={setSidebarMinimized}
    >
      {/* Header */}
      <Header
        onCreateWant={handleCreateWant}
        searchQuery={searchQuery}
        onSearchChange={setSearchQuery}
      />

      {/* Main content area with sidebar-aware layout */}
      <main className="flex-1 flex overflow-hidden bg-gray-50">
        {/* Left content area - main dashboard */}
        <div className="flex-1 overflow-y-auto">
          <div className="p-6 pb-24">
            {/* Error message */}
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

            {/* Want Grid */}
            <div>
              <WantGrid
                wants={wants}
                loading={loading}
                searchQuery={searchQuery}
                statusFilters={statusFilters}
                selectedWant={selectedWant}
                onViewWant={handleViewWant}
                onViewAgentsWant={handleViewAgents}
                onEditWant={handleEditWant}
                onDeleteWant={setDeleteWantState}
                onSuspendWant={handleSuspendWant}
                onResumeWant={handleResumeWant}
                onGetFilteredWants={setFilteredWants}
                expandedParents={expandedParents}
                onToggleExpand={handleToggleExpand}
              />
            </div>
          </div>
        </div>

        {/* Right sidebar area - reserved for statistics (hidden when sidebar is open) */}
        <div className={`w-[480px] bg-white border-l border-gray-200 overflow-y-auto transition-opacity duration-300 ease-in-out ${selectedWant ? 'opacity-0 pointer-events-none' : 'opacity-100'}`}>
          <div className="p-6 space-y-6">
            <div>
              <h3 className="text-lg font-semibold text-gray-900 mb-4">Statistics</h3>
              <div>
                <StatsOverview wants={wants} loading={loading} layout="vertical" />
              </div>
            </div>

            {/* Filters section */}
            <div>
              <h3 className="text-lg font-semibold text-gray-900 mb-4">Filters</h3>
              <WantFilters
                searchQuery={searchQuery}
                onSearchChange={setSearchQuery}
                selectedStatuses={statusFilters}
                onStatusFilter={setStatusFilters}
              />
            </div>
          </div>
        </div>
      </main>

      {/* Right Sidebar for Want Details */}
      <RightSidebar
        isOpen={!!selectedWant}
        onClose={() => setSelectedWantId(null)}
        title={selectedWant ? (selectedWant.metadata?.name || selectedWant.metadata?.id || 'Want Details') : undefined}
        backgroundStyle={sidebarBackgroundStyle}
        headerActions={headerActions}
      >
        <WantDetailsSidebar
          want={selectedWant}
          initialTab={sidebarInitialTab}
          onWantUpdate={() => {
            if (selectedWant?.metadata?.id || selectedWant?.id) {
              const wantId = (selectedWant.metadata?.id || selectedWant.id) as string;
              const { fetchWantDetails } = useWantStore.getState();
              fetchWantDetails(wantId);
            }
          }}
          onHeaderStateChange={setHeaderState}
          onStart={handleStartWant}
          onStop={handleStopWant}
          onSuspend={handleSuspendWant}
          onResume={handleResumeWant}
          onDelete={setDeleteWantState}
        />
      </RightSidebar>

      {/* Modals */}
      <WantForm
        isOpen={showCreateForm}
        onClose={handleCloseModals}
        editingWant={editingWant}
      />

      <ConfirmDeleteModal
        isOpen={!!deleteWantState}
        onClose={handleCloseModals}
        onConfirm={handleDeleteWantConfirm}
        want={deleteWantState}
        loading={loading}
        childrenCount={
          deleteWantState
            ? wants.filter(w =>
                w.metadata?.ownerReferences?.some(
                  ref => ref.id === deleteWantState.metadata?.id
                )
              ).length
            : 0
        }
      />
    </Layout>
  );
};