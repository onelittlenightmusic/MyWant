import React, { useEffect, useState, useCallback, useMemo, useRef } from 'react';
import { Settings, Eye, AlertTriangle, Clock, Bot, Save, Edit, FileText, ChevronDown, ChevronRight, X, Database, Plus, BookOpen, Copy, Check, History, Eraser, MessageSquare, Send, Sparkles, ThumbsUp } from 'lucide-react';
import { Want, WantExecutionStatus, WhenSpec } from '@/types/want';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { ErrorDisplay } from '@/components/common/ErrorDisplay';
import { FormYamlToggle } from '@/components/common/FormYamlToggle';
import { YamlEditor } from '@/components/forms/YamlEditor';
import { LabelAutocomplete } from '@/components/forms/LabelAutocomplete';
import { LabelSelectorAutocomplete } from '@/components/forms/LabelSelectorAutocomplete';
import { useWantStore } from '@/stores/wantStore';
import { POLLING_INTERVAL_MS } from '@/constants/polling';
import { useConfigStore } from '@/stores/configStore';
import { formatDate, formatDuration, formatRelativeTime, classNames, truncateText } from '@/utils/helpers';
import { stringifyYaml, validateYaml, validateYamlWithSpec, WantTypeDefinition } from '@/utils/yaml';
import { updateWantParameters, updateWantScheduling, updateWantLabels, updateWantDependencies } from '@/utils/wantUtils';
import { WantControlButtons } from '@/components/dashboard/WantControlButtons';
import { WantCardContent } from '@/components/dashboard/WantCardContent';
import { ArrayResultTable } from '@/components/common/ArrayResultTable';
import { StatusBadge } from '@/components/common/StatusBadge';
import { ParametersSection } from '@/components/forms/sections/ParametersSection';
import { LabelsSection } from '@/components/forms/sections/LabelsSection';
import { DependenciesSection } from '@/components/forms/sections/DependenciesSection';
import { SchedulingSection } from '@/components/forms/sections/SchedulingSection';
import { SummarySidebarContent } from './SummarySidebarContent';
import { ConfirmationBubble } from '@/components/notifications';
import { apiClient } from '@/api/client';
import { Recommendation } from '@/types/interact';
import {
  DetailsSidebar,
  TabContent,
  TabSection,
  TabGrid,
  EmptyState,
  InfoRow,
  TabConfig
} from './DetailsSidebar';
import { useInputActions } from '@/hooks/useInputActions';

interface WantDetailsSidebarProps {
  want: Want | null;
  initialTab?: 'settings' | 'results' | 'logs' | 'agents' | 'versions' | 'chat';
  initialTabVersion?: number;
  seriesWants?: Want[]; // All wants in the same series (for Versions tab)
  onRecommendationSelect?: (rec: Recommendation) => void;
  onWantUpdate?: () => void;
  onHeaderStateChange?: (state: { autoRefresh: boolean; loading: boolean; status: WantExecutionStatus }) => void;
  onRegisterHeaderActions?: (handlers: { handleRefresh: () => void; handleToggleAutoRefresh: () => void }) => void;
  onStart?: (want: Want) => void;
  onStop?: (want: Want) => void;
  onSuspend?: (want: Want) => void;
  onResume?: (want: Want) => void;
  onDelete?: (want: Want) => void;
  onSaveRecipe?: (want: Want) => void;
  onTabChange?: (tab: 'settings' | 'results' | 'logs' | 'agents' | 'versions' | 'chat') => void;
  
  // Summary related props (added for non-want state)
  summaryProps?: {
    wants: Want[];
    loading: boolean;
    searchQuery: string;
    onSearchChange: (query: string) => void;
    statusFilters: any[];
    onStatusFilter: (filters: any[]) => void;
    allLabels: Map<string, Set<string>>;
    onLabelClick: (key: string, value: string) => void;
    selectedLabel: { key: string; value: string } | null;
    onClearSelectedLabel: () => void;
    labelOwners: Want[];
    labelUsers: Want[];
    onViewWant: (want: Want) => void;
    onExportWants: () => void;
    onImportWants: () => void;
    isExporting: boolean;
    isImporting: boolean;
    fetchLabels: () => Promise<void>;
    fetchWants: () => Promise<void>;
  };
}

type TabType = 'settings' | 'results' | 'logs' | 'agents' | 'versions' | 'chat';

// Unified section container styling for all metadata/state sections
const SECTION_CONTAINER_CLASS = 'border border-gray-200 dark:border-gray-700 rounded-lg bg-gray-100 dark:bg-gray-900 overflow-hidden p-2 sm:p-4';

