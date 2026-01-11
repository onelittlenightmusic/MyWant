import React, { useEffect, useState } from 'react';
import { RefreshCw, Eye, BarChart3, AlertTriangle, User, Users, Clock, CheckCircle, XCircle, Minus, Bot, Save, Edit, X, Check } from 'lucide-react';
import { Want } from '@/types/want';
import { StatusBadge } from '@/components/common/StatusBadge';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { ErrorDisplay } from '@/components/common/ErrorDisplay';
import { YamlEditor } from '@/components/forms/YamlEditor';
import { useWantStore } from '@/stores/wantStore';
import { formatDate, formatDuration, classNames, truncateText } from '@/utils/helpers';
import { stringifyYaml, validateYaml } from '@/utils/yaml';
import { BaseModal } from './BaseModal';

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
        fetchWants();
      }
    }
  }, [isOpen, want]);

  // Auto-enable refresh when modal opens with running want
  useEffect(() => {
    if (isOpen && selectedWantDetails && selectedWantDetails.status === 'reaching') {
      setAutoRefresh(true);
    } else if (isOpen && selectedWantDetails && selectedWantDetails.status !== 'reaching' && autoRefresh) {
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
          fetchWants();
        }
      }
    }, 3000);

    return () => clearInterval(interval);
  }, [isOpen, want, autoRefresh, selectedWantDetails?.status]);

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

  const handleEditStart = () => {
    const details = wantDetails as any || {};
    const currentConfig = stringifyYaml({
      metadata: {
        name: details?.metadata?.name || want?.metadata?.name,
        type: details?.metadata?.type || want?.metadata?.type,
        labels: details?.metadata?.labels || want?.metadata?.labels || {}
      },
      spec: {
        params: details?.spec?.params || want?.spec?.params || {},
        ...(details?.spec?.using || want?.spec?.using) && { using: details?.spec?.using || want?.spec?.using },
        ...(details?.spec?.recipe || want?.spec?.recipe) && { recipe: details?.spec?.recipe || want?.spec?.recipe }
      }
    });
    setEditedConfig(currentConfig);
    setIsEditing(true);
    setUpdateError(null);
  };

  if (!want) return null;

  const wantDetails = selectedWantDetails;
  const hasAgentData = !!(wantDetails?.current_agent ||
    (wantDetails?.running_agents && wantDetails.running_agents.length > 0) ||
    (wantDetails?.history?.agentHistory && wantDetails.history.agentHistory.length > 0) ||
    (wantDetails?.state?.current_agent) ||
    (Array.isArray(wantDetails?.state?.running_agents) && (wantDetails?.state?.running_agents as any[]).length > 0) ||
    (Array.isArray(wantDetails?.state?.agent_history) && (wantDetails?.state?.agent_history as any[]).length > 0));

  const tabs = [
    { id: 'overview', label: 'Overview', icon: Eye },
    { id: 'config', label: 'Configuration', icon: Bot },
    { id: 'logs', label: 'Logs', icon: Clock },
    ...(hasAgentData ? [{ id: 'agents', label: 'Agents', icon: Users }] : []),
    { id: 'results', label: 'Results', icon: BarChart3 },
  ];

  const footer = (
    <div className="flex items-center justify-between">
      <div className="flex items-center space-x-4">
        {want.status === 'reaching' && (
          <label className="flex items-center text-xs font-bold text-gray-500 uppercase tracking-wider cursor-pointer">
            <input
              type="checkbox"
              checked={autoRefresh}
              onChange={(e) => setAutoRefresh(e.target.checked)}
              className="rounded-md border-gray-300 text-blue-600 focus:ring-blue-500 mr-2 h-4 w-4"
            />
            Auto-refresh
          </label>
        )}
      </div>
      <div className="flex items-center space-x-3">
        <button
          onClick={handleRefresh}
          disabled={loading}
          className="flex items-center px-4 py-2 text-sm font-bold text-gray-700 bg-white border border-gray-200 rounded-xl hover:bg-gray-50 transition-all shadow-sm disabled:opacity-50"
        >
          {loading ? <LoadingSpinner size="sm" className="mr-2" /> : <RefreshCw className="h-4 w-4 mr-2" />}
          Refresh
        </button>
        <button
          onClick={onClose}
          className="px-6 py-2 text-sm font-bold text-white bg-gray-900 rounded-xl hover:bg-gray-800 transition-all shadow-md"
        >
          Close
        </button>
      </div>
    </div>
  );

  return (
    <BaseModal
      isOpen={isOpen}
      onClose={onClose}
      title={wantDetails?.metadata?.name || want.metadata?.name || 'Want Details'}
      footer={footer}
      size="xl"
    >
      <div className="space-y-6">
        <div className="flex items-center space-x-3 -mt-2">
          <StatusBadge status={want.status} size="md" />
          <span className="text-xs font-mono text-gray-400">{want.metadata?.id || want.id}</span>
        </div>

        <div className="border-b border-gray-100">
          <nav className="flex space-x-8">
            {tabs.map((tab) => {
              const Icon = tab.icon;
              return (
                <button
                  key={tab.id}
                  onClick={() => setActiveTab(tab.id as TabType)}
                  className={classNames(
                    'flex items-center py-4 px-1 border-b-2 font-bold text-sm transition-all',
                    activeTab === tab.id
                      ? 'border-blue-600 text-blue-600'
                      : 'border-transparent text-gray-400 hover:text-gray-600'
                  )}
                >
                  <Icon className="h-4 w-4 mr-2" />
                  {tab.label}
                </button>
              );
            })}
          </nav>
        </div>

        <div className="h-[50vh] overflow-y-auto pr-2 custom-scrollbar">
          {activeTab === 'overview' && (
            <div className="space-y-6 animate-in fade-in slide-in-from-bottom-2">
              {/* Basic Info & Timeline Grid */}
              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div className="bg-gray-50 rounded-2xl p-5 border border-gray-100">
                  <h4 className="font-bold text-gray-900 mb-4 text-xs uppercase tracking-wider">Basic Information</h4>
                  <dl className="space-y-3">
                    <div className="flex justify-between items-center">
                      <dt className="text-sm text-gray-500">Type</dt>
                      <dd className="text-sm font-bold text-gray-900">{wantDetails?.metadata?.type || 'Unknown'}</dd>
                    </div>
                    <div className="flex justify-between items-center">
                      <dt className="text-sm text-gray-500">Name</dt>
                      <dd className="text-sm font-bold text-gray-900 truncate max-w-[200px]">{wantDetails?.metadata?.name || 'Unnamed'}</dd>
                    </div>
                  </dl>
                </div>

                <div className="bg-gray-50 rounded-2xl p-5 border border-gray-100">
                  <h4 className="font-bold text-gray-900 mb-4 text-xs uppercase tracking-wider">Timeline</h4>
                  <dl className="space-y-3">
                    <div className="flex justify-between items-center">
                      <dt className="text-sm text-gray-500">Created</dt>
                      <dd className="text-xs font-mono text-gray-700">{formatDate(wantDetails?.stats?.created_at)}</dd>
                    </div>
                    <div className="flex justify-between items-center">
                      <dt className="text-sm text-gray-500">Achieved</dt>
                      <dd className="text-xs font-mono text-gray-700">{formatDate(wantDetails?.stats?.completed_at)}</dd>
                    </div>
                  </dl>
                </div>
              </div>

              {/* Parameters Box */}
              <div className="space-y-3">
                <h4 className="font-bold text-gray-900 text-xs uppercase tracking-wider">Input Parameters</h4>
                <div className="bg-gray-900 rounded-2xl p-5 shadow-inner">
                  <pre className="text-xs text-blue-300 overflow-auto font-mono leading-relaxed">
                    {JSON.stringify(wantDetails?.spec?.params, null, 2)}
                  </pre>
                </div>
              </div>
            </div>
          )}

          {activeTab === 'config' && (
            <div className="space-y-4 h-full animate-in fade-in">
              <YamlEditor
                value={stringifyYaml(wantDetails || want)}
                onChange={() => {}}
                readOnly={true}
                height="100%"
              />
            </div>
          )}

          {activeTab === 'results' && (
            <div className="space-y-4 animate-in fade-in">
              <div className="bg-blue-900 rounded-2xl p-6 shadow-xl border border-blue-800">
                <h4 className="text-blue-200 font-bold text-xs uppercase tracking-wider mb-4">Runtime Result</h4>
                <pre className="text-sm text-white overflow-auto font-mono leading-relaxed custom-scrollbar max-h-[400px]">
                  {JSON.stringify(wantDetails?.state || want.state, null, 2)}
                </pre>
              </div>
            </div>
          )}
        </div>
      </div>
    </BaseModal>
  );
};