import React, { useState, useEffect, useRef, useMemo } from 'react';
import { RefreshCw, Download, Upload, ChevronDown } from 'lucide-react';
import { WantExecutionStatus, Want } from '@/types/want';
import { useWantStore } from '@/stores/wantStore';
import { usePolling } from '@/hooks/usePolling';
import { useHierarchicalKeyboardNavigation } from '@/hooks/useHierarchicalKeyboardNavigation';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { useRightSidebarExclusivity } from '@/hooks/useRightSidebarExclusivity';
import { StatusBadge } from '@/components/common/StatusBadge';
import { classNames, truncateText } from '@/utils/helpers';
import { addLabelToRegistry } from '@/utils/labelUtils';
import { apiClient } from '@/api/client';

// Components
import { Layout } from '@/components/layout/Layout';
import { Header } from '@/components/layout/Header';
import { RightSidebar } from '@/components/layout/RightSidebar';
import { StatsOverview } from '@/components/dashboard/StatsOverview';
import { WantFilters } from '@/components/dashboard/WantFilters';
import { WantGrid } from '@/components/dashboard/WantGrid';
import { WantForm } from '@/components/forms/WantForm';
import { WantDetailsSidebar } from '@/components/sidebar/WantDetailsSidebar';
import { WantBatchControlPanel } from '@/components/dashboard/WantBatchControlPanel';
import { ConfirmationMessageNotification } from '@/components/common/ConfirmationMessageNotification';
import { MessageNotification } from '@/components/common/MessageNotification';
import { DragOverlay } from '@/components/dashboard/DragOverlay';

