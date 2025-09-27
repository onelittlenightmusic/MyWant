import React, { useState } from 'react';
import { BookOpen, Settings, List, FileText } from 'lucide-react';
import { GenericRecipe } from '@/types/recipe';
import {
  DetailsSidebar,
  TabContent,
  TabSection,
  TabGrid,
  EmptyState,
  InfoRow,
  TabConfig
} from './DetailsSidebar';

interface RecipeDetailsSidebarProps {
  recipe: GenericRecipe | null;
}

type TabType = 'overview' | 'parameters' | 'wants' | 'results';

export const RecipeDetailsSidebar: React.FC<RecipeDetailsSidebarProps> = ({
  recipe
}) => {
  const [activeTab, setActiveTab] = useState<TabType>('overview');

  if (!recipe) {
    return <EmptyState icon={BookOpen} message="Select a recipe to view details" />;
  }

  const tabs: TabConfig[] = [
    { id: 'overview', label: 'Overview', icon: FileText },
    { id: 'parameters', label: 'Parameters', icon: Settings },
    { id: 'wants', label: 'Wants', icon: List },
    { id: 'results', label: 'Results', icon: BookOpen }
  ];

  const badge = (
    <div className="inline-flex items-center px-3 py-1 rounded-full text-sm font-medium border bg-blue-100 text-blue-800 border-blue-200">
      <BookOpen className="h-4 w-4 mr-2" />
      Recipe
    </div>
  );

  return (
    <DetailsSidebar
      title={recipe.recipe.metadata.name}
      subtitle={recipe.recipe.metadata.description}
      badge={badge}
      tabs={tabs}
      defaultTab="overview"
      onTabChange={(tabId) => setActiveTab(tabId as TabType)}
    >
      {activeTab === 'overview' && <OverviewTab recipe={recipe} />}
      {activeTab === 'parameters' && <ParametersTab recipe={recipe} />}
      {activeTab === 'wants' && <WantsTab recipe={recipe} />}
      {activeTab === 'results' && <ResultsTab recipe={recipe} />}
    </DetailsSidebar>
  );
};

// Helper functions
const formatParameterValue = (value: any) => {
  if (typeof value === 'object') {
    return JSON.stringify(value, null, 2);
  }
  return String(value);
};

const formatWantParams = (params: any) => {
  if (!params || Object.keys(params).length === 0) {
    return 'No parameters';
  }
  return JSON.stringify(params, null, 2);
};

// Tab Components
const OverviewTab: React.FC<{ recipe: GenericRecipe }> = ({ recipe }) => (
  <TabContent>
    <TabSection title="Metadata">
      <div className="space-y-3">
        <InfoRow label="Name" value={recipe.recipe.metadata.name} />
        {recipe.recipe.metadata.version && (
          <InfoRow label="Version" value={<span className="font-mono">{recipe.recipe.metadata.version}</span>} />
        )}
        {recipe.recipe.metadata.custom_type && (
          <InfoRow label="Custom Type" value={recipe.recipe.metadata.custom_type} />
        )}
        {recipe.recipe.metadata.description && (
          <div>
            <dt className="text-sm text-gray-600 mb-1">Description:</dt>
            <dd className="text-sm font-medium text-gray-900">{recipe.recipe.metadata.description}</dd>
          </div>
        )}
      </div>
    </TabSection>

    <TabSection title="Summary">
      <div className="space-y-3">
        <InfoRow
          label="Parameters"
          value={recipe.recipe.parameters ? Object.keys(recipe.recipe.parameters).length : 0}
        />
        <InfoRow label="Wants" value={recipe.recipe.wants.length} />
        <InfoRow
          label="Results"
          value={recipe.recipe.result ? recipe.recipe.result.length : 0}
        />
      </div>
    </TabSection>
  </TabContent>
);

const ParametersTab: React.FC<{ recipe: GenericRecipe }> = ({ recipe }) => (
  <TabContent>
    {recipe.recipe.parameters && Object.keys(recipe.recipe.parameters).length > 0 ? (
      <div className="space-y-4">
        {Object.entries(recipe.recipe.parameters).map(([key, value]) => (
          <TabSection key={key} title={key}>
            <pre className="text-xs text-gray-800 bg-gray-50 p-3 rounded border overflow-x-auto whitespace-pre-wrap">
              {formatParameterValue(value)}
            </pre>
          </TabSection>
        ))}
      </div>
    ) : (
      <EmptyState icon={Settings} message="No parameters defined for this recipe" />
    )}
  </TabContent>
);

