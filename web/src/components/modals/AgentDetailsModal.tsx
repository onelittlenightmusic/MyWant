import React, { useEffect, useState } from 'react';
import { X, Bot, Monitor, Zap, Settings, Eye, Code } from 'lucide-react';
import { AgentResponse } from '@/types/agent';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { ErrorDisplay } from '@/components/common/ErrorDisplay';
import { useAgentStore } from '@/stores/agentStore';
import { classNames } from '@/utils/helpers';

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

  if (!isOpen || !agent) return null;

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

  return (
    <div className="fixed inset-0 z-50 overflow-y-auto">
      <div className="flex items-center justify-center min-h-screen px-4 pt-4 pb-20 text-center sm:block sm:p-0">
        {/* Background overlay */}
        <div
          className="fixed inset-0 transition-opacity bg-gray-500 bg-opacity-75"
          onClick={onClose}
        />

        {/* Modal panel */}
        <div className="inline-block w-full max-w-4xl my-8 overflow-hidden text-left align-middle transition-all transform bg-white shadow-xl rounded-lg">
          {/* Header */}
          <div className="flex items-center justify-between p-6 border-b border-gray-200">
            <div className="flex items-center space-x-3">
              <div className={classNames(
                'inline-flex items-center px-3 py-1 rounded-full text-sm font-medium border',
                getTypeColor()
              )}>
                {getTypeIcon()}
                <span className="ml-2 capitalize">{agent.type} Agent</span>
              </div>
              <div>
                <h3 className="text-lg font-semibold text-gray-900">
                  {agent.name}
                </h3>
              </div>
            </div>

            <button
              onClick={onClose}
              className="text-gray-400 hover:text-gray-600 transition-colors"
            >
              <X className="h-6 w-6" />
            </button>
          </div>

          {/* Tabs */}
          <div className="border-b border-gray-200">
            <nav className="flex space-x-8 px-6">
              {tabs.map((tab) => {
                const Icon = tab.icon;
                return (
                  <button
                    key={tab.id}
                    onClick={() => setActiveTab(tab.id as TabType)}
                    className={classNames(
                      'flex items-center py-4 px-1 border-b-2 font-medium text-sm transition-colors',
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

          {/* Content */}
          <div className="p-6">
            {loading && (
              <div className="flex items-center justify-center py-8">
                <LoadingSpinner size="lg" />
                <span className="ml-3 text-gray-600">Loading agent details...</span>
              </div>
            )}

            {error && (
              <ErrorDisplay
                error={error}
                className="mb-4"
              />
            )}

            {/* Overview Tab */}
            {activeTab === 'overview' && (
              <div className="space-y-6">
                <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                  {/* Basic Info */}
                  <div className="bg-gray-50 rounded-lg p-4">
                    <h4 className="font-medium text-gray-900 mb-3">Basic Information</h4>
                    <dl className="space-y-2">
                      <div className="flex justify-between">
                        <dt className="text-sm text-gray-600">Name:</dt>
                        <dd className="text-sm font-medium text-gray-900">{agent.name}</dd>
                      </div>
                      <div className="flex justify-between">
                        <dt className="text-sm text-gray-600">Type:</dt>
                        <dd className="text-sm font-medium text-gray-900 capitalize">{agent.type}</dd>
                      </div>
                      <div className="flex justify-between">
                        <dt className="text-sm text-gray-600">Status:</dt>
                        <dd className="flex items-center">
                          <div className="w-2 h-2 rounded-full bg-green-500 mr-2" />
                          <span className="text-sm text-green-600 font-medium">Active</span>
                        </dd>
                      </div>
                    </dl>
                  </div>

                  {/* Statistics */}
                  <div className="bg-gray-50 rounded-lg p-4">
                    <h4 className="font-medium text-gray-900 mb-3">Statistics</h4>
                    <dl className="space-y-2">
                      <div className="flex justify-between">
                        <dt className="text-sm text-gray-600">Capabilities:</dt>
                        <dd className="text-sm font-medium text-gray-900">{agent.capabilities.length}</dd>
                      </div>
                      <div className="flex justify-between">
                        <dt className="text-sm text-gray-600">Dependencies:</dt>
                        <dd className="text-sm font-medium text-gray-900">{agent.uses.length}</dd>
                      </div>
                    </dl>
                  </div>
                </div>
              </div>
            )}

            {/* Capabilities Tab */}
            {activeTab === 'capabilities' && (
              <div>
                <h4 className="font-medium text-gray-900 mb-4">Agent Capabilities</h4>
                {agent.capabilities.length > 0 ? (
                  <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
                    {agent.capabilities.map((capability, index) => (
                      <div
                        key={index}
                        className="bg-purple-50 border border-purple-200 rounded-lg p-3"
                      >
                        <div className="flex items-center">
                          <Settings className="h-4 w-4 text-purple-600 mr-2" />
                          <span className="text-sm font-medium text-purple-800">
                            {capability}
                          </span>
                        </div>
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className="text-center py-8 text-gray-500">
                    No capabilities defined for this agent.
                  </div>
                )}
              </div>
            )}

            {/* Dependencies Tab */}
            {activeTab === 'dependencies' && (
              <div>
                <h4 className="font-medium text-gray-900 mb-4">Agent Dependencies</h4>
                {agent.uses.length > 0 ? (
                  <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
                    {agent.uses.map((dependency, index) => (
                      <div
                        key={index}
                        className="bg-orange-50 border border-orange-200 rounded-lg p-3"
                      >
                        <div className="flex items-center">
                          <Bot className="h-4 w-4 text-orange-600 mr-2" />
                          <span className="text-sm font-medium text-orange-800">
                            {dependency}
                          </span>
                        </div>
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className="text-center py-8 text-gray-500">
                    No dependencies defined for this agent.
                  </div>
                )}
              </div>
            )}

            {/* Configuration Tab */}
            {activeTab === 'config' && (
              <div>
                <h4 className="font-medium text-gray-900 mb-4">Agent Configuration</h4>
                <div className="bg-gray-50 rounded-lg p-4">
                  <pre className="text-sm text-gray-800 overflow-auto">
                    {JSON.stringify(agent, null, 2)}
                  </pre>
                </div>
              </div>
            )}
          </div>

          {/* Footer */}
          <div className="flex justify-end px-6 py-4 bg-gray-50 border-t border-gray-200">
            <button
              onClick={onClose}
              className="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
            >
              Close
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};