import React, { useState, useEffect } from 'react';
import { RefreshCw } from 'lucide-react';
import { WantExecutionStatus, Want } from '@/types/want';
import { useWantStore } from '@/stores/wantStore';
import { usePolling } from '@/hooks/usePolling';
import { useHierarchicalKeyboardNavigation } from '@/hooks/useHierarchicalKeyboardNavigation';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { StatusBadge } from '@/components/common/StatusBadge';
import { classNames, truncateText } from '@/utils/helpers';
import { addLabelToRegistry } from '@/utils/labelUtils';

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
  const [sidebarMinimized, setSidebarMinimized] = useState(true); // Start minimized
  const [sidebarInitialTab, setSidebarInitialTab] = useState<'settings' | 'results' | 'logs' | 'agents'>('settings');
  const [expandedParents, setExpandedParents] = useState<Set<string>>(new Set());
  const [showAddLabelForm, setShowAddLabelForm] = useState(false);
  const [newLabel, setNewLabel] = useState<{ key: string; value: string }>({ key: '', value: '' });
  const [selectedLabel, setSelectedLabel] = useState<{ key: string; value: string } | null>(null);
  const [labelOwners, setLabelOwners] = useState<Want[]>([]);
  const [labelUsers, setLabelUsers] = useState<Want[]>([]);
  const [allLabels, setAllLabels] = useState<Map<string, Set<string>>>(new Map());
  const [showSummary, setShowSummary] = useState(false);

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

  // Fetch labels from API
  const fetchLabels = async () => {
    try {
      const response = await fetch('http://localhost:8080/api/v1/labels');
      if (response.ok) {
        const data = await response.json();
        const labelsMap = new Map<string, Set<string>>();

        // Process labelValues from API response
        if (data.labelValues) {
          for (const [key, valuesArray] of Object.entries(data.labelValues)) {
            if (!labelsMap.has(key)) {
              labelsMap.set(key, new Set());
            }
            if (Array.isArray(valuesArray)) {
              (valuesArray as any[]).forEach(item => {
                const value = typeof item === 'string' ? item : item.value;
                if (value) {
                  labelsMap.get(key)!.add(value);
                }
              });
            }
          }
        }

        setAllLabels(labelsMap);
      }
    } catch (error) {
      console.error('Error fetching labels:', error);
    }
  };

  // Load initial data
  useEffect(() => {
    fetchWants();
    fetchLabels();
  }, [fetchWants]);

  // Auto-refresh wants every 1 second
  usePolling(
    () => {
      if (wants.length > 0) {
        fetchWants();
      }
      // Also refresh labels in case new labels were added
      fetchLabels();
    },
    {
      interval: 1000,
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

  // Prevent sidebar overlap by ensuring mutual exclusivity
  // Auto-deselect Details sidebar when Add Want form opens
  useEffect(() => {
    if (showCreateForm) {
      setSelectedWantId(null);
    }
  }, [showCreateForm]);

  // Close Add Want form when Details sidebar opens
  useEffect(() => {
    if (selectedWantId) {
      setShowCreateForm(false);
    }
  }, [selectedWantId]);

  // Handlers
  const handleCreateWant = () => {
    setEditingWant(null);
    setSelectedWantId(null);
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

  // Fetch wants that use a specific label
  const handleLabelClick = async (key: string, value: string) => {
    setSelectedLabel({ key, value });
    // Reset state immediately when clicking a new label
    setLabelOwners([]);
    setLabelUsers([]);

    try {
      // Fetch the label data which includes owners and users
      const response = await fetch('http://localhost:8080/api/v1/labels');
      if (!response.ok) {
        console.error('Failed to fetch labels');
        return;
      }

      const data = await response.json();
      console.log('[DEBUG] Label data received:', data);

      // Find the label values for this key
      if (data.labelValues && data.labelValues[key]) {
        const labelValueInfo = data.labelValues[key].find(
          (item: { value: string; owners: string[]; users: string[] }) => item.value === value
        );

        console.log(`[DEBUG] Label ${key}:${value} info:`, labelValueInfo);

        if (labelValueInfo) {
          // Fetch all wants and filter by the owner and user IDs
          const wantResponse = await fetch('http://localhost:8080/api/v1/wants');
          if (wantResponse.ok) {
            const wantData = await wantResponse.json();

            // Separate owners and users
            const owners = wantData.wants.filter((w: Want) =>
              labelValueInfo.owners.includes(w.metadata?.id || w.id || '')
            );
            const users = wantData.wants.filter((w: Want) =>
              labelValueInfo.users.includes(w.metadata?.id || w.id || '')
            );

            console.log(`[DEBUG] Filtered owners (count: ${owners.length}):`, owners.map(w => w.metadata?.name || w.id));
            console.log(`[DEBUG] Filtered users (count: ${users.length}):`, users.map(w => w.metadata?.name || w.id));

            setLabelOwners(owners);
            setLabelUsers(users);
          }
        } else {
          console.log(`[DEBUG] Label ${key}:${value} not found in API response`);
        }
      } else {
        console.log(`[DEBUG] Key ${key} not found in label values`);
      }
    } catch (error) {
      console.error('Error fetching label owners/users:', error);
    }
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

  // Handler for when a label is dropped on a want
  const handleLabelDropped = async (wantId: string) => {
    // Refresh the wants list to get the updated want with new label
    await fetchWants();

    // Select the want and open the sidebar to show the newly added label
    setSelectedWantId(wantId);
    setSidebarInitialTab('settings');
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
        showSummary={showSummary}
        onSummaryToggle={() => setShowSummary(!showSummary)}
        sidebarMinimized={sidebarMinimized}
      />

      {/* Main content area with sidebar-aware layout */}
      <main className="flex-1 flex overflow-hidden bg-gray-50 mt-16 mr-[480px]">
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
                onCreateWant={handleCreateWant}
                onLabelDropped={handleLabelDropped}
              />
            </div>
          </div>
        </div>

        {/* Summary Sidebar */}
        <RightSidebar
          isOpen={showSummary && !selectedWant}
          onClose={() => setShowSummary(false)}
          title="Summary"
        >
          <div className="space-y-6">
            {/* All Labels Section */}
            <div>
              <div className="flex items-center justify-between mb-4">
                <h3 className="text-lg font-semibold text-gray-900">Labels</h3>
                <button
                  onClick={() => setShowAddLabelForm(!showAddLabelForm)}
                  className="p-1.5 rounded-md text-gray-400 hover:text-gray-600 hover:bg-gray-100 transition-colors"
                  title="Add label"
                >
                  <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                  </svg>
                </button>
              </div>

              {/* Add Label Form */}
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
                        onClick={async () => {
                          if (newLabel.key.trim() && newLabel.value.trim()) {
                            const success = await addLabelToRegistry(newLabel.key, newLabel.value);
                            if (success) {
                              // Refresh labels and wants to show the new label
                              await fetchLabels();
                              fetchWants();
                              setNewLabel({ key: '', value: '' });
                              setShowAddLabelForm(false);
                            }
                          }
                        }}
                        disabled={!newLabel.key.trim() || !newLabel.value.trim()}
                        className="flex-1 px-3 py-2 text-sm font-medium text-white bg-blue-600 rounded-md hover:bg-blue-700 disabled:bg-gray-400 disabled:cursor-not-allowed transition-colors"
                      >
                        Add
                      </button>
                    </div>
                  </div>
                </div>
              )}

              <div>
                {allLabels.size === 0 ? (
                  <p className="text-sm text-gray-500">No labels found</p>
                ) : (
                  <div className="flex flex-wrap gap-2">
                    {Array.from(allLabels.entries()).map(([key, values]) => (
                      Array.from(values).map((value) => (
                        <div
                          key={`${key}-${value}`}
                          draggable
                          onDragStart={(e) => {
                            e.dataTransfer.effectAllowed = 'copy';
                            e.dataTransfer.setData('application/json', JSON.stringify({ key, value }));
                            // Create custom drag image
                            const dragImage = document.createElement('div');
                            dragImage.textContent = `${key}: ${value}`;
                            dragImage.style.position = 'absolute';
                            dragImage.style.left = '-9999px';
                            dragImage.style.padding = '6px 12px';
                            dragImage.style.borderRadius = '9999px';
                            dragImage.style.backgroundColor = '#dbeafe';
                            dragImage.style.color = '#1e40af';
                            dragImage.style.fontSize = '14px';
                            dragImage.style.fontWeight = '500';
                            dragImage.style.whiteSpace = 'nowrap';
                            dragImage.style.opacity = '0.8';
                            document.body.appendChild(dragImage);
                            e.dataTransfer.setDragImage(dragImage, 0, 0);
                            setTimeout(() => document.body.removeChild(dragImage), 0);
                          }}
                          onClick={() => handleLabelClick(key, value)}
                          title={`${key}: ${value}`.length > 20 ? `${key}: ${value}` : undefined}
                          className={classNames(
                            'inline-flex items-center px-3 py-1.5 rounded-full text-sm font-medium cursor-pointer hover:shadow-md transition-all select-none',
                            selectedLabel?.key === key && selectedLabel?.value === value
                              ? 'bg-blue-500 text-white shadow-md'
                              : 'bg-blue-100 text-blue-800 hover:bg-blue-200'
                          )}
                        >
                          {truncateText(`${key}: ${value}`, 20)}
                        </div>
                      ))
                    ))}
                  </div>
                )}
              </div>
            </div>

            {/* Owners and Users Section - Display wants using selected label */}
            {selectedLabel && (
              <div>
                <div className="flex items-center justify-between mb-4">
                  <h3 className="text-lg font-semibold text-gray-900">
                    {selectedLabel.key}: {selectedLabel.value}
                  </h3>
                  <button
                    onClick={() => {
                      setSelectedLabel(null);
                      setLabelOwners([]);
                      setLabelUsers([]);
                    }}
                    className="p-1.5 rounded-md text-gray-400 hover:text-gray-600 hover:bg-gray-100 transition-colors"
                    title="Clear selection"
                  >
                    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                    </svg>
                  </button>
                </div>

                {/* Owners Section */}
                {labelOwners.length > 0 && (
                  <div className="mb-4">
                    <h4 className="text-xs font-semibold text-gray-700 mb-2 uppercase">Owners</h4>
                    <div className="grid grid-cols-2 gap-2 max-h-40 overflow-y-auto">
                      {labelOwners.map((want) => {
                        const wantId = want.metadata?.id || want.id;
                        return (
                          <div
                            key={wantId}
                            onClick={() => handleViewWant(want)}
                            className="p-2 bg-blue-50 border border-blue-200 rounded hover:bg-blue-100 cursor-pointer transition-colors text-center"
                            title={want.metadata?.name || wantId}
                          >
                            <p className="text-xs font-medium text-gray-900 truncate">
                              {want.metadata?.name || wantId}
                            </p>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                )}

                {/* Users Section */}
                {labelUsers.length > 0 && (
                  <div>
                    <h4 className="text-xs font-semibold text-gray-700 mb-2 uppercase">Users</h4>
                    <div className="grid grid-cols-2 gap-2 max-h-40 overflow-y-auto">
                      {labelUsers.map((want) => {
                        const wantId = want.metadata?.id || want.id;
                        return (
                          <div
                            key={wantId}
                            onClick={() => handleViewWant(want)}
                            className="p-2 bg-green-50 border border-green-200 rounded hover:bg-green-100 cursor-pointer transition-colors text-center"
                            title={want.metadata?.name || wantId}
                          >
                            <p className="text-xs font-medium text-gray-900 truncate">
                              {want.metadata?.name || wantId}
                            </p>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                )}

                {labelOwners.length === 0 && labelUsers.length === 0 && (
                  <p className="text-sm text-gray-500">No owners or users found for this label</p>
                )}
              </div>
            )}

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
        </RightSidebar>
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