export const Dashboard: React.FC = () => {
  const {
    wants,
    loading,
    error,
    fetchWants,
    deleteWant,
    deleteWants,
    suspendWant,
    resumeWant,
    stopWant,
    startWant,
    suspendWants,
    resumeWants,
    stopWants,
    startWants,
    clearError
  } = useWantStore();

  // UI State
  const sidebar = useRightSidebarExclusivity<Want>();
  const [editingWant, setEditingWant] = useState<Want | null>(null);
  const [lastSelectedWantId, setLastSelectedWantId] = useState<string | null>(null);
  const [deleteWantState, setDeleteWantState] = useState<Want | null>(null);
  const [showDeleteConfirmation, setShowDeleteConfirmation] = useState(false);
  const [isDeletingWant, setIsDeletingWant] = useState(false);

  // Batch action confirmation state
  const [showBatchConfirmation, setShowBatchConfirmation] = useState(false);
  const [batchAction, setBatchAction] = useState<'start' | 'stop' | 'delete' | null>(null);
  const [isBatchProcessing, setIsBatchProcessing] = useState(false);

  // Reminder reaction (approve/deny) confirmation state
  const [reactionWantState, setReactionWantState] = useState<Want | null>(null);
  const [showReactionConfirmation, setShowReactionConfirmation] = useState(false);
  const [reactionAction, setReactionAction] = useState<'approve' | 'deny' | null>(null);
  const [isSubmittingReaction, setIsSubmittingReaction] = useState(false);

  const [sidebarMinimized, setSidebarMinimized] = useState(true); // Start minimized
  const [sidebarInitialTab, setSidebarInitialTab] = useState<'settings' | 'results' | 'logs' | 'agents'>('settings');
  const [expandedParents, setExpandedParents] = useState<Set<string>>(new Set());
  const [showAddLabelForm, setShowAddLabelForm] = useState(false);
  const [newLabel, setNewLabel] = useState<{ key: string; value: string }>({ key: '', value: '' });
  const [selectedLabel, setSelectedLabel] = useState<{ key: string; value: string } | null>(null);
  const [expandedLabels, setExpandedLabels] = useState(false);
  const [labelOwners, setLabelOwners] = useState<Want[]>([]);
  const [labelUsers, setLabelUsers] = useState<Want[]>([]);
  const [allLabels, setAllLabels] = useState<Map<string, Set<string>>>(new Map());

  // Multi-select mode state
  const [isSelectMode, setIsSelectMode] = useState(false);
  const [selectedWantIds, setSelectedWantIds] = useState<Set<string>>(new Set());

  // Export/Import state
  const [isExporting, setIsExporting] = useState(false);
  const [isImporting, setIsImporting] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Message notification state
  const [notificationMessage, setNotificationMessage] = useState<string | null>(null);
  const [isNotificationVisible, setIsNotificationVisible] = useState(false);

  // Helper function to show notification message
  const showNotification = (message: string) => {
    setNotificationMessage(message);
    setIsNotificationVisible(true);
  };

  const dismissNotification = () => {
    setIsNotificationVisible(false);
    setNotificationMessage(null);
  };

  // Use sidebar.selectedItem to get the ID, but find the latest version from the wants list
  // This ensures the sidebar reflects live status updates from polling
  const selectedWant = useMemo(() => {
    if (!sidebar.selectedItem) return null;
    const wantId = sidebar.selectedItem.metadata?.id || sidebar.selectedItem.id;
    return wants.find(w => (w.metadata?.id === wantId) || (w.id === wantId)) || sidebar.selectedItem;
  }, [sidebar.selectedItem, wants]);

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

  // Auto-refresh wants every 2 seconds (only if enabled)
  usePolling(
    () => {
      if (wants.length > 0) {
        fetchWants();
      }
      // Also refresh labels in case new labels were added
      fetchLabels();
    },
    {
      interval: 2000,
      enabled: headerState?.autoRefresh ?? false,
      immediate: false
    }
  );

  // Clear selection if selected want was deleted
  useEffect(() => {
    if (sidebar.selectedItem) {
      const wantId = sidebar.selectedItem.metadata?.id || sidebar.selectedItem.id;
      const stillExists = wants.some(w =>
        (w.metadata?.id === wantId) || (w.id === wantId)
      );

      // Only clear selection if the want was actually deleted
      if (!stillExists) {
        sidebar.clearSelection();
      }
    }
  }, [wants, sidebar.selectedItem]);

  // Clear errors after 5 seconds
  useEffect(() => {
    if (error) {
      const timer = setTimeout(() => {
        clearError();
      }, 5000);
      return () => clearTimeout(timer);
    }
  }, [error, clearError]);

  // Multi-select handlers
  const handleToggleSelectMode = () => {
    if (isSelectMode) {
      // Exiting select mode - clear selection and close batch sidebar
      setSelectedWantIds(new Set());
      sidebar.closeBatch();
      setIsSelectMode(false);
    } else {
      // Entering select mode - don't open batch sidebar yet
      setIsSelectMode(true);
    }
  };

  const handleSelectWant = (wantId: string) => {
    setLastSelectedWantId(wantId);
    setSelectedWantIds(prev => {
      const newSet = new Set(prev);
      if (newSet.has(wantId)) {
        newSet.delete(wantId);
        // If no wants are selected, close batch sidebar
        if (newSet.size === 0) {
          sidebar.closeBatch();
        }
      } else {
        newSet.add(wantId);
        // Open batch sidebar when first want is selected
        if (newSet.size === 1) {
          sidebar.openBatch();
        }
      }
      return newSet;
    });
  };

  const handleBatchStart = () => {
    setBatchAction('start');
    setShowBatchConfirmation(true);
  };

  const handleBatchStop = () => {
    setBatchAction('stop');
    setShowBatchConfirmation(true);
  };

  const handleBatchDelete = () => {
    setBatchAction('delete');
    setShowBatchConfirmation(true);
  };

  const handleBatchConfirm = async () => {
    if (!batchAction || selectedWantIds.size === 0) return;
    
    setIsBatchProcessing(true);
    const ids = Array.from(selectedWantIds);
    try {
      if (batchAction === 'start') {
        await startWants(ids);
        showNotification(`Started ${selectedWantIds.size} wants`);
      } else if (batchAction === 'stop') {
        await stopWants(ids);
        showNotification(`Stopped ${selectedWantIds.size} wants`);
      } else if (batchAction === 'delete') {
        await deleteWants(ids);
        showNotification(`Deleted ${selectedWantIds.size} wants`);
        setSelectedWantIds(new Set()); // Clear selection after delete
        sidebar.closeBatch();
      }
      setShowBatchConfirmation(false);
      setBatchAction(null);
    } catch (error) {
      console.error(`Batch ${batchAction} failed:`, error);
      showNotification(`Failed to ${batchAction} some wants`);
    } finally {
      setIsBatchProcessing(false);
    }
  };

  const handleBatchCancel = () => {
    setShowBatchConfirmation(false);
    setBatchAction(null);
  };

  // Handlers
  const handleCreateWant = () => {
    setEditingWant(null);
    sidebar.openForm();
  };

  const handleEditWant = (want: Want) => {
    setEditingWant(want);
    sidebar.openForm();
  };

  const handleViewWant = (want: Want) => {
    sidebar.selectItem(want);
    setSidebarInitialTab('settings');
    const wantId = want.metadata?.id || want.id;
    if (wantId) setLastSelectedWantId(wantId);
  };

  const handleViewAgents = (want: Want) => {
    sidebar.selectItem(want);
    setSidebarInitialTab('agents');
    const wantId = want.metadata?.id || want.id;
    if (wantId) setLastSelectedWantId(wantId);
  };

  const handleViewResults = (want: Want) => {
    sidebar.selectItem(want);
    setSidebarInitialTab('results');
    const wantId = want.metadata?.id || want.id;
    if (wantId) setLastSelectedWantId(wantId);
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
        setIsDeletingWant(true);
        const wantId = deleteWantState.metadata?.id || deleteWantState.id;
        if (!wantId) {
          console.error('No want ID found for deletion');
          return;
        }
        await deleteWant(wantId);
        setShowDeleteConfirmation(false);
        setDeleteWantState(null);

        // Close the details sidebar if the deleted want is currently selected
        if (selectedWant && (selectedWant.metadata?.id === wantId || selectedWant.id === wantId)) {
          sidebar.clearSelection();
        }
      } catch (error) {
        console.error('Failed to delete want:', error);
      } finally {
        setIsDeletingWant(false);
      }
    }
  };

  const handleDeleteWantCancel = () => {
    setShowDeleteConfirmation(false);
    setDeleteWantState(null);
  };

  const handleShowDeleteConfirmation = (want: Want) => {
    setDeleteWantState(want);
    setShowDeleteConfirmation(true);
  };

  const handleShowReactionConfirmation = (want: Want, action: 'approve' | 'deny') => {
    setReactionWantState(want);
    setReactionAction(action);
    setShowReactionConfirmation(true);
  };

  const handleReactionConfirm = async () => {
    if (!reactionWantState || !reactionAction) return;

    setIsSubmittingReaction(true);
    try {
      const queueId = reactionWantState.state?.reaction_queue_id as string | undefined;
      console.log('[DASHBOARD] Reaction submission starting...');
      console.log('[DASHBOARD] Want state:', reactionWantState.state);
      console.log('[DASHBOARD] Queue ID:', queueId);

      if (!queueId) {
        console.error('[DASHBOARD] No reaction queue ID found');
        return;
      }

      const requestBody = {
        approved: reactionAction === 'approve',
        comment: `User ${reactionAction === 'approve' ? 'approved' : 'denied'} reminder`
      };
      console.log('[DASHBOARD] Request body:', requestBody);

      const url = `/api/v1/reactions/${queueId}`;
      console.log('[DASHBOARD] Sending PUT request to:', url);

      const response = await fetch(url, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify(requestBody)
      });

      console.log('[DASHBOARD] Response status:', response.status);
      console.log('[DASHBOARD] Response ok:', response.ok);

      if (!response.ok) {
        const errorText = await response.text();
        console.error('[DASHBOARD] Error response:', errorText);
        throw new Error(`Failed to submit reaction: ${response.statusText}`);
      }

      const responseData = await response.json();
      console.log('[DASHBOARD] Response data:', responseData);

      setShowReactionConfirmation(false);
      setReactionWantState(null);
      setReactionAction(null);
      console.log(`Reminder ${reactionAction === 'approve' ? 'approved' : 'denied'} successfully`);
    } catch (error) {
      console.error('Error submitting reaction:', error);
    } finally {
      setIsSubmittingReaction(false);
    }
  };

  const handleReactionCancel = () => {
    setShowReactionConfirmation(false);
    setReactionWantState(null);
    setReactionAction(null);
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

  const handleSaveRecipeFromWant = async (want: Want) => {
    const wantId = want.metadata?.id || want.id;
    if (!wantId) return;

    try {
      const result = await apiClient.saveRecipeFromWant(wantId, {
        name: `${want.metadata.name}-recipe`,
        description: `Recipe saved from ${want.metadata.name}`,
        version: '1.0.0'
      });
      showNotification(`✓ Recipe '${result.id}' saved successfully with ${result.wants} children`);
    } catch (error) {
      console.error('Failed to save recipe:', error);
      showNotification(`✗ Failed to save recipe: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  };

  const handleCloseModals = () => {
    sidebar.closeForm();
    setEditingWant(null);
    setDeleteWantState(null);
    setShowDeleteConfirmation(false);
    setReactionWantState(null);
    setShowReactionConfirmation(false);
    setReactionAction(null);
  };

  // Export wants as YAML
  const handleExportWants = async () => {
    setIsExporting(true);
    try {
      const response = await fetch('http://localhost:8080/api/v1/wants/export', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
      });

      if (!response.ok) {
        throw new Error(`Export failed: ${response.statusText}`);
      }

      // Get the filename from Content-Disposition header or use default
      const contentDisposition = response.headers.get('Content-Disposition');
      let filename = 'wants-export.yaml';
      if (contentDisposition) {
        const match = contentDisposition.match(/filename="?([^";\n]+)"?/);
        if (match) filename = match[1];
      }

      // Create blob from response and download
      const blob = await response.blob();
      const url = window.URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = filename;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      window.URL.revokeObjectURL(url);
      showNotification('✓ Exported wants to YAML');
    } catch (error) {
      console.error('Failed to export wants:', error);
      showNotification(`✗ Export failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    } finally {
      setIsExporting(false);
    }
  };

  // Import wants from YAML
  const handleImportWants = async (file: File) => {
    setIsImporting(true);
    try {
      const formData = new FormData();
      formData.append('file', file);

      // Read file as text for YAML import
      const fileText = await file.text();

      const response = await fetch('http://localhost:8080/api/v1/wants/import', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/yaml',
        },
        body: fileText,
      });

      if (!response.ok) {
        const errorData = await response.text();
        throw new Error(`Import failed: ${errorData || response.statusText}`);
      }

      const result = await response.json();
      showNotification(`✓ Imported ${result.wants} want(s)`);

      // Refresh wants list
      fetchWants();

      // Clear file input
      if (fileInputRef.current) {
        fileInputRef.current.value = '';
      }
    } catch (error) {
      console.error('Failed to import wants:', error);
      showNotification(`✗ Import failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    } finally {
      setIsImporting(false);
    }
  };

  // Handle file input change
  const handleFileInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (file) {
      // Validate file extension
      if (!file.name.endsWith('.yaml') && !file.name.endsWith('.yml')) {
        alert('Please select a YAML file (.yaml or .yml)');
        return;
      }
      handleImportWants(file);
    }
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
    const want = wants.find(w => (w.metadata?.id === wantId) || (w.id === wantId));
    if (want) {
      sidebar.selectItem(want);
      setSidebarInitialTab('settings');
    }
  };

  // Handler for when a want is dropped on a target want
  const handleWantDropped = async (draggedWantId: string, targetWantId: string) => {
    try {
      const draggedWant = wants.find(w => (w.metadata?.id === draggedWantId) || (w.id === draggedWantId));
      const targetWant = wants.find(w => (w.metadata?.id === targetWantId) || (w.id === targetWantId));

      if (!draggedWant || !targetWant) {
        showNotification('Want not found');
        return;
      }

      // Check if already a child
      const isAlreadyChild = draggedWant.metadata.ownerReferences?.some(ref => ref.id === targetWantId);
      if (isAlreadyChild) {
        showNotification(`${draggedWant.metadata.name} is already a child of ${targetWant.metadata.name}`);
        return;
      }

      // Add owner reference
      const ownerRef = {
        apiVersion: 'mywant/v1',
        kind: 'Want',
        name: targetWant.metadata.name,
        id: targetWantId,
        controller: true,
        blockOwnerDeletion: true
      };

      const updatedWant = {
        ...draggedWant,
        metadata: {
          ...draggedWant.metadata,
          ownerReferences: [
            ...(draggedWant.metadata.ownerReferences || []),
            ownerRef
          ]
        }
      };

      await apiClient.updateWant(draggedWantId, updatedWant);
      showNotification(`✓ Added ${draggedWant.metadata.name} to ${targetWant.metadata.name}`);
      
      // Refresh wants list
      await fetchWants();
      
      // Auto-expand the parent to show the new child
      handleToggleExpand(targetWantId);
    } catch (error) {
      console.error('Failed to update want owner:', error);
      showNotification(`✗ Failed to add child want: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  };

  // Use hierarchical keyboard navigation hook
  useHierarchicalKeyboardNavigation({
    items: hierarchicalWants,
    currentItem: currentHierarchicalWant,
    onNavigate: handleHierarchicalNavigate,
    onToggleExpand: handleToggleExpand,
    onSelect: isSelectMode ? handleSelectWant : undefined,
    expandedItems: expandedParents,
    lastSelectedItemId: lastSelectedWantId,
    enabled: !sidebar.showForm && filteredWants.length > 0 // Disable when form is open
  });

  // Handle ESC key to close any open sidebar
  const handleEscapeKey = () => {
    if (showBatchConfirmation) {
      handleBatchCancel();
    } else if (selectedWant) {
      // Remember the last selected want before deselecting
      const wantId = selectedWant.metadata?.id || selectedWant.id;
      if (wantId) {
        setLastSelectedWantId(wantId);
      }
      sidebar.clearSelection();
    } else if (sidebar.showSummary) {
      sidebar.closeSummary();
    } else if (sidebar.showForm) {
      sidebar.closeForm();
    } else if (isSelectMode) {
      // Exit select mode
      sidebar.closeBatch();
      setSelectedWantIds(new Set());
      setIsSelectMode(false);
    }
  };

  useEscapeKey({
    onEscape: handleEscapeKey,
    enabled: !!selectedWant || sidebar.showSummary || sidebar.showForm || isSelectMode
  });

  // Keyboard shortcuts: a (add), s (summary), Shift+S (select)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Don't intercept if user is typing in an input
      const target = e.target as HTMLElement;
      const isInputElement =
        target.tagName === 'INPUT' ||
        target.tagName === 'TEXTAREA' ||
        target.isContentEditable;

      if (isInputElement) return;

      // Handle shortcuts
      if (e.key === 'a' && !e.shiftKey && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault();
        handleCreateWant();
      } else if (e.key === 's' && !e.shiftKey && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault();
        sidebar.toggleSummary();
      } else if (e.key === 'S' && e.shiftKey && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault();
        handleToggleSelectMode();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [handleCreateWant, handleToggleSelectMode, sidebar]);

  // Determine background style for want details sidebar
  // Maps want types to background images, ensuring each type has a consistent visual
  const getWantBackgroundImage = (type?: string) => {
    if (!type) return undefined;

    const lowerType = type.toLowerCase();

    // Travel types - always show background
    if (lowerType === 'flight') return '/resources/flight.png';
    if (lowerType === 'hotel') return '/resources/hotel.png';
    if (lowerType === 'restaurant') return '/resources/restaurant.png';
    if (lowerType === 'buffet') return '/resources/buffet.png';

    // Evidence type
    if (lowerType === 'evidence') return '/resources/evidence.png';

    // Coordinator types - apply agent background
    if (lowerType.includes('coordinator')) return '/resources/agent.png';

    // Mathematics types - apply numbers background
    if (
      lowerType.includes('prime') ||
      lowerType.includes('fibonacci') ||
      lowerType.includes('numbers')
    ) {
      return '/resources/numbers.png';
    }

    // System/Execution category - applies to scheduler, execution_result, and related types
    if (
      lowerType === 'scheduler' ||
      lowerType === 'execution_result' ||
      lowerType === 'execution result' ||
      lowerType === 'command execution' ||
      lowerType === 'command_execution' ||
      lowerType.includes('execution') ||
      lowerType.includes('scheduler')
    ) {
      return '/resources/screen.png';
    }

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
        onClick={() => sidebar.toggleHeaderAction?.('autoRefresh')}
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
        onClick={() => sidebar.toggleHeaderAction?.('refresh')}
        disabled={headerState.loading}
        className="p-2 text-gray-400 hover:text-gray-600 hover:bg-gray-100 rounded-md transition-colors disabled:opacity-50"
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
        showSummary={sidebar.showSummary}
        onSummaryToggle={sidebar.toggleSummary}
        sidebarMinimized={sidebarMinimized}
        showSelectMode={isSelectMode}
        onToggleSelectMode={handleToggleSelectMode}
      />

      {/* Main content area with sidebar-aware layout */}
      <main className="flex-1 flex overflow-hidden bg-gray-50 mt-16 mr-[480px] relative">
        {/* Confirmation Notification - Dashboard Right Layout */}
        {(showDeleteConfirmation || showReactionConfirmation || showBatchConfirmation) && (
          <ConfirmationMessageNotification
            message={
              showDeleteConfirmation
                ? (deleteWantState ? `Delete: ${deleteWantState.metadata?.name || deleteWantState.metadata?.id || deleteWantState.id}` : null)
                : showBatchConfirmation
                ? `${batchAction === 'delete' ? 'Delete' : batchAction === 'start' ? 'Start' : 'Stop'} ${selectedWantIds.size} wants?`
                : (reactionWantState ? `${reactionAction === 'approve' ? 'Approve' : 'Deny'}: ${reactionWantState.metadata?.name || reactionWantState.metadata?.id || reactionWantState.id}` : null)
            }
            isVisible={showDeleteConfirmation || showReactionConfirmation || showBatchConfirmation}
            onDismiss={() => {
              setShowDeleteConfirmation(false);
              setShowReactionConfirmation(false);
              setShowBatchConfirmation(false);
            }}
            onConfirm={
              showDeleteConfirmation 
                ? handleDeleteWantConfirm 
                : showBatchConfirmation 
                ? handleBatchConfirm 
                : handleReactionConfirm
            }
            onCancel={
              showDeleteConfirmation 
                ? handleDeleteWantCancel 
                : showBatchConfirmation 
                ? handleBatchCancel 
                : handleReactionCancel
            }
            loading={isDeletingWant || isSubmittingReaction || isBatchProcessing}
            title={showDeleteConfirmation ? "Delete Want" : showBatchConfirmation ? `Batch ${batchAction}` : "Confirm"}
            layout="dashboard-right"
          />
        )}

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
                onViewResultsWant={handleViewResults}
                onEditWant={handleEditWant}
                onDeleteWant={handleShowDeleteConfirmation}
                onSuspendWant={handleSuspendWant}
                onResumeWant={handleResumeWant}
                onGetFilteredWants={setFilteredWants}
                expandedParents={expandedParents}
                onToggleExpand={handleToggleExpand}
                onCreateWant={handleCreateWant}
                onLabelDropped={handleLabelDropped}
                onWantDropped={handleWantDropped}
                onShowReactionConfirmation={handleShowReactionConfirmation}
                isSelectMode={isSelectMode}
                selectedWantIds={selectedWantIds}
                onSelectWant={handleSelectWant}
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

              <div
                className={classNames(
                  'overflow-hidden transition-all duration-300 ease-in-out',
                  expandedLabels ? 'max-h-none' : 'max-h-[200px]'
                )}
              >
                {allLabels.size === 0 ? (
                  <p className="text-sm text-gray-500">No labels found</p>
                ) : (
                  <div className={classNames(
                    'flex flex-wrap gap-2',
                    !expandedLabels && 'overflow-y-auto pr-2'
                  )}>
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

              {/* Expand/Collapse button below labels */}
              {allLabels.size > 0 && (
                <div className="flex justify-center mt-3 w-full">
                  <button
                    onClick={() => setExpandedLabels(!expandedLabels)}
                    className="w-full flex justify-center py-2 px-4 rounded-lg text-gray-400 hover:text-gray-600 hover:bg-gray-100 transition-all"
                    title={expandedLabels ? "Collapse labels" : "Expand labels"}
                  >
                    <ChevronDown
                      className={classNames(
                        'w-4 h-4 transition-transform',
                        expandedLabels && 'rotate-180'
                      )}
                    />
                  </button>
                </div>
              )}
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

              {/* Export and Import buttons */}
              <div className="mt-6 flex gap-3">
                {/* Export button */}
                <button
                  onClick={handleExportWants}
                  disabled={isExporting || wants.length === 0}
                  className="flex-1 flex items-center justify-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:bg-gray-400 disabled:cursor-not-allowed transition-colors"
                  title={wants.length === 0 ? 'No wants to export' : 'Download all wants as YAML'}
                >
                  <Download className="h-4 w-4" />
                  <span>{isExporting ? 'Exporting...' : 'Export'}</span>
                </button>

                {/* Import button */}
                <button
                  onClick={() => fileInputRef.current?.click()}
                  disabled={isImporting}
                  className="flex-1 flex items-center justify-center gap-2 px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 disabled:bg-gray-400 disabled:cursor-not-allowed transition-colors"
                  title="Upload YAML file to import wants"
                >
                  <Upload className="h-4 w-4" />
                  <span>{isImporting ? 'Importing...' : 'Import'}</span>
                </button>

                {/* Hidden file input */}
                <input
                  ref={fileInputRef}
                  type="file"
                  accept=".yaml,.yml"
                  onChange={handleFileInputChange}
                  className="hidden"
                  disabled={isImporting}
                />
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

      {/* Right Sidebar for Want Details or Batch Control */}
      <RightSidebar
        isOpen={!!selectedWant || sidebar.showBatch}
        onClose={() => {
          if (isSelectMode) {
            // In select mode, closing sidebar exits select mode
            sidebar.closeBatch();
            setSelectedWantIds(new Set());
          } else {
            sidebar.clearSelection();
          }
        }}
        title={
          isSelectMode 
            ? 'Batch Actions' 
            : (selectedWant ? (selectedWant.metadata?.name || selectedWant.metadata?.id || 'Want Details') : undefined)
        }
        backgroundStyle={!isSelectMode ? sidebarBackgroundStyle : undefined}
        headerActions={!isSelectMode ? headerActions : undefined}
      >
        {isSelectMode ? (
          <WantBatchControlPanel 
            selectedCount={selectedWantIds.size}
            onBatchStart={handleBatchStart}
            onBatchStop={handleBatchStop}
            onBatchDelete={handleBatchDelete}
            onBatchCancel={handleToggleSelectMode}
            loading={isBatchProcessing}
          />
        ) : (
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
            onRegisterHeaderActions={sidebar.registerHeaderActions}
            onStart={handleStartWant}
            onStop={handleStopWant}
            onSuspend={handleSuspendWant}
            onResume={handleResumeWant}
            onDelete={handleShowDeleteConfirmation}
            onSaveRecipe={handleSaveRecipeFromWant}
          />
        )}
      </RightSidebar>

      {/* Modals */}
      <WantForm
        isOpen={sidebar.showForm}
        onClose={handleCloseModals}
        editingWant={editingWant}
      />

                  {/* Message Notification */}

                  <MessageNotification

                    message={notificationMessage}

                    isVisible={isNotificationVisible}

                    onDismiss={dismissNotification}

                  />

                </Layout>

              );

            };

            

      