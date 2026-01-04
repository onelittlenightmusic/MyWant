import React, { useEffect, useState, useCallback, useMemo, useRef } from 'react';
import { Settings, Eye, AlertTriangle, Clock, Bot, Save, Edit, FileText, ChevronDown, ChevronRight, X, Database, Plus } from 'lucide-react';
import { Want, WantExecutionStatus } from '@/types/want';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { ErrorDisplay } from '@/components/common/ErrorDisplay';
import { FormYamlToggle } from '@/components/common/FormYamlToggle';
import { YamlEditor } from '@/components/forms/YamlEditor';
import { LabelAutocomplete } from '@/components/forms/LabelAutocomplete';
import { LabelSelectorAutocomplete } from '@/components/forms/LabelSelectorAutocomplete';
import { useWantStore } from '@/stores/wantStore';
import { formatDate, formatDuration, formatRelativeTime, classNames, truncateText } from '@/utils/helpers';
import { stringifyYaml, validateYaml, validateYamlWithSpec, WantTypeDefinition } from '@/utils/yaml';
import { updateWantParameters, updateWantScheduling, updateWantLabels, updateWantDependencies } from '@/utils/wantUtils';
import { WantControlButtons } from '@/components/dashboard/WantControlButtons';
import { ParametersSection } from '@/components/forms/sections/ParametersSection';
import { LabelsSection } from '@/components/forms/sections/LabelsSection';
import { DependenciesSection } from '@/components/forms/sections/DependenciesSection';
import { SchedulingSection } from '@/components/forms/sections/SchedulingSection';
import {
  DetailsSidebar,
  TabContent,
  TabSection,
  TabGrid,
  EmptyState,
  InfoRow,
  TabConfig
} from './DetailsSidebar';

interface WantDetailsSidebarProps {
  want: Want | null;
  initialTab?: 'settings' | 'results' | 'logs' | 'agents';
  onWantUpdate?: () => void;
  onHeaderStateChange?: (state: { autoRefresh: boolean; loading: boolean; status: WantExecutionStatus }) => void;
  onRegisterHeaderActions?: (handlers: { handleRefresh: () => void; handleToggleAutoRefresh: () => void }) => void;
  onStart?: (want: Want) => void;
  onStop?: (want: Want) => void;
  onSuspend?: (want: Want) => void;
  onResume?: (want: Want) => void;
  onDelete?: (want: Want) => void;
}

type TabType = 'settings' | 'results' | 'logs' | 'agents';

// Unified section container styling for all metadata/state sections
const SECTION_CONTAINER_CLASS = 'border border-gray-200 rounded-lg bg-white bg-opacity-50 overflow-hidden';

