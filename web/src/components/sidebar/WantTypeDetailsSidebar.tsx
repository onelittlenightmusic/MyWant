import React, { useState } from 'react';
import { Zap, Settings, Database, Share2, BookOpen, FileText, List, Download } from 'lucide-react';
import { WantTypeDefinition } from '@/types/wantType';
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
}

type TabType = 'overview' | 'parameters' | 'state' | 'connectivity' | 'agents' | 'examples' | 'constraints';

export const WantTypeDetailsSidebar: React.FC<WantTypeDetailsSidebarProps> = ({
  wantType,
  onDownload
}) => {
  const [activeTab, setActiveTab] = useState<TabType>('overview');

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

  // Get background style for want type detail sidebar
  const sidebarBackgroundStyle = getBackgroundStyle(wantType.metadata.name, true);

  const tabs = [
    { id: 'overview' as TabType, label: 'Overview', icon: FileText },
    { id: 'parameters' as TabType, label: 'Parameters', icon: Settings },
    { id: 'state' as TabType, label: 'State', icon: Database },
    { id: 'connectivity' as TabType, label: 'Connectivity', icon: Share2 },
    { id: 'agents' as TabType, label: 'Agents', icon: Zap },
    { id: 'examples' as TabType, label: 'Examples', icon: BookOpen },
    { id: 'constraints' as TabType, label: 'Constraints', icon: List }
  ];

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
        <div className="flex-shrink-0 border-b border-gray-200 px-4 py-2 flex gap-1 justify-center">
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
      <div className="border-b border-gray-200 px-6 py-4">
        <div className="flex space-x-1 bg-gray-100 rounded-lg p-1 overflow-x-auto">
          {tabs.map((tab) => {
            const Icon = tab.icon;
            return (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={classNames(
                  'flex items-center justify-center space-x-1 px-2 py-2 text-sm font-medium rounded-md transition-colors flex-shrink-0',
                  activeTab === tab.id
                    ? 'bg-white text-blue-600 shadow-sm'
                    : 'text-gray-600 hover:text-gray-900'
                )}
              >
                <Icon className="h-4 w-4 flex-shrink-0" />
                <span className="truncate text-xs whitespace-nowrap">{tab.label}</span>
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
        {activeTab === 'examples' && <ExamplesTab wantType={wantType} />}
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
      <div className="space-y-3">
        <InfoRow label="Name" value={wantType.metadata.name} />
        <InfoRow label="Title" value={wantType.metadata.title} />
        <InfoRow label="Version" value={<span className="font-mono">{wantType.metadata.version}</span>} />
        <InfoRow label="Category" value={<span className="capitalize">{wantType.metadata.category}</span>} />
        <InfoRow label="Pattern" value={<span className="capitalize">{wantType.metadata.pattern}</span>} />
      </div>
    </TabSection>

    <TabSection title="Description">
      <p className="text-sm text-gray-600 whitespace-pre-wrap">{wantType.metadata.description}</p>
    </TabSection>

    <TabSection title="Summary">
      <div className="space-y-3">
        <InfoRow label="Parameters" value={wantType.parameters.length} />
        <InfoRow label="State Keys" value={wantType.state.length} />
        <InfoRow label="Inputs" value={wantType.connectivity.inputs.length} />
        <InfoRow label="Outputs" value={wantType.connectivity.outputs.length} />
        <InfoRow label="Agents" value={wantType.agents.length} />
        <InfoRow label="Examples" value={wantType.examples.length} />
      </div>
    </TabSection>

    {wantType.relatedTypes && wantType.relatedTypes.length > 0 && (
      <TabSection title="Related Types">
        <div className="space-y-2">
          {wantType.relatedTypes.map((type) => (
            <div key={type} className="px-3 py-2 bg-gray-50 rounded text-sm text-gray-700">
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
    {wantType.parameters.length > 0 ? (
      <div className="space-y-4">
        {wantType.parameters.map((param) => (
          <TabSection key={param.name} title={param.name}>
            <div className="space-y-2 text-sm">
              <div>
                <span className="text-gray-600">Type:</span>
                <span className="ml-2 font-mono text-gray-900">{param.type}</span>
              </div>
              {param.default !== undefined && (
                <div>
                  <span className="text-gray-600">Default:</span>
                  <span className="ml-2 font-mono text-gray-900">{String(param.default)}</span>
                </div>
              )}
              <div>
                <span className="text-gray-600">Required:</span>
                <span className="ml-2 text-gray-900">{param.required ? 'Yes' : 'No'}</span>
              </div>
              {param.description && (
                <div>
                  <span className="text-gray-600">Description:</span>
                  <p className="mt-1 text-gray-700">{param.description}</p>
                </div>
              )}
              {param.validation && (
                <div className="mt-2 p-2 bg-gray-50 rounded">
                  <span className="text-gray-600 text-xs">Validation Rules:</span>
                  {param.validation.min !== undefined && (
                    <div className="text-xs text-gray-700">Min: {param.validation.min}</div>
                  )}
                  {param.validation.max !== undefined && (
                    <div className="text-xs text-gray-700">Max: {param.validation.max}</div>
                  )}
                  {param.validation.enum && (
                    <div className="text-xs text-gray-700">
                      Enum: {param.validation.enum.join(', ')}
                    </div>
                  )}
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
    {wantType.state.length > 0 ? (
      <div className="space-y-4">
        {wantType.state.map((state) => (
          <TabSection key={state.name} title={state.name}>
            <div className="space-y-2 text-sm">
              <div>
                <span className="text-gray-600">Type:</span>
                <span className="ml-2 font-mono text-gray-900">{state.type}</span>
              </div>
              <div>
                <span className="text-gray-600">Persistent:</span>
                <span className="ml-2 text-gray-900">{state.persistent ? 'Yes' : 'No'}</span>
              </div>
              {state.description && (
                <div>
                  <span className="text-gray-600">Description:</span>
                  <p className="mt-1 text-gray-700">{state.description}</p>
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
    <TabSection title="Inputs">
      {wantType.connectivity.inputs.length > 0 ? (
        <div className="space-y-3">
          {wantType.connectivity.inputs.map((input) => (
            <div key={input.name} className="p-2 bg-blue-50 rounded">
              <div className="font-semibold text-blue-900">{input.name}</div>
              <div className="text-xs text-blue-700">{input.type}</div>
              {input.description && (
                <p className="text-xs text-blue-800 mt-1">{input.description}</p>
              )}
            </div>
          ))}
        </div>
      ) : (
        <p className="text-sm text-gray-500">No inputs</p>
      )}
    </TabSection>

    <TabSection title="Outputs">
      {wantType.connectivity.outputs.length > 0 ? (
        <div className="space-y-3">
          {wantType.connectivity.outputs.map((output) => (
            <div key={output.name} className="p-2 bg-green-50 rounded">
              <div className="font-semibold text-green-900">{output.name}</div>
              <div className="text-xs text-green-700">{output.type}</div>
              {output.description && (
                <p className="text-xs text-green-800 mt-1">{output.description}</p>
              )}
            </div>
          ))}
        </div>
      ) : (
        <p className="text-sm text-gray-500">No outputs</p>
      )}
    </TabSection>
  </TabContent>
);

const AgentsTab: React.FC<{ wantType: WantTypeDefinition }> = ({ wantType }) => (
  <TabContent>
    {wantType.agents.length > 0 ? (
      <div className="space-y-4">
        {wantType.agents.map((agent) => (
          <TabSection key={agent.name} title={agent.name}>
            <div className="space-y-2 text-sm">
              <div>
                <span className="text-gray-600">Role:</span>
                <span className="ml-2 font-semibold text-gray-900 capitalize">{agent.role}</span>
              </div>
              {agent.description && (
                <div>
                  <span className="text-gray-600">Description:</span>
                  <p className="mt-1 text-gray-700">{agent.description}</p>
                </div>
              )}
              {agent.example && (
                <div className="mt-2 p-2 bg-gray-50 rounded">
                  <span className="text-gray-600 text-xs">Example:</span>
                  <p className="text-xs text-gray-700">{agent.example}</p>
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

const ExamplesTab: React.FC<{ wantType: WantTypeDefinition }> = ({ wantType }) => (
  <TabContent>
    {wantType.examples.length > 0 ? (
      <div className="space-y-4">
        {wantType.examples.map((example, index) => (
          <TabSection key={index} title={example.name}>
            <div className="space-y-2 text-sm">
              <div>
                <span className="text-gray-600">Description:</span>
                <p className="mt-1 text-gray-700">{example.description}</p>
              </div>
              {example.params && Object.keys(example.params).length > 0 && (
                <div>
                  <span className="text-gray-600">Parameters:</span>
                  <pre className="mt-1 text-xs bg-gray-50 p-2 rounded overflow-x-auto">
                    {JSON.stringify(example.params, null, 2)}
                  </pre>
                </div>
              )}
              {example.expectedBehavior && (
                <div>
                  <span className="text-gray-600">Expected Behavior:</span>
                  <p className="mt-1 text-gray-700 whitespace-pre-wrap text-xs">
                    {example.expectedBehavior}
                  </p>
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
    {wantType.constraints.length > 0 ? (
      <div className="space-y-4">
        {wantType.constraints.map((constraint, index) => (
          <TabSection key={index} title={`Constraint ${index + 1}`}>
            <div className="space-y-2 text-sm">
              <div>
                <span className="text-gray-600">Description:</span>
                <p className="mt-1 text-gray-700">{constraint.description}</p>
              </div>
              <div>
                <span className="text-gray-600">Validation:</span>
                <pre className="mt-1 text-xs bg-gray-50 p-2 rounded font-mono">
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

