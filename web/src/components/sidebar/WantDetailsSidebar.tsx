import React, { useEffect, useState } from 'react';
import { RefreshCw, Eye, AlertTriangle, User, Users, Clock, CheckCircle, XCircle, Minus, Bot, Save, Edit, FileText, ChevronDown, ChevronRight, X, Play, Pause, Square, Trash2 } from 'lucide-react';
import { Want, WantExecutionStatus } from '@/types/want';
import { StatusBadge } from '@/components/common/StatusBadge';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { ErrorDisplay } from '@/components/common/ErrorDisplay';
import { YamlEditor } from '@/components/forms/YamlEditor';
import { LabelAutocomplete } from '@/components/forms/LabelAutocomplete';
import { LabelSelectorAutocomplete } from '@/components/forms/LabelSelectorAutocomplete';
import { useWantStore } from '@/stores/wantStore';
import { formatDate, formatDuration, classNames } from '@/utils/helpers';
import { stringifyYaml, validateYaml, validateYamlWithSpec, WantTypeDefinition } from '@/utils/yaml';
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
  initialTab?: 'overview' | 'config' | 'logs' | 'agents';
  onWantUpdate?: () => void;
  onHeaderStateChange?: (state: { autoRefresh: boolean; loading: boolean; status: WantExecutionStatus }) => void;
  onStart?: (want: Want) => void;
  onStop?: (want: Want) => void;
  onSuspend?: (want: Want) => void;
  onResume?: (want: Want) => void;
  onDelete?: (want: Want) => void;
}

type TabType = 'overview' | 'config' | 'logs' | 'agents';