const WantsTab: React.FC<{ recipe: GenericRecipe }> = ({ recipe }) => (
  <TabContent>
    <div className="space-y-4">
      {recipe.recipe.wants.map((want, index) => (
        <TabSection
          key={index}
          title={`Want ${index + 1}${(want.metadata?.name || want.name) ? ` (${want.metadata?.name || want.name})` : ''}`}
        >
          <div className="space-y-4">
            {/* Type */}
            <div className="flex items-center justify-between">
              <span className="text-sm text-gray-600">Type:</span>
              <span className="text-xs bg-blue-100 text-blue-800 px-2 py-1 rounded-full">
                {want.type || want.metadata?.type || 'Unknown type'}
              </span>
            </div>

            {/* Parameters */}
            <div>
              <h5 className="text-sm font-medium text-gray-700 mb-2">Parameters</h5>
              <pre className="text-xs text-gray-900 bg-gray-50 p-3 rounded border overflow-x-auto whitespace-pre-wrap">
                {formatWantParams(want.params || want.spec?.params)}
              </pre>
            </div>

            {/* Using selectors */}
            {want.using && want.using.length > 0 && (
              <div>
                <h5 className="text-sm font-medium text-gray-700 mb-2">Using Selectors</h5>
                <div className="space-y-2">
                  {want.using.map((selector, selectorIndex) => (
                    <div key={selectorIndex} className="text-xs bg-gray-50 p-2 rounded border">
                      {Object.entries(selector).map(([key, value]) => (
                        <div key={key} className="flex justify-between">
                          <span className="text-gray-600">{key}:</span>
                          <span className="text-gray-900 font-mono">{value}</span>
                        </div>
                      ))}
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Labels */}
            {((want.metadata?.labels && Object.keys(want.metadata.labels).length > 0) ||
              (want.labels && Object.keys(want.labels).length > 0)) && (
              <div>
                <h5 className="text-sm font-medium text-gray-700 mb-2">Labels</h5>
                <div className="flex flex-wrap gap-1">
                  {Object.entries(want.metadata?.labels || want.labels || {}).map(([key, value]) => (
                    <span key={key} className="text-xs bg-green-100 text-green-800 px-2 py-1 rounded">
                      {key}={value}
                    </span>
                  ))}
                </div>
              </div>
            )}

            {/* Requirements */}
            {want.requires && want.requires.length > 0 && (
              <div>
                <h5 className="text-sm font-medium text-gray-700 mb-2">Requirements</h5>
                <div className="flex flex-wrap gap-1">
                  {want.requires.map((req, reqIndex) => (
                    <span key={reqIndex} className="text-xs bg-yellow-100 text-yellow-800 px-2 py-1 rounded">
                      {req}
                    </span>
                  ))}
                </div>
              </div>
            )}
          </div>
        </TabSection>
      ))}
    </div>
  </TabContent>
);

const ResultsTab: React.FC<{ recipe: GenericRecipe }> = ({ recipe }) => (
  <TabContent>
    {recipe.recipe.result && recipe.recipe.result.length > 0 ? (
      <div className="space-y-4">
        {recipe.recipe.result.map((result, index) => (
          <TabSection key={index} title={`Result ${index + 1}`}>
            <div className="space-y-3">
              <InfoRow label="Want Name" value={<span className="font-mono">{result.want_name}</span>} />
              <InfoRow label="Stat Name" value={<span className="font-mono">{result.stat_name}</span>} />
              {result.description && (
                <div>
                  <dt className="text-sm text-gray-600 mb-1">Description:</dt>
                  <dd className="text-sm font-medium text-gray-900">{result.description}</dd>
                </div>
              )}
            </div>
          </TabSection>
        ))}
      </div>
    ) : (
      <EmptyState icon={BookOpen} message="No result configuration defined for this recipe" />
    )}
  </TabContent>
);