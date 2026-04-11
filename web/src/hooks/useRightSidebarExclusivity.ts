import { useState, useCallback } from 'react';

export type SidebarType = 'summary' | 'details' | 'form' | 'batch' | 'memo' | null;

export interface UseRightSidebarExclusivityReturn<T> {
  // State
  showSummary: boolean;
  selectedItem: T | null;
  showForm: boolean;
  showBatch: boolean;
  showMemo: boolean;
  activeSidebar: SidebarType;

  // Actions
  openSummary: () => void;
  closeSummary: () => void;
  toggleSummary: () => void;
  selectItem: (item: T | null) => void;
  clearSelection: () => void;
  openForm: () => void;
  closeForm: () => void;
  toggleForm: () => void;
  openBatch: () => void;
  closeBatch: () => void;
  openMemo: () => void;
  closeMemo: () => void;
  toggleMemo: () => void;
  closeAll: () => void;

  // Header actions (for Details sidebar)
  toggleHeaderAction?: ((action: 'refresh' | 'autoRefresh') => void) | null;
  registerHeaderActions?: (handlers: { handleRefresh: () => void; handleToggleAutoRefresh: () => void }) => void;
}

/**
 * Hook for managing mutually exclusive RightSidebar instances
 * Ensures only one of Summary, Details, Form, Batch, or Memo sidebars is visible at a time
 */
export function useRightSidebarExclusivity<T = any>(): UseRightSidebarExclusivityReturn<T> {
  const [activeSidebar, setActiveSidebar] = useState<SidebarType>(null);
  const [selectedItem, setSelectedItem] = useState<T | null>(null);
  const [headerActionHandlers, setHeaderActionHandlers] = useState<{ handleRefresh: () => void; handleToggleAutoRefresh: () => void } | null>(null);

  // Computed states for easier usage
  const showSummary = activeSidebar === 'summary';
  const showForm = activeSidebar === 'form';
  const showBatch = activeSidebar === 'batch';
  const showMemo = activeSidebar === 'memo';

  /**
   * Open Summary sidebar
   */
  const openSummary = useCallback(() => {
    setActiveSidebar('summary');
    setSelectedItem(null);
  }, []);

  /**
   * Close Summary sidebar
   */
  const closeSummary = useCallback(() => {
    if (activeSidebar === 'summary') {
      setActiveSidebar(null);
    }
  }, [activeSidebar]);

  /**
   * Toggle Summary sidebar
   */
  const toggleSummary = useCallback(() => {
    if (activeSidebar === 'summary') {
      setActiveSidebar(null);
    } else {
      setActiveSidebar('summary');
      setSelectedItem(null);
    }
  }, [activeSidebar]);

  /**
   * Select an item to display in Details sidebar
   */
  const selectItem = useCallback(
    (item: T | null) => {
      setSelectedItem(item);
      if (item) {
        setActiveSidebar('details');
      } else {
        if (activeSidebar === 'details') {
          setActiveSidebar(null);
        }
      }
    },
    [activeSidebar]
  );

  /**
   * Clear selected item and close Details sidebar
   */
  const clearSelection = useCallback(() => {
    setSelectedItem(null);
    if (activeSidebar === 'details') {
      setActiveSidebar(null);
    }
  }, [activeSidebar]);

  /**
   * Open Form sidebar
   */
  const openForm = useCallback(() => {
    setActiveSidebar('form');
    setSelectedItem(null);
  }, []);

  /**
   * Close Form sidebar
   */
  const closeForm = useCallback(() => {
    if (activeSidebar === 'form') {
      setActiveSidebar(null);
    }
  }, [activeSidebar]);

  /**
   * Toggle Form sidebar
   */
  const toggleForm = useCallback(() => {
    if (activeSidebar === 'form') {
      setActiveSidebar(null);
    } else {
      setActiveSidebar('form');
      setSelectedItem(null);
    }
  }, [activeSidebar]);

  /**
   * Open Batch sidebar
   */
  const openBatch = useCallback(() => {
    setActiveSidebar('batch');
    setSelectedItem(null);
  }, []);

  /**
   * Close Batch sidebar
   */
  const closeBatch = useCallback(() => {
    if (activeSidebar === 'batch') {
      setActiveSidebar(null);
    }
  }, [activeSidebar]);

  /**
   * Open Memo sidebar
   */
  const openMemo = useCallback(() => {
    setActiveSidebar('memo');
    setSelectedItem(null);
  }, []);

  /**
   * Close Memo sidebar
   */
  const closeMemo = useCallback(() => {
    if (activeSidebar === 'memo') {
      setActiveSidebar(null);
    }
  }, [activeSidebar]);

  /**
   * Toggle Memo sidebar
   */
  const toggleMemo = useCallback(() => {
    if (activeSidebar === 'memo') {
      setActiveSidebar(null);
    } else {
      setActiveSidebar('memo');
      setSelectedItem(null);
    }
  }, [activeSidebar]);

  /**
   * Close all sidebars
   */
  const closeAll = useCallback(() => {
    setActiveSidebar(null);
    setSelectedItem(null);
  }, []);

  /**
   * Register header action handlers from Details sidebar
   */
  const registerHeaderActions = useCallback((handlers: { handleRefresh: () => void; handleToggleAutoRefresh: () => void }) => {
    setHeaderActionHandlers(handlers);
  }, []);

  /**
   * Execute header actions registered from Details sidebar
   */
  const toggleHeaderAction = useCallback((action: 'refresh' | 'autoRefresh') => {
    if (!headerActionHandlers) return;

    if (action === 'refresh') {
      headerActionHandlers.handleRefresh();
    } else if (action === 'autoRefresh') {
      headerActionHandlers.handleToggleAutoRefresh();
    }
  }, [headerActionHandlers]);

  return {
    // State
    showSummary,
    selectedItem,
    showForm,
    showBatch,
    showMemo,
    activeSidebar,

    // Actions
    openSummary,
    closeSummary,
    toggleSummary,
    selectItem,
    clearSelection,
    openForm,
    closeForm,
    toggleForm,
    openBatch,
    closeBatch,
    openMemo,
    closeMemo,
    toggleMemo,
    closeAll,

    // Header actions
    toggleHeaderAction,
    registerHeaderActions,
  };
}