export const WantDetailsSidebar: React.FC<WantDetailsSidebarProps> = ({
  want,
  initialTab = 'overview',
  onWantUpdate,
  onHeaderStateChange,
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

  const [activeTab, setActiveTab] = useState<TabType>('overview');
  const [autoRefresh, setAutoRefresh] = useState(false);
  const [isEditing, setIsEditing] = useState(false);
  const [editedConfig, setEditedConfig] = useState<string>('');
  const [updateLoading, setUpdateLoading] = useState(false);
  const [updateError, setUpdateError] = useState<string | null>(null);

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

  // Reset state when want ID changes (not on every want object change from polling)
  useEffect(() => {
    if (want) {
      setIsEditing(false);
      setUpdateError(null);
      // Only reset tab if initialTab has changed from outside
      setActiveTab(initialTab);
    }
  }, [initialTab]);

  // Auto-enable refresh for running wants
  useEffect(() => {
    if (want && selectedWantDetails && selectedWantDetails.status === 'reaching') {
      setAutoRefresh(true);
    } else if (want && selectedWantDetails && selectedWantDetails.status !== 'reaching' && autoRefresh) {
      // Auto-disable when want stops running
      setAutoRefresh(false);
    }
  }, [want, selectedWantDetails?.status]);

  // Auto refresh setup (only refresh specific want details, not the whole list)
  useEffect(() => {
    if (autoRefresh && wantId) {
      const interval = setInterval(() => {
        fetchWantDetails(wantId);
        fetchWantResults(wantId);
      }, 5000);

      return () => clearInterval(interval);
    }
  }, [autoRefresh, wantId, fetchWantDetails, fetchWantResults]);

  const handleRefresh = () => {
    if (want) {
      const wantId = want.metadata?.id || want.id;
      if (wantId) {
        fetchWantDetails(wantId);
        fetchWantResults(wantId);
        fetchWants();
      }
    }
  };

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

  // Notify parent of header state changes - must be before early return to keep hook order consistent
  useEffect(() => {
    if (want && selectedWantDetails) {
      onHeaderStateChange?.({
        autoRefresh,
        loading,
        status: (selectedWantDetails.status as WantExecutionStatus) || 'created'
      });
    } else if (want) {
      onHeaderStateChange?.({
        autoRefresh,
        loading,
        status: (want.status as WantExecutionStatus) || 'created'
      });
    }
  }, [autoRefresh, loading, want, selectedWantDetails, onHeaderStateChange]);

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
    { id: 'overview' as TabType, label: 'Overview', icon: Eye },
    { id: 'config' as TabType, label: 'Config', icon: Edit },
    { id: 'logs' as TabType, label: 'Logs', icon: FileText },
    { id: 'agents' as TabType, label: 'Agents', icon: Bot },
  ];

  return (
    <div className="h-full flex flex-col">
      {/* Control Panel Buttons - Icon Only, Minimal Height */}
      {want && (
        <div className="flex-shrink-0 border-b border-gray-200 px-4 py-2 flex gap-1 justify-center">
          {/* Start / Resume */}
          <button
            onClick={handleStartClick}
            disabled={!canStart || loading}
            title={canStart ? (isSuspended ? 'Resume execution' : 'Start execution') : 'Cannot start in current state'}
            className={classNames(
              'p-2 rounded-md transition-colors',
              canStart && !loading
                ? 'bg-green-100 text-green-600 hover:bg-green-200'
                : 'bg-gray-100 text-gray-400 cursor-not-allowed'
            )}
          >
            <Play className="h-4 w-4" />
          </button>

          {/* Suspend */}
          {canSuspend && (
            <button
              onClick={handleSuspendClick}
              disabled={!canSuspend || loading}
              title="Suspend execution"
              className="p-2 rounded-md transition-colors bg-orange-100 text-orange-600 hover:bg-orange-200"
            >
              <Pause className="h-4 w-4" />
            </button>
          )}

          {/* Stop */}
          <button
            onClick={handleStopClick}
            disabled={!canStop || loading}
            title={canStop ? 'Stop execution' : 'Cannot stop in current state'}
            className={classNames(
              'p-2 rounded-md transition-colors',
              canStop && !loading
                ? 'bg-red-100 text-red-600 hover:bg-red-200'
                : 'bg-gray-100 text-gray-400 cursor-not-allowed'
            )}
          >
            <Square className="h-4 w-4" />
          </button>

          {/* Delete */}
          <button
            onClick={handleDeleteClick}
            disabled={!canDelete || loading}
            title={canDelete ? 'Delete want' : 'No want selected'}
            className={classNames(
              'p-2 rounded-md transition-colors',
              canDelete && !loading
                ? 'bg-gray-100 text-gray-600 hover:bg-gray-200'
                : 'bg-gray-100 text-gray-400 cursor-not-allowed'
            )}
          >
            <Trash2 className="h-4 w-4" />
          </button>
        </div>
      )}

      {/* Tab navigation */}
      <div className="border-b border-gray-200 px-8 py-4">
        <div className="flex space-x-1 bg-gray-100 rounded-lg p-1">
          {tabs.map((tab) => {
            const Icon = tab.icon;
            return (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={classNames(
                  'flex-1 flex items-center justify-center space-x-1 px-3 py-2 text-sm font-medium rounded-lg transition-colors',
                  activeTab === tab.id
                    ? 'bg-white text-blue-600 shadow-sm'
                    : 'text-gray-600 hover:text-gray-900'
                )}
              >
                <Icon className="h-4 w-4" />
                <span className="inline">{tab.label}</span>
              </button>
            );
          })}
        </div>
      </div>

      {/* Tab content */}
      <div className="flex-1 overflow-y-auto">
        {loading && !selectedWantDetails ? (
          <div className="flex items-center justify-center py-12">
            <LoadingSpinner size="lg" />
          </div>
        ) : (
          <>
            {activeTab === 'overview' && (
              <OverviewTab
                want={wantDetails}
                onWantUpdate={() => {
                  const wantId = want.metadata?.id || want.id;
                  if (wantId) {
                    fetchWantDetails(wantId);
                    fetchWants();
                  }
                }}
              />
            )}

            {activeTab === 'config' && (
              <ConfigTab
                want={wantDetails}
                isEditing={isEditing}
                editedConfig={editedConfig}
                updateLoading={updateLoading}
                updateError={updateError}
                onEdit={handleEditConfig}
                onSave={handleSaveConfig}
                onCancel={handleCancelEdit}
                onConfigChange={setEditedConfig}
              />
            )}

            {activeTab === 'logs' && (
              <LogsTab want={wantDetails} results={selectedWantResults} />
            )}

            {activeTab === 'agents' && (
              <AgentsTab want={wantDetails} />
            )}
          </>
        )}
      </div>
    </div>
  );
};

