import React, { useState, useEffect } from 'react';
import { Bot, Monitor, Zap, Settings, Eye, Code } from 'lucide-react';
import { AgentResponse } from '@/types/agent';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { ErrorDisplay } from '@/components/common/ErrorDisplay';
import { useAgentStore } from '@/stores/agentStore';
import { classNames } from '@/utils/helpers';
import {
  DetailsSidebar,
  TabContent,
  TabSection,
  TabGrid,
  EmptyState,
  InfoRow,
  TabConfig
} from './DetailsSidebar';

interface AgentDetailsSidebarProps {
  agent: AgentResponse | null;
}

type TabType = 'overview' | 'capabilities' | 'dependencies' | 'config';

export const AgentDetailsSidebar: React.FC<AgentDetailsSidebarProps> = ({
  agent
}) => {
  const { loading, error } = useAgentStore();
  const [activeTab, setActiveTab] = useState<TabType>('overview');

  const tabs: TabConfig[] = [
    { id: 'overview', label: 'Overview', icon: Eye },
    { id: 'capabilities', label: 'Capabilities', icon: Settings },
    { id: 'dependencies', label: 'Dependencies', icon: Bot },
    { id: 'config', label: 'Config', icon: Code }
  ];

  // Tab switching shortcut
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement;
      const isInputElement =
        target.tagName === 'INPUT' ||
        target.tagName === 'TEXTAREA' ||
        target.isContentEditable;

      if (isInputElement) return;

      if (e.key === 'Tab') {
        // Look for any element with data-keyboard-nav-id (generic card focus)
        const isFocusOnCard = !!target.closest('[data-keyboard-nav-id]');
        if (isFocusOnCard) {
          e.preventDefault();
          const currentIndex = tabs.findIndex(t => t.id === activeTab);
          const nextIndex = (currentIndex + (e.shiftKey ? -1 : 1) + tabs.length) % tabs.length;
          setActiveTab(tabs[nextIndex].id as TabType);
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [activeTab, tabs]);

  const getTypeIcon = () => {
    if (!agent) return <Bot className="h-5 w-5" />;
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
    if (!agent) return 'bg-gray-100 text-gray-800 border-gray-200';
    switch (agent.type) {
      case 'do':
        return 'bg-blue-100 text-blue-800 border-blue-200';
      case 'monitor':
        return 'bg-green-100 text-green-800 border-green-200';
      default:
        return 'bg-gray-100 text-gray-800 border-gray-200';
    }
  };

  if (!agent) {
    return <EmptyState icon={Bot} message="Select an agent to view details" />;
  }

  const tabs: TabConfig[] = [
    { id: 'overview', label: 'Overview', icon: Eye },
    { id: 'capabilities', label: 'Capabilities', icon: Settings },
    { id: 'dependencies', label: 'Dependencies', icon: Bot },
    { id: 'config', label: 'Config', icon: Code }
  ];

  const badge = (
    <div className={classNames(
      'inline-flex items-center px-3 py-1 rounded-full text-sm font-medium border',
      getTypeColor()
    )}>
      {getTypeIcon()}
      <span className="ml-2 capitalize">{agent.type} Agent</span>
    </div>
  );

  return (
    <DetailsSidebar
      title={agent.name}
      badge={badge}
      tabs={tabs}
      defaultTab="overview"
      onTabChange={(tabId) => setActiveTab(tabId as TabType)}
    >
      {loading ? (
        <div className="flex items-center justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      ) : (
        <>
          {error && (
            <div className="p-6">
              <ErrorDisplay error={error} />
            </div>
          )}

          {activeTab === 'overview' && <OverviewTab agent={agent} />}
          {activeTab === 'capabilities' && <CapabilitiesTab agent={agent} />}
          {activeTab === 'dependencies' && <DependenciesTab agent={agent} />}
          {activeTab === 'config' && <ConfigurationTab agent={agent} />}
        </>
      )}
    </DetailsSidebar>
  );
};

// Tab Components
const OverviewTab: React.FC<{ agent: AgentResponse }> = ({ agent }) => (
  <TabContent>
    <TabGrid columns={2}>
      <TabSection title="Basic Information">
        <dl className="space-y-2">
          <InfoRow label="Name" value={agent.name} />
          <InfoRow label="Type" value={<span className="capitalize">{agent.type}</span>} />
          <InfoRow
            label="Status"
            value={
              <div className="flex items-center">
                <div className="w-2 h-2 rounded-full bg-green-500 mr-2" />
                <span className="text-sm text-green-600 font-medium">Active</span>
              </div>
            }
          />
        </dl>
      </TabSection>

      <TabSection title="Statistics">
        <dl className="space-y-2">
          <InfoRow label="Capabilities" value={agent.capabilities.length} />
          <InfoRow label="Dependencies" value={agent.uses.length} />
        </dl>
      </TabSection>
    </TabGrid>
  </TabContent>
);

const CapabilitiesTab: React.FC<{ agent: AgentResponse }> = ({ agent }) => (
  <TabContent>
    {agent.capabilities.length > 0 ? (
      <div className="space-y-3">
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
      <EmptyState icon={Settings} message="No capabilities defined for this agent." />
    )}
  </TabContent>
);

const DependenciesTab: React.FC<{ agent: AgentResponse }> = ({ agent }) => (
  <TabContent>
    {agent.uses.length > 0 ? (
      <div className="space-y-3">
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
      <EmptyState icon={Bot} message="No dependencies defined for this agent." />
    )}
  </TabContent>
);

const ConfigurationTab: React.FC<{ agent: AgentResponse }> = ({ agent }) => (
  <TabContent>
    <TabSection title="Agent Configuration">
      <pre className="text-xs text-gray-800 overflow-auto whitespace-pre-wrap">
        {JSON.stringify(agent, null, 2)}
      </pre>
    </TabSection>
  </TabContent>
);