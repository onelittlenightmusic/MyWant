import React, { useEffect, useState } from 'react';
import { RefreshCw, Eye, AlertTriangle, User, Users, Clock, CheckCircle, XCircle, Minus, Bot, Save, Edit, FileText, ChevronDown, ChevronRight } from 'lucide-react';
import { Want } from '@/types/want';
import { StatusBadge } from '@/components/common/StatusBadge';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { ErrorDisplay } from '@/components/common/ErrorDisplay';
import { YamlEditor } from '@/components/forms/YamlEditor';
import { useWantStore } from '@/stores/wantStore';
import { formatDate, formatDuration, classNames } from '@/utils/helpers';
import { stringifyYaml, validateYaml } from '@/utils/yaml';
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
}

type TabType = 'overview' | 'config' | 'logs' | 'agents';

export const WantDetailsSidebar: React.FC<WantDetailsSidebarProps> = ({
  want
}) => {
  // Check if this is a flight want
  const isFlightWant = want?.metadata?.type === 'flight';
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

  // Fetch details when want changes
  useEffect(() => {
    if (want) {
      const wantId = want.metadata?.id || want.id;
      if (wantId) {
        fetchWantDetails(wantId);
        fetchWantResults(wantId);
        fetchWants(); // Also refresh main wants list
      }
    }
  }, [want, fetchWantDetails, fetchWantResults, fetchWants]);

  // Reset state when want changes
  useEffect(() => {
    if (want) {
      setActiveTab('overview');
      setIsEditing(false);
      setUpdateError(null);
    }
  }, [want]);

  // Auto-enable refresh for running wants
  useEffect(() => {
    if (want && selectedWantDetails && selectedWantDetails.status === 'running') {
      setAutoRefresh(true);
    } else if (want && selectedWantDetails && selectedWantDetails.status !== 'running' && autoRefresh) {
      // Auto-disable when want stops running
      setAutoRefresh(false);
    }
  }, [want, selectedWantDetails?.status]);

  // Auto refresh setup
  useEffect(() => {
    if (autoRefresh && want) {
      const interval = setInterval(() => {
        const wantId = want.metadata?.id || want.id;
        if (wantId) {
          fetchWantDetails(wantId);
          fetchWantResults(wantId);
        }
      }, 5000);

      return () => clearInterval(interval);
    }
  }, [autoRefresh, want, fetchWantDetails, fetchWantResults]);

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
    if (!want || !editedConfig) return;

    const wantId = want.metadata?.id || want.id;
    if (!wantId) return;

    setUpdateLoading(true);
    setUpdateError(null);

    try {
      // Parse YAML to want object
      const yamlValidation = validateYaml(editedConfig);
      if (!yamlValidation.isValid) {
        setUpdateError(`Invalid YAML: ${yamlValidation.error}`);
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
      {/* Header with tabs */}
      <div className="border-b border-gray-200 px-8 py-6">
        <div className="space-y-4">
          {/* Top row: Status and controls */}
          <div className="flex items-center justify-between">
            <StatusBadge status={wantDetails.status} size="sm" />
            <div className="flex items-center space-x-3">
              {/* Auto refresh toggle */}
              <label className="flex items-center space-x-2">
                <input
                  type="checkbox"
                  checked={autoRefresh}
                  onChange={(e) => setAutoRefresh(e.target.checked)}
                  className="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                />
                <span className="text-sm text-gray-600 whitespace-nowrap">Auto refresh</span>
              </label>
              <button
                onClick={handleRefresh}
                disabled={loading}
                className="p-2 text-gray-400 hover:text-gray-600 hover:bg-gray-100 rounded-md transition-colors"
                title="Refresh"
              >
                <RefreshCw className={classNames('h-4 w-4', loading && 'animate-spin')} />
              </button>
            </div>
          </div>

          {/* Bottom row: Tab navigation */}
          <div className="flex space-x-1 bg-gray-100 rounded-lg p-1">
            {tabs.map((tab) => {
              const Icon = tab.icon;
              return (
                <button
                  key={tab.id}
                  onClick={() => setActiveTab(tab.id)}
                  className={classNames(
                    'flex-1 flex items-center justify-center space-x-1 px-3 py-2 text-sm font-medium rounded-md transition-colors',
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
              <OverviewTab want={wantDetails} />
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
const OverviewTab: React.FC<{ want: Want }> = ({ want }) => (
  <div className="p-8">
    <div className="space-y-8">
      {/* Status Section */}
      <div className="bg-gray-50 rounded-lg p-6">
        <h4 className="text-base font-medium text-gray-900 mb-4">Status Information</h4>
        <div className="space-y-3">
          <div className="flex justify-between items-center">
            <span className="text-gray-600 text-sm">Current Status:</span>
            <StatusBadge status={want.status} size="sm" />
          </div>
          {want.suspended && (
            <div className="flex justify-between items-center">
              <span className="text-gray-600 text-sm">Suspended:</span>
              <span className="text-orange-600 font-medium text-sm">Yes</span>
            </div>
          )}
        </div>
      </div>

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
      {want.metadata?.labels && Object.keys(want.metadata.labels).length > 0 && (
        <div className="bg-gray-50 rounded-lg p-6">
          <h4 className="text-base font-medium text-gray-900 mb-4">Labels</h4>
          <div className="flex flex-wrap gap-2">
            {Object.entries(want.metadata.labels).map(([key, value]) => (
              <span
                key={key}
                className="inline-flex items-center px-3 py-1.5 rounded-full text-sm font-medium bg-blue-100 text-blue-800"
              >
                {key}: {value}
              </span>
            ))}
          </div>
        </div>
      )}

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
                <span className="text-gray-600 text-sm">Completed:</span>
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
                <LoadingSpinner size="xs" className="mr-1" />
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

const AgentsTab: React.FC<{ want: Want }> = ({ want }) => (
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
            <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse" title="Running" />
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

    {/* Agent History */}
    {want.history?.agentHistory && want.history.agentHistory.length > 0 && (
      <div className="bg-gray-50 rounded-lg p-4">
        <h4 className="text-sm font-medium text-gray-900 mb-3">Agent History</h4>
        <div className="space-y-3">
          {want.history?.agentHistory?.map((execution, index) => (
            <div key={index} className="border border-gray-200 rounded-md p-3 bg-white">
              <div className="flex items-center justify-between mb-2">
                <span className="font-medium text-sm">{execution.agent_name}</span>
                <div className="flex items-center space-x-2">
                  <span className="text-xs text-gray-500">{execution.agent_type}</span>
                  <div className={classNames(
                    'w-2 h-2 rounded-full',
                    execution.status === 'completed' && 'bg-green-500',
                    execution.status === 'failed' && 'bg-red-500',
                    execution.status === 'running' && 'bg-blue-500 animate-pulse',
                    execution.status === 'terminated' && 'bg-gray-500'
                  )} />
                </div>
              </div>
              <div className="text-xs text-gray-600">
                <div>Started: {formatDate(execution.start_time)}</div>
                {execution.end_time && (
                  <div>Ended: {formatDate(execution.end_time)}</div>
                )}
                {execution.error && (
                  <div className="text-red-600 mt-1">Error: {execution.error}</div>
                )}
              </div>
            </div>
          ))}
        </div>
      </div>
    )}

    {!want.current_agent && (!want.running_agents || want.running_agents.length === 0) && (!want.history?.agentHistory || want.history.agentHistory.length === 0) && (
      <div className="text-center py-8">
        <Bot className="h-12 w-12 text-gray-400 mx-auto mb-4" />
        <p className="text-gray-500">No agent information available</p>
      </div>
    )}
  </div>
);


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