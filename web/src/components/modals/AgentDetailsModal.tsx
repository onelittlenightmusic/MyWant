import React, { useEffect, useState } from 'react';
import { Bot, Monitor, Zap, Settings, Eye, Code } from 'lucide-react';
import { AgentResponse } from '@/types/agent';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { ErrorDisplay } from '@/components/common/ErrorDisplay';
import { useAgentStore } from '@/stores/agentStore';
import { classNames } from '@/utils/helpers';
import { BaseModal } from './BaseModal';

interface AgentDetailsModalProps {
  isOpen: boolean;
  onClose: () => void;
  agent: AgentResponse | null;
}

type TabType = 'overview' | 'capabilities' | 'dependencies' | 'config';

export const AgentDetailsModal: React.FC<AgentDetailsModalProps> = ({
  isOpen,
  onClose,
  agent
}) => {
  const { loading, error } = useAgentStore();
  const [activeTab, setActiveTab] = useState<TabType>('overview');

  // Reset tab when modal opens
  useEffect(() => {
    if (isOpen) {
      setActiveTab('overview');
    }
  }, [isOpen]);

  if (!agent) return null;

  const getTypeIcon = () => {
    switch (agent.type) {
      case 'do':
        return <Zap className="h-5 w-5" />;
      case 'monitor':
        return <Monitor className="h-5 w-5" />;
      default:
        return <Bot className="h-5 w-5" />;
    }
  };

  const getTypeColor = () => {
    switch (agent.type) {
      case 'do':
        return 'bg-blue-100 text-blue-800 border-blue-200';
      case 'monitor':
        return 'bg-green-100 text-green-800 border-green-200';
      default:
        return 'bg-gray-100 text-gray-800 border-gray-200';
    }
  };

  const tabs = [
    { id: 'overview', label: 'Overview', icon: Eye },
    { id: 'capabilities', label: 'Capabilities', icon: Settings },
    { id: 'dependencies', label: 'Dependencies', icon: Bot },
    { id: 'config', label: 'Configuration', icon: Code }
  ];

  const footer = (
    <div className="flex justify-end">
      <button
        onClick={onClose}
        className="px-6 py-2 text-sm font-bold text-gray-700 bg-white border border-gray-200 rounded-xl hover:bg-gray-50 transition-colors shadow-sm"
      >
        Close
      </button>
    </div>
  );

  return (
    <BaseModal
      isOpen={isOpen}
      onClose={onClose}
      title={agent.name}
      footer={footer}
      size="lg"
    >
      <div className="space-y-6">
        {/* Header Tags */}
        <div className="flex items-center space-x-3 -mt-2 mb-4">
          <div className={classNames(
            'inline-flex items-center px-3 py-1 rounded-full text-sm font-medium border',
            getTypeColor()
          )}>
            {getTypeIcon()}
            <span className="ml-2 capitalize">{agent.type} Agent</span>
          </div>
        </div>

        {/* Tabs */}
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
                      : 'border-transparent text-gray-400 hover:text-gray-600 hover:border-gray-200'
                  )}
                >
                  <Icon className="h-4 w-4 mr-2" />
                  {tab.label}
                </button>
              );
            })}
          </nav>
        </div>

        {/* Content */}
        <div className="min-h-[300px]">
          {loading && (
            <div className="flex flex-col items-center justify-center py-12">
              <LoadingSpinner size="lg" />
              <span className="mt-4 text-sm font-medium text-gray-500">Loading agent details...</span>
            </div>
          )}

          {error && (
            <ErrorDisplay
              error={error}
              className="mb-4"
            />
          )}

          {/* Overview Tab */}
          {activeTab === 'overview' && !loading && (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-6 animate-in fade-in slide-in-from-bottom-2">
              <div className="bg-gray-50 rounded-2xl p-5 border border-gray-100">
                <h4 className="font-bold text-gray-900 mb-4 text-sm uppercase tracking-wider">Basic Information</h4>
                <dl className="space-y-3">
                  <div className="flex justify-between items-center">
                    <dt className="text-sm text-gray-500">Name</dt>
                    <dd className="text-sm font-bold text-gray-900">{agent.name}</dd>
                  </div>
                  <div className="flex justify-between items-center">
                    <dt className="text-sm text-gray-500">Type</dt>
                    <dd className="text-sm font-bold text-gray-900 capitalize">{agent.type}</dd>
                  </div>
                  <div className="flex justify-between items-center">
                    <dt className="text-sm text-gray-500">Status</dt>
                    <dd className="flex items-center">
                      <div className="w-2 h-2 rounded-full bg-green-500 mr-2 animate-pulse" />
                      <span className="text-sm text-green-600 font-bold">Active</span>
                    </dd>
                  </div>
                </dl>
              </div>

              <div className="bg-gray-50 rounded-2xl p-5 border border-gray-100">
                <h4 className="font-bold text-gray-900 mb-4 text-sm uppercase tracking-wider">Statistics</h4>
                <dl className="space-y-3">
                  <div className="flex justify-between items-center">
                    <dt className="text-sm text-gray-500">Capabilities</dt>
                    <dd className="text-sm font-bold text-gray-900 bg-white px-2 py-1 rounded-lg border border-gray-200">
                      {agent.capabilities.length}
                    </dd>
                  </div>
                  <div className="flex justify-between items-center">
                    <dt className="text-sm text-gray-500">Dependencies</dt>
                    <dd className="text-sm font-bold text-gray-900 bg-white px-2 py-1 rounded-lg border border-gray-200">
                      {agent.uses.length}
                    </dd>
                  </div>
                </dl>
              </div>
            </div>
          )}

          {/* Capabilities Tab */}
          {activeTab === 'capabilities' && !loading && (
            <div className="animate-in fade-in slide-in-from-bottom-2">
              <h4 className="font-bold text-gray-900 mb-4 text-sm uppercase tracking-wider">Agent Capabilities</h4>
              {agent.capabilities.length > 0 ? (
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  {agent.capabilities.map((capability, index) => (
                    <div
                      key={index}
                      className="bg-purple-50 border border-purple-100 rounded-xl p-4 flex items-center group hover:bg-purple-100 transition-colors"
                    >
                      <Settings className="h-5 w-5 text-purple-600 mr-3" />
                      <span className="text-sm font-bold text-purple-900">
                        {capability}
                      </span>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="text-center py-12 bg-gray-50 rounded-2xl border border-dashed border-gray-200">
                  <p className="text-gray-400 font-medium">No capabilities defined for this agent.</p>
                </div>
              )}
            </div>
          )}

          {/* Dependencies Tab */}
          {activeTab === 'dependencies' && !loading && (
            <div className="animate-in fade-in slide-in-from-bottom-2">
              <h4 className="font-bold text-gray-900 mb-4 text-sm uppercase tracking-wider">Agent Dependencies</h4>
              {agent.uses.length > 0 ? (
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  {agent.uses.map((dependency, index) => (
                    <div
                      key={index}
                      className="bg-orange-50 border border-orange-100 rounded-xl p-4 flex items-center group hover:bg-orange-100 transition-colors"
                    >
                      <Bot className="h-5 w-5 text-orange-600 mr-3" />
                      <span className="text-sm font-bold text-orange-900">
                        {dependency}
                      </span>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="text-center py-12 bg-gray-50 rounded-2xl border border-dashed border-gray-200">
                  <p className="text-gray-400 font-medium">No dependencies defined for this agent.</p>
                </div>
              )}
            </div>
          )}

          {/* Configuration Tab */}
          {activeTab === 'config' && !loading && (
            <div className="animate-in fade-in slide-in-from-bottom-2">
              <h4 className="font-bold text-gray-900 mb-4 text-sm uppercase tracking-wider">Agent Configuration</h4>
              <div className="bg-gray-900 rounded-2xl p-6 shadow-inner">
                <pre className="text-xs text-blue-300 overflow-auto font-mono leading-relaxed custom-scrollbar max-h-[400px]">
                  {JSON.stringify(agent, null, 2)}
                </pre>
              </div>
            </div>
          )}
        </div>
      </div>
    </BaseModal>
  );
};