export const WantDetailsSidebar: React.FC<WantDetailsSidebarProps> = ({
  want,
  initialTab = 'results',
  initialTabVersion = 0,
  onRecommendationSelect,
  onWantUpdate,
  onHeaderStateChange,
  onRegisterHeaderActions,
  onStart,
  onStop,
  onSuspend,
  onResume,
  onDelete,
  onSaveRecipe,
  onTabChange,
  summaryProps,
  seriesWants = [],
}) => {
  // Check if this is a flight want
  const isFlightWant = want?.metadata?.type === 'flight';

  const {
    wants: allWants,
    selectedWantDetails,
    selectedWantResults,
    fetchWantDetails,
    fetchWantResults,
    fetchWants,
    updateWant,
    loading
  } = useWantStore();

  // Identify if this want is a Target (can have children)
  const wantType = want?.metadata?.type?.toLowerCase() || '';
  const wantId = want?.metadata?.id || want?.id;
  const hasChildren = allWants.some(w =>
    w.metadata?.ownerReferences?.some(ref => ref.id === wantId)
  );
  const isTargetWant = wantType.includes('target') ||
                       wantType === 'owner' ||
                       wantType.includes('approval') ||
                       hasChildren;

  const [activeTab, setActiveTab] = useState<TabType>('results');
  const [prevTabIndex, setPrevTabIndex] = useState(0);
  const [isInitialLoad, setIsInitialLoad] = useState(true);
  const [autoRefresh, setAutoRefresh] = useState(false);
  const [isEditing, setIsEditing] = useState(false);
  const [showClearStateConfirmation, setShowClearStateConfirmation] = useState(false);
  const [editedConfig, setEditedConfig] = useState<string>('');
  const [updateLoading, setUpdateLoading] = useState(false);
  const [updateError, setUpdateError] = useState<string | null>(null);
  const [configMode, setConfigMode] = useState<'form' | 'yaml'>('form');

  // Memoize handlers to prevent recreation on every render
  const handleRefresh = useCallback(() => {
    if (want) {
      const wantId = want.metadata?.id || want.id;
      if (wantId) {
        fetchWantDetails(wantId);
        fetchWantResults(wantId);
        fetchWants();
      }
    }
  }, [want, fetchWantDetails, fetchWantResults, fetchWants]);

  const handleToggleAutoRefresh = useCallback(() => {
    setAutoRefresh(prev => !prev);
  }, []);

  const wantDetails = selectedWantDetails || want;

  // Control panel logic (use wantDetails for status since it includes updated state from polling)
  const isRunning = wantDetails?.status === 'reaching' || wantDetails?.status === 'reaching_with_warning';
  const isSuspended = wantDetails?.status === 'suspended';
  const isCompleted = wantDetails?.status === 'achieved' || wantDetails?.status === 'achieved_with_warning';
  const isStopped = wantDetails?.status === 'stopped' || wantDetails?.status === 'created' || wantDetails?.status === 'terminated';
  const isFailed = wantDetails?.status === 'failed' || wantDetails?.status === 'module_error' || wantDetails?.status === 'config_error';

  // Ensure wantDetails exists before checking control states
  const canStart = !!wantDetails && (isStopped || isCompleted || isFailed || isSuspended);
  const canStop = !!wantDetails && isRunning && !isSuspended;
  const canSuspend = !!wantDetails && isRunning && !isSuspended;
  const canDelete = !!wantDetails;
  const canSaveRecipe = !!wantDetails && isTargetWant;

  const handleStartClick = () => {
    if (wantDetails) {
      if (isSuspended && onResume) {
        onResume(wantDetails);
      } else if (canStart && onStart) {
        onStart(wantDetails);
      }
      // Immediately fetch updated details after starting execution
      if (wantId) {
        setTimeout(() => {
          fetchWantDetails(wantId);
          fetchWantResults(wantId);
        }, 100);
      }
    }
  };

  const handleStopClick = () => {
    if (wantDetails && canStop && onStop) onStop(wantDetails);
  };

  const handleSuspendClick = () => {
    if (wantDetails && canSuspend && onSuspend) onSuspend(wantDetails);
  };

  const handleDeleteClick = () => {
    if (wantDetails && canDelete && onDelete) onDelete(wantDetails);
  };

  const handleSaveRecipeClick = () => {
    if (wantDetails && canSaveRecipe && onSaveRecipe) onSaveRecipe(wantDetails);
  };

  const handleClearState = async () => {
    if (!wantId) return;
    try {
      await apiClient.clearWantState(wantId);
      await fetchWantDetails(wantId);
    } catch (e) {
      console.error('Failed to clear want state:', e);
    } finally {
      setShowClearStateConfirmation(false);
    }
  };

  // Fetch details when want ID changes (not on every want object change)
  useEffect(() => {
    if (wantId) {
      fetchWantDetails(wantId);
      fetchWantResults(wantId);
    }
  }, [wantId, fetchWantDetails, fetchWantResults]);

  // Reset state when initialTab prop changes (from parent handling onViewResults)
  // initialTabVersion ensures the effect fires even when the tab value is the same
  // wantId is included so that when a new want is selected with a specific initialTab, it applies correctly
  useEffect(() => {
    if (want) {
      setIsEditing(false);
      setUpdateError(null);
      setActiveTab(initialTab);
    }
  }, [initialTab, initialTabVersion, wantId]);

  // Keyboard shortcuts for want control buttons and tab switching
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Don't trigger if user is typing in an input/textarea
      const target = e.target as HTMLElement;
      const isInputElement =
        target.tagName === 'INPUT' ||
        target.tagName === 'TEXTAREA' ||
        target.isContentEditable;

      if (isInputElement) return;

      // Tab switching shortcut: when focus is on a want card, Tab cycles sidebar tabs
      if (e.key === 'Tab') {
        const isFocusOnCard = !!target.closest('[data-keyboard-nav-id]');
        if (isFocusOnCard) {
          e.preventDefault();
          const currentIndex = tabs.findIndex(t => t.id === activeTab);
          const nextIndex = (currentIndex + (e.shiftKey ? -1 : 1) + tabs.length) % tabs.length;
          handleTabChange(tabs[nextIndex].id);
          return;
        }
      }

      switch (e.key.toLowerCase()) {
        case 'd':
          // Delete button
          if (canDelete) {
            e.preventDefault();
            e.stopImmediatePropagation();
            handleDeleteClick();
          }
          break;
        case 's':
          // Start/Resume button (if available), otherwise Suspend button
          if (canStart) {
            e.preventDefault();
            e.stopImmediatePropagation();
            handleStartClick();
          } else if (canSuspend) {
            e.preventDefault();
            e.stopImmediatePropagation();
            handleSuspendClick();
          }
          break;
        case 'x':
          // Stop button (square icon - reset)
          if (canStop) {
            e.preventDefault();
            e.stopImmediatePropagation();
            handleStopClick();
          }
          break;
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [canStart, canStop, canSuspend, canDelete, handleStartClick, handleStopClick, handleSuspendClick, handleDeleteClick]);

  // Auto-enable refresh for running wants (but don't auto-disable - let user control it)
  // Only depends on status, not want object, to avoid infinite loops from polling
  useEffect(() => {
    if ((selectedWantDetails?.status === 'reaching' || selectedWantDetails?.status === 'reaching_with_warning') && !autoRefresh) {
      // Auto-enable only if currently disabled
      setAutoRefresh(true);
    }
  }, [selectedWantDetails?.status, autoRefresh]);

  // Auto-enable refresh when Chat tab is active (need live updates for conversation)
  useEffect(() => {
    if (activeTab === 'chat' && !autoRefresh) {
      setAutoRefresh(true);
    }
  }, [activeTab, autoRefresh]);

  // Auto refresh setup (only refresh specific want details, not the whole list)
  useEffect(() => {
    if (autoRefresh && wantId) {
      const interval = setInterval(async () => {
        const { updated } = await fetchWantDetails(wantId);
        // Only re-fetch results when the want actually changed (not a 304).
        // Results = GetAllState(), so they can't change without the want changing.
        if (updated) fetchWantResults(wantId);
      }, POLLING_INTERVAL_MS);

      return () => clearInterval(interval);
    }
  }, [autoRefresh, wantId, fetchWantDetails, fetchWantResults]);

  // Register header action handlers with the sidebar
  useEffect(() => {
    if (onRegisterHeaderActions) {
      onRegisterHeaderActions({
        handleRefresh,
        handleToggleAutoRefresh
      });
    }
  }, [onRegisterHeaderActions, handleRefresh, handleToggleAutoRefresh]);

  const handleEditConfig = () => {
    if (selectedWantDetails) {
      const yamlContent = stringifyYaml({
        metadata: selectedWantDetails.metadata,
        spec: selectedWantDetails.spec
      });
      setEditedConfig(yamlContent);
      setIsEditing(true);
    }
  };

  const handleSaveConfig = async () => {
    if (!want || !editedConfig || !selectedWantDetails) return;

    const wantId = want.metadata?.id || want.id;
    if (!wantId) return;

    setUpdateLoading(true);
    setUpdateError(null);

    try {
      // Get the want type
      const wantType = selectedWantDetails.metadata?.type;
      if (!wantType) {
        setUpdateError('Cannot determine want type');
        setUpdateLoading(false);
        return;
      }

      // Fetch want type specification from backend
      const specResponse = await fetch(`/api/v1/want-types/${wantType}`);
      let spec: WantTypeDefinition | undefined;

      if (specResponse.ok) {
        spec = await specResponse.json();
      }

      // Validate YAML against spec (or just basic validation if spec not available)
      const yamlValidation = spec
        ? validateYamlWithSpec(editedConfig, wantType, spec)
        : validateYaml(editedConfig);

      if (!yamlValidation.isValid) {
        setUpdateError(yamlValidation.error || 'Invalid YAML');
        setUpdateLoading(false);
        return;
      }

      // Use parsed YAML as update request
      const updateRequest = yamlValidation.data;
      await updateWant(wantId, updateRequest);
      setIsEditing(false);
      // Refresh details after update
      await fetchWantDetails(wantId);
      await fetchWants();
    } catch (error) {
      setUpdateError(error instanceof Error ? error.message : 'Failed to update want');
    } finally {
      setUpdateLoading(false);
    }
  };

  const handleCancelEdit = () => {
    setIsEditing(false);
    setEditedConfig('');
    setUpdateError(null);
  };

  // Memoize header state to only trigger when values actually change (not object reference)
  const headerState = useMemo(() => ({
    autoRefresh,
    loading,
    status: (selectedWantDetails?.status || want?.status || 'created') as WantExecutionStatus
  }), [autoRefresh, loading, selectedWantDetails?.status, want?.status]);

  // Notify parent of header state changes - must be before early return to keep hook order consistent
  // Only depends on memoized state object, not on want/selectedWantDetails objects
  useEffect(() => {
    if (want) {
      onHeaderStateChange?.(headerState);
    }
  }, [want, headerState, onHeaderStateChange]);

  // Trigger animation when want changes (new want selected)
  // Note: Don't set activeTab here - let the initialTab effect handle it
  // This ensures initialTab prop takes precedence over wantId changes
  useEffect(() => {
    if (wantId) {
      setIsInitialLoad(true);
      setPrevTabIndex(-1); // Force animation on initial load
    }
  }, [wantId]);

  // Must be before early return to keep hook order consistent
  const config = useConfigStore(state => state.config);
  const isBottom = config?.header_position === 'bottom';

  // Ref used by the gamepad hook below to access tabs/index defined after the
  // early return, without violating Rules of Hooks.
  const gamepadTabRef = useRef<{
    tabs: Array<{ id: string }>;
    currentTabIndex: number;
    handleTabChange: (id: string) => void;
  } | null>(null);

  // Gamepad L/R bumpers switch sidebar tabs. Must be before early return.
  // Keyboard Tab is handled by the raw keydown listener; gamepadOnly avoids conflict.
  useInputActions({
    gamepadOnly: true,
    ignoreWhenInputFocused: false,
    enabled: !!want,
    onTabForward: () => {
      const ctx = gamepadTabRef.current;
      if (!ctx) return;
      const nextIndex = (ctx.currentTabIndex + 1) % ctx.tabs.length;
      ctx.handleTabChange(ctx.tabs[nextIndex].id);
    },
    onTabBackward: () => {
      const ctx = gamepadTabRef.current;
      if (!ctx) return;
      const nextIndex = (ctx.currentTabIndex - 1 + ctx.tabs.length) % ctx.tabs.length;
      ctx.handleTabChange(ctx.tabs[nextIndex].id);
    },
  });

  if (!want) {
    if (summaryProps) {
      return (
        <div className="px-4 py-8">
          <SummarySidebarContent {...summaryProps} />
        </div>
      );
    }
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-center">
          <Eye className="h-12 w-12 text-gray-400 dark:text-gray-500 mx-auto mb-4" />
          <p className="text-gray-500 dark:text-gray-400">Select a want to view details</p>
        </div>
      </div>
    );
  }

  const hasMultipleVersions = seriesWants.length > 1;
  const isInteractiveWant = wantDetails?.state?.current?.interactive === true;
  const tabs = [
    { id: 'results' as TabType, label: 'Results', icon: Database },
    { id: 'settings' as TabType, label: 'Settings', icon: Settings },
    { id: 'logs' as TabType, label: 'Logs', icon: FileText },
    { id: 'agents' as TabType, label: 'Agents', icon: Bot },
    ...(hasMultipleVersions ? [{ id: 'versions' as TabType, label: 'Versions', icon: History }] : []),
    ...(isInteractiveWant ? [{ id: 'chat' as TabType, label: 'Chat', icon: MessageSquare }] : []),
  ];

  // Get current tab index
  const currentTabIndex = tabs.findIndex(t => t.id === activeTab);

  // Handle tab change with animation direction
  const handleTabChange = (tabId: TabType) => {
    const newIndex = tabs.findIndex(t => t.id === tabId);
    setPrevTabIndex(currentTabIndex);
    setActiveTab(tabId);
  };

  // Determine animation direction (true = moving right/forward, false = moving left/backward)
  const isMovingRight = currentTabIndex > prevTabIndex;

  // Get previous tab ID for simultaneous animation
  const prevTabId = tabs[prevTabIndex]?.id;
  const showPrevTab = prevTabId && prevTabId !== activeTab && prevTabIndex >= 0;

  // Keep the gamepad hook's context ref up-to-date with current render values.
  gamepadTabRef.current = { tabs, currentTabIndex, handleTabChange };

  return (
    <>
    <div className="h-full flex flex-col relative overflow-hidden">
      {/* Content container */}
      <div className="h-full flex flex-col relative z-10">
      {/* Control Panel Buttons */}
      {want && (
        <div className={classNames(
          'flex-shrink-0 h-9 sm:h-14 relative overflow-hidden',
          isBottom ? 'order-last border-t border-gray-200 dark:border-gray-700' : 'border-b border-gray-200 dark:border-gray-700'
        )}>
          <WantControlButtons
            onStart={handleStartClick}
            onStop={handleStopClick}
            onSuspend={handleSuspendClick}
            onDelete={handleDeleteClick}
            onSaveRecipe={handleSaveRecipeClick}
            canStart={canStart}
            canStop={canStop}
            canSuspend={canSuspend}
            canDelete={canDelete}
            canSaveRecipe={canSaveRecipe}
            isSuspended={isSuspended}
            loading={loading}
          />
          <ConfirmationBubble
            isVisible={showClearStateConfirmation}
            onConfirm={handleClearState}
            onCancel={() => setShowClearStateConfirmation(false)}
            onDismiss={() => setShowClearStateConfirmation(false)}
            title="Clear State"
            message="Clear all state data? This cannot be undone."
            layout="header-overlay"
          />
        </div>
      )}

      {/* Tab navigation */}
      <div className={classNames(
        'flex border-gray-200 dark:border-gray-700 bg-gray-50/50 dark:bg-gray-900/50',
        isBottom ? 'border-t' : 'border-b'
      )}>
        {tabs.map((tab) => {
          const Icon = tab.icon;
          const isActive = activeTab === tab.id;
          return (
            <button
              key={tab.id}
              onClick={() => handleTabChange(tab.id)}
              className={classNames(
                'flex-1 flex flex-col items-center justify-center py-1 sm:py-2.5 px-1 sm:px-2 transition-all relative min-w-0',
                isActive
                  ? 'text-blue-600 dark:text-blue-400 bg-white dark:bg-gray-800 shadow-sm'
                  : 'text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-white/50 dark:hover:bg-gray-800/30'
              )}
            >
              <Icon className="h-3.5 w-3.5 sm:h-4 sm:w-4 mb-0 sm:mb-1 flex-shrink-0" />
              <span className="text-[9px] sm:text-[10px] font-bold uppercase tracking-tighter truncate w-full text-center">
                {tab.label}
              </span>
              {/* Active indicator bar - adjacent to content */}
              {isActive && (
                <div className={classNames(
                  "absolute h-0.5 bg-blue-600 dark:bg-blue-400",
                  isBottom ? "top-0 left-0 right-0" : "bottom-0 left-0 right-0"
                )} />
              )}
            </button>
          );
        })}
      </div>

      {/* Tab content */}
      <div className={classNames('flex-1 overflow-y-auto overflow-x-hidden relative', isBottom ? 'order-first' : '')}>
        {loading && !selectedWantDetails ? (
          <div className="flex items-center justify-center py-12">
            <LoadingSpinner size="lg" />
          </div>
        ) : (
          <>
            {/* Previous tab - animate out */}
            {showPrevTab && prevTabId === 'settings' && (
              <div className={classNames('absolute inset-0 overflow-y-auto pointer-events-none', isMovingRight ? 'animate-slide-out-left' : 'animate-slide-out-right')}>
                <SettingsTab
                  want={wantDetails}
                  isEditing={isEditing}
                  editedConfig={editedConfig}
                  updateLoading={updateLoading}
                  updateError={updateError}
                  configMode={configMode}
                  onEdit={handleEditConfig}
                  onSave={handleSaveConfig}
                  onCancel={handleCancelEdit}
                  onConfigChange={setEditedConfig}
                  onConfigModeChange={setConfigMode}
                  onWantUpdate={() => {
                    const wantId = want.metadata?.id || want.id;
                    if (wantId) {
                      fetchWantDetails(wantId);
                      fetchWants();
                    }
                  }}
                  updateWant={updateWant}
                />
              </div>
            )}
            {showPrevTab && prevTabId === 'results' && (
              <div className={classNames('absolute inset-0 overflow-y-auto pointer-events-none', isMovingRight ? 'animate-slide-out-left' : 'animate-slide-out-right')}>
                <ResultsTab
                  want={wantDetails}
                  onRecommendationSelect={onRecommendationSelect}
                  onClearState={() => setShowClearStateConfirmation(true)}
                />
              </div>
            )}
            {showPrevTab && prevTabId === 'logs' && (
              <div className={classNames('absolute inset-0 overflow-y-auto pointer-events-none', isMovingRight ? 'animate-slide-out-left' : 'animate-slide-out-right')}>
                <LogsTab want={wantDetails} results={selectedWantResults} />
              </div>
            )}
            {showPrevTab && prevTabId === 'agents' && (
              <div className={classNames('absolute inset-0 overflow-y-auto pointer-events-none', isMovingRight ? 'animate-slide-out-left' : 'animate-slide-out-right')}>
                <AgentsTab want={wantDetails} />
              </div>
            )}
            {showPrevTab && prevTabId === 'versions' && (
              <div className={classNames('absolute inset-0 overflow-y-auto pointer-events-none', isMovingRight ? 'animate-slide-out-left' : 'animate-slide-out-right')}>
                <VersionsTab seriesWants={seriesWants} currentWantId={wantId} />
              </div>
            )}
            {showPrevTab && prevTabId === 'chat' && (
              <div className={classNames('absolute inset-0 overflow-hidden pointer-events-none', isMovingRight ? 'animate-slide-out-left' : 'animate-slide-out-right')}>
                <ChatTab want={wantDetails} />
              </div>
            )}

            {/* Current tab - animate in */}
            {activeTab === 'settings' && (
              <div className={classNames('relative z-10', isMovingRight ? 'animate-slide-in-right' : 'animate-slide-in-left')}>
                <SettingsTab
                  want={wantDetails}
                  isEditing={isEditing}
                  editedConfig={editedConfig}
                  updateLoading={updateLoading}
                  updateError={updateError}
                  configMode={configMode}
                  onEdit={handleEditConfig}
                  onSave={handleSaveConfig}
                  onCancel={handleCancelEdit}
                  onConfigChange={setEditedConfig}
                  onConfigModeChange={setConfigMode}
                  onWantUpdate={() => {
                    const wantId = want.metadata?.id || want.id;
                    if (wantId) {
                      fetchWantDetails(wantId);
                      fetchWants();
                    }
                  }}
                  updateWant={updateWant}
                />
              </div>
            )}

            {activeTab === 'results' && (
              <div className={classNames('relative z-10', isMovingRight ? 'animate-slide-in-right' : 'animate-slide-in-left')}>
                <ResultsTab
                  want={wantDetails}
                  onRecommendationSelect={onRecommendationSelect}
                  onClearState={() => setShowClearStateConfirmation(true)}
                />
              </div>
            )}

            {activeTab === 'logs' && (
              <div className={classNames('relative z-10', isMovingRight ? 'animate-slide-in-right' : 'animate-slide-in-left')}>
                <LogsTab want={wantDetails} results={selectedWantResults} />
              </div>
            )}

            {activeTab === 'agents' && (
              <div className={classNames('relative z-10', isMovingRight ? 'animate-slide-in-right' : 'animate-slide-in-left')}>
                <AgentsTab want={wantDetails} />
              </div>
            )}

            {activeTab === 'versions' && (
              <div className={classNames('relative z-10', isMovingRight ? 'animate-slide-in-right' : 'animate-slide-in-left')}>
                <VersionsTab seriesWants={seriesWants} currentWantId={wantId} />
              </div>
            )}

            {activeTab === 'chat' && (
              <div className={classNames('relative z-10 h-full', isMovingRight ? 'animate-slide-in-right' : 'animate-slide-in-left')}>
                <ChatTab want={wantDetails} />
              </div>
            )}
          </>
        )}
      </div>
      </div>
    </div>

    </>
  );
};

