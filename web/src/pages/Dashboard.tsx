import React, { useState, useEffect } from 'react';
import { WantExecutionStatus, Want } from '@/types/want';
import { useWantStore } from '@/stores/wantStore';
import { usePolling } from '@/hooks/usePolling';

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
import { WantControlPanel } from '@/components/dashboard/WantControlPanel';

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
  const [deleteWantState, setDeleteWantState] = useState<Want | null>(null);
  const [sidebarMinimized, setSidebarMinimized] = useState(false);

  // Derive selectedWant from wants array using selectedWantId
  // This ensures selectedWant always reflects the current data from polling
  const selectedWant = selectedWantId
    ? wants.find(w => (w.metadata?.id === selectedWantId) || (w.id === selectedWantId)) || null
    : null;

  // Filters
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilters, setStatusFilters] = useState<WantExecutionStatus[]>([]);

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
          setSelectedWant(null);
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

  return (
    <Layout
      sidebarMinimized={sidebarMinimized}
      onSidebarMinimizedChange={setSidebarMinimized}
    >
      {/* Header */}
      <Header onCreateWant={handleCreateWant} />

      {/* Main content area with padding for fixed control panel */}
      <main className="flex-1 p-6 pb-24">
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

          {/* Stats Overview */}
          <div className="mb-8">
            <StatsOverview wants={wants} loading={loading} />
          </div>

          {/* Filters */}
          <WantFilters
            searchQuery={searchQuery}
            onSearchChange={setSearchQuery}
            selectedStatuses={statusFilters}
            onStatusFilter={setStatusFilters}
          />

          {/* Want Grid */}
          <div>
            <WantGrid
              wants={wants}
              loading={loading}
              searchQuery={searchQuery}
              statusFilters={statusFilters}
              selectedWant={selectedWant}
              onViewWant={handleViewWant}
              onEditWant={handleEditWant}
              onDeleteWant={setDeleteWantState}
              onSuspendWant={handleSuspendWant}
              onResumeWant={handleResumeWant}
            />
          </div>
      </main>

      {/* Right Sidebar for Want Details */}
      <RightSidebar
        isOpen={!!selectedWant}
        onClose={() => setSelectedWant(null)}
        title={selectedWant ? (selectedWant.metadata?.name || selectedWant.metadata?.id || 'Want Details') : undefined}
      >
        <WantDetailsSidebar want={selectedWant} />
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

      {/* Control Panel */}
      <WantControlPanel
        selectedWant={selectedWant}
        onStart={handleStartWant}
        onStop={handleStopWant}
        onSuspend={handleSuspendWant}
        onResume={handleResumeWant}
        onDelete={setDeleteWantState}
        loading={loading}
        sidebarMinimized={sidebarMinimized}
      />
    </Layout>
  );
};