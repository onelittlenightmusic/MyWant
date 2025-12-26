import { useState, useCallback } from 'react';

export type SidebarType = 'summary' | 'details' | 'form' | null;

export interface UseRightSidebarExclusivityReturn<T> {
  // State
  showSummary: boolean;
  selectedItem: T | null;
  showForm: boolean;
  activeSidebar: SidebarType;

  // Actions
  openSummary: () => void;
  closeSummary: () => void;
  toggleSummary: () => void;
  selectItem: (item: T | null) => void;
  clearSelection: () => void;
  openForm: () => void;
  closeForm: () => void;
  closeAll: () => void;

  // Header actions (for Details sidebar)
  toggleHeaderAction?: ((action: 'refresh' | 'autoRefresh') => void) | null;
  registerHeaderActions?: (handlers: { handleRefresh: () => void; handleToggleAutoRefresh: () => void }) => void;
}

/**
 * Hook for managing mutually exclusive RightSidebar instances
 * Ensures only one of Summary, Details, or Form sidebars is visible at a time
 *
 * @template T - Type of item in Details sidebar
 * @returns {UseRightSidebarExclusivityReturn<T>} Sidebar state and control methods
 *
 * @example
 * const sidebar = useRightSidebarExclusivity<Want>();
 *
 * // Open Summary (auto-closes Details and Form)
 * sidebar.openSummary();
 *
 * // Select item for Details (auto-closes Summary and Form)
 * sidebar.selectItem(want);
 *
 * // Open Form (auto-closes Summary and Details)
 * sidebar.openForm();
 */
export function useRightSidebarExclusivity<T = any>(): UseRightSidebarExclusivityReturn<T> {
  const [activeSidebar, setActiveSidebar] = useState<SidebarType>(null);
  const [selectedItem, setSelectedItem] = useState<T | null>(null);
  const [headerActionHandlers, setHeaderActionHandlers] = useState<{ handleRefresh: () => void; handleToggleAutoRefresh: () => void } | null>(null);

  // Computed states for easier usage
  const showSummary = activeSidebar === 'summary';
  const showForm = activeSidebar === 'form';

  /**
   * Open Summary sidebar
   * Automatically closes Details and Form sidebars
   */
  const openSummary = useCallback(() => {
    setActiveSidebar('summary');
    setSelectedItem(null);
  }, []);

  /**
   * Close Summary sidebar
   * Only closes if Summary is currently active
   */
  const closeSummary = useCallback(() => {
    if (activeSidebar === 'summary') {
      setActiveSidebar(null);
    }
  }, [activeSidebar]);

  /**
   * Toggle Summary sidebar
   * If Summary is open, closes it
   * If any other sidebar is open or all are closed, opens Summary
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
   * Automatically closes Summary and Form sidebars
   * Pass null to deselect and close Details sidebar
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
   * Only closes Details if it's currently active
   */
  const clearSelection = useCallback(() => {
    setSelectedItem(null);
    if (activeSidebar === 'details') {
      setActiveSidebar(null);
    }
  }, [activeSidebar]);

  /**
   * Open Form sidebar (for Create/Edit operations)
   * Automatically closes Summary and Details sidebars
   */
  const openForm = useCallback(() => {
    setActiveSidebar('form');
    setSelectedItem(null);
  }, []);

  /**
   * Close Form sidebar
   * Only closes if Form is currently active
   */
  const closeForm = useCallback(() => {
    if (activeSidebar === 'form') {
      setActiveSidebar(null);
    }
  }, [activeSidebar]);

  /**
   * Close all sidebars
   * Clears both active sidebar and selected item
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
    activeSidebar,

    // Actions
    openSummary,
    closeSummary,
    toggleSummary,
    selectItem,
    clearSelection,
    openForm,
    closeForm,
    closeAll,

    // Header actions
    toggleHeaderAction,
    registerHeaderActions,
  };
}
