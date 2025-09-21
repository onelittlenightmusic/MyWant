import React, { useEffect, useState } from 'react';
import { X, RefreshCw, Eye, BarChart3 } from 'lucide-react';
import { Want } from '@/types/want';
import { StatusBadge } from '@/components/common/StatusBadge';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { YamlEditor } from '@/components/forms/YamlEditor';
import { useWantStore } from '@/stores/wantStore';
import { formatDate, formatDuration, classNames } from '@/utils/helpers';
import { stringifyYaml } from '@/utils/yaml';

interface WantDetailsModalProps {
  isOpen: boolean;
  onClose: () => void;
  want: Want | null;
}

type TabType = 'overview' | 'config' | 'logs' | 'results';

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
    loading
  } = useWantStore();

  const [activeTab, setActiveTab] = useState<TabType>('overview');
  const [autoRefresh, setAutoRefresh] = useState(false);

  // Fetch details when modal opens
  useEffect(() => {
    if (isOpen && want) {
      fetchWantDetails(want.id);
      fetchWantResults(want.id);
    }
  }, [isOpen, want, fetchWantDetails, fetchWantResults]);

  // Auto-refresh for running wants
  useEffect(() => {
    if (!isOpen || !want || !autoRefresh) return;

    const interval = setInterval(() => {
      if (want.status === 'running') {
        fetchWantDetails(want.id);
        fetchWantResults(want.id);
      }
    }, 3000);

    return () => clearInterval(interval);
  }, [isOpen, want, autoRefresh, fetchWantDetails, fetchWantResults]);

  const handleRefresh = () => {
    if (want) {
      fetchWantDetails(want.id);
      fetchWantResults(want.id);
    }
  };

  if (!isOpen || !want) return null;

  const tabs = [
    { id: 'overview', label: 'Overview', icon: Eye },
    { id: 'config', label: 'Configuration', icon: Eye },
    { id: 'logs', label: 'Logs', icon: Eye },
    { id: 'results', label: 'Results', icon: BarChart3 },
  ];

  const wantDetails = selectedWantDetails?.wants?.[0];
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
              {wantDetails?.metadata?.name || want.id}
            </h3>
            <StatusBadge status={want.status} />
            {want.status === 'running' && (
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
                {/* Status Overview */}
                <div className="bg-gray-50 rounded-lg p-4">
                  <h4 className="text-sm font-medium text-gray-900 mb-3">Execution Status</h4>
                  <div className="flex items-center justify-between">
                    <div className="flex items-center space-x-3">
                      <StatusBadge status={want.status} />
                      <span className="text-sm text-gray-600">
                        {want.status === 'running' && 'Execution in progress...'}
                        {want.status === 'completed' && 'Execution completed successfully'}
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
                        <dd className="text-gray-900 font-mono">{want.id}</dd>
                      </div>
                      <div>
                        <dt className="text-gray-500">Type:</dt>
                        <dd className="text-gray-900">{wantDetails?.metadata?.type || 'Unknown'}</dd>
                      </div>
                      <div>
                        <dt className="text-gray-500">Name:</dt>
                        <dd className="text-gray-900">{wantDetails?.metadata?.name || want.id}</dd>
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
                        <dt className="text-gray-500">Completed:</dt>
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

                {/* Individual Want Statuses */}
                {selectedWantDetails?.wants && selectedWantDetails.wants.length > 1 && (
                  <div>
                    <h4 className="text-sm font-medium text-gray-900 mb-2">Individual Want Status</h4>
                    <div className="space-y-2">
                      {selectedWantDetails.wants.map((want, index) => (
                        <div key={index} className="flex items-center justify-between p-3 bg-gray-50 rounded-md">
                          <div className="flex items-center space-x-3">
                            <span className="text-sm font-medium text-gray-900">
                              {want.metadata?.name || `Want ${index + 1}`}
                            </span>
                            <span className="text-xs text-gray-500">
                              ({want.metadata?.type})
                            </span>
                          </div>
                          <div className="flex items-center space-x-2">
                            {want.status?.phase && (
                              <StatusBadge status={want.status.phase} size="sm" />
                            )}
                            {want.status?.message && (
                              <span className="text-xs text-gray-500 max-w-32 truncate">
                                {want.status.message}
                              </span>
                            )}
                          </div>
                        </div>
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
              <div>
                <YamlEditor
                  value={stringifyYaml(want.config)}
                  onChange={() => {}} // Read-only
                  readOnly={true}
                  height="350px"
                />
              </div>
            )}

            {activeTab === 'logs' && (
              <div className="bg-gray-900 text-gray-100 p-4 rounded-md font-mono text-sm">
                <div className="space-y-1">
                  <div>[2024-01-20 10:30:15] Want execution started</div>
                  <div>[2024-01-20 10:30:16] Initializing chain builder...</div>
                  <div>[2024-01-20 10:30:17] Registering want types...</div>
                  <div>[2024-01-20 10:30:18] Starting execution...</div>
                  {want.status === 'running' && (
                    <div className="text-blue-400">[2024-01-20 10:30:19] Processing... <span className="animate-pulse">●</span></div>
                  )}
                  {want.status === 'completed' && (
                    <div className="text-green-400">[2024-01-20 10:35:22] Execution completed successfully</div>
                  )}
                  {want.status === 'failed' && (
                    <div className="text-red-400">[2024-01-20 10:33:45] Execution failed: Connection timeout</div>
                  )}
                </div>
              </div>
            )}

            {activeTab === 'results' && (
              <div>
                {selectedWantResults ? (
                  <div className="space-y-4">
                    {selectedWantResults.metrics && (
                      <div>
                        <h4 className="text-sm font-medium text-gray-900 mb-2">Metrics</h4>
                        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                          {Object.entries(selectedWantResults.metrics).map(([key, value]) => (
                            <div key={key} className="bg-gray-50 rounded-md p-3">
                              <div className="text-sm text-gray-500 capitalize">
                                {key.replace(/_/g, ' ')}
                              </div>
                              <div className="text-lg font-semibold text-gray-900">
                                {typeof value === 'number' ? value.toLocaleString() : String(value)}
                              </div>
                            </div>
                          ))}
                        </div>
                      </div>
                    )}

                    {selectedWantResults.data && (
                      <div>
                        <h4 className="text-sm font-medium text-gray-900 mb-2">Data</h4>
                        <div className="bg-gray-50 rounded-md p-3">
                          <pre className="text-sm text-gray-700 whitespace-pre-wrap">
                            {JSON.stringify(selectedWantResults.data, null, 2)}
                          </pre>
                        </div>
                      </div>
                    )}
                  </div>
                ) : (
                  <div className="text-center py-8 text-gray-500">
                    No results available yet
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};