// ---------------------------------------------------------------------------
// ChatTab component
// ---------------------------------------------------------------------------

interface CCMessage {
  sender: string;
  text: string;
  timestamp: string;
}

interface CCResponse {
  text: string;
  timestamp: string;
  subtype?: string;
}

const ChatTab: React.FC<{ want: Want }> = ({ want }) => {
  const [inputText, setInputText] = useState('');
  const [sending, setSending] = useState(false);
  const [sendError, setSendError] = useState<string | null>(null);
  const [isComposing, setIsComposing] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  const phase = want.state?.current?.phase as string | undefined;
  const ccMessages = (want.state?.current?.cc_messages as CCMessage[] | undefined) ?? [];
  const ccResponses = (want.state?.current?.cc_responses as CCResponse[] | undefined) ?? [];
  const lastResponseRaw = want.state?.current?.last_response_raw as Record<string, unknown> | undefined;
  const wantName = want.metadata?.name;

  // Interleave user messages and assistant responses by index:
  // [user[0], response[0], user[1], response[1], ...]
  const conversationItems: Array<{ role: 'user' | 'assistant'; text: string; timestamp?: string }> = [];
  const len = Math.max(ccMessages.length, ccResponses.length);
  for (let i = 0; i < len; i++) {
    const msg = ccMessages[i];
    const res = ccResponses[i];
    if (msg?.text) conversationItems.push({ role: 'user', text: msg.text, timestamp: msg.timestamp });
    if (res?.text) conversationItems.push({ role: 'assistant', text: res.text, timestamp: res.timestamp });
  }

  // Scroll to bottom when messages update
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [conversationItems.length]);

  const handleSend = async () => {
    if (!inputText.trim() || !wantName || sending) return;
    setSending(true);
    setSendError(null);
    try {
      await apiClient.sendWebhookMessage(wantName, inputText.trim(), 'user');
      setInputText('');
    } catch (err: unknown) {
      setSendError(err instanceof Error ? err.message : 'Failed to send message');
    } finally {
      setSending(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey && !isComposing) {
      e.preventDefault();
      handleSend();
    }
  };

  const phaseBadgeClass = classNames(
    'text-xs px-2 py-0.5 rounded-full font-medium',
    phase === 'monitoring'        ? 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300'
    : phase === 'requesting'      ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
    : phase === 'awaiting_response' ? 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-300 animate-pulse'
    : phase === 'response_received' ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
    : phase === 'achieved'        ? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-200'
    : phase === 'error'           ? 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    : 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300'
  );

  const responseSubtype = (ccResponses[ccResponses.length - 1]?.subtype) ?? (lastResponseRaw?.subtype as string | undefined);

  return (
    <div className="flex flex-col h-full">
      {/* Phase indicator */}
      <div className="flex-shrink-0 px-4 py-2 border-b border-gray-200 dark:border-gray-700 flex items-center gap-2">
        <span className="text-xs text-gray-500 dark:text-gray-400">Phase:</span>
        {phase && <span className={phaseBadgeClass}>{phase}</span>}
        {responseSubtype && (
          <span className={classNames(
            'text-xs px-2 py-0.5 rounded-full font-medium ml-auto',
            responseSubtype === 'success' ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
            : responseSubtype === 'error' ? 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
            : 'bg-gray-100 text-gray-500 dark:bg-gray-700 dark:text-gray-400'
          )}>
            {responseSubtype}
          </span>
        )}
      </div>

      {/* Message thread */}
      <div className="flex-1 overflow-y-auto px-4 py-4 space-y-3 min-h-0">
        {conversationItems.length === 0 ? (
          <div className="text-center py-12">
            <MessageSquare className="h-10 w-10 text-gray-400 dark:text-gray-500 mx-auto mb-3" />
            <p className="text-sm text-gray-500 dark:text-gray-400">No messages yet</p>
            <p className="text-xs text-gray-400 dark:text-gray-500 mt-1">Send a message below</p>
          </div>
        ) : (
          conversationItems.map((item, i) => (
            <div key={i} className={classNames('flex', item.role === 'user' ? 'justify-end' : 'justify-start')}>
              <div className={classNames(
                'max-w-[80%] rounded-lg px-3 py-2 text-sm',
                item.role === 'user'
                  ? 'bg-blue-600 text-white'
                  : 'bg-gray-100 dark:bg-gray-800 text-gray-900 dark:text-gray-100'
              )}>
                <p className="whitespace-pre-wrap break-words">{item.text}</p>
                {item.timestamp && (
                  <p className={classNames(
                    'text-xs mt-1',
                    item.role === 'user' ? 'text-blue-200' : 'text-gray-400 dark:text-gray-500'
                  )}>
                    {formatRelativeTime(item.timestamp)}
                  </p>
                )}
              </div>
            </div>
          ))
        )}
        <div ref={messagesEndRef} />
      </div>

      {/* Input area */}
      <div className="flex-shrink-0 px-4 py-3 border-t border-gray-200 dark:border-gray-700">
        {sendError && <p className="text-xs text-red-500 mb-2">{sendError}</p>}
        <div className="flex gap-2 items-end">
          <textarea
            value={inputText}
            onChange={e => setInputText(e.target.value)}
            onKeyDown={handleKeyDown}
            onCompositionStart={() => setIsComposing(true)}
            onCompositionEnd={() => setIsComposing(false)}
            placeholder="Send a message... (Enter to send, Shift+Enter for newline)"
            rows={2}
            className="flex-1 resize-none rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-sm text-gray-900 dark:text-gray-100 px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:focus:ring-blue-400 placeholder-gray-400 dark:placeholder-gray-500"
          />
          <button
            onClick={handleSend}
            disabled={!inputText.trim() || sending || !wantName}
            className="flex-shrink-0 p-2 rounded-lg bg-blue-600 hover:bg-blue-700 disabled:bg-gray-300 dark:disabled:bg-gray-600 text-white disabled:text-gray-400 dark:disabled:text-gray-500 transition-colors"
          >
            {sending ? <LoadingSpinner size="sm" /> : <Send className="h-4 w-4" />}
          </button>
        </div>
      </div>
    </div>
  );
};