export const WantDetailsSidebar: React.FC<WantDetailsSidebarProps> = ({
  want,
  initialTab = 'settings',
  onWantUpdate,
  onHeaderStateChange,
  onRegisterHeaderActions,
  onStart,
  onStop,
  onSuspend,
  onResume,
  onDelete
}) => {
  // Check if this is a flight want
  const isFlightWant = want?.metadata?.type === 'flight';

  // Memoize wantId to avoid dependency array issues
  const wantId = want?.metadata?.id || want?.id;

  const {
    selectedWantDetails,
    selectedWantResults,
    fetchWantDetails,
    fetchWantResults,
    fetchWants,
    updateWant,
    loading
  } = useWantStore();

  const [activeTab, setActiveTab] = useState<TabType>('settings');
  const [prevTabIndex, setPrevTabIndex] = useState(0);
  const [isInitialLoad, setIsInitialLoad] = useState(true);
  const [autoRefresh, setAutoRefresh] = useState(false);
  const [isEditing, setIsEditing] = useState(false);
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

  // Control panel logic (use want for status since it comes from the live dashboard state)
  const isRunning = want?.status === 'reaching';
  const isSuspended = want?.status === 'suspended';
  const isCompleted = want?.status === 'achieved';
  const isStopped = want?.status === 'stopped' || want?.status === 'created';
  const isFailed = want?.status === 'failed';

  // Ensure want exists before checking control states
  const canStart = !!want && (isStopped || isCompleted || isFailed || isSuspended);
  const canStop = !!want && isRunning && !isSuspended;
  const canSuspend = !!want && isRunning && !isSuspended;
  const canDelete = !!want;

  const handleStartClick = () => {
    if (want) {
      if (isSuspended && onResume) {
        onResume(want);
      } else if (canStart && onStart) {
        onStart(want);
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
    if (want && canStop && onStop) onStop(want);
  };

  const handleSuspendClick = () => {
    if (want && canSuspend && onSuspend) onSuspend(want);
  };

  const handleDeleteClick = () => {
    if (want && canDelete && onDelete) onDelete(want);
  };

  // Fetch details when want ID changes (not on every want object change)
  useEffect(() => {
    if (wantId) {
      fetchWantDetails(wantId);
      fetchWantResults(wantId);
    }
  }, [wantId, fetchWantDetails, fetchWantResults]);

  // Reset state when initialTab prop changes (from parent handling onViewResults)
  // Only depends on initialTab, not on want, to avoid infinite loops from polling
  useEffect(() => {
    if (want) {
      setIsEditing(false);
      setUpdateError(null);
      // Only reset tab if initialTab has changed from outside
      setActiveTab(initialTab);
    }
  }, [initialTab]);

  // Keyboard shortcuts for want control buttons
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Don't trigger if user is typing in an input/textarea
      const target = e.target as HTMLElement;
      const isInputElement =
        target.tagName === 'INPUT' ||
        target.tagName === 'TEXTAREA' ||
        target.isContentEditable;

      if (isInputElement) return;

      switch (e.key.toLowerCase()) {
        case 'd':
          // Delete button
          if (canDelete) {
            e.preventDefault();
            handleDeleteClick();
          }
          break;
        case 'p':
          // Start/Resume button (if available), otherwise Suspend button
          if (canStart) {
            e.preventDefault();
            handleStartClick();
          } else if (canSuspend) {
            e.preventDefault();
            handleSuspendClick();
          }
          break;
        case 'r':
          // Stop button (square icon - reset)
          if (canStop) {
            e.preventDefault();
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
    if (selectedWantDetails?.status === 'reaching' && !autoRefresh) {
      // Auto-enable only if currently disabled
      setAutoRefresh(true);
    }
  }, [selectedWantDetails?.status, autoRefresh]);

  // Auto refresh setup (only refresh specific want details, not the whole list)
  useEffect(() => {
    if (autoRefresh && wantId) {
      const interval = setInterval(() => {
        fetchWantDetails(wantId);
        fetchWantResults(wantId);
      }, 2000);

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
      const specResponse = await fetch(`http://localhost:8080/api/v1/want-types/${wantType}`);
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

  if (!want) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-center">
          <Eye className="h-12 w-12 text-gray-400 mx-auto mb-4" />
          <p className="text-gray-500">Select a want to view details</p>
        </div>
      </div>
    );
  }

  const wantDetails = selectedWantDetails || want;

  const tabs = [
    { id: 'settings' as TabType, label: 'Settings', icon: Settings },
    { id: 'results' as TabType, label: 'Results', icon: Database },
    { id: 'logs' as TabType, label: 'Logs', icon: FileText },
    { id: 'agents' as TabType, label: 'Agents', icon: Bot },
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

  return (
    <div className="h-full flex flex-col relative overflow-hidden">
      {/* Content container */}
      <div className="h-full flex flex-col relative z-10">
      {/* Control Panel Buttons - Icon Only, Minimal Height */}
      {want && (
        <div className="flex-shrink-0 border-b border-gray-200 px-4 py-2">
          <WantControlButtons
            onStart={handleStartClick}
            onStop={handleStopClick}
            onSuspend={handleSuspendClick}
            onDelete={handleDeleteClick}
            canStart={canStart}
            canStop={canStop}
            canSuspend={canSuspend}
            canDelete={canDelete}
            isSuspended={isSuspended}
            loading={loading}
          />
        </div>
      )}

      {/* Tab navigation */}
      <div className="border-b border-gray-200 px-8 py-4">
        <div className="flex space-x-1 bg-gray-100 rounded-lg p-1 overflow-hidden">
          {tabs.map((tab) => {
            const Icon = tab.icon;
            return (
              <button
                key={tab.id}
                onClick={() => handleTabChange(tab.id)}
                className={classNames(
                  'flex-1 flex items-center justify-center space-x-1 px-2 py-2 text-xs font-medium rounded-lg transition-colors whitespace-nowrap min-w-0',
                  activeTab === tab.id
                    ? 'bg-white text-blue-600 shadow-sm'
                    : 'text-gray-600 hover:text-gray-900'
                )}
              >
                <Icon className="h-4 w-4 flex-shrink-0" />
                <span className="truncate">{tab.label}</span>
              </button>
            );
          })}
        </div>
      </div>

      {/* Tab content */}
      <div className="flex-1 overflow-y-auto overflow-x-hidden relative">
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
                <ResultsTab want={wantDetails} />
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
                <ResultsTab want={wantDetails} />
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
          </>
        )}
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

  // Section collapsed states
  const [isParametersCollapsed, setIsParametersCollapsed] = useState(true);
  const [isLabelsCollapsed, setIsLabelsCollapsed] = useState(true);
  const [isDependenciesCollapsed, setIsDependenciesCollapsed] = useState(true);
  const [isSchedulingCollapsed, setIsSchedulingCollapsed] = useState(true);

  // Form data states
  const [params, setParams] = useState<Record<string, unknown>>(want.spec?.params || {});
  const [labels, setLabels] = useState<Record<string, string>>(want.metadata?.labels || {});
  const [using, setUsing] = useState<Array<Record<string, string>>>(want.spec?.using || []);
  const [when, setWhen] = useState<Array<{ at?: string; every: string }>>(want.spec?.when || []);

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

    try {
      await updateWantParameters(want.metadata.id, want, newParams, updateWant);
      onWantUpdate?.();
    } catch (error) {
      setLocalUpdateError(error instanceof Error ? error.message : 'Failed to update parameters');
      setParams(oldParams); // Revert on error
    }
  }, [want.metadata?.id, want, params, updateWant, onWantUpdate, setLocalUpdateError]);

  // Handler for label changes - saves to API
  const handleLabelsChange = useCallback(async (newLabels: Record<string, string>) => {
    if (!want.metadata?.id) return;

    const oldLabels = labels;
    setLabels(newLabels);

    try {
      await updateWantLabels(want.metadata.id, oldLabels, newLabels);
      onWantUpdate?.();
    } catch (error) {
      setLocalUpdateError(error instanceof Error ? error.message : 'Failed to update labels');
      setLabels(oldLabels); // Revert on error
    }
  }, [want.metadata?.id, labels, onWantUpdate, setLocalUpdateError]);

  // Handler for dependency changes - saves to API
  const handleDependenciesChange = useCallback(async (newUsing: Array<Record<string, string>>) => {
    if (!want.metadata?.id) return;

    const oldUsing = using;
    setUsing(newUsing);

    try {
      await updateWantDependencies(want.metadata.id, oldUsing, newUsing);
      onWantUpdate?.();
    } catch (error) {
      setLocalUpdateError(error instanceof Error ? error.message : 'Failed to update dependencies');
      setUsing(oldUsing); // Revert on error
    }
  }, [want.metadata?.id, using, onWantUpdate, setLocalUpdateError]);

  // Handler for scheduling changes - saves to API
  const handleSchedulingChange = useCallback(async (newWhen: Array<{ at?: string; every: string }>) => {
    if (!want.metadata?.id) return;

    const oldWhen = when;
    setWhen(newWhen);

    try {
      await updateWantScheduling(want.metadata.id, want, newWhen, updateWant);
      onWantUpdate?.();
    } catch (error) {
      setLocalUpdateError(error instanceof Error ? error.message : 'Failed to update scheduling');
      setWhen(oldWhen); // Revert on error
    }
  }, [want.metadata?.id, want, when, updateWant, onWantUpdate, setLocalUpdateError]);

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

  // Reset form state when want changes or metadata is updated (including labels/dependencies)
  useEffect(() => {
    setParams(want.spec?.params || {});
    setLabels(want.metadata?.labels || {});
    setUsing(want.spec?.using || []);
    setWhen(want.spec?.when || []);
    // Reset collapsed states
    setIsParametersCollapsed(true);
    setIsLabelsCollapsed(true);
    setIsDependenciesCollapsed(true);
    setIsSchedulingCollapsed(true);
  }, [want.metadata?.id, want.metadata?.updatedAt]);

  return (
    <div className="h-full flex flex-col">
      {/* Config/Overview Toggle */}
      <div className="flex-shrink-0 px-8 py-4 flex justify-end">
        <FormYamlToggle
          mode={configMode}
          onModeChange={onConfigModeChange}
        />
      </div>

      {/* Content Area */}
      <div className="flex-1 overflow-y-auto p-8 focusable-container">
        {configMode === 'form' ? (
          <div className="space-y-2">
            {/* Metadata Section */}
            <div className={SECTION_CONTAINER_CLASS}>
              <h4 className="text-base font-medium text-gray-900 mb-4">Metadata</h4>
              <div className="space-y-3">
                <div className="flex justify-between items-center">
                  <span className="text-gray-600 text-sm">Name:</span>
                  <span className="font-medium text-sm">{want.metadata?.name || 'N/A'}</span>
                </div>
                <div className="flex justify-between items-center">
                  <span className="text-gray-600 text-sm">Type:</span>
                  <span className="font-medium text-sm">{want.metadata?.type || 'N/A'}</span>
                </div>
                <div className="flex justify-between items-center">
                  <span className="text-gray-600 text-sm">ID:</span>
                  <span className="font-mono text-xs break-all">{want.metadata?.id || want.id || 'N/A'}</span>
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
                <h4 className="text-base font-medium text-gray-900 mb-4">Timeline</h4>
                <div className="space-y-3">
                  {want.stats.created_at && (
                    <div className="flex justify-between items-center">
                      <span className="text-gray-600 text-sm">Created:</span>
                      <span className="text-sm">{formatDate(want.stats.created_at)}</span>
                    </div>
                  )}
                  {want.stats.started_at && (
                    <div className="flex justify-between items-center">
                      <span className="text-gray-600 text-sm">Started:</span>
                      <span className="text-sm">{formatDate(want.stats.started_at)}</span>
                    </div>
                  )}
                  {want.stats.completed_at && (
                    <div className="flex justify-between items-center">
                      <span className="text-gray-600 text-sm">Achieved:</span>
                      <span className="text-sm">{formatDate(want.stats.completed_at)}</span>
                    </div>
                  )}
                  {want.stats.started_at && (
                    <div className="flex justify-between items-center">
                      <span className="text-gray-600 text-sm">Duration:</span>
                      <span className="text-sm">{formatDuration(want.stats.started_at, want.stats.completed_at)}</span>
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
            {want.status === 'failed' && want.state?.error && (
              <div className="bg-red-50 border border-red-200 rounded-lg p-6">
                <div className="flex items-start">
                  <AlertTriangle className="h-5 w-5 text-red-600 mt-0.5 mr-3 flex-shrink-0" />
                  <div className="flex-1 min-w-0">
                    <h4 className="text-base font-medium text-red-800 mb-3">Error Details</h4>
                    <p className="text-sm text-red-600 break-words leading-relaxed">
                      {typeof want.state.error === 'string' ? want.state.error : JSON.stringify(want.state.error)}
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
                  <h4 className="text-sm font-medium text-gray-900">Configuration</h4>
                  <button
                    onClick={onEdit}
                    className="inline-flex items-center px-3 py-1.5 border border-gray-300 shadow-sm text-xs font-medium rounded text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
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
                  <h4 className="text-sm font-medium text-gray-900">Edit Configuration</h4>
                  <div className="flex space-x-2">
                    <button
                      onClick={onCancel}
                      disabled={updateLoading}
                      className="inline-flex items-center px-3 py-1.5 border border-gray-300 shadow-sm text-xs font-medium rounded text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50"
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
        className="flex items-center gap-2 font-medium text-gray-800 text-sm hover:text-gray-900 py-1"
      >
        {isExpanded ? (
          <ChevronDown className="h-4 w-4 text-gray-500" />
        ) : (
          <ChevronRight className="h-4 w-4 text-gray-500" />
        )}
        {label}:
        {!isExpanded && <span className="text-xs text-gray-400 ml-1">Array({items.length})</span>}
      </button>
      {isExpanded && (
        <div className="ml-4 border-l border-gray-200 pl-3 space-y-2">
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
      <div className="text-sm text-gray-700 font-mono ml-4">
        [{index}]: {String(item)}
      </div>
    );
  }

  return (
    <div className="border-l border-gray-300 pl-3 ml-2">
      <button
        onClick={() => setIsExpanded(!isExpanded)}
        className="flex items-center gap-2 text-sm text-gray-700 hover:text-gray-900 py-1"
      >
        {isExpanded ? (
          <ChevronDown className="h-4 w-4 text-gray-500" />
        ) : (
          <ChevronRight className="h-4 w-4 text-gray-500" />
        )}
        <span className="text-xs text-gray-500">[{index}]</span>
        {!isExpanded && (
          <span className="text-xs text-gray-400">
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

// Helper to render key-value pairs recursively
const renderKeyValuePairs = (obj: any, depth: number = 0): React.ReactNode[] => {
  const items: React.ReactNode[] = [];

  if (obj === null || obj === undefined) {
    return [<span key="null" className="text-gray-500 italic">null</span>];
  }

  if (typeof obj !== 'object') {
    return [
      <span key="value" className="text-gray-700 font-mono break-all">
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
          <div className="font-medium text-gray-800 text-sm">{key}:</div>
          <div className="ml-4 border-l border-gray-200 pl-3">
            {renderKeyValuePairs(value, depth + 1)}
          </div>
        </div>
      );
    } else {
      // For simple key-value pairs, display in left-right format with dots
      const valueStr = String(value);
      // Calculate spacing to fill with dots
      const keyLength = key.length;
      const valueLength = valueStr.length;
      const minDots = 3;
      // Estimate: key (small) + value (large) + spacing
      const totalAvailableChars = 50; // Approximate character width available
      const dotsNeeded = Math.max(minDots, totalAvailableChars - keyLength - valueLength);
      const dots = '.'.repeat(Math.max(minDots, Math.min(dotsNeeded, 30)));

      items.push(
        <div key={key} className="flex justify-between items-center text-sm gap-2">
          <span className="text-gray-600 font-normal text-xs whitespace-nowrap">{key}</span>
          <div className="flex items-center gap-1 flex-shrink-0">
            <span className="text-gray-400 text-xs">{dots}</span>
            <span className="text-gray-800 font-semibold text-base whitespace-nowrap">{valueStr}</span>
          </div>
        </div>
      );
    }
  });

  return items;
};

const ResultsTab: React.FC<{ want: Want }> = ({ want }) => {
  const hasState = want.state && Object.keys(want.state).length > 0;
  const hasHiddenState = want.hidden_state && Object.keys(want.hidden_state).length > 0;
  const [isHiddenStateExpanded, setIsHiddenStateExpanded] = useState(false);

  return (
    <div className="p-8 h-full overflow-y-auto">
      {hasState || hasHiddenState ? (
        <div className="space-y-2">
          {hasState && (
            <div className={SECTION_CONTAINER_CLASS}>
              <h4 className="text-base font-medium text-gray-900 mb-4">Want State</h4>
              <div className="space-y-4">
                {renderKeyValuePairs(want.state)}
              </div>
            </div>
          )}

          {hasHiddenState && (
            <>
              <button
                onClick={() => setIsHiddenStateExpanded(!isHiddenStateExpanded)}
                className="flex items-center gap-2 font-medium text-gray-800 text-sm hover:text-gray-900 py-2 mt-4"
              >
                {isHiddenStateExpanded ? (
                  <ChevronDown className="h-4 w-4 text-gray-500" />
                ) : (
                  <ChevronRight className="h-4 w-4 text-gray-500" />
                )}
                Hidden State
                <span className="text-xs text-gray-400 ml-1">({Object.keys(want.hidden_state).length})</span>
              </button>
              {isHiddenStateExpanded && (
                <div className={SECTION_CONTAINER_CLASS}>
                  <div className="space-y-4">
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
  );
};

interface AgentExecution {
  agent_name: string;
  agent_type: string;
  start_time: string;
  end_time?: string;
  status: string;
  error?: string;
  activity?: string; // Description of agent action performed
}

const AgentsTab: React.FC<{ want: Want }> = ({ want }) => {
  const [groupedAgents, setGroupedAgents] = useState<Record<string, AgentExecution[]> | null>(null);
  const [groupBy, setGroupBy] = useState<'name' | 'type'>('name');
  const [loadingGrouped, setLoadingGrouped] = useState(false);
  const [expandedGroups, setExpandedGroups] = useState<Record<string, boolean>>({});

  // Fetch grouped agent history
  useEffect(() => {
    const fetchGroupedAgents = async () => {
      if (want?.metadata?.id) {
        setLoadingGrouped(true);
        try {
          const response = await fetch(
            `http://localhost:8080/api/v1/wants/${want.metadata.id}?groupBy=${groupBy}`
          );
          if (response.ok) {
            const data = await response.json();
            if (data.history?.groupedAgentHistory) {
              setGroupedAgents(data.history.groupedAgentHistory);
              // Auto-expand first group
              const groups = Object.keys(data.history.groupedAgentHistory);
              if (groups.length > 0) {
                setExpandedGroups({ [groups[0]]: true });
              }
            }
          }
        } catch (error) {
          console.error('Failed to fetch grouped agents:', error);
        } finally {
          setLoadingGrouped(false);
        }
      }
    };

    fetchGroupedAgents();
  }, [want?.metadata?.id, groupBy]);

  const toggleGroup = (groupName: string) => {
    setExpandedGroups(prev => ({
      ...prev,
      [groupName]: !prev[groupName]
    }));
  };

  return (
    <div className="p-8 space-y-2">
      {/* Current Agent */}
      {want.current_agent && (
        <div className="bg-blue-50 border border-blue-200 rounded-lg p-4">
          <div className="flex items-center">
            <Bot className="h-5 w-5 text-blue-600 mr-2" />
            <div>
              <h4 className="text-sm font-medium text-blue-900">Current Agent</h4>
              <p className="text-sm text-blue-700">{want.current_agent}</p>
            </div>
            <div className="ml-auto">
              <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse" title="Reaching" />
            </div>
          </div>
        </div>
      )}

      {/* Running Agents */}
      {want.running_agents && want.running_agents.length > 0 && (
        <div className={SECTION_CONTAINER_CLASS}>
          <h4 className="text-sm font-medium text-gray-900 mb-3">Running Agents</h4>
          <div className="space-y-2">
            {want.running_agents.map((agent, index) => (
              <div key={index} className="flex items-center justify-between">
                <span className="text-sm text-gray-700">{agent}</span>
                <div className="w-2 h-2 bg-blue-500 rounded-full animate-pulse" />
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Grouped Agent History */}
      {groupedAgents && Object.keys(groupedAgents).length > 0 && (
        <div className={SECTION_CONTAINER_CLASS}>
          <div className="flex items-center justify-between mb-4">
            <h4 className="text-sm font-medium text-gray-900">Agent Execution History</h4>
            <div className="flex space-x-2">
              <button
                onClick={() => setGroupBy('name')}
                className={classNames(
                  'px-3 py-1 text-xs rounded-md font-medium transition-colors',
                  groupBy === 'name'
                    ? 'bg-blue-600 text-white'
                    : 'bg-gray-300 text-gray-700 hover:bg-gray-400'
                )}
              >
                By Name
              </button>
              <button
                onClick={() => setGroupBy('type')}
                className={classNames(
                  'px-3 py-1 text-xs rounded-md font-medium transition-colors',
                  groupBy === 'type'
                    ? 'bg-blue-600 text-white'
                    : 'bg-gray-300 text-gray-700 hover:bg-gray-400'
                )}
              >
                By Type
              </button>
            </div>
          </div>

          {loadingGrouped && (
            <div className="flex items-center justify-center py-4">
              <LoadingSpinner />
            </div>
          )}

          {!loadingGrouped && (
            <div className="space-y-3">
              {Object.entries(groupedAgents).map(([groupName, executions]) => (
                <div key={groupName} className="border border-gray-200 rounded-md overflow-hidden">
                  {/* Group Header */}
                  <button
                    onClick={() => toggleGroup(groupName)}
                    className="w-full flex items-center justify-between px-3 py-2 bg-white hover:bg-gray-50 transition-colors"
                  >
                    <div className="flex items-center space-x-2">
                      {expandedGroups[groupName] ? (
                        <ChevronDown className="h-4 w-4 text-gray-600" />
                      ) : (
                        <ChevronRight className="h-4 w-4 text-gray-600" />
                      )}
                      <span className="font-medium text-sm text-gray-900">{groupName}</span>
                      <span className="text-xs text-gray-500">({executions.length})</span>
                    </div>
                  </button>

                  {/* Group Content */}
                  {expandedGroups[groupName] && (
                    <div className="px-3 py-2 bg-white border-t border-gray-200 space-y-2">
                      {executions.map((execution, index) => (
                        <div
                          key={index}
                          className="p-2 bg-gray-50 rounded border border-gray-200 text-xs"
                        >
                          <div className="flex items-center justify-between mb-2">
                            <span className="font-medium text-gray-800">{execution.agent_name}</span>
                            <div className="flex items-center space-x-2">
                              <span className="text-gray-500 text-xs">{execution.agent_type}</span>
                              <div className={classNames(
                                'w-2 h-2 rounded-full',
                                execution.status === 'achieved' && 'bg-green-500',
                                execution.status === 'failed' && 'bg-red-500',
                                execution.status === 'reaching' && 'bg-blue-500 animate-pulse',
                                execution.status === 'terminated' && 'bg-gray-500'
                              )} />
                            </div>
                          </div>

                          {/* Activity Label */}
                          {execution.activity && (
                            <div className="mb-2">
                              <span className="inline-block bg-blue-100 text-blue-800 px-2 py-1 rounded text-xs font-medium">
                                {execution.activity}
                              </span>
                            </div>
                          )}

                          <div className="text-gray-600 space-y-1">
                            <div>Start: {formatDate(execution.start_time)}</div>
                            {execution.end_time && (
                              <div>End: {formatDate(execution.end_time)}</div>
                            )}
                            {execution.error && (
                              <div className="text-red-600">Error: {execution.error}</div>
                            )}
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {!want.current_agent && (!want.running_agents || want.running_agents.length === 0) && (!groupedAgents || Object.keys(groupedAgents).length === 0) && (
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
    return [<span key="null" className="text-gray-600">null</span>];
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
          <div className="font-medium text-gray-800 text-xs mb-1">{key}:</div>
          <div className="ml-3 space-y-1">
            {renderStateAsItems(value, depth + 1)}
          </div>
        </div>
      );
    } else {
      items.push(
        <div key={key} className={`${depth > 0 ? 'ml-4' : ''} text-xs text-gray-700 mb-1`}>
          <span className="font-medium text-gray-800">{key}:</span> <span className="text-gray-600">{String(value)}</span>
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
    <div className="bg-white border border-gray-200 rounded-md overflow-hidden">
      {/* Collapsed/Header View */}
      <button
        onClick={() => setIsExpanded(!isExpanded)}
        className="w-full px-4 py-3 flex items-center justify-between hover:bg-gray-50 transition-colors"
      >
        <div className="flex items-center space-x-3 flex-1 text-left">
          {isExpanded ? (
            <ChevronDown className="h-4 w-4 text-gray-400 flex-shrink-0" />
          ) : (
            <ChevronRight className="h-4 w-4 text-gray-400 flex-shrink-0" />
          )}
          <div className="flex-1 min-w-0">
            {paramTimestamp && (
              <div className="text-xs text-gray-500">
                {formatDate(paramTimestamp)}
              </div>
            )}
          </div>
        </div>
      </button>

      {/* Expanded View - Itemized Format */}
      {isExpanded && (
        <div className="border-t border-gray-200 px-4 py-3 bg-gray-50">
          <div className="bg-white rounded p-3 text-xs overflow-auto max-h-96 border space-y-2">
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
  const agentBgColor = isMonitorAgent ? 'bg-green-100' : 'bg-blue-100';
  const agentTextColor = isMonitorAgent ? 'text-green-700' : 'text-blue-700';

  return (
    <div className="bg-white border border-gray-200 rounded-md overflow-hidden">
      {/* Collapsed/Header View */}
      <button
        onClick={() => setIsExpanded(!isExpanded)}
        className="w-full px-4 py-1.5 flex items-center justify-between hover:bg-gray-50 transition-colors"
      >
        <div className="flex items-center space-x-2 flex-1 text-left">
          {isExpanded ? (
            <ChevronDown className="h-4 w-4 text-gray-400 flex-shrink-0" />
          ) : (
            <ChevronRight className="h-4 w-4 text-gray-400 flex-shrink-0" />
          )}
          <div className="flex items-center space-x-1 min-w-0">
            <div className="text-xs font-medium text-gray-900">
              #{index + 1}
            </div>
            {stateTimestamp && (
              <div className="text-xs text-gray-500">
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
        <div className="border-t border-gray-200 px-4 py-3 bg-gray-50">
          <div className="bg-white rounded p-3 text-xs overflow-auto max-h-96 border space-y-2">
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
    <div className="bg-white border border-gray-200 rounded-md overflow-hidden">
      {/* Collapsed/Header View */}
      <button
        onClick={() => setIsExpanded(!isExpanded)}
        className="w-full px-4 py-1.5 flex items-center justify-between hover:bg-gray-50 transition-colors"
      >
        <div className="flex items-center space-x-2 flex-1 text-left">
          {isExpanded ? (
            <ChevronDown className="h-4 w-4 text-gray-400 flex-shrink-0" />
          ) : (
            <ChevronRight className="h-4 w-4 text-gray-400 flex-shrink-0" />
          )}
          <div className="flex items-center space-x-1 min-w-0">
            <div className="text-xs font-medium text-gray-900">
              #{index + 1} - {logLines.length} line{logLines.length !== 1 ? 's' : ''}
            </div>
            {logTimestamp && (
              <div className="text-xs text-gray-500">
                {formatRelativeTime(logTimestamp)}
              </div>
            )}
          </div>
        </div>
      </button>

      {/* Expanded View - Display Logs */}
      {isExpanded && (
        <div className="border-t border-gray-200 px-4 py-3 bg-gray-50">
          <div className="bg-white rounded p-3 text-xs overflow-auto max-h-96 border">
            <pre className="text-xs text-gray-800 whitespace-pre-wrap break-words font-mono">
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
    <div className="p-8 space-y-2">
      {/* Parameter History Section */}
      {hasParameterHistory && (
        <div className={SECTION_CONTAINER_CLASS}>
          <h4 className="text-base font-medium text-gray-900 mb-4">Parameter History</h4>
          <div className="space-y-3">
            {want.history!.parameterHistory!.map((entry, index) => (
              <ParameterHistoryItem key={index} entry={entry} index={index} />
            ))}
          </div>
        </div>
      )}

      {/* Execution Logs Section */}
      {hasLogs && (
        <div className={SECTION_CONTAINER_CLASS}>
          <h4 className="text-base font-medium text-gray-900 mb-4">Execution Logs</h4>
          <div className="space-y-2">
            {results.logs.map((log: string, index: number) => (
              <div key={index} className="bg-white border border-gray-200 rounded-md p-3">
                <pre className="text-xs text-gray-800 whitespace-pre-wrap break-words">
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
          <h4 className="text-base font-medium text-gray-900 mb-4">State History</h4>
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
          <h4 className="text-base font-medium text-gray-900 mb-4">Log History</h4>
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