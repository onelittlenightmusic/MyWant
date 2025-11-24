import React, { useEffect, useState } from 'react';
import { X, RefreshCw, Eye, BarChart3, AlertTriangle, User, Users, Clock, CheckCircle, XCircle, Minus, Bot, Save, Edit } from 'lucide-react';
import { Want } from '@/types/want';
import { StatusBadge } from '@/components/common/StatusBadge';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { ErrorDisplay } from '@/components/common/ErrorDisplay';
import { YamlEditor } from '@/components/forms/YamlEditor';
import { useWantStore } from '@/stores/wantStore';
import { formatDate, formatDuration, classNames } from '@/utils/helpers';
import { stringifyYaml, validateYaml } from '@/utils/yaml';

interface WantDetailsModalProps {
  isOpen: boolean;
  onClose: () => void;
  want: Want | null;
}

type TabType = 'overview' | 'config' | 'logs' | 'agents' | 'results';

export const WantDetailsModal: React.FC<WantDetailsModalProps> = ({
  isOpen,
  onClose,
  want
}) => {
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

  // Fetch details when modal opens
  useEffect(() => {
    if (isOpen && want) {
      const wantId = want.metadata?.id || want.id;
      if (wantId) {
        fetchWantDetails(wantId);
        fetchWantResults(wantId);
        fetchWants(); // Also refresh main wants list
      }
    }
  }, [isOpen, want, fetchWantDetails, fetchWantResults, fetchWants]);

  // Auto-enable refresh when modal opens with running want
  useEffect(() => {
    if (isOpen && selectedWantDetails && selectedWantDetails.status === 'reaching') {
      setAutoRefresh(true);
    } else if (isOpen && selectedWantDetails && selectedWantDetails.status !== 'reaching' && autoRefresh) {
      // Auto-disable when want stops running
      setAutoRefresh(false);
    }
  }, [isOpen, selectedWantDetails?.status]);

  // Auto-refresh for running wants
  useEffect(() => {
    if (!isOpen || !want || !autoRefresh) return;

    const interval = setInterval(() => {
      if (selectedWantDetails && selectedWantDetails.status === 'reaching') {
        const wantId = want.metadata?.id || want.id;
        if (wantId) {
          fetchWantDetails(wantId);
          fetchWantResults(wantId);
          fetchWants(); // Also refresh main wants list
        }
      }
    }, 3000);

    return () => clearInterval(interval);
  }, [isOpen, want, autoRefresh, selectedWantDetails?.status, fetchWantDetails, fetchWantResults, fetchWants]);

  const handleRefresh = () => {
    if (want) {
      const wantId = want.metadata?.id || want.id;
      if (wantId) {
        // Refresh both the modal details and the main wants list
        fetchWantDetails(wantId);
        fetchWantResults(wantId);
        fetchWants(); // This ensures the parent component gets updated data
      }
    }
  };

  const handleEditStart = () => {
    // Initialize the editor with current want configuration as single want object
    const details = wantDetails as any || {};
    const currentConfig = stringifyYaml({
      metadata: {
        name: details?.metadata?.name || want.metadata?.name,
        type: details?.metadata?.type || want.metadata?.type,
        labels: details?.metadata?.labels || want.metadata?.labels || {}
      },
      spec: {
        params: details?.spec?.params || want.spec?.params || {},
        ...(details?.spec?.using || want.spec?.using) && { using: details?.spec?.using || want.spec?.using },
        ...(details?.spec?.recipe || want.spec?.recipe) && { recipe: details?.spec?.recipe || want.spec?.recipe }
      }
    });
    setEditedConfig(currentConfig);
    setIsEditing(true);
    setUpdateError(null);
  };

  const handleEditCancel = () => {
    setIsEditing(false);
    setEditedConfig('');
    setUpdateError(null);
  };

  const handleSaveConfig = async () => {
    if (!want || !editedConfig.trim()) return;

    const wantId = want.metadata?.id;
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
      setEditedConfig('');

      // Refresh the want details and main wants list to show updated data
      await fetchWantDetails(wantId);
      await fetchWantResults(wantId);
      await fetchWants(); // This ensures the parent component gets updated data
    } catch (error) {
      console.error('Failed to update want:', error);
      setUpdateError(error instanceof Error ? error.message : 'Failed to update want configuration');
    } finally {
      setUpdateLoading(false);
    }
  };

  if (!isOpen || !want) return null;

  const wantDetails = selectedWantDetails;

  // Debug logging
  console.log('WantDetailsModal Debug:', {
    wantDetails,
    current_agent: wantDetails?.current_agent,
    running_agents: wantDetails?.running_agents,
    agent_history: wantDetails?.history?.agentHistory,
    state_agent_history: wantDetails?.state?.agent_history,
    state_current_agent: wantDetails?.state?.current_agent,
    state_running_agents: wantDetails?.state?.running_agents
  });

  const hasAgentData = (wantDetails?.current_agent ||
    (wantDetails?.running_agents && wantDetails.running_agents.length > 0) ||
    (wantDetails?.history?.agentHistory && wantDetails.history.agentHistory.length > 0) ||
    (wantDetails?.state?.current_agent) ||
    (wantDetails?.state?.running_agents && Array.isArray(wantDetails.state.running_agents) && wantDetails.state.running_agents.length > 0) ||
    (wantDetails?.state?.agent_history && Array.isArray(wantDetails.state.agent_history) && wantDetails.state.agent_history.length > 0));

  console.log('hasAgentData:', hasAgentData);

  const tabs = [
    { id: 'overview', label: 'Overview', icon: Eye },
    { id: 'config', label: 'Configuration', icon: Eye },
    { id: 'logs', label: 'Logs', icon: Eye },
    ...(hasAgentData ? [{ id: 'agents', label: 'Agents', icon: Bot }] : []),
    { id: 'results', label: 'Results', icon: BarChart3 },
  ];
  const createdAt = wantDetails?.stats?.created_at;
  const startedAt = wantDetails?.stats?.started_at;
  const completedAt = wantDetails?.stats?.completed_at;

  return (
    <div className="fixed inset-0 bg-gray-600 bg-opacity-50 overflow-y-auto h-full w-full z-50">
      <div className="relative top-10 mx-auto p-5 border w-11/12 md:w-4/5 lg:w-3/4 xl:w-2/3 shadow-lg rounded-md bg-white">
        {/* Header */}
        <div className="flex items-center justify-between pb-4 border-b border-gray-200">
          <div className="flex items-center space-x-4">
            <h3 className="text-xl font-semibold text-gray-900">
              {wantDetails?.metadata?.name || want.metadata?.name || want.metadata?.id || want.id || 'Unnamed Want'}
            </h3>
            <StatusBadge status={want.status} />
            {want.status === 'reaching' && (
              <div className="flex items-center space-x-2">
                <label className="flex items-center text-sm text-gray-600">
                  <input
                    type="checkbox"
                    checked={autoRefresh}
                    onChange={(e) => setAutoRefresh(e.target.checked)}
                    className="rounded border-gray-300 text-primary-600 focus:ring-primary-500 mr-2"
                  />
                  Auto-refresh
                </label>
              </div>
            )}
          </div>

          <div className="flex items-center space-x-2">
            <button
              onClick={handleRefresh}
              disabled={loading}
              className="inline-flex items-center px-3 py-1 border border-gray-300 shadow-sm text-sm leading-4 font-medium rounded-md text-gray-700 bg-white hover:bg-gray-50"
            >
              {loading ? (
                <LoadingSpinner size="sm" className="mr-2" />
              ) : (
                <RefreshCw className="h-4 w-4 mr-2" />
              )}
              Refresh
            </button>
            <button
              onClick={onClose}
              className="text-gray-400 hover:text-gray-600"
            >
              <X className="h-6 w-6" />
            </button>
          </div>
        </div>

        {/* Tabs */}
        <div className="mt-6">
          <div className="border-b border-gray-200">
            <nav className="-mb-px flex space-x-8">
              {tabs.map((tab) => {
                const Icon = tab.icon;
                return (
                  <button
                    key={tab.id}
                    onClick={() => setActiveTab(tab.id as TabType)}
                    className={classNames(
                      'group inline-flex items-center py-2 px-1 border-b-2 font-medium text-sm',
                      activeTab === tab.id
                        ? 'border-primary-500 text-primary-600'
                        : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
                    )}
                  >
                    <Icon className="h-4 w-4 mr-2" />
                    {tab.label}
                  </button>
                );
              })}
            </nav>
          </div>

          {/* Tab Content */}
          <div className="mt-6 h-96 overflow-y-auto custom-scrollbar">
            {activeTab === 'overview' && (
              <div className="space-y-6">
                {/* Runtime Error Display */}
                {want.status === 'failed' && want.state?.error && (
                  <div className="bg-red-50 border border-red-200 rounded-lg p-4">
                    <div className="flex items-start">
                      <AlertTriangle className="h-5 w-5 text-red-600 mt-0.5 mr-3 flex-shrink-0" />
                      <div className="flex-1 min-w-0">
                        <h4 className="text-sm font-medium text-red-800 mb-2">Runtime Error</h4>
                        <p className="text-sm text-red-700">{String(want.state?.error || 'Unknown error')}</p>
                        {String(want.state?.error || '').includes('Unknown want type:') && (
                          <div className="mt-3 bg-red-100 bg-opacity-50 p-3 rounded border">
                            <p className="text-xs text-red-600 font-medium mb-1">This want failed during creation because:</p>
                            <ul className="list-disc list-inside space-y-1 text-xs text-red-600">
                              <li>The want type doesn't exist or is misspelled</li>
                              <li>A custom type may not be properly registered</li>
                              <li>Check the available types listed in the error</li>
                            </ul>
                          </div>
                        )}
                      </div>
                    </div>
                  </div>
                )}

                {/* Status Overview */}
                <div className="bg-gray-50 rounded-lg p-4">
                  <h4 className="text-sm font-medium text-gray-900 mb-3">Execution Status</h4>
                  <div className="flex items-center justify-between">
                    <div className="flex items-center space-x-3">
                      <StatusBadge status={want.status} />
                      <span className="text-sm text-gray-600">
                        {want.status === 'reaching' && 'Execution in progress...'}
                        {want.status === 'achieved' && 'Execution completed successfully'}
                        {want.status === 'failed' && 'Execution failed'}
                        {want.status === 'created' && 'Want created, ready to execute'}
                        {want.status === 'stopped' && 'Execution stopped'}
                      </span>
                    </div>
                    {selectedWantDetails?.execution_status && selectedWantDetails.execution_status !== want.status && (
                      <div className="text-xs text-gray-500">
                        Detail Status: <StatusBadge status={selectedWantDetails.execution_status} size="sm" />
                      </div>
                    )}
                  </div>
                </div>

                {/* Basic Info */}
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <h4 className="text-sm font-medium text-gray-900 mb-2">Basic Information</h4>
                    <dl className="space-y-2 text-sm">
                      <div>
                        <dt className="text-gray-500">ID:</dt>
                        <dd className="text-gray-900 font-mono">{want.metadata?.id || want.id || 'N/A'}</dd>
                      </div>
                      <div>
                        <dt className="text-gray-500">Type:</dt>
                        <dd className="text-gray-900">{wantDetails?.metadata?.type || 'Unknown'}</dd>
                      </div>
                      <div>
                        <dt className="text-gray-500">Name:</dt>
                        <dd className="text-gray-900">{wantDetails?.metadata?.name || want.metadata?.name || want.metadata?.id || want.id || 'Unnamed'}</dd>
                      </div>
                    </dl>
                  </div>

                  <div>
                    <h4 className="text-sm font-medium text-gray-900 mb-2">Timeline</h4>
                    <dl className="space-y-2 text-sm">
                      <div>
                        <dt className="text-gray-500">Created:</dt>
                        <dd className="text-gray-900">{formatDate(createdAt)}</dd>
                      </div>
                      <div>
                        <dt className="text-gray-500">Started:</dt>
                        <dd className="text-gray-900">{formatDate(startedAt)}</dd>
                      </div>
                      <div>
                        <dt className="text-gray-500">Achieved:</dt>
                        <dd className="text-gray-900">{formatDate(completedAt)}</dd>
                      </div>
                      {startedAt && (
                        <div>
                          <dt className="text-gray-500">Duration:</dt>
                          <dd className="text-gray-900">{formatDuration(startedAt, completedAt)}</dd>
                        </div>
                      )}
                    </dl>
                  </div>
                </div>

                {/* Labels */}
                {wantDetails?.metadata?.labels && Object.keys(wantDetails.metadata.labels).length > 0 && (
                  <div>
                    <h4 className="text-sm font-medium text-gray-900 mb-2">Labels</h4>
                    <div className="flex flex-wrap gap-2">
                      {Object.entries(wantDetails.metadata.labels).map(([key, value]) => (
                        <span
                          key={key}
                          className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-gray-100 text-gray-800"
                        >
                          {key}: {value}
                        </span>
                      ))}
                    </div>
                  </div>
                )}



                {/* Parameters */}
                {wantDetails?.spec?.params && Object.keys(wantDetails.spec.params).length > 0 && (
                  <div>
                    <h4 className="text-sm font-medium text-gray-900 mb-2">Parameters</h4>
                    <div className="bg-gray-50 rounded-md p-3">
                      <pre className="text-sm text-gray-700 whitespace-pre-wrap">
                        {JSON.stringify(wantDetails.spec.params, null, 2)}
                      </pre>
                    </div>
                  </div>
                )}
              </div>
            )}

            {activeTab === 'config' && (
              <div className="space-y-4">
                {/* Configuration Header with Edit/Save controls */}
                <div className="flex items-center justify-between">
                  <div className="flex items-center space-x-2">
                    <h4 className="text-sm font-medium text-gray-900">Want Configuration</h4>
                    {isEditing && (
                      <span className="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium bg-blue-100 text-blue-800">
                        <Edit className="h-3 w-3 mr-1" />
                        Editing
                      </span>
                    )}
                  </div>

                  <div className="flex items-center space-x-2">
                    {!isEditing ? (
                      <button
                        onClick={handleEditStart}
                        className="inline-flex items-center px-3 py-1 border border-gray-300 shadow-sm text-sm leading-4 font-medium rounded-md text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
                      >
                        <Edit className="h-4 w-4 mr-1" />
                        Edit
                      </button>
                    ) : (
                      <>
                        <button
                          onClick={handleEditCancel}
                          disabled={updateLoading}
                          className="inline-flex items-center px-3 py-1 border border-gray-300 shadow-sm text-sm leading-4 font-medium rounded-md text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-gray-500"
                        >
                          Cancel
                        </button>
                        <button
                          onClick={handleSaveConfig}
                          disabled={updateLoading || !editedConfig.trim()}
                          className="inline-flex items-center px-3 py-1 border border-transparent shadow-sm text-sm leading-4 font-medium rounded-md text-white bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 disabled:bg-gray-400 disabled:cursor-not-allowed"
                        >
                          {updateLoading ? (
                            <>
                              <LoadingSpinner size="sm" className="mr-1" />
                              Saving...
                            </>
                          ) : (
                            <>
                              <Save className="h-4 w-4 mr-1" />
                              Save Changes
                            </>
                          )}
                        </button>
                      </>
                    )}
                  </div>
                </div>

                {/* Error Display */}
                {updateError && (
                  <div className="bg-red-50 border border-red-200 rounded-lg p-3">
                    <div className="flex items-start">
                      <AlertTriangle className="h-4 w-4 text-red-600 mt-0.5 mr-2 flex-shrink-0" />
                      <div>
                        <h4 className="text-sm font-medium text-red-800">Failed to Update Configuration</h4>
                        <p className="text-sm text-red-700 mt-1">{updateError}</p>
                      </div>
                    </div>
                  </div>
                )}

                {/* YAML Editor */}
                <YamlEditor
                  value={isEditing ? editedConfig : stringifyYaml({
                    metadata: {
                      name: wantDetails?.metadata?.name || want.metadata?.name,
                      type: wantDetails?.metadata?.type || want.metadata?.type,
                      labels: wantDetails?.metadata?.labels || want.metadata?.labels || {}
                    },
                    spec: {
                      params: wantDetails?.spec?.params || want.spec?.params || {},
                      ...(wantDetails?.spec?.using || want.spec?.using) && { using: wantDetails?.spec?.using || want.spec?.using },
                      ...(wantDetails?.spec?.recipe || want.spec?.recipe) && { recipe: wantDetails?.spec?.recipe || want.spec?.recipe }
                    },
                    status: wantDetails?.status || want.status,
                    ...(wantDetails?.state || want.state) && { state: wantDetails?.state || want.state }
                  })}
                  onChange={isEditing ? setEditedConfig : () => {}}
                  readOnly={!isEditing}
                  height="350px"
                />

                {/* Help Text */}
                {isEditing && (
                  <div className="bg-blue-50 border border-blue-200 rounded-lg p-3">
                    <div className="flex items-start">
                      <div className="flex-shrink-0">
                        <div className="w-4 h-4 rounded-full bg-blue-400 flex items-center justify-center">
                          <span className="text-xs text-white font-bold">i</span>
                        </div>
                      </div>
                      <div className="ml-3">
                        <h4 className="text-sm font-medium text-blue-800">Editing Tips</h4>
                        <div className="text-sm text-blue-700 mt-1">
                          <ul className="list-disc list-inside space-y-1">
                            <li>You can modify parameters, labels, and other want specifications</li>
                            <li>Changes will take effect immediately after saving</li>
                            <li>Invalid YAML syntax will be rejected</li>
                          </ul>
                        </div>
                      </div>
                    </div>
                  </div>
                )}
              </div>
            )}

            {activeTab === 'logs' && (
              <div>
                {(wantDetails?.history?.parameterHistory || want.history?.parameterHistory) && (wantDetails?.history?.parameterHistory || want.history?.parameterHistory).length > 0 ? (
                  <div className="space-y-4">
                    <div>
                      <h4 className="text-sm font-medium text-gray-900 mb-2">Parameter History</h4>
                      <div className="space-y-3">
                        {(wantDetails?.history?.parameterHistory || want.history?.parameterHistory || []).map((entry, index) => (
                          <div key={index} className="bg-gray-50 rounded-md p-3 border">
                            <div className="flex items-center justify-between mb-2">
                              <span className="text-sm font-medium text-gray-700">
                                {entry.wantName}
                              </span>
                              <span className="text-xs text-gray-500">
                                {new Date(entry.timestamp).toLocaleString()}
                              </span>
                            </div>
                            <div className="bg-white rounded p-2">
                              <pre className="text-sm text-gray-700 whitespace-pre-wrap">
                                {JSON.stringify(entry.stateValue, null, 2)}
                              </pre>
                            </div>
                          </div>
                        ))}
                      </div>
                    </div>
                  </div>
                ) : (
                  <div className="bg-gray-900 text-gray-100 p-4 rounded-md font-mono text-sm">
                    <div className="space-y-1">
                      <div>[{new Date().toLocaleString()}] Want execution started</div>
                      <div>[{new Date().toLocaleString()}] Initializing chain builder...</div>
                      <div>[{new Date().toLocaleString()}] Registering want types...</div>
                      <div>[{new Date().toLocaleString()}] Starting execution...</div>
                      {want.status === 'reaching' && (
                        <div className="text-blue-400">[{new Date().toLocaleString()}] Processing... <span className="animate-pulse">●</span></div>
                      )}
                      {want.status === 'achieved' && (
                        <div className="text-green-400">[{new Date().toLocaleString()}] Execution completed successfully</div>
                      )}
                      {want.status === 'failed' && (
                        <div className="text-red-400">[{new Date().toLocaleString()}] Execution failed: Connection timeout</div>
                      )}
                    </div>
                  </div>
                )}
              </div>
            )}

            {activeTab === 'results' && (
              <div>
                {/* Show error details prominently if the want failed */}
                {want.status === 'failed' && want.state?.error && (
                  <div className="mb-6">
                    <ErrorDisplay
                      error={{
                        message: 'Want execution failed',
                        status: 500,
                        type: 'runtime',
                        details: String(want.state?.error || 'Unknown error')
                      }}
                    />
                  </div>
                )}

                {(wantDetails?.history?.stateHistory || want.history?.stateHistory) && (wantDetails?.history?.stateHistory || want.history?.stateHistory).length > 0 ? (
                  <div className="space-y-4">
                    <div>
                      <h4 className="text-sm font-medium text-gray-900 mb-2">State History</h4>
                      <div className="space-y-3">
                        {(wantDetails?.history?.stateHistory || want.history?.stateHistory || []).map((entry: any, index: number) => {
                          const stateValue = entry.state_value || entry;
                          const hasAgentInfo = stateValue && (
                            stateValue.current_agent ||
                            stateValue.running_agents ||
                            stateValue.agent_history
                          );

                          return (
                            <div key={index} className={`rounded-md p-3 border ${hasAgentInfo ? 'bg-blue-50 border-blue-200' : 'bg-gray-50'}`}>
                              <div className="flex items-center justify-between mb-2">
                                <div className="flex items-center space-x-2">
                                  <span className="text-sm font-medium text-gray-700">
                                    {entry.want_name || 'State Entry'}
                                  </span>
                                  {hasAgentInfo && (
                                    <span className="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium bg-blue-100 text-blue-800">
                                      <User className="h-3 w-3 mr-1" />
                                      Agent State
                                    </span>
                                  )}
                                </div>
                                <span className="text-xs text-gray-500">
                                  {entry.timestamp ? new Date(entry.timestamp).toLocaleString() : 'No timestamp'}
                                </span>
                              </div>

                              {hasAgentInfo && stateValue.current_agent && (
                                <div className="mb-2 p-2 bg-white rounded border">
                                  <div className="text-xs font-medium text-blue-700 mb-1">Current Agent</div>
                                  <div className="text-sm text-blue-900">{stateValue.current_agent}</div>
                                </div>
                              )}

                              {hasAgentInfo && stateValue.running_agents && Array.isArray(stateValue.running_agents) && stateValue.running_agents.length > 0 && (
                                <div className="mb-2 p-2 bg-white rounded border">
                                  <div className="text-xs font-medium text-blue-700 mb-1">Running Agents</div>
                                  <div className="text-sm text-blue-900">{stateValue.running_agents.join(', ')}</div>
                                </div>
                              )}

                              <div className="bg-white rounded p-2">
                                <pre className="text-sm text-gray-700 whitespace-pre-wrap">
                                  {JSON.stringify(stateValue, null, 2)}
                                </pre>
                              </div>
                            </div>
                          );
                        })}
                      </div>
                    </div>
                  </div>
                ) : (wantDetails?.state || want.state) ? (
                  <div className="space-y-4">
                    <div>
                      <h4 className="text-sm font-medium text-gray-900 mb-2">Current State</h4>
                      <div className="bg-gray-50 rounded-md p-3">
                        <pre className="text-sm text-gray-700 whitespace-pre-wrap">
                          {JSON.stringify(wantDetails?.state || want.state, null, 2)}
                        </pre>
                      </div>
                    </div>
                  </div>
                ) : (
                  <div className="text-center py-8 text-gray-500">
                    No state data available yet
                  </div>
                )}
              </div>
            )}

            {activeTab === 'agents' && (
              <div>
                <div className="space-y-6">
                  {/* Current Agent */}
                  {(wantDetails?.current_agent || wantDetails?.state?.current_agent) && (
                    <div className="bg-blue-50 border border-blue-200 rounded-lg p-4">
                      <h4 className="text-sm font-medium text-blue-900 mb-3 flex items-center">
                        <User className="h-4 w-4 mr-2" />
                        Current Agent
                      </h4>
                      <div className="flex items-center justify-between p-3 bg-blue-100 rounded-md">
                        <div className="flex items-center">
                          <div className="w-3 h-3 bg-green-500 rounded-full mr-3 animate-pulse" />
                          <div>
                            <div className="text-sm font-medium text-blue-900">{(wantDetails as any)?.current_agent || (wantDetails as any)?.state?.current_agent}</div>
                            <div className="text-xs text-blue-700">Currently executing</div>
                          </div>
                        </div>
                        <span className="bg-green-100 text-green-800 text-xs font-medium px-2.5 py-0.5 rounded-full">
                          Active
                        </span>
                      </div>
                    </div>
                  )}

                  {/* Running Agents Summary */}
                  {(((wantDetails as any)?.running_agents && (wantDetails as any)?.running_agents?.length > 0) ||
                    ((wantDetails as any)?.state?.running_agents && Array.isArray((wantDetails as any)?.state?.running_agents) && (wantDetails as any)?.state?.running_agents?.length > 0)) && (
                    <div className="bg-blue-50 border border-blue-200 rounded-lg p-4">
                      <h4 className="text-sm font-medium text-blue-900 mb-3 flex items-center">
                        <Users className="h-4 w-4 mr-2" />
                        Running Agents
                      </h4>
                      {(() => {
                        const runningAgents = ((wantDetails as any)?.running_agents || (wantDetails as any)?.state?.running_agents || []) as any[];
                        return (
                          <>
                            <div className="text-sm text-gray-700">
                              <span className="font-medium">{runningAgents.length}</span> agent{runningAgents.length !== 1 ? 's' : ''} currently running:
                            </div>
                            <div className="mt-2 space-y-1">
                              {runningAgents.map((agentName: any, index: number) => (
                                <div key={index} className="flex items-center text-sm text-blue-700">
                                  <div className="w-2 h-2 bg-blue-500 rounded-full mr-2 animate-pulse" />
                                  {agentName}
                                </div>
                              ))}
                            </div>
                          </>
                        );
                      })()}
                    </div>
                  )}

                  {/* Agent Execution History */}
                  {(((wantDetails as any)?.history?.agentHistory && (wantDetails as any)?.history?.agentHistory?.length > 0) ||
                    ((wantDetails as any)?.state?.agent_history && Array.isArray((wantDetails as any)?.state?.agent_history) && (wantDetails as any)?.state?.agent_history?.length > 0)) && (
                    <div className="bg-gray-50 border border-gray-200 rounded-lg p-4">
                      <h4 className="text-sm font-medium text-gray-900 mb-3 flex items-center">
                        <Clock className="h-4 w-4 mr-2" />
                        Execution History
                        {(() => {
                          const agentHistory = ((wantDetails as any)?.history?.agentHistory || (wantDetails as any)?.state?.agent_history || []) as any[];
                          return (
                            <span className="ml-2 text-xs text-gray-500">
                              ({agentHistory.length} execution{agentHistory.length !== 1 ? 's' : ''})
                            </span>
                          );
                        })()}
                      </h4>

                      <div className="space-y-3 max-h-96 overflow-y-auto">
                        {(() => {
                          const agentHistory = ((wantDetails as any)?.history?.agentHistory || (wantDetails as any)?.state?.agent_history || []) as any[];
                          return agentHistory.map((execution: any, index: number) => {
                          const getStatusIcon = (status: string) => {
                            switch (status) {
                              case 'achieved':
                                return <CheckCircle className="h-4 w-4 text-green-600" />;
                              case 'failed':
                                return <XCircle className="h-4 w-4 text-red-600" />;
                              case 'reaching':
                                return <Clock className="h-4 w-4 text-blue-600 animate-pulse" />;
                              default:
                                return <Minus className="h-4 w-4 text-gray-600" />;
                            }
                          };

                          const getStatusColor = (status: string) => {
                            switch (status) {
                              case 'achieved':
                                return 'bg-green-50 border-green-200';
                              case 'failed':
                                return 'bg-red-50 border-red-200';
                              case 'reaching':
                                return 'bg-blue-50 border-blue-200';
                              default:
                                return 'bg-gray-50 border-gray-200';
                            }
                          };

                          const duration = execution.end_time
                            ? new Date(execution.end_time).getTime() - new Date(execution.start_time).getTime()
                            : null;

                          return (
                            <div key={index} className={`rounded-md p-3 border ${getStatusColor(execution.status)}`}>
                              <div className="flex items-start justify-between">
                                <div className="flex items-center space-x-2 flex-1">
                                  {getStatusIcon(execution.status)}
                                  <div className="flex-1">
                                    <div className="flex items-center space-x-2">
                                      <span className="text-sm font-medium text-gray-900">
                                        {execution.agent_name}
                                      </span>
                                      <span className="text-xs text-gray-500 bg-gray-100 px-2 py-0.5 rounded-full">
                                        {execution.agent_type}
                                      </span>
                                    </div>
                                    <div className="text-xs text-gray-500 mt-1">
                                      Started: {formatDate(execution.start_time)}
                                      {execution.end_time && (
                                        <> • Ended: {formatDate(execution.end_time)}</>
                                      )}
                                      {duration && (
                                        <> • Duration: {formatDuration(execution.start_time, execution.end_time)}</>
                                      )}
                                    </div>
                                  </div>
                                </div>
                                <span className={classNames(
                                  'px-2 py-1 rounded-full text-xs font-medium',
                                  execution.status === 'reaching' && 'bg-blue-100 text-blue-800',
                                  execution.status === 'achieved' && 'bg-green-100 text-green-800',
                                  execution.status === 'failed' && 'bg-red-100 text-red-800',
                                  execution.status === 'terminated' && 'bg-gray-100 text-gray-800'
                                )}>
                                  {execution.status}
                                </span>
                              </div>
                              {execution.error && (
                                <div className="mt-2 p-2 bg-white border border-red-200 rounded text-xs text-red-700">
                                  <div className="font-medium">Error:</div>
                                  <div className="mt-1">{execution.error}</div>
                                </div>
                              )}
                            </div>
                          );
                          });
                        })()}
                      </div>
                    </div>
                  )}

                  {/* No agent data */}
                  {!hasAgentData && (
                    <div className="text-center py-8 text-gray-500">
                      No agent execution data available
                    </div>
                  )}
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};