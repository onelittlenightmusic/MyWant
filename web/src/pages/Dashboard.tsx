import React, { useState, useEffect } from 'react';
import { Menu } from 'lucide-react';
import { WantExecutionStatus, Want } from '@/types/want';
import { useWantStore } from '@/stores/wantStore';
import { usePolling } from '@/hooks/usePolling';

// Components
import { Header } from '@/components/layout/Header';
import { Sidebar } from '@/components/layout/Sidebar';
import { StatsOverview } from '@/components/dashboard/StatsOverview';
import { WantFilters } from '@/components/dashboard/WantFilters';
import { WantGrid } from '@/components/dashboard/WantGrid';
import { WantForm } from '@/components/forms/WantForm';
import { WantDetailsModal } from '@/components/modals/WantDetailsModal';
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
    clearError
  } = useWantStore();

  // UI State
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [showCreateForm, setShowCreateForm] = useState(false);
  const [editingWant, setEditingWant] = useState<Want | null>(null);
  const [selectedWant, setSelectedWant] = useState<Want | null>(null);
  const [deleteWantState, setDeleteWantState] = useState<Want | null>(null);

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
    setSelectedWant(want);
  };

  const handleDeleteWantConfirm = async () => {
    if (deleteWantState) {
      try {
        await deleteWant(deleteWantState.id);
        setDeleteWantState(null);
      } catch (error) {
        console.error('Failed to delete want:', error);
      }
    }
  };

  const handleSuspendWant = async (want: Want) => {
    if (!want.id) return;
    try {
      await suspendWant(want.id);
    } catch (error) {
      console.error('Failed to suspend want:', error);
    }
  };

  const handleResumeWant = async (want: Want) => {
    if (!want.id) return;
    try {
      await resumeWant(want.id);
    } catch (error) {
      console.error('Failed to resume want:', error);
    }
  };

  const handleCloseModals = () => {
    setShowCreateForm(false);
    setEditingWant(null);
    setSelectedWant(null);
    setDeleteWantState(null);
  };

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
        onClose={() => setSidebarOpen(false)}
      />

      {/* Main content */}
      <div className="flex-1 lg:ml-0 flex flex-col">
        {/* Header */}
        <Header onCreateWant={handleCreateWant} />

        {/* Main content area */}
        <main className="flex-1 p-6">
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
              onViewWant={handleViewWant}
              onEditWant={handleEditWant}
              onDeleteWant={setDeleteWantState}
              onSuspendWant={handleSuspendWant}
              onResumeWant={handleResumeWant}
            />
          </div>
        </main>
      </div>

      {/* Modals */}
      <WantForm
        isOpen={showCreateForm}
        onClose={handleCloseModals}
        editingWant={editingWant}
      />

      <WantDetailsModal
        isOpen={!!selectedWant}
        onClose={handleCloseModals}
        want={selectedWant}
      />

      <ConfirmDeleteModal
        isOpen={!!deleteWantState}
        onClose={handleCloseModals}
        onConfirm={handleDeleteWantConfirm}
        want={deleteWantState}
        loading={loading}
      />
    </div>
  );
};