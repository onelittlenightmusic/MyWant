import React, { useState } from 'react';
import { Zap, Settings, Database, Share2, BookOpen, FileText, List, Download, Play, Rocket } from 'lucide-react';
import { WantTypeDefinition, ExampleDef } from '@/types/wantType';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { classNames } from '@/utils/helpers';
import { getBackgroundStyle, getBackgroundOverlayClass } from '@/utils/backgroundStyles';
import {
  TabContent,
  TabSection,
  InfoRow,
  EmptyState,
} from './DetailsSidebar';

interface WantTypeDetailsSidebarProps {
  wantType: WantTypeDefinition | null;
  onDownload?: (wantType: WantTypeDefinition) => void;
  onDeployExample?: (example: ExampleDef) => void;
}

type TabType = 'overview' | 'parameters' | 'state' | 'connectivity' | 'agents' | 'examples' | 'constraints';

export const WantTypeDetailsSidebar: React.FC<WantTypeDetailsSidebarProps> = ({
  wantType,
  onDownload,
  onDeployExample
}) => {
  const [activeTab, setActiveTab] = useState<TabType>('overview');
  const [deployingExample, setDeployingExample] = useState<string | null>(null);

  const tabs = React.useMemo(() => [
    { id: 'overview' as TabType, label: 'Overview', icon: FileText },
    { id: 'parameters' as TabType, label: 'Parameters', icon: Settings },
    { id: 'state' as TabType, label: 'State', icon: Database },
    { id: 'connectivity' as TabType, label: 'Connectivity', icon: Share2 },
    { id: 'agents' as TabType, label: 'Agents', icon: Zap },
    { id: 'examples' as TabType, label: 'Examples', icon: BookOpen },
    { id: 'constraints' as TabType, label: 'Constraints', icon: List }
  ], []);

  // Tab switching shortcut
  React.useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement;
      const isInputElement =
        target.tagName === 'INPUT' ||
        target.tagName === 'TEXTAREA' ||
        target.isContentEditable;

      if (isInputElement) return;

      if (e.key === 'Tab') {
        const isFocusOnCard = !!target.closest('[data-keyboard-nav-id]');
        if (isFocusOnCard) {
          e.preventDefault();
          const currentIndex = tabs.findIndex(t => t.id === activeTab);
          const nextIndex = (currentIndex + (e.shiftKey ? -1 : 1) + tabs.length) % tabs.length;
          setActiveTab(tabs[nextIndex].id);
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [activeTab, tabs]);

  if (!wantType) {
    return (
      <div className="text-center py-12">
        <BookOpen className="h-12 w-12 text-gray-400 mx-auto mb-4" />
        <p className="text-gray-500">Select a want type to view details</p>
      </div>
    );
  }

  const handleDownloadClick = () => {
    if (wantType && onDownload) {
      onDownload(wantType);
    }
  };

  const handleDeployExample = async (example: ExampleDef) => {
    if (!onDeployExample) return;

    setDeployingExample(example.name);
    try {
      await onDeployExample(example);
    } catch (error) {
      console.error('Failed to deploy example:', error);
    } finally {
      setDeployingExample(null);
    }
  };

  // Get background style for want type detail sidebar
  const sidebarBackgroundStyle = getBackgroundStyle(wantType.metadata.name, true);

  return (
    <div className="h-full flex flex-col" style={sidebarBackgroundStyle.style}>
      {/* Overlay - semi-transparent background */}
      {sidebarBackgroundStyle.hasBackgroundImage && (
        <div className={getBackgroundOverlayClass()}></div>
      )}

      {/* Sidebar content */}
      <div className="h-full flex flex-col relative z-10">
      {/* Control Panel Buttons - Icon Only, Minimal Height */}
      {wantType && (
        <div className="flex-shrink-0 border-b border-gray-200 dark:border-gray-700 px-4 py-2 flex gap-1 justify-center">
          {/* Deploy first example */}
          {wantType.examples.length > 0 && (
            <button
              onClick={() => {
                handleDeployExample(wantType.examples[0]);
              }}
              disabled={deployingExample !== null}
              title={`Deploy ${wantType.examples[0].name}`}
              className={classNames(
                'p-2 rounded-md transition-colors',
                deployingExample !== null
                  ? 'bg-gray-200 text-gray-600 cursor-wait'
                  : 'bg-blue-100 text-blue-600 hover:bg-blue-200'
              )}
            >
              {deployingExample ? <Rocket className="h-4 w-4 animate-bounce" /> : <Rocket className="h-4 w-4" />}
            </button>
          )}

          {/* Download */}
          <button
            onClick={handleDownloadClick}
            disabled={!wantType}
            title={wantType ? 'Download want type as JSON' : 'No want type selected'}
            className={classNames(
              'p-2 rounded-md transition-colors',
              wantType
                ? 'bg-purple-100 text-purple-600 hover:bg-purple-200'
                : 'bg-gray-100 text-gray-400 cursor-not-allowed'
            )}
          >
            <Download className="h-4 w-4" />
          </button>
        </div>
      )}

      {/* Tab Navigation */}
      <div className="border-b border-gray-200 dark:border-gray-700 px-3 sm:px-6 py-2 sm:py-4">
        <div className="flex space-x-1 bg-gray-100 dark:bg-gray-800 rounded-lg p-1 overflow-x-auto">
          {tabs.map((tab) => {
            const Icon = tab.icon;
            return (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={classNames(
                  'flex items-center justify-center space-x-1 px-2 py-1.5 sm:py-2 text-xs sm:text-sm font-medium rounded-md transition-colors flex-shrink-0',
                  activeTab === tab.id
                    ? 'bg-white dark:bg-gray-700 text-blue-600 dark:text-blue-400 shadow-sm'
                    : 'text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-200'
                )}
              >
                <Icon className="h-3.5 w-3.5 sm:h-4 sm:w-4 flex-shrink-0" />
                <span className="truncate text-[10px] sm:text-xs whitespace-nowrap">{tab.label}</span>
              </button>
            );
          })}
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto">
        {activeTab === 'overview' && <OverviewTab wantType={wantType} />}
        {activeTab === 'parameters' && <ParametersTab wantType={wantType} />}
        {activeTab === 'state' && <StateTab wantType={wantType} />}
        {activeTab === 'connectivity' && <ConnectivityTab wantType={wantType} />}
        {activeTab === 'agents' && <AgentsTab wantType={wantType} />}
        {activeTab === 'examples' && (
          <ExamplesTab
            wantType={wantType}
            onDeployExample={onDeployExample}
            deployingExample={deployingExample}
          />
        )}
        {activeTab === 'constraints' && <ConstraintsTab wantType={wantType} />}
      </div>
      </div>
    </div>
  );
};

// Tab Components
const OverviewTab: React.FC<{ wantType: WantTypeDefinition }> = ({ wantType }) => (
  <TabContent>
    <TabSection title="Metadata">
      <div className="space-y-2 sm:space-y-3">
        <InfoRow label="Name" value={wantType.metadata.name} />
        <InfoRow label="Title" value={wantType.metadata.title} />
        <InfoRow label="Version" value={<span className="font-mono">{wantType.metadata.version}</span>} />
        <InfoRow label="Category" value={<span className="capitalize">{wantType.metadata.category}</span>} />
        <InfoRow label="Pattern" value={<span className="capitalize">{wantType.metadata.pattern}</span>} />
      </div>
    </TabSection>

    <TabSection title="Description">
      <p className="text-xs sm:text-sm text-gray-600 dark:text-gray-400 whitespace-pre-wrap leading-relaxed">{wantType.metadata.description}</p>
    </TabSection>

    <TabSection title="Summary">
      <div className="space-y-2 sm:space-y-3">
        <InfoRow label="Parameters" value={wantType.parameters.length} />
        <InfoRow label="State Keys" value={wantType.state.length} />
        <InfoRow label="Require Type" value={wantType.require?.type || 'none'} />
        <InfoRow label="Agents" value={wantType.agents.length} />
        <InfoRow label="Examples" value={wantType.examples.length} />
      </div>
    </TabSection>

    {wantType.relatedTypes && wantType.relatedTypes.length > 0 && (
      <TabSection title="Related Types">
        <div className="flex flex-wrap gap-2">
          {wantType.relatedTypes.map((type) => (
            <div key={type} className="px-2.5 py-1 bg-gray-100 dark:bg-gray-700 rounded text-xs text-gray-700 dark:text-gray-300 border border-gray-200 dark:border-gray-600">
              {type}
            </div>
          ))}
        </div>
      </TabSection>
    )}
  </TabContent>
);

const ParametersTab: React.FC<{ wantType: WantTypeDefinition }> = ({ wantType }) => (
  <TabContent>
    {wantType.parameters && wantType.parameters.length > 0 ? (
      <div className="space-y-3 sm:space-y-4">
        {wantType.parameters.map((param) => (
          <TabSection key={param.name} title={param.name}>
            <div className="space-y-2 text-xs sm:text-sm">
              <div className="flex justify-between">
                <span className="text-gray-600 dark:text-gray-400">Type:</span>
                <span className="font-mono text-gray-900 dark:text-gray-200">{param.type}</span>
              </div>
              {param.default !== undefined && (
                <div className="flex justify-between">
                  <span className="text-gray-600 dark:text-gray-400">Default:</span>
                  <span className="font-mono text-gray-900 dark:text-gray-200">{String(param.default)}</span>
                </div>
              )}
              <div className="flex justify-between">
                <span className="text-gray-600 dark:text-gray-400">Required:</span>
                <span className="text-gray-900 dark:text-gray-200">{param.required ? 'Yes' : 'No'}</span>
              </div>
              {param.description && (
                <div className="pt-1">
                  <span className="text-gray-600 dark:text-gray-400">Description:</span>
                  <p className="mt-1 text-gray-700 dark:text-gray-300 leading-relaxed">{param.description}</p>
                </div>
              )}
              {param.validation && (
                <div className="mt-2 p-2 bg-gray-100 dark:bg-gray-700/50 rounded border border-gray-200 dark:border-gray-600">
                  <span className="text-gray-600 dark:text-gray-400 text-[10px] sm:text-xs font-medium uppercase tracking-wider">Validation Rules:</span>
                  <div className="mt-1 space-y-1">
                    {param.validation.min !== undefined && (
                      <div className="text-[10px] sm:text-xs text-gray-700 dark:text-gray-300">Min: {param.validation.min}</div>
                    )}
                    {param.validation.max !== undefined && (
                      <div className="text-[10px] sm:text-xs text-gray-700 dark:text-gray-300">Max: {param.validation.max}</div>
                    )}
                    {param.validation.enum && (
                      <div className="text-[10px] sm:text-xs text-gray-700 dark:text-gray-300">
                        Enum: {param.validation.enum.join(', ')}
                      </div>
                    )}
                  </div>
                </div>
              )}
            </div>
          </TabSection>
        ))}
      </div>
    ) : (
      <EmptyState icon={Settings} message="No parameters defined" />
    )}
  </TabContent>
);

const StateTab: React.FC<{ wantType: WantTypeDefinition }> = ({ wantType }) => (
  <TabContent>
    {wantType.state && wantType.state.length > 0 ? (
      <div className="space-y-3 sm:space-y-4">
        {wantType.state.map((state) => (
          <TabSection key={state.name} title={state.name}>
            <div className="space-y-2 text-xs sm:text-sm">
              <div className="flex justify-between">
                <span className="text-gray-600 dark:text-gray-400">Type:</span>
                <span className="font-mono text-gray-900 dark:text-gray-200">{state.type}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-600 dark:text-gray-400">Persistent:</span>
                <span className="text-gray-900 dark:text-gray-200">{state.persistent ? 'Yes' : 'No'}</span>
              </div>
              {state.description && (
                <div className="pt-1">
                  <span className="text-gray-600 dark:text-gray-400">Description:</span>
                  <p className="mt-1 text-gray-700 dark:text-gray-300 leading-relaxed">{state.description}</p>
                </div>
              )}
            </div>
          </TabSection>
        ))}
      </div>
    ) : (
      <EmptyState icon={Database} message="No state keys defined" />
    )}
  </TabContent>
);

const ConnectivityTab: React.FC<{ wantType: WantTypeDefinition }> = ({ wantType }) => (
  <TabContent>
    {wantType.require && (
      <TabSection title="Require">
        <div className="space-y-3 text-xs sm:text-sm">
          <div className="flex justify-between">
            <span className="text-gray-600 dark:text-gray-400">Type:</span>
            <span className="font-semibold capitalize text-gray-900 dark:text-gray-200">{wantType.require.type}</span>
          </div>

          {wantType.require.providers && wantType.require.providers.length > 0 && (
            <div>
              <span className="text-gray-600 dark:text-gray-400">Providers (Input Connections):</span>
              <div className="mt-2 space-y-2">
                {wantType.require.providers.map((provider) => (
                  <div key={provider.name} className="p-2.5 bg-blue-50 dark:bg-blue-900/20 rounded border border-blue-100 dark:border-blue-800/50">
                    <div className="font-semibold text-blue-900 dark:text-blue-300">{provider.name}</div>
                    <div className="text-[10px] sm:text-xs text-blue-700 dark:text-blue-400 font-mono mt-0.5">{provider.type}</div>
                    {provider.description && (
                      <p className="text-[10px] sm:text-xs text-blue-800 dark:text-blue-300/80 mt-1.5 leading-relaxed">{provider.description}</p>
                    )}
                    <div className="text-[10px] sm:text-xs text-blue-700 dark:text-blue-400 mt-2 flex gap-3">
                      <span>Required: <span className="font-medium">{provider.required ? 'Yes' : 'No'}</span></span>
                      <span>Multiple: <span className="font-medium">{provider.multiple ? 'Yes' : 'No'}</span></span>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {wantType.require.users && wantType.require.users.length > 0 && (
            <div>
              <span className="text-gray-600 dark:text-gray-400">Users (Output Connections):</span>
              <div className="mt-2 space-y-2">
                {wantType.require.users.map((user) => (
                  <div key={user.name} className="p-2.5 bg-green-50 dark:bg-green-900/20 rounded border border-green-100 dark:border-green-800/50">
                    <div className="font-semibold text-green-900 dark:text-green-300">{user.name}</div>
                    <div className="text-[10px] sm:text-xs text-green-700 dark:text-green-400 font-mono mt-0.5">{user.type}</div>
                    {user.description && (
                      <p className="text-[10px] sm:text-xs text-green-800 dark:text-green-300/80 mt-1.5 leading-relaxed">{user.description}</p>
                    )}
                    <div className="text-[10px] sm:text-xs text-green-700 dark:text-green-400 mt-2 flex gap-3">
                      <span>Required: <span className="font-medium">{user.required ? 'Yes' : 'No'}</span></span>
                      <span>Multiple: <span className="font-medium">{user.multiple ? 'Yes' : 'No'}</span></span>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </TabSection>
    )}

    <TabSection title="Inputs (Legacy)">
      {wantType.connectivity?.inputs && wantType.connectivity.inputs.length > 0 ? (
        <div className="space-y-2 sm:space-y-3">
          {wantType.connectivity.inputs.map((input) => (
            <div key={input.name} className="p-2.5 bg-blue-50 dark:bg-blue-900/10 rounded border border-blue-100/50 dark:border-blue-900/30">
              <div className="font-semibold text-blue-900 dark:text-blue-300 text-xs sm:text-sm">{input.name}</div>
              <div className="text-[10px] sm:text-xs text-blue-700 dark:text-blue-400 font-mono">{input.type}</div>
              {input.description && (
                <p className="text-[10px] sm:text-xs text-blue-800 dark:text-blue-400/80 mt-1">{input.description}</p>
              )}
            </div>
          ))}
        </div>
      ) : (
        <p className="text-xs sm:text-sm text-gray-500 dark:text-gray-400 italic">No legacy inputs</p>
      )}
    </TabSection>

    <TabSection title="Outputs (Legacy)">
      {wantType.connectivity?.outputs && wantType.connectivity.outputs.length > 0 ? (
        <div className="space-y-2 sm:space-y-3">
          {wantType.connectivity.outputs.map((output) => (
            <div key={output.name} className="p-2.5 bg-green-50 dark:bg-green-900/10 rounded border border-green-100/50 dark:border-green-900/30">
              <div className="font-semibold text-green-900 dark:text-green-300 text-xs sm:text-sm">{output.name}</div>
              <div className="text-[10px] sm:text-xs text-green-700 dark:text-green-400 font-mono">{output.type}</div>
              {output.description && (
                <p className="text-[10px] sm:text-xs text-green-800 dark:text-green-400/80 mt-1">{output.description}</p>
              )}
            </div>
          ))}
        </div>
      ) : (
        <p className="text-xs sm:text-sm text-gray-500 dark:text-gray-400 italic">No legacy outputs</p>
      )}
    </TabSection>
  </TabContent>
);

const AgentsTab: React.FC<{ wantType: WantTypeDefinition }> = ({ wantType }) => (
  <TabContent>
    {wantType.agents && wantType.agents.length > 0 ? (
      <div className="space-y-3 sm:space-y-4">
        {wantType.agents.map((agent) => (
          <TabSection key={agent.name} title={agent.name}>
            <div className="space-y-2 text-xs sm:text-sm">
              <div className="flex justify-between">
                <span className="text-gray-600 dark:text-gray-400">Role:</span>
                <span className="font-semibold text-gray-900 dark:text-gray-200 capitalize">{agent.role}</span>
              </div>
              {agent.description && (
                <div className="pt-1">
                  <span className="text-gray-600 dark:text-gray-400">Description:</span>
                  <p className="mt-1 text-gray-700 dark:text-gray-300 leading-relaxed">{agent.description}</p>
                </div>
              )}
              {agent.example && (
                <div className="mt-2 p-2 bg-gray-100 dark:bg-gray-700/50 rounded border border-gray-200 dark:border-gray-600">
                  <span className="text-gray-600 dark:text-gray-400 text-[10px] sm:text-xs font-medium uppercase tracking-wider">Example:</span>
                  <p className="mt-1 text-[10px] sm:text-xs text-gray-700 dark:text-gray-300 font-mono break-all">{agent.example}</p>
                </div>
              )}
            </div>
          </TabSection>
        ))}
      </div>
    ) : (
      <EmptyState icon={Zap} message="No agents defined" />
    )}
  </TabContent>
);

const ExamplesTab: React.FC<{
  wantType: WantTypeDefinition;
  onDeployExample?: (example: ExampleDef) => void | Promise<void>;
  deployingExample?: string | null;
}> = ({ wantType, onDeployExample, deployingExample }) => (
  <TabContent>
    {wantType.examples && wantType.examples.length > 0 ? (
      <div className="space-y-3 sm:space-y-4">
        {wantType.examples.map((example, index) => (
          <TabSection key={index} title={example.name}>
            <div className="space-y-3 text-xs sm:text-sm">
              <div>
                <span className="text-gray-600 dark:text-gray-400">Description:</span>
                <p className="mt-1 text-gray-700 dark:text-gray-300 leading-relaxed">{example.description}</p>
              </div>
              <div>
                <span className="text-gray-600 dark:text-gray-400 font-medium">Want Configuration:</span>
                <div className="mt-1.5 relative group">
                  <pre className="text-[10px] sm:text-xs bg-gray-100 dark:bg-gray-900 p-2.5 rounded border border-gray-200 dark:border-gray-700 overflow-x-auto font-mono text-gray-800 dark:text-gray-200 max-h-48">
                    {JSON.stringify(example.want, null, 2)}
                  </pre>
                </div>
              </div>
              {example.expectedBehavior && (
                <div>
                  <span className="text-gray-600 dark:text-gray-400 font-medium">Expected Behavior:</span>
                  <p className="mt-1 text-gray-700 dark:text-gray-300 whitespace-pre-wrap text-[10px] sm:text-xs leading-relaxed italic">
                    {example.expectedBehavior}
                  </p>
                </div>
              )}
              {onDeployExample && (
                <div className="mt-2 pt-3 border-t border-gray-200 dark:border-gray-700 flex justify-end">
                  <button
                    onClick={() => onDeployExample(example)}
                    disabled={deployingExample === example.name}
                    className={classNames(
                      'flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-md transition-all shadow-sm',
                      deployingExample === example.name
                        ? 'bg-gray-100 dark:bg-gray-800 text-gray-400 cursor-wait'
                        : 'bg-blue-600 text-white hover:bg-blue-700 active:scale-95'
                    )}
                  >
                    {deployingExample === example.name ? (
                      <LoadingSpinner size="sm" />
                    ) : (
                      <Play className="h-3 w-3 fill-current" />
                    )}
                    {deployingExample === example.name ? 'Deploying...' : 'Deploy Example'}
                  </button>
                </div>
              )}
            </div>
          </TabSection>
        ))}
      </div>
    ) : (
      <EmptyState icon={BookOpen} message="No examples provided" />
    )}
  </TabContent>
);

const ConstraintsTab: React.FC<{ wantType: WantTypeDefinition }> = ({ wantType }) => (
  <TabContent>
    {wantType.constraints && wantType.constraints.length > 0 ? (
      <div className="space-y-3 sm:space-y-4">
        {wantType.constraints.map((constraint, index) => (
          <TabSection key={index} title={`Constraint ${index + 1}`}>
            <div className="space-y-2 text-xs sm:text-sm">
              <div>
                <span className="text-gray-600 dark:text-gray-400">Description:</span>
                <p className="mt-1 text-gray-700 dark:text-gray-300 leading-relaxed">{constraint.description}</p>
              </div>
              <div className="mt-2">
                <span className="text-gray-600 dark:text-gray-400 font-medium">Validation:</span>
                <pre className="mt-1 text-[10px] sm:text-xs bg-gray-100 dark:bg-gray-900 p-2 rounded border border-gray-200 dark:border-gray-700 font-mono text-pink-600 dark:text-pink-400 overflow-x-auto">
                  {constraint.validation}
                </pre>
              </div>
            </div>
          </TabSection>
        ))}
      </div>
    ) : (
      <EmptyState icon={List} message="No constraints defined" />
    )}
  </TabContent>
);
// Helper function for want type download
function downloadWantTypeJSON(wantType: WantTypeDefinition): void {
  const filename = `${wantType.metadata.name}.json`;
  const jsonContent = JSON.stringify(wantType, null, 2);

  const element = document.createElement('a');
  element.setAttribute(
    'href',
    `data:application/json;charset=utf-8,${encodeURIComponent(jsonContent)}`
  );
  element.setAttribute('download', filename);
  element.style.display = 'none';

  document.body.appendChild(element);
  element.click();
  document.body.removeChild(element);
}

