import React, { useState, useEffect } from 'react';
import { AgentResponse } from '@/types/agent';
import { useAgentStore } from '@/stores/agentStore';

import { classNames } from '@/utils/helpers';
import { useKeyboardNavigation } from '@/hooks/useKeyboardNavigation';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { useRightSidebarExclusivity } from '@/hooks/useRightSidebarExclusivity';

// Components
import { Layout } from '@/components/layout/Layout';
import { Header } from '@/components/layout/Header';
import { RightSidebar } from '@/components/layout/RightSidebar';
import { AgentFilters } from '@/components/dashboard/AgentFilters';
import { AgentGrid } from '@/components/dashboard/AgentGrid';
import { AgentDetailsSidebar } from '@/components/sidebar/AgentDetailsSidebar';
import { ConfirmDeleteModal } from '@/components/modals/ConfirmDeleteModal';
import { AgentStatsOverview } from '@/components/dashboard/AgentStatsOverview';

export const AgentsPage: React.FC = () => {
  const {
    agents,
    loading,
    error,
    fetchAgents,
    deleteAgent,
    clearError
  } = useAgentStore();

  // UI State
  const sidebar = useRightSidebarExclusivity<AgentResponse>();
  const [sidebarMinimized, setSidebarMinimized] = useState(true); // Start minimized
  const [editingAgent, setEditingAgent] = useState<AgentResponse | null>(null);
  const [deleteAgentState, setDeleteAgentState] = useState<AgentResponse | null>(null);
  const [filteredAgents, setFilteredAgents] = useState<AgentResponse[]>([]);

  // Filters
  const [searchQuery, setSearchQuery] = useState('');
  const [typeFilters, setTypeFilters] = useState<('do' | 'monitor')[]>([]);

  // Load initial data
  useEffect(() => {
    fetchAgents();
  }, [fetchAgents]);

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
  const handleCreateAgent = () => {
    setEditingAgent(null);
    sidebar.openForm();
  };

  const handleEditAgent = (agent: AgentResponse) => {
    setEditingAgent(agent);
    sidebar.openForm();
  };

  const handleViewAgent = (agent: AgentResponse) => {
    sidebar.selectItem(agent);
  };

  const handleDeleteAgentConfirm = async () => {
    if (deleteAgentState) {
      try {
        await deleteAgent(deleteAgentState.name);
        setDeleteAgentState(null);
      } catch (error) {
        console.error('Failed to delete agent:', error);
      }
    }
  };

  const handleCloseModals = () => {
    sidebar.closeForm();
    setEditingAgent(null);
    setDeleteAgentState(null);
  };

  // Keyboard navigation handler
  const handleKeyboardNavigate = (index: number) => {
    if (filteredAgents[index]) {
      sidebar.selectItem(filteredAgents[index]);
    }
  };

  // Calculate current agent index for keyboard navigation
  const currentAgentIndex = sidebar.selectedItem
    ? filteredAgents.findIndex(agent => agent.name === sidebar.selectedItem?.name)
    : -1;

  // Keyboard navigation hook
  useKeyboardNavigation({
    itemCount: filteredAgents.length,
    currentIndex: currentAgentIndex,
    onNavigate: handleKeyboardNavigate,
    enabled: !sidebar.showForm && filteredAgents.length > 0
  });

  // Handle ESC key to close details sidebar and deselect
  const handleEscapeKey = () => {
    if (sidebar.selectedItem) {
      sidebar.clearSelection();
    }
  };

  useEscapeKey({
    onEscape: handleEscapeKey,
    enabled: !!sidebar.selectedItem
  });

  // Stats calculation
  const stats = {
    total: agents.length,
    doAgents: agents.filter(a => a.type === 'do').length,
    monitorAgents: agents.filter(a => a.type === 'monitor').length,
    totalCapabilities: agents.reduce((acc, agent) => acc + agent.capabilities.length, 0)
  };

  return (
    <Layout
      sidebarMinimized={sidebarMinimized}
      onSidebarMinimizedChange={setSidebarMinimized}
    >
      {/* Header */}
      <Header
        onCreateWant={handleCreateAgent}
        title="Agents"
        createButtonLabel="Add Agent"
        itemCount={agents.length}
        itemLabel="agent"
        showSummary={sidebar.showSummary}
        onSummaryToggle={sidebar.toggleSummary}
        sidebarMinimized={sidebarMinimized}
      />

      {/* Main content area with sidebar-aware layout */}
      <main className="flex-1 flex overflow-hidden bg-gray-50 dark:bg-gray-950 lg:mr-[480px] mr-0">
        {/* Left content area - main dashboard */}
        <div className="flex-1 overflow-y-auto">
          <div className="p-6 pb-24">
            {/* Error message */}
            {error && (
              <div className="mb-6 p-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md">
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
                    <p className="text-sm text-red-700 dark:text-red-300">{error}</p>
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

            {/* Agent Grid */}
            <div>
              <AgentGrid
                agents={agents}
                loading={loading}
                searchQuery={searchQuery}
                typeFilters={typeFilters}
                selectedAgent={sidebar.selectedItem}
                onViewAgent={handleViewAgent}
                onEditAgent={handleEditAgent}
                onDeleteAgent={setDeleteAgentState}
                onGetFilteredAgents={setFilteredAgents}
              />
            </div>
          </div>
        </div>

        {/* Summary Sidebar */}
        <RightSidebar
          isOpen={sidebar.showSummary}
          onClose={sidebar.closeSummary}
          title="Summary"
        >
          <div className="space-y-6">
            <div>
              <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Statistics</h3>
              <div>
                <AgentStatsOverview agents={agents} loading={loading} />
              </div>
            </div>

            {/* Filters section */}
            <div>
              <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Filters</h3>
              <AgentFilters
                searchQuery={searchQuery}
                onSearchChange={setSearchQuery}
                selectedTypes={typeFilters}
                onTypeFilter={setTypeFilters}
              />
            </div>
          </div>
        </RightSidebar>
      </main>

      {/* Right Sidebar for Agent Details */}
      <RightSidebar
        isOpen={!!sidebar.selectedItem}
        onClose={sidebar.clearSelection}
        title={sidebar.selectedItem ? sidebar.selectedItem.name : undefined}
      >
        <AgentDetailsSidebar agent={sidebar.selectedItem} />
      </RightSidebar>

      {/* Modals */}
      <ConfirmDeleteModal
        isOpen={!!deleteAgentState}
        onClose={handleCloseModals}
        onConfirm={handleDeleteAgentConfirm}
        want={null}
        loading={loading}
        title="Delete Agent"
        message={`Are you sure you want to delete the agent "${deleteAgentState?.name}"? This action cannot be undone.`}
      />
    </Layout>
  );
};