// Tab Components
const SettingsTab: React.FC<{
  want: Want;
  isEditing: boolean;
  editedConfig: string;
  updateLoading: boolean;
  updateError: string | null;
  configMode: 'form' | 'yaml';
  onEdit: () => void;
  onSave: () => void;
  onCancel: () => void;
  onConfigChange: (value: string) => void;
  onConfigModeChange: (mode: 'form' | 'yaml') => void;
  onWantUpdate?: () => void;
  updateWant: (id: string, request: any) => Promise<void>;
}> = ({
  want,
  isEditing,
  editedConfig,
  updateLoading,
  updateError,
  configMode,
  onEdit,
  onSave,
  onCancel,
  onConfigChange,
  onConfigModeChange,
  onWantUpdate,
  updateWant
}) => {
  const [localUpdateLoading, setLocalUpdateLoading] = useState(false);
  const [localUpdateError, setLocalUpdateError] = useState<string | null>(null);

  // F: Inline name editing
  const [isEditingName, setIsEditingName] = useState(false);
  const [editedName, setEditedName] = useState(want.metadata?.name || '');
  const nameInputRef = useRef<HTMLInputElement>(null);

  // G: Saved indicator
  const [savedIndicator, setSavedIndicator] = useState(false);
  const showSaved = useCallback(() => {
    setSavedIndicator(true);
    setTimeout(() => setSavedIndicator(false), 1500);
  }, []);

  // Section collapsed states
  const [isParametersCollapsed, setIsParametersCollapsed] = useState(true);
  const [isLabelsCollapsed, setIsLabelsCollapsed] = useState(true);
  const [isDependenciesCollapsed, setIsDependenciesCollapsed] = useState(true);
  const [isSchedulingCollapsed, setIsSchedulingCollapsed] = useState(true);

  // Section editing states to prevent polling from overwriting user input
  const [isEditingParameters, setIsEditingParameters] = useState(false);
  const [isEditingLabels, setIsEditingLabels] = useState(false);
  const [isEditingDependencies, setIsEditingDependencies] = useState(false);
  const [isEditingScheduling, setIsEditingScheduling] = useState(false);

  // Form data states
  const [params, setParams] = useState<Record<string, unknown>>(want.spec?.params || {});
  const [labels, setLabels] = useState<Record<string, string>>(want.metadata?.labels || {});
  const [using, setUsing] = useState<Array<Record<string, string>>>(want.spec?.using || []);
  const [when, setWhen] = useState<WhenSpec[]>(want.spec?.when || []);

  // Section refs for keyboard navigation
  const paramsSectionRef = useRef<HTMLButtonElement>(null);
  const labelsSectionRef = useRef<HTMLButtonElement>(null);
  const dependenciesSectionRef = useRef<HTMLButtonElement>(null);
  const schedulingSectionRef = useRef<HTMLButtonElement>(null);

  // Handler for parameter changes - saves to API
  const handleParametersChange = useCallback(async (newParams: Record<string, any>) => {
    if (!want.metadata?.id) return;

    const oldParams = params;
    setParams(newParams);
    setIsEditingParameters(true);

    try {
      await updateWantParameters(want.metadata.id, want, newParams, updateWant);
      onWantUpdate?.();
      showSaved();
    } catch (error) {
      setLocalUpdateError(error instanceof Error ? error.message : 'Failed to update parameters');
      setParams(oldParams); // Revert on error
    } finally {
      setIsEditingParameters(false);
    }
  }, [want, params, updateWant, onWantUpdate]);

  // Handler for label changes - saves to API
  const handleLabelsChange = useCallback(async (newLabels: Record<string, string>) => {
    if (!want.metadata?.id) return;

    const oldLabels = labels;
    setLabels(newLabels);
    setIsEditingLabels(true);

    try {
      await updateWantLabels(want.metadata.id, oldLabels, newLabels);
      onWantUpdate?.();
      showSaved();
    } catch (error) {
      setLocalUpdateError(error instanceof Error ? error.message : 'Failed to update labels');
      setLabels(oldLabels); // Revert on error
    } finally {
      setIsEditingLabels(false);
    }
  }, [want.metadata?.id, labels, onWantUpdate]);

  // Handler for dependency changes - saves to API
  const handleDependenciesChange = useCallback(async (newUsing: Array<Record<string, string>>) => {
    if (!want.metadata?.id) return;

    const oldUsing = using;
    setUsing(newUsing);
    setIsEditingDependencies(true);

    try {
      await updateWantDependencies(want.metadata.id, oldUsing, newUsing);
      onWantUpdate?.();
      showSaved();
    } catch (error) {
      setLocalUpdateError(error instanceof Error ? error.message : 'Failed to update dependencies');
      setUsing(oldUsing); // Revert on error
    } finally {
      setIsEditingDependencies(false);
    }
  }, [want.metadata?.id, using, onWantUpdate]);

  // Handler for scheduling changes - saves to API
  const handleSchedulingChange = useCallback(async (newWhen: WhenSpec[]) => {
    if (!want.metadata?.id) return;

    const oldWhen = when;
    setWhen(newWhen);
    setIsEditingScheduling(true);

    try {
      await updateWantScheduling(want.metadata.id, want, newWhen, updateWant);
      onWantUpdate?.();
      showSaved();
    } catch (error) {
      setLocalUpdateError(error instanceof Error ? error.message : 'Failed to update scheduling');
      setWhen(oldWhen); // Revert on error
    } finally {
      setIsEditingScheduling(false);
    }
  }, [want, when, updateWant, onWantUpdate]);

  // Handle arrow key navigation for form fields based on DOM order
  const handleArrowKeyNavigation = useCallback((e: React.KeyboardEvent) => {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;

    // Find the closest container that holds all sections
    const currentTarget = e.currentTarget || (e as any).target;
    const container = currentTarget?.closest('.focusable-container');
    if (!container) return;

    const focusableElements = Array.from(container.querySelectorAll('.focusable-section-header')) as HTMLElement[];
    const currentIndex = focusableElements.indexOf(document.activeElement as HTMLElement);

    if (currentIndex === -1) {
      if (e.key === 'ArrowDown' && focusableElements.length > 0) {
        if (typeof e.preventDefault === 'function') e.preventDefault();
        focusableElements[0].focus();
      }
      return;
    }

    if (e.key === 'ArrowDown' && currentIndex < focusableElements.length - 1) {
      if (typeof e.preventDefault === 'function') e.preventDefault();
      focusableElements[currentIndex + 1].focus();
    } else if (e.key === 'ArrowUp' && currentIndex > 0) {
      if (typeof e.preventDefault === 'function') e.preventDefault();
      focusableElements[currentIndex - 1].focus();
    }
  }, []);

  // Reset form state when want changes or metadata is updated
  // Only update fields that are NOT currently being edited to avoid losing user input
  useEffect(() => {
    if (!isEditingParameters) setParams(want.spec?.params || {});
    if (!isEditingLabels) setLabels(want.metadata?.labels || {});
    if (!isEditingDependencies) setUsing(want.spec?.using || []);
    if (!isEditingScheduling) setWhen(want.spec?.when || []);
  }, [want.metadata?.id, want.metadata?.updatedAt, isEditingParameters, isEditingLabels, isEditingDependencies, isEditingScheduling]);

  // Reset all states when switching to a different want
  useEffect(() => {
    setIsEditingParameters(false);
    setIsEditingLabels(false);
    setIsEditingDependencies(false);
    setIsEditingScheduling(false);
    setIsParametersCollapsed(true);
    setIsLabelsCollapsed(true);
    setIsDependenciesCollapsed(true);
    setIsSchedulingCollapsed(true);
    setIsEditingName(false);
    setEditedName(want.metadata?.name || '');
  }, [want.metadata?.id]);

  return (
    <div className="h-full flex flex-col">
      {/* Config/Overview Toggle + G: Saved indicator */}
      <div className="flex-shrink-0 px-3 sm:px-8 py-1 sm:py-2 flex items-center justify-between">
        <div className={`flex items-center gap-1 text-xs text-green-600 dark:text-green-400 transition-opacity duration-300 ${savedIndicator ? 'opacity-100' : 'opacity-0'}`}>
          <Check className="w-3 h-3" />
          <span>Saved</span>
        </div>
        <FormYamlToggle
          mode={configMode}
          onModeChange={onConfigModeChange}
        />
      </div>

      {/* Content Area */}
      <div className="flex-1 overflow-y-auto px-3 sm:px-4 pt-0 pb-3 sm:py-4 focusable-container">
        {configMode === 'form' ? (
          <div className="space-y-2">
            {/* Metadata Section */}
            <div className={SECTION_CONTAINER_CLASS}>
              <h4 className="text-sm sm:text-base font-medium text-gray-900 dark:text-white mb-2 sm:mb-4">Metadata</h4>
              <div className="space-y-2 sm:space-y-3">
                <div className="flex justify-between items-center gap-2">
                  <span className="text-gray-600 dark:text-gray-300 text-xs sm:text-sm flex-shrink-0">Name:</span>
                  {isEditingName ? (
                    <input
                      ref={nameInputRef}
                      type="text"
                      value={editedName}
                      onChange={(e) => setEditedName(e.target.value)}
                      onBlur={async () => {
                        const trimmed = editedName.trim();
                        if (trimmed && trimmed !== want.metadata?.name && want.metadata?.id) {
                          try {
                            await updateWant(want.metadata.id, {
                              metadata: { ...want.metadata, name: trimmed },
                              spec: want.spec
                            });
                            onWantUpdate?.();
                            showSaved();
                          } catch {
                            setEditedName(want.metadata?.name || '');
                          }
                        }
                        setIsEditingName(false);
                      }}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') nameInputRef.current?.blur();
                        if (e.key === 'Escape') { setEditedName(want.metadata?.name || ''); setIsEditingName(false); }
                      }}
                      autoFocus
                      className="font-medium text-xs sm:text-sm bg-white dark:bg-gray-800 border border-blue-400 dark:border-blue-500 rounded px-1.5 py-0.5 text-right min-w-0 flex-1 focus:outline-none focus:ring-1 focus:ring-blue-500"
                    />
                  ) : (
                    <span
                      onClick={() => { setEditedName(want.metadata?.name || ''); setIsEditingName(true); }}
                      className="font-medium text-xs sm:text-sm cursor-pointer hover:bg-gray-200 dark:hover:bg-gray-700 rounded px-1.5 py-0.5 -mr-1.5 transition-colors truncate"
                      title="Click to edit"
                    >
                      {want.metadata?.name || 'N/A'}
                    </span>
                  )}
                </div>
                <div className="flex justify-between items-center">
                  <span className="text-gray-600 dark:text-gray-300 text-xs sm:text-sm">Type:</span>
                  <span className="font-medium text-xs sm:text-sm">{want.metadata?.type || 'N/A'}</span>
                </div>
                <div className="flex justify-between items-center">
                  <span className="text-gray-600 dark:text-gray-300 text-xs sm:text-sm">ID:</span>
                  <span className="font-mono text-[10px] sm:text-xs break-all ml-4 text-right">{want.metadata?.id || want.id || 'N/A'}</span>
                </div>
              </div>
            </div>

            {/* Parameters - Using Common Component */}
            <ParametersSection
              ref={paramsSectionRef}
              parameters={params}
              onChange={handleParametersChange}
              isCollapsed={isParametersCollapsed}
              onToggleCollapse={() => setIsParametersCollapsed(!isParametersCollapsed)}
              navigationCallbacks={{
                onNavigateUp: (e) => e && handleArrowKeyNavigation(e),
                onNavigateDown: (e) => e && handleArrowKeyNavigation(e),
              }}
            />

            {/* Timeline */}
            {want.stats && (
              <div className={SECTION_CONTAINER_CLASS}>
                <h4 className="text-sm sm:text-base font-medium text-gray-900 dark:text-white mb-2 sm:mb-4">Timeline</h4>
                <div className="space-y-2 sm:space-y-3">
                  {want.stats.created_at && (
                    <div className="flex justify-between items-center">
                      <span className="text-gray-600 dark:text-gray-300 text-xs sm:text-sm">Created:</span>
                      <span className="text-xs sm:text-sm">{formatDate(want.stats.created_at)}</span>
                    </div>
                  )}
                  {want.stats.started_at && (
                    <div className="flex justify-between items-center">
                      <span className="text-gray-600 dark:text-gray-300 text-xs sm:text-sm">Started:</span>
                      <span className="text-xs sm:text-sm">{formatDate(want.stats.started_at)}</span>
                    </div>
                  )}
                  {want.stats.completed_at && (
                    <div className="flex justify-between items-center">
                      <span className="text-gray-600 dark:text-gray-300 text-xs sm:text-sm">Achieved:</span>
                      <span className="text-xs sm:text-sm">{formatDate(want.stats.completed_at)}</span>
                    </div>
                  )}
                  {want.stats.started_at && (
                    <div className="flex justify-between items-center">
                      <span className="text-gray-600 dark:text-gray-300 text-xs sm:text-sm">Duration:</span>
                      <span className="text-xs sm:text-sm">{formatDuration(want.stats.started_at, want.stats.completed_at)}</span>
                    </div>
                  )}
                </div>
              </div>
            )}

            {/* Scheduling - Using Common Component */}
            <SchedulingSection
              ref={schedulingSectionRef}
              schedules={when}
              onChange={handleSchedulingChange}
              isCollapsed={isSchedulingCollapsed}
              onToggleCollapse={() => setIsSchedulingCollapsed(!isSchedulingCollapsed)}
              navigationCallbacks={{
                onNavigateUp: (e) => e && handleArrowKeyNavigation(e),
                onNavigateDown: (e) => e && handleArrowKeyNavigation(e),
              }}
            />

            {/* Labels - Using Common Component */}
            <LabelsSection
              ref={labelsSectionRef}
              labels={labels}
              onChange={handleLabelsChange}
              isCollapsed={isLabelsCollapsed}
              onToggleCollapse={() => setIsLabelsCollapsed(!isLabelsCollapsed)}
              navigationCallbacks={{
                onNavigateUp: (e) => e && handleArrowKeyNavigation(e),
                onNavigateDown: (e) => e && handleArrowKeyNavigation(e),
              }}
            />

            {/* Dependencies - Using Common Component */}
            <DependenciesSection
              ref={dependenciesSectionRef}
              dependencies={using}
              onChange={handleDependenciesChange}
              isCollapsed={isDependenciesCollapsed}
              onToggleCollapse={() => setIsDependenciesCollapsed(!isDependenciesCollapsed)}
              navigationCallbacks={{
                onNavigateUp: (e) => e && handleArrowKeyNavigation(e),
                onNavigateDown: (e) => e && handleArrowKeyNavigation(e),
              }}
            />

            {/* Error Information */}
            {want.status === 'failed' && want.state?.current?.error && (
              <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-6">
                <div className="flex items-start">
                  <AlertTriangle className="h-5 w-5 text-red-600 dark:text-red-400 mt-0.5 mr-3 flex-shrink-0" />
                  <div className="flex-1 min-w-0">
                    <h4 className="text-base font-medium text-red-800 dark:text-red-300 mb-3">Error Details</h4>
                    <p className="text-sm text-red-600 dark:text-red-400 break-words leading-relaxed">
                      {typeof want.state.current.error === 'string' ? want.state.current.error : JSON.stringify(want.state.current.error)}
                    </p>
                  </div>
                </div>
              </div>
            )}
          </div>
        ) : (
          /* Config Editor View */
          <div className="flex flex-col h-full">
            {!isEditing ? (
              <div className="flex flex-col flex-1">
                <div className="flex items-center justify-between mb-4">
                  <h4 className="text-sm font-medium text-gray-900 dark:text-white">Configuration</h4>
                  <button
                    onClick={onEdit}
                    className="inline-flex items-center px-3 py-1.5 border border-gray-300 dark:border-gray-600 shadow-sm text-xs font-medium rounded text-gray-700 dark:text-gray-200 bg-white dark:bg-gray-800 hover:bg-gray-50 dark:hover:bg-gray-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
                  >
                    <Edit className="h-3 w-3 mr-1" />
                    Edit
                  </button>
                </div>
                <div className="flex-1">
                  <YamlEditor
                    value={stringifyYaml({
                      metadata: want.metadata,
                      spec: want.spec
                    })}
                    onChange={() => {}}
                    readOnly={true}
                    height="100%"
                  />
                </div>
              </div>
            ) : (
              <div className="flex flex-col flex-1">
                <div className="flex items-center justify-between mb-4">
                  <h4 className="text-sm font-medium text-gray-900 dark:text-white">Edit Configuration</h4>
                  <div className="flex space-x-2">
                    <button
                      onClick={onCancel}
                      disabled={updateLoading}
                      className="inline-flex items-center px-3 py-1.5 border border-gray-300 dark:border-gray-600 shadow-sm text-xs font-medium rounded text-gray-700 dark:text-gray-200 bg-white dark:bg-gray-800 hover:bg-gray-50 dark:hover:bg-gray-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50"
                    >
                      Cancel
                    </button>
                    <button
                      onClick={onSave}
                      disabled={updateLoading}
                      className="inline-flex items-center px-3 py-1.5 border border-transparent shadow-sm text-xs font-medium rounded text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50"
                    >
                      {updateLoading ? (
                        <LoadingSpinner size="sm" className="mr-1" />
                      ) : (
                        <Save className="h-3 w-3 mr-1" />
                      )}
                      Save
                    </button>
                  </div>
                </div>

                {updateError && (
                  <div className="mb-4">
                    <ErrorDisplay error={updateError} />
                  </div>
                )}

                <div className="flex-1">
                  <YamlEditor
                    value={editedConfig}
                    onChange={onConfigChange}
                    readOnly={updateLoading}
                    height="100%"
                  />
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
};

// Component for collapsible array fields
const CollapsibleArray: React.FC<{ label: string; items: any[]; depth: number }> = ({ label, items, depth }) => {
  const [isExpanded, setIsExpanded] = useState(false);

  return (
    <div className="space-y-1">
      <button
        onClick={() => setIsExpanded(!isExpanded)}
        className="flex items-center gap-2 font-medium text-gray-800 dark:text-gray-100 text-sm hover:text-gray-900 dark:hover:text-gray-300 py-1"
      >
        {isExpanded ? (
          <ChevronDown className="h-4 w-4 text-gray-500 dark:text-gray-400" />
        ) : (
          <ChevronRight className="h-4 w-4 text-gray-500 dark:text-gray-400" />
        )}
        {label}:
        {!isExpanded && <span className="text-xs text-gray-400 dark:text-gray-500 ml-1">Array({items.length})</span>}
      </button>
      {isExpanded && (
        <div className="ml-4 border-l border-gray-200 dark:border-gray-700 pl-3 space-y-2">
          {items.map((item, index) => (
            <ArrayItemRenderer key={index} item={item} index={index} depth={depth + 1} />
          ))}
        </div>
      )}
    </div>
  );
};

// Component for expandable array items
const ArrayItemRenderer: React.FC<{ item: any; index: number; depth: number }> = ({ item, index, depth }) => {
  const [isExpanded, setIsExpanded] = useState(false);
  const isNested = item !== null && typeof item === 'object';

  if (!isNested) {
    return (
      <div className="text-sm text-gray-700 dark:text-gray-200 font-mono ml-4">
        [{index}]: {String(item)}
      </div>
    );
  }

  return (
    <div className="border-l border-gray-300 dark:border-gray-600 pl-3 ml-2">
      <button
        onClick={() => setIsExpanded(!isExpanded)}
        className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200 hover:text-gray-900 dark:hover:text-gray-300 py-1"
      >
        {isExpanded ? (
          <ChevronDown className="h-4 w-4 text-gray-500 dark:text-gray-400" />
        ) : (
          <ChevronRight className="h-4 w-4 text-gray-500 dark:text-gray-400" />
        )}
        <span className="text-xs text-gray-500 dark:text-gray-400">[{index}]</span>
        {!isExpanded && (
          <span className="text-xs text-gray-400 dark:text-gray-500">
            {Array.isArray(item) ? `Array(${item.length})` : 'Object'}
          </span>
        )}
      </button>
      {isExpanded && (
        <div className="mt-2 ml-2 space-y-2">
          {renderKeyValuePairs(item, depth + 1)}
        </div>
      )}
    </div>
  );
};

// Copy button for state values
const CopyValueButton: React.FC<{ value: string }> = ({ value }) => {
  const [copied, setCopied] = useState(false);
  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation();
    navigator.clipboard.writeText(value);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };
  return (
    <button
      onClick={handleCopy}
      className="opacity-0 group-hover/kv:opacity-100 flex-shrink-0 p-0.5 rounded hover:bg-gray-200 dark:hover:bg-gray-700 transition-opacity"
      title="Copy value"
    >
      {copied ? <Check className="w-3.5 h-3.5 text-green-500" /> : <Copy className="w-3.5 h-3.5 text-gray-400" />}
    </button>
  );
};

// Helper to render key-value pairs recursively
const renderKeyValuePairs = (obj: any, depth: number = 0): React.ReactNode[] => {
  const items: React.ReactNode[] = [];

  if (obj === null || obj === undefined) {
    return [<span key="null" className="text-gray-500 dark:text-gray-400 italic">null</span>];
  }

  if (typeof obj !== 'object') {
    return [
      <span key="value" className="text-gray-700 dark:text-gray-200 font-mono break-all">
        {String(obj)}
      </span>
    ];
  }

  if (Array.isArray(obj)) {
    return [
      <div key="array" className="space-y-2">
        {obj.map((item, index) => (
          <ArrayItemRenderer key={index} item={item} index={index} depth={depth} />
        ))}
      </div>
    ];
  }

  Object.entries(obj).forEach(([key, value]) => {
    const isNested = value !== null && typeof value === 'object';
    const isArray = Array.isArray(value);

    if (isArray) {
      // For array values, wrap in a collapsible container
      items.push(
        <CollapsibleArray key={key} label={key} items={value} depth={depth} />
      );
    } else if (isNested) {
      // For nested objects, show key with collapsible content
      items.push(
        <div key={key} className="space-y-1">
          <div className="font-medium text-gray-800 dark:text-gray-100 text-xs sm:text-sm">{key}:</div>
          <div className="ml-2 sm:ml-4 border-l border-gray-200 dark:border-gray-700 pl-2 sm:pl-3">
            {renderKeyValuePairs(value, depth + 1)}
          </div>
        </div>
      );
    } else {
      // For simple key-value pairs, display in left-right format with dots
      const valueStr = String(value);

      // Truncate key if too long
      const displayKey = key.length > 25 ? key.slice(0, 25) + '~' : key;
      
      // Calculate spacing to fill with dots
      const keyLength = displayKey.length;
      const valueLength = valueStr.length;
      const minDots = 3;
      // Estimate: key (small) + value (large) + spacing
      const totalAvailableChars = 50; // Approximate character width available
      const dotsNeeded = Math.max(minDots, totalAvailableChars - keyLength - valueLength);
      const dots = '.'.repeat(Math.max(minDots, Math.min(dotsNeeded, 30)));

      items.push(
        <div key={key} className="flex justify-between items-center text-xs sm:text-sm gap-2 group/kv">
          <span className="text-gray-600 dark:text-gray-300 font-normal text-[10px] sm:text-xs whitespace-nowrap flex-shrink-0" title={key}>{displayKey}</span>
          <div className="flex items-center gap-1 min-w-0">
            <span className="text-gray-400 dark:text-gray-500 text-[10px] sm:text-xs flex-shrink-0">{dots}</span>
            <span className="text-gray-800 dark:text-gray-100 font-semibold text-sm sm:text-base truncate group-hover/kv:whitespace-normal group-hover/kv:break-all group-hover/kv:overflow-visible" title={valueStr}>{valueStr}</span>
            <CopyValueButton value={valueStr} />
          </div>
        </div>
      );
    }
  });

  return items;
};

// Sort an object's entries by state_timestamps descending (most recently updated first).
// Keys without a timestamp are placed at the end in their original order.
const sortByTimestamp = (obj: Record<string, unknown>, timestamps?: Record<string, string>): Record<string, unknown> => {
  if (!timestamps) return obj;
  const entries = Object.entries(obj);
  entries.sort(([aKey], [bKey]) => {
    const aTs = timestamps[aKey] ? new Date(timestamps[aKey]).getTime() : 0;
    const bTs = timestamps[bKey] ? new Date(timestamps[bKey]).getTime() : 0;
    return bTs - aTs;
  });
  return Object.fromEntries(entries);
};

const ResultsTab: React.FC<{
  want: Want;
  onRecommendationSelect?: (rec: Recommendation) => void;
  onClearState?: () => void;
}> = ({ want, onRecommendationSelect, onClearState }) => {
  const recommendations: Recommendation[] =
    (want.state?.current?.proposed_recommendations as Recommendation[]) ||
    (want.state?.current?.recommendations as Recommendation[]) || [];
  const ts = want.state_timestamps;
  const hasCurrent = want.state?.current && Object.keys(want.state.current).length > 0;
  const hasGoal = want.state?.goal && Object.keys(want.state.goal).length > 0;
  const hasPlan = want.state?.plan && Object.keys(want.state.plan).length > 0;
  const hasAnyState = hasCurrent || hasGoal || hasPlan;
  const hasHiddenState = want.hidden_state && Object.keys(want.hidden_state).length > 0;
  const [isHiddenStateExpanded, setIsHiddenStateExpanded] = useState(false);
  const hasFinalResult = want.state?.final_result != null && want.state?.final_result !== '';
  const [finalResultCopied, setFinalResultCopied] = useState(false);
  
  // Extract goal proposal info
  const proposedBreakdown = want.state?.current?.proposed_breakdown as any[] | undefined;
  const proposedResponse = want.state?.current?.proposed_response as string | undefined;

  const handleCopyFinalResult = () => {
    const value = want.state?.final_result;
    const text = typeof value === 'string' ? value : JSON.stringify(value);
    navigator.clipboard.writeText(text).then(() => {
      setFinalResultCopied(true);
      setTimeout(() => setFinalResultCopied(false), 1500);
    });
  };

  return (
    <div className="h-full flex flex-col">
      <div className="flex-1 overflow-y-auto px-3 sm:px-4 pt-0 pb-3 sm:py-4">
        <div className="space-y-4 pt-3">
          {/* AI Ideas Section (for drafts) */}
          {recommendations.length > 0 && (
            <TabSection title="AI Ideas" className="bg-blue-50/50 dark:bg-blue-900/10 border border-blue-100 dark:border-blue-900/30">
              <div className="flex items-center gap-2 mb-3 text-blue-700 dark:text-blue-300">
                <Sparkles className="h-4 w-4" />
                <span className="text-xs font-medium italic">Select an idea to materialize into a real want.</span>
              </div>
              <div className="grid grid-cols-1 gap-2">
                {recommendations.map((rec) => (
                  <button
                    key={rec.id}
                    onClick={() => onRecommendationSelect?.(rec)}
                    className="flex items-center gap-3 w-full text-left p-3 rounded-lg bg-white dark:bg-gray-800 border border-blue-200 dark:border-blue-800 hover:border-blue-400 dark:hover:border-blue-600 hover:shadow-md transition-all group"
                  >
                    <div className="flex-1 min-w-0">
                      <div className="text-sm font-semibold text-gray-900 dark:text-white group-hover:text-blue-600 dark:group-hover:text-blue-400">
                        {rec.title}
                      </div>
                      {rec.description && (
                        <div className="text-xs text-gray-500 dark:text-gray-400 mt-1 line-clamp-2">
                          {rec.description}
                        </div>
                      )}
                    </div>
                    <div className="flex-shrink-0 w-8 h-8 rounded-full bg-blue-50 dark:bg-blue-900/40 flex items-center justify-center text-blue-600 dark:text-blue-400 opacity-0 group-hover:opacity-100 transition-opacity">
                      <Plus className="h-5 w-5" />
                    </div>
                  </button>
                ))}
              </div>
            </TabSection>
          )}

          {/* AI Decomposition Proposal Section (for goals) */}
          {proposedBreakdown && proposedBreakdown.length > 0 && (
            <TabSection title="AI Decomposition Proposal" className="bg-purple-50/50 dark:bg-purple-900/10 border border-purple-100 dark:border-purple-900/30">
              <div className="flex items-center gap-2 mb-3 text-purple-700 dark:text-purple-300">
                <Bot className="h-4 w-4" />
                <span className="text-xs font-medium italic">Approve this plan on the card to execute.</span>
              </div>
              
              {proposedResponse && (
                <div className="mb-4 text-sm text-purple-800 dark:text-purple-300 leading-relaxed italic border-l-2 border-purple-300 dark:border-purple-700 pl-3">
                  "{proposedResponse}"
                </div>
              )}

              <div className="space-y-3">
                {proposedBreakdown.map((item, idx) => (
                  <div key={idx} className="flex items-start gap-3 p-2.5 rounded-md bg-white/60 dark:bg-gray-800/60 border border-purple-100/50 dark:border-purple-800/50">
                    <div className="mt-1 w-5 h-5 rounded-full bg-purple-100 dark:bg-purple-900 flex items-center justify-center text-[10px] font-bold text-purple-600 dark:text-purple-400 flex-shrink-0">
                      {idx + 1}
                    </div>
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 mb-1">
                        <span className="px-1.5 py-0.5 rounded bg-purple-100 dark:bg-purple-900 text-[10px] font-bold text-purple-700 dark:text-purple-300 uppercase tracking-tight">
                          {item.type}
                        </span>
                      </div>
                      <p className="text-xs text-gray-700 dark:text-gray-200 font-medium leading-normal">
                        {item.description}
                      </p>
                    </div>
                  </div>
                ))}
              </div>
            </TabSection>
          )}

          {hasAnyState || hasHiddenState || hasFinalResult ? (
            <div className="space-y-4">
              {hasFinalResult && (
                <div className={SECTION_CONTAINER_CLASS}>
                  <div className="flex items-baseline justify-between mb-1 sm:mb-3">
                    <h4 className="text-sm sm:text-base font-medium text-green-600 dark:text-green-400">Final Result</h4>
                    {want.state_timestamps?.final_result && (
                      <span className="text-xs text-gray-400 dark:text-gray-500">
                        {new Date(want.state_timestamps.final_result).toLocaleString()}
                      </span>
                    )}
                  </div>
                  <div className="group/finalresult relative">
                    {Array.isArray(want.state!.final_result) &&
                     (want.state!.final_result as unknown[]).length > 0 &&
                     typeof (want.state!.final_result as unknown[])[0] === 'object' &&
                     (want.state!.final_result as unknown[])[0] !== null ? (
                      <>
                        <ArrayResultTable
                          data={want.state!.final_result as Record<string, unknown>[]}
                          maxRows={10}
                          size="normal"
                        />
                        <button
                          onClick={handleCopyFinalResult}
                          className="absolute right-0 top-0 opacity-100 sm:opacity-0 sm:group-hover/finalresult:opacity-100 transition-opacity p-0.5 rounded text-green-500 hover:text-green-400 hover:bg-gray-200 dark:hover:bg-gray-700"
                          title="Copy to clipboard"
                        >
                          {finalResultCopied ? <Check className="w-4 h-4" /> : <Copy className="w-4 h-4" />}
                        </button>
                      </>
                    ) : (
                      <>
                        <pre className="text-xs sm:text-sm font-mono text-gray-800 dark:text-gray-200 whitespace-pre-wrap break-all pr-7">
                          {typeof want.state!.final_result === 'string'
                            ? want.state!.final_result as string
                            : JSON.stringify(want.state!.final_result, null, 2)}
                        </pre>
                        <button
                          onClick={handleCopyFinalResult}
                          className="absolute right-0 top-0 opacity-100 sm:opacity-0 sm:group-hover/finalresult:opacity-100 transition-opacity p-0.5 rounded text-green-500 hover:text-green-400 hover:bg-gray-200 dark:hover:bg-gray-700"
                          title="Copy to clipboard"
                        >
                          {finalResultCopied ? <Check className="w-4 h-4" /> : <Copy className="w-4 h-4" />}
                        </button>
                      </>
                    )}
                  </div>
                </div>
              )}

              {hasCurrent && (
                <div className={SECTION_CONTAINER_CLASS}>
                  <h4 className="text-sm sm:text-base font-medium text-gray-900 dark:text-white mb-1 sm:mb-3">Current</h4>
                  <div className="space-y-1 sm:space-y-2">
                    {renderKeyValuePairs(sortByTimestamp(want.state!.current as Record<string, unknown>, ts))}
                  </div>
                </div>
              )}

              {hasGoal && (
                <div className={SECTION_CONTAINER_CLASS}>
                  <h4 className="text-sm sm:text-base font-medium text-gray-900 dark:text-white mb-1 sm:mb-3">Goal</h4>
                  <div className="space-y-1 sm:space-y-2">
                    {renderKeyValuePairs(sortByTimestamp(want.state!.goal as Record<string, unknown>, ts))}
                  </div>
                </div>
              )}

              {hasPlan && (
                <div className={SECTION_CONTAINER_CLASS}>
                  <h4 className="text-sm sm:text-base font-medium text-gray-900 dark:text-white mb-1 sm:mb-3">Plan</h4>
                  <div className="space-y-1 sm:space-y-2">
                    {renderKeyValuePairs(sortByTimestamp(want.state!.plan as Record<string, unknown>, ts))}
                  </div>
                </div>
              )}

              {hasHiddenState && (
                <>
                  <button
                    onClick={() => setIsHiddenStateExpanded(!isHiddenStateExpanded)}
                    className="flex items-center gap-2 font-medium text-gray-800 dark:text-gray-200 text-sm hover:text-gray-900 dark:hover:text-white py-2 mt-4 transition-colors"
                  >
                    {isHiddenStateExpanded ? (
                      <ChevronDown className="h-4 w-4 text-gray-500 dark:text-gray-400" />
                    ) : (
                      <ChevronRight className="h-4 w-4 text-gray-500 dark:text-gray-400" />
                    )}
                    Hidden State
                    <span className="text-xs text-gray-400 dark:text-gray-500 ml-1">({Object.keys(want.hidden_state).length})</span>
                  </button>
                  {isHiddenStateExpanded && (
                    <div className={SECTION_CONTAINER_CLASS}>
                      <div className="space-y-1 sm:space-y-2">
                        {renderKeyValuePairs(want.hidden_state)}
                      </div>
                    </div>
                  )}
                </>
              )}
            </div>
          ) : (
            <div className="text-center py-12">
              <Database className="h-12 w-12 text-gray-400 mx-auto mb-4" />
              <p className="text-gray-500">No state data available</p>
              <p className="text-xs text-gray-400 mt-2">State will appear here once the want executes</p>
            </div>
          )}
        </div>
      </div>
      {onClearState && (hasAnyState || hasHiddenState) && (
        <div className="flex-shrink-0 flex justify-end px-3 sm:px-4 py-1 border-t border-gray-100 dark:border-gray-800">
          <button
            onClick={onClearState}
            className="p-1.5 rounded-md text-gray-400 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 transition-colors"
            title="Clear all state data"
          >
            <Eraser className="h-4 w-4" />
          </button>
        </div>
      )}
    </div>
  );
};

const AgentsTab: React.FC<{ want: Want }> = ({ want }) => {
  const agentHistory = want.history?.agentHistory ?? [];
  const hasActivity = want.current_agent ||
    (want.running_agents && want.running_agents.length > 0) ||
    agentHistory.length > 0;

  const statusDot = (status: string) => classNames(
    'w-2 h-2 rounded-full flex-shrink-0',
    (status === 'achieved' || status === 'achieved_with_warning')   && 'bg-green-500',
    status === 'failed'     && 'bg-red-500',
    status === 'running'    && 'bg-blue-500 animate-pulse',
    status === 'terminated' && 'bg-gray-500',
  );

  const agentTypeBadge = (type: string) => classNames(
    'text-xs px-1.5 py-0.5 rounded font-medium',
    type === 'do'      && 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
    type === 'monitor' && 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300',
    type === 'think'   && 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300',
    !['do', 'monitor', 'think'].includes(type) && 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400',
  );

  return (
    <div className="px-3 sm:px-4 pt-0 pb-3 sm:py-4 space-y-2">
      {/* Current Agent */}
      {want.current_agent && (
        <div className="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg p-3 sm:p-4">
          <div className="flex items-center">
            <Bot className="h-4 w-4 sm:h-5 sm:w-5 text-blue-600 dark:text-blue-400 mr-2" />
            <div>
              <h4 className="text-xs sm:text-sm font-medium text-blue-900 dark:text-blue-300">Current Agent</h4>
              <p className="text-xs sm:text-sm text-blue-700 dark:text-blue-400">{want.current_agent}</p>
            </div>
            <div className="ml-auto">
              <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse" />
            </div>
          </div>
        </div>
      )}

      {/* Running Agents */}
      {want.running_agents && want.running_agents.length > 0 && (
        <div className={SECTION_CONTAINER_CLASS}>
          <h4 className="text-sm font-medium text-gray-900 dark:text-white mb-3">Running Agents</h4>
          <div className="space-y-2">
            {want.running_agents.map((agent, index) => (
              <div key={index} className="flex items-center justify-between">
                <span className="text-sm text-gray-700 dark:text-gray-300">{agent}</span>
                <div className="w-2 h-2 bg-blue-500 rounded-full animate-pulse" />
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Agent Event Log */}
      {agentHistory.length > 0 && (
        <div className={SECTION_CONTAINER_CLASS}>
          <h4 className="text-sm font-medium text-gray-900 dark:text-white mb-3">Agent Execution Log</h4>
          <div className="space-y-2">
            {[...agentHistory].reverse().map((event, index) => (
              <div
                key={index}
                className="p-2 bg-gray-50 dark:bg-gray-900 rounded border border-gray-200 dark:border-gray-700 text-xs"
              >
                <div className="flex items-center justify-between mb-1">
                  <span className="font-medium text-gray-800 dark:text-gray-200">{event.agent_name}</span>
                  <div className="flex items-center space-x-2">
                    {event.agent_type && (
                      <span className={agentTypeBadge(event.agent_type)}>{event.agent_type}</span>
                    )}
                    <div className={statusDot(event.status)} title={event.status} />
                  </div>
                </div>
                {event.activity && (
                  <div className="mb-1">
                    <span className="inline-block bg-blue-100 dark:bg-blue-900/30 text-blue-800 dark:text-blue-300 px-2 py-0.5 rounded font-medium">
                      {event.activity}
                    </span>
                  </div>
                )}
                <div className="text-gray-500 dark:text-gray-400 space-y-0.5">
                  <div>{event.status} · {formatDate(event.timestamp)}</div>
                  {event.error && (
                    <div className="text-red-600 dark:text-red-400">Error: {event.error}</div>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {!hasActivity && (
        <div className="text-center py-8">
          <Bot className="h-12 w-12 text-gray-400 mx-auto mb-4" />
          <p className="text-gray-500">No agent information available</p>
        </div>
      )}
    </div>
  );
};


// Helper function to render JSON as itemized list
const renderStateAsItems = (obj: any, depth: number = 0): React.ReactNode[] => {
  const items: React.ReactNode[] = [];

  if (obj === null || obj === undefined) {
    return [<span key="null" className="text-gray-600 dark:text-gray-400">null</span>];
  }

  if (typeof obj !== 'object') {
    return [<span key="value">{String(obj)}</span>];
  }

  // Skip the opening braces and format as items
  Object.entries(obj).forEach(([key, value], index) => {
    const isNested = value !== null && typeof value === 'object' && !Array.isArray(value);
    const isArray = Array.isArray(value);

    if (isNested || isArray) {
      items.push(
        <div key={key} className={`${depth > 0 ? 'ml-4' : ''} mb-2`}>
          <div className="font-medium text-gray-800 dark:text-gray-200 text-xs mb-1">{key}:</div>
          <div className="ml-3 space-y-1">
            {renderStateAsItems(value, depth + 1)}
          </div>
        </div>
      );
    } else {
      items.push(
        <div key={key} className={`${depth > 0 ? 'ml-4' : ''} text-xs text-gray-700 dark:text-gray-300 mb-1`}>
          <span className="font-medium text-gray-800 dark:text-gray-200">{key}:</span> <span className="text-gray-600 dark:text-gray-400">{String(value)}</span>
        </div>
      );
    }
  });

  return items;
};

// Parameter History Item Component with expand/collapse
const ParameterHistoryItem: React.FC<{ entry: any; index: number }> = ({ entry, index }) => {
  const [isExpanded, setIsExpanded] = useState(false);
  const paramTimestamp = entry.timestamp;

  return (
    <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-md overflow-hidden">
      {/* Collapsed/Header View */}
      <button
        onClick={() => setIsExpanded(!isExpanded)}
        className="w-full px-4 py-3 flex items-center justify-between hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors"
      >
        <div className="flex items-center space-x-3 flex-1 text-left">
          {isExpanded ? (
            <ChevronDown className="h-4 w-4 text-gray-400 flex-shrink-0" />
          ) : (
            <ChevronRight className="h-4 w-4 text-gray-400 flex-shrink-0" />
          )}
          <div className="flex-1 min-w-0">
            {paramTimestamp && (
              <div className="text-xs text-gray-500 dark:text-gray-400">
                {formatDate(paramTimestamp)}
              </div>
            )}
          </div>
        </div>
      </button>

      {/* Expanded View - Itemized Format */}
      {isExpanded && (
        <div className="border-t border-gray-200 dark:border-gray-700 px-4 py-3 bg-gray-50 dark:bg-gray-900">
          <div className="bg-white dark:bg-gray-800 rounded p-3 text-xs overflow-auto max-h-96 border dark:border-gray-700 space-y-2">
            {renderStateAsItems(entry.stateValue || {})}
          </div>
        </div>
      )}
    </div>
  );
};

// State History Item Component with expand/collapse
const StateHistoryItem: React.FC<{ state: any; index: number }> = ({ state, index }) => {
  const [isExpanded, setIsExpanded] = useState(false);

  // Extract flight_status and action_by_agent from stateValue
  const flightStatus = state.stateValue?.flight_status;
  const actionByAgent = state.stateValue?.action_by_agent;
  const stateTimestamp = state.timestamp;

  // Determine agent badge color based on action_by_agent type
  const isMonitorAgent = actionByAgent?.includes('Monitor');
  const agentBgColor = isMonitorAgent ? 'bg-green-100 dark:bg-green-900/30' : 'bg-blue-100 dark:bg-blue-900/30';
  const agentTextColor = isMonitorAgent ? 'text-green-700 dark:text-green-400' : 'text-blue-700 dark:text-blue-400';

  return (
    <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-md overflow-hidden">
      {/* Collapsed/Header View */}
      <button
        onClick={() => setIsExpanded(!isExpanded)}
        className="w-full px-4 py-1.5 flex items-center justify-between hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors"
      >
        <div className="flex items-center space-x-2 flex-1 text-left">
          {isExpanded ? (
            <ChevronDown className="h-4 w-4 text-gray-400 flex-shrink-0" />
          ) : (
            <ChevronRight className="h-4 w-4 text-gray-400 flex-shrink-0" />
          )}
          <div className="flex items-center space-x-1 min-w-0">
            <div className="text-xs font-medium text-gray-900 dark:text-white">
              #{index + 1}
            </div>
            {stateTimestamp && (
              <div className="text-xs text-gray-500 dark:text-gray-400">
                {formatRelativeTime(stateTimestamp)}
              </div>
            )}
          </div>
          {/* Agent Icon and Flight Status - Unified Badge */}
          {actionByAgent && !isExpanded && flightStatus && (
            <div className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full ${agentBgColor} ${agentTextColor}`}>
              <Bot className="h-3 w-3 flex-shrink-0" />
              <span className="text-xs font-medium">{flightStatus}</span>
            </div>
          )}
          {actionByAgent && !isExpanded && !flightStatus && (
            <div className={`inline-flex items-center px-2 py-0.5 rounded-full ${agentBgColor} ${agentTextColor}`}>
              <Bot className="h-3 w-3 flex-shrink-0" />
            </div>
          )}
        </div>
      </button>

      {/* Expanded View - Itemized Format (only stateValue contents) */}
      {isExpanded && (
        <div className="border-t border-gray-200 dark:border-gray-700 px-4 py-3 bg-gray-50 dark:bg-gray-900">
          <div className="bg-white dark:bg-gray-800 rounded p-3 text-xs overflow-auto max-h-96 border dark:border-gray-700 space-y-2">
            {renderStateAsItems(state.stateValue || {})}
          </div>
        </div>
      )}
    </div>
  );
};

const LogHistoryItem: React.FC<{ logEntry: any; index: number }> = ({ logEntry, index }) => {
  const [isExpanded, setIsExpanded] = useState(false);

  const logTimestamp = logEntry.timestamp;
  const logsText = logEntry.logs || '';
  // Split logs by newline for display
  const logLines = logsText.split('\n').filter((line: string) => line.trim().length > 0);

  return (
    <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-md overflow-hidden">
      {/* Collapsed/Header View */}
      <button
        onClick={() => setIsExpanded(!isExpanded)}
        className="w-full px-4 py-1.5 flex items-center justify-between hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors"
      >
        <div className="flex items-center space-x-2 flex-1 text-left">
          {isExpanded ? (
            <ChevronDown className="h-4 w-4 text-gray-400 flex-shrink-0" />
          ) : (
            <ChevronRight className="h-4 w-4 text-gray-400 flex-shrink-0" />
          )}
          <div className="flex items-center space-x-1 min-w-0">
            <div className="text-xs font-medium text-gray-900 dark:text-white">
              #{index + 1} - {logLines.length} line{logLines.length !== 1 ? 's' : ''}
            </div>
            {logTimestamp && (
              <div className="text-xs text-gray-500 dark:text-gray-400">
                {formatRelativeTime(logTimestamp)}
              </div>
            )}
          </div>
        </div>
      </button>

      {/* Expanded View - Display Logs */}
      {isExpanded && (
        <div className="border-t border-gray-200 dark:border-gray-700 px-4 py-3 bg-gray-50 dark:bg-gray-900">
          <div className="bg-white dark:bg-gray-800 rounded p-3 text-xs overflow-auto max-h-96 border dark:border-gray-700">
            <pre className="text-xs text-gray-800 dark:text-gray-200 whitespace-pre-wrap break-words font-mono">
              {logsText}
            </pre>
          </div>
        </div>
      )}
    </div>
  );
};

const LogsTab: React.FC<{ want: Want; results: any }> = ({ want, results }) => {
  const hasParameterHistory = want.history?.parameterHistory && want.history.parameterHistory.length > 0;
  const hasLogHistory = want.history?.logHistory && want.history.logHistory.length > 0;
  const hasLogs = results?.logs && results.logs.length > 0;

  return (
    <div className="px-3 sm:px-4 pt-0 pb-3 sm:py-4 space-y-2">
      {/* Parameter History Section */}
      {hasParameterHistory && (
        <div className={SECTION_CONTAINER_CLASS}>
          <h4 className="text-sm sm:text-base font-medium text-gray-900 dark:text-white mb-2 sm:mb-4">Parameter History</h4>
          <div className="space-y-2 sm:space-y-3">
            {want.history!.parameterHistory!.map((entry, index) => (
              <ParameterHistoryItem key={index} entry={entry} index={index} />
            ))}
          </div>
        </div>
      )}

      {/* Execution Logs Section */}
      {hasLogs && (
        <div className={SECTION_CONTAINER_CLASS}>
          <h4 className="text-base font-medium text-gray-900 dark:text-white mb-4">Execution Logs</h4>
          <div className="space-y-2">
            {results.logs.map((log: string, index: number) => (
              <div key={index} className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-md p-3">
                <pre className="text-xs text-gray-800 dark:text-gray-200 whitespace-pre-wrap break-words">
                  {log}
                </pre>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* State History Section */}
      {want.history?.stateHistory && want.history.stateHistory.length > 0 && (
        <div className={SECTION_CONTAINER_CLASS}>
          <h4 className="text-base font-medium text-gray-900 dark:text-white mb-4">State History</h4>
          <div className="space-y-0">
            {want.history.stateHistory.slice().reverse().map((state, index) => (
              <StateHistoryItem key={index} state={state} index={want.history.stateHistory.length - index - 1} />
            ))}
          </div>
        </div>
      )}

      {/* Log History Section */}
      {hasLogHistory && (
        <div className={SECTION_CONTAINER_CLASS}>
          <h4 className="text-base font-medium text-gray-900 dark:text-white mb-4">Log History</h4>
          <div className="space-y-0">
            {want.history!.logHistory!.slice().reverse().map((logEntry, index) => (
              <LogHistoryItem key={index} logEntry={logEntry} index={want.history!.logHistory!.length - index - 1} />
            ))}
          </div>
        </div>
      )}

      {/* Empty State */}
      {!hasParameterHistory && !hasLogs && !hasLogHistory && (!want.history?.stateHistory || want.history.stateHistory.length === 0) && (
        <div className="text-center py-8">
          <FileText className="h-12 w-12 text-gray-400 mx-auto mb-4" />
          <p className="text-gray-500">No logs or parameter history available</p>
        </div>
      )}
    </div>
  );
};

// ─── VersionsTab ─────────────────────────────────────────────────────────────

const VersionsTab: React.FC<{ seriesWants: Want[]; currentWantId?: string }> = ({
  seriesWants,
  currentWantId,
}) => {
  const sorted = [...seriesWants].sort(
    (a, b) => (b.metadata?.version ?? 1) - (a.metadata?.version ?? 1)
  );

  if (sorted.length === 0) {
    return (
      <div className="text-center py-8 px-4">
        <History className="h-12 w-12 text-gray-400 dark:text-gray-500 mx-auto mb-4" />
        <p className="text-gray-500 dark:text-gray-400">No version history available</p>
      </div>
    );
  }

  return (
    <div className="overflow-y-auto p-4 space-y-3">
      {sorted.map((want) => (
        <div
          key={want.metadata?.id}
          className={classNames(
            'rounded-lg border p-3 transition-colors',
            want.metadata?.id === currentWantId
              ? 'border-blue-500 bg-blue-50 dark:bg-blue-900/20'
              : 'border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800',
          )}
        >
          <div className="flex items-center justify-between mb-2">
            <span className="text-xs font-semibold text-gray-500 dark:text-gray-400">
              v{want.metadata?.version ?? 1}
              {want.metadata?.id === currentWantId && (
                <span className="ml-2 text-blue-600 dark:text-blue-400">(current)</span>
              )}
            </span>
            <StatusBadge status={want.status} showLabel={true} />
          </div>
          <WantCardContent
            want={want}
            isChild={true}
            onView={() => {}}
          />
        </div>
      ))}
    </div>
  );
};