// Tab Components
const OverviewTab: React.FC<{ want: Want; onWantUpdate?: () => void }> = ({ want, onWantUpdate }) => {
  const [editingLabelKey, setEditingLabelKey] = useState<string | null>(null);
  const [editingLabelDraft, setEditingLabelDraft] = useState<{ key: string; value: string }>({ key: '', value: '' });
  const [editingUsingIndex, setEditingUsingIndex] = useState<number | null>(null);
  const [editingUsingDraft, setEditingUsingDraft] = useState<{ key: string; value: string }>({ key: '', value: '' });
  const [updateLoading, setUpdateLoading] = useState(false);
  const [updateError, setUpdateError] = useState<string | null>(null);

  const handleSaveLabel = async (oldKey: string) => {
    if (!editingLabelDraft.key.trim() || !want.metadata?.id) return;

    setUpdateLoading(true);
    setUpdateError(null);

    try {
      const newLabels = { ...(want.metadata.labels || {}) };
      if (oldKey !== editingLabelDraft.key) {
        delete newLabels[oldKey];
      }
      newLabels[editingLabelDraft.key] = editingLabelDraft.value;

      const updatePayload = {
        metadata: {
          ...want.metadata,
          labels: newLabels
        },
        spec: want.spec
      };

      const response = await fetch(`http://localhost:8080/api/v1/wants/${want.metadata.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(updatePayload)
      });

      if (!response.ok) {
        throw new Error('Failed to update label');
      }

      setEditingLabelKey(null);
      setEditingLabelDraft({ key: '', value: '' });
      onWantUpdate?.();
    } catch (error) {
      setUpdateError(error instanceof Error ? error.message : 'Failed to update label');
    } finally {
      setUpdateLoading(false);
    }
  };

  const handleRemoveLabel = async (key: string) => {
    if (!want.metadata?.id) return;

    setUpdateLoading(true);
    setUpdateError(null);

    try {
      const newLabels = { ...(want.metadata.labels || {}) };
      delete newLabels[key];

      const updatePayload = {
        metadata: {
          ...want.metadata,
          labels: newLabels
        },
        spec: want.spec
      };

      const response = await fetch(`http://localhost:8080/api/v1/wants/${want.metadata.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(updatePayload)
      });

      if (!response.ok) {
        throw new Error('Failed to remove label');
      }

      onWantUpdate?.();
    } catch (error) {
      setUpdateError(error instanceof Error ? error.message : 'Failed to remove label');
    } finally {
      setUpdateLoading(false);
    }
  };

  return (
    <div className="p-8">
      <div className="space-y-8">
        {/* Metadata Section */}
        <div className="bg-gray-50 rounded-lg p-6">
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

        {/* Labels */}
        {want.metadata ? (
          <div className="bg-gray-50 rounded-lg p-6">
            <div className="flex items-center justify-between mb-4">
              <h4 className="text-base font-medium text-gray-900">Labels</h4>
              {editingLabelKey === null && (
                <button
                  onClick={() => {
                    setEditingLabelKey('__new__');
                    setEditingLabelDraft({ key: '', value: '' });
                  }}
                  disabled={updateLoading}
                  className="text-blue-600 hover:text-blue-800 text-sm font-medium disabled:opacity-50"
                >
                  +
                </button>
              )}
            </div>

            {/* Display existing labels as styled chips */}
            {want.metadata?.labels && Object.keys(want.metadata.labels).length > 0 && (
              <div className="flex flex-wrap gap-2 mb-4">
                {Object.entries(want.metadata.labels).map(([key, value]) => {
                  if (editingLabelKey === key) return null;
                  return (
                    <button
                      key={key}
                      type="button"
                      onClick={() => {
                        setEditingLabelKey(key);
                        setEditingLabelDraft({ key, value });
                      }}
                      className="inline-flex items-center px-3 py-1.5 rounded-full text-sm font-medium bg-blue-100 text-blue-800 hover:bg-blue-200 transition-colors cursor-pointer"
                      disabled={updateLoading}
                    >
                      {key}: {value}
                      <X
                        className="w-3 h-3 ml-2 hover:text-blue-900"
                        onClick={(e) => {
                          e.stopPropagation();
                          handleRemoveLabel(key);
                        }}
                      />
                    </button>
                  );
                })}
              </div>
            )}

            {/* Edit form */}
            {editingLabelKey !== null && (
              <div className="space-y-3 pt-4 border-t border-gray-200">
                <LabelAutocomplete
                  keyValue={editingLabelDraft.key}
                  valueValue={editingLabelDraft.value}
                  onKeyChange={(key) => setEditingLabelDraft(prev => ({ ...prev, key }))}
                  onValueChange={(value) => setEditingLabelDraft(prev => ({ ...prev, value }))}
                  onRemove={() => {
                    setEditingLabelKey(null);
                    setEditingLabelDraft({ key: '', value: '' });
                  }}
                />
                <div className="flex gap-2">
                  <button
                    onClick={() => {
                      handleSaveLabel(editingLabelKey === '__new__' ? '' : editingLabelKey);
                    }}
                    disabled={updateLoading || !editingLabelDraft.key.trim()}
                    className="px-3 py-1.5 bg-blue-600 text-white text-sm rounded-md hover:bg-blue-700 disabled:opacity-50"
                  >
                    {updateLoading ? 'Saving...' : 'Save'}
                  </button>
                  <button
                    onClick={() => {
                      setEditingLabelKey(null);
                      setEditingLabelDraft({ key: '', value: '' });
                      setUpdateError(null);
                    }}
                    disabled={updateLoading}
                    className="px-3 py-1.5 border border-gray-300 text-gray-700 text-sm rounded-md hover:bg-gray-100 disabled:opacity-50"
                  >
                    Cancel
                  </button>
                </div>
                {updateError && (
                  <div className="text-red-600 text-xs">{updateError}</div>
                )}
              </div>
            )}
          </div>
        ) : null}

      {/* Timeline */}
      {want.stats && (
        <div className="bg-gray-50 rounded-lg p-6">
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

      {/* Dependencies (Using) */}
      {want.spec && (
        <div className="bg-gray-50 rounded-lg p-6">
          <div className="flex items-center justify-between mb-4">
            <h4 className="text-base font-medium text-gray-900">Dependencies (using)</h4>
            {editingUsingIndex === null && (
              <button
                onClick={() => {
                  setEditingUsingIndex(-1);
                  setEditingUsingDraft({ key: '', value: '' });
                }}
                disabled={updateLoading}
                className="text-blue-600 hover:text-blue-800 text-sm font-medium disabled:opacity-50"
              >
                +
              </button>
            )}
          </div>

          {/* Display existing dependencies as styled chips */}
          {want.spec.using && want.spec.using.length > 0 && (
            <div className="flex flex-wrap gap-2 mb-4">
              {want.spec.using.map((usingItem, index) => {
                if (editingUsingIndex === index) return null;
                return Object.entries(usingItem).map(([key, value], keyIndex) => (
                  <button
                    key={`${index}-${keyIndex}`}
                    type="button"
                    onClick={() => {
                      setEditingUsingIndex(index);
                      setEditingUsingDraft({ key, value });
                    }}
                    className="inline-flex items-center px-3 py-1.5 rounded-full text-sm font-medium bg-blue-100 text-blue-800 hover:bg-blue-200 transition-colors cursor-pointer"
                    disabled={updateLoading}
                  >
                    {key}: {value}
                    <X
                      className="w-3 h-3 ml-2 hover:text-blue-900"
                      onClick={(e) => {
                        e.stopPropagation();
                        // Remove this dependency
                        const newUsing = want.spec.using ? want.spec.using.filter((_, i) => i !== index) : [];
                        const updatePayload = {
                          metadata: want.metadata,
                          spec: { ...want.spec, using: newUsing }
                        };
                        fetch(`http://localhost:8080/api/v1/wants/${want.metadata?.id}`, {
                          method: 'PUT',
                          headers: { 'Content-Type': 'application/json' },
                          body: JSON.stringify(updatePayload)
                        }).then(() => onWantUpdate?.());
                      }}
                    />
                  </button>
                ));
              })}
            </div>
          )}

          {/* Edit form for dependencies */}
          {editingUsingIndex !== null && (
            <div className="space-y-3 pt-4 border-t border-gray-200">
              <LabelSelectorAutocomplete
                keyValue={editingUsingDraft.key}
                valuValue={editingUsingDraft.value}
                onKeyChange={(key) => setEditingUsingDraft(prev => ({ ...prev, key }))}
                onValueChange={(value) => setEditingUsingDraft(prev => ({ ...prev, value }))}
                onRemove={() => {
                  setEditingUsingIndex(null);
                  setEditingUsingDraft({ key: '', value: '' });
                }}
              />
              <div className="flex gap-2">
                <button
                  onClick={() => {
                    if (!editingUsingDraft.key.trim() || !want.metadata?.id) return;

                    setUpdateLoading(true);
                    try {
                      let newUsing = want.spec.using ? [...want.spec.using] : [];
                      if (editingUsingIndex === -1) {
                        // Adding new dependency
                        newUsing.push({ [editingUsingDraft.key]: editingUsingDraft.value });
                      } else {
                        // Editing existing dependency
                        const oldItem = newUsing[editingUsingIndex];
                        const oldKey = Object.keys(oldItem)[0];
                        newUsing[editingUsingIndex] = { [editingUsingDraft.key]: editingUsingDraft.value };
                      }

                      const updatePayload = {
                        metadata: want.metadata,
                        spec: { ...want.spec, using: newUsing }
                      };

                      fetch(`http://localhost:8080/api/v1/wants/${want.metadata.id}`, {
                        method: 'PUT',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify(updatePayload)
                      }).then(() => {
                        setEditingUsingIndex(null);
                        setEditingUsingDraft({ key: '', value: '' });
                        onWantUpdate?.();
                      });
                    } finally {
                      setUpdateLoading(false);
                    }
                  }}
                  disabled={updateLoading || !editingUsingDraft.key.trim()}
                  className="px-3 py-1.5 bg-blue-600 text-white text-sm rounded-md hover:bg-blue-700 disabled:opacity-50"
                >
                  {updateLoading ? 'Saving...' : 'Save'}
                </button>
                <button
                  onClick={() => {
                    setEditingUsingIndex(null);
                    setEditingUsingDraft({ key: '', value: '' });
                  }}
                  disabled={updateLoading}
                  className="px-3 py-1.5 border border-gray-300 text-gray-700 text-sm rounded-md hover:bg-gray-100 disabled:opacity-50"
                >
                  Cancel
                </button>
              </div>
            </div>
          )}
        </div>
      )}

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
  </div>
  );
};

const ConfigTab: React.FC<{
  want: Want;
  isEditing: boolean;
  editedConfig: string;
  updateLoading: boolean;
  updateError: string | null;
  onEdit: () => void;
  onSave: () => void;
  onCancel: () => void;
  onConfigChange: (value: string) => void;
}> = ({ want, isEditing, editedConfig, updateLoading, updateError, onEdit, onSave, onCancel, onConfigChange }) => (
  <div className="p-8 h-full flex flex-col">
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
);

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
    <div className="p-8 space-y-6">
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
        <div className="bg-gray-50 rounded-lg p-4">
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
        <div className="bg-gray-50 rounded-lg p-4">
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

  // Extract flight_status if it exists in stateValue
  const flightStatus = state.stateValue?.flight_status;
  const stateTimestamp = state.timestamp;

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
            <div className="text-sm font-medium text-gray-900">
              #{index + 1}
            </div>
            {stateTimestamp && (
              <div className="text-xs text-gray-500 mt-1">
                {formatDate(stateTimestamp)}
              </div>
            )}
          </div>
          {/* Flight Status Highlight in Shrink Mode */}
          {!isExpanded && flightStatus && (
            <div className="flex items-center space-x-2 ml-2 flex-shrink-0">
              <span className="text-xs font-medium px-2 py-0.5 rounded-full bg-blue-100 text-blue-700">
                {flightStatus}
              </span>
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

const LogsTab: React.FC<{ want: Want; results: any }> = ({ want, results }) => {
  const hasParameterHistory = want.history?.parameterHistory && want.history.parameterHistory.length > 0;
  const hasLogs = results?.logs && results.logs.length > 0;

  return (
    <div className="p-8 space-y-8">
      {/* Parameter History Section */}
      {hasParameterHistory && (
        <div className="bg-gray-50 rounded-lg p-6">
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
        <div className="bg-gray-50 rounded-lg p-6">
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
        <div className="bg-gray-50 rounded-lg p-6">
          <h4 className="text-base font-medium text-gray-900 mb-4">State History</h4>
          <div className="space-y-3">
            {want.history.stateHistory.slice().reverse().map((state, index) => (
              <StateHistoryItem key={index} state={state} index={want.history.stateHistory.length - index - 1} />
            ))}
          </div>
        </div>
      )}

      {/* Empty State */}
      {!hasParameterHistory && !hasLogs && (!want.history?.stateHistory || want.history.stateHistory.length === 0) && (
        <div className="text-center py-8">
          <FileText className="h-12 w-12 text-gray-400 mx-auto mb-4" />
          <p className="text-gray-500">No logs or parameter history available</p>
        </div>
      )}
    </div>
  );
};