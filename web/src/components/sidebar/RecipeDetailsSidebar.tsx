import React, { useState } from 'react';
import { BookOpen, Settings, List, FileText, Copy } from 'lucide-react';
import { GenericRecipe } from '@/types/recipe';
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
    {recipe.recipe.wants && recipe.recipe.wants.length > 0 ? (
      <div className="space-y-3">
        {recipe.recipe.wants.map((want, index) => (
          <RecipeWantCard key={index} want={want} index={index} />
        ))}
      </div>
    ) : (
      <EmptyState icon={List} message="No wants defined for this recipe" />
    )}
  </TabContent>
);

// Child want card component for recipe wants display
interface RecipeWantCardProps {
  want: any;
  index: number;
}

const RecipeWantCard: React.FC<RecipeWantCardProps> = ({ want, index }) => {
  const wantType = want.type || want.metadata?.type || 'unknown';
  const wantName = want.metadata?.name || want.name || `Want ${index + 1}`;
  const labels = want.metadata?.labels || want.labels || {};
  const params = want.params || want.spec?.params || {};
  const using = want.using || want.spec?.using || [];

  return (
    <div className={classNames(
      "relative overflow-hidden rounded-md border bg-white p-3 hover:shadow-sm transition-all duration-200",
      "border-gray-200 hover:border-gray-300"
    )}>
      {/* Header with Type and Name */}
      <div className="flex items-start justify-between mb-3">
        <div className="flex-1 min-w-0">
          <h4 className="text-sm font-semibold text-gray-900 truncate">
            {wantType}
          </h4>
          <p className="text-xs text-gray-500 mt-1 truncate">
            {wantName}
          </p>
        </div>
      </div>

      {/* Content Grid */}
      <div className="space-y-3">
        {/* Parameters Section */}
        {Object.keys(params).length > 0 && (
          <div>
            <h5 className="text-xs font-medium text-gray-700 mb-1">Parameters</h5>
            <div className="flex flex-wrap gap-1">
              {Object.entries(params).map(([key, value]) => (
                <span key={key} className="text-xs bg-blue-50 text-blue-800 px-2 py-1 rounded border border-blue-200">
                  <span className="font-medium">{key}:</span> {String(value).substring(0, 20)}
                  {String(value).length > 20 ? '...' : ''}
                </span>
              ))}
            </div>
          </div>
        )}

        {/* Labels Section */}
        {Object.keys(labels).length > 0 && (
          <div>
            <h5 className="text-xs font-medium text-gray-700 mb-1">Labels</h5>
            <div className="flex flex-wrap gap-1">
              {Object.entries(labels).map(([key, value]) => (
                <span key={key} className="text-xs bg-green-100 text-green-800 px-2 py-1 rounded border border-green-300">
                  {key}={value}
                </span>
              ))}
            </div>
          </div>
        )}

        {/* Using Selectors Section */}
        {using && using.length > 0 && (
          <div>
            <h5 className="text-xs font-medium text-gray-700 mb-1">Dependencies</h5>
            <div className="flex flex-wrap gap-1">
              {using.map((selector, idx) => (
                <span key={idx} className="text-xs bg-amber-50 text-amber-800 px-2 py-1 rounded border border-amber-200">
                  {Object.entries(selector as Record<string, string>)
                    .map(([k, v]) => `${k}:${v}`)
                    .join(', ')}
                </span>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
};

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