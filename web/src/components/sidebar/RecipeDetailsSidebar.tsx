import React, { useState } from 'react';
import { BookOpen, Settings, List, FileText } from 'lucide-react';
import { GenericRecipe } from '@/types/recipe';
import { classNames } from '@/utils/helpers';
import {
  TabContent,
  TabSection,
  TabGrid,
  InfoRow,
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
    return (
      <div className="text-center py-12">
        <BookOpen className="h-12 w-12 text-gray-400 mx-auto mb-4" />
        <p className="text-gray-500">Select a recipe to view details</p>
      </div>
    );
  }

  const tabs = [
    { id: 'overview' as TabType, label: 'Overview', icon: FileText },
    { id: 'parameters' as TabType, label: 'Parameters', icon: Settings },
    { id: 'wants' as TabType, label: 'Wants', icon: List },
    { id: 'results' as TabType, label: 'Results', icon: BookOpen }
  ];

  return (
    <div className="h-full flex flex-col">
      {/* Tab Navigation */}
      <div className="border-b border-gray-200 px-6 py-4">
        <div className="flex space-x-1 bg-gray-100 rounded-lg p-1">
          {tabs.map((tab) => {
            const Icon = tab.icon;
            return (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={classNames(
                  'flex-1 flex items-center justify-center space-x-1 px-2 py-2 text-sm font-medium rounded-md transition-colors min-w-0',
                  activeTab === tab.id
                    ? 'bg-white text-blue-600 shadow-sm'
                    : 'text-gray-600 hover:text-gray-900'
                )}
              >
                <Icon className="h-4 w-4 flex-shrink-0" />
                <span className="truncate text-xs">{tab.label}</span>
              </button>
            );
          })}
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto">
        {activeTab === 'overview' && <OverviewTab recipe={recipe} />}
        {activeTab === 'parameters' && <ParametersTab recipe={recipe} />}
        {activeTab === 'wants' && <WantsTab recipe={recipe} />}
        {activeTab === 'results' && <ResultsTab recipe={recipe} />}
      </div>
    </div>
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

  // Get background image based on want type (same logic as WantCard)
  const getBackgroundImage = (type: string) => {
    if (type === 'flight') return '/resources/flight.png';
    if (type === 'hotel') return '/resources/hotel.png';
    if (type === 'restaurant') return '/resources/restaurant.png';
    if (type === 'buffet') return '/resources/buffet.png';
    if (type === 'evidence') return '/resources/evidence.png';
    if (type?.endsWith('coordinator')) return '/resources/agent.png';
    return undefined;
  };

  const backgroundImage = getBackgroundImage(wantType);

  return (
    <div
      className={classNames(
        "relative overflow-hidden rounded-md border hover:shadow-sm transition-all duration-200 cursor-default",
        "border-gray-200 hover:border-gray-300"
      )}
      style={backgroundImage ? {
        backgroundImage: `url(${backgroundImage})`,
        backgroundSize: '100% auto',
        backgroundPosition: 'center center',
        backgroundRepeat: 'no-repeat',
        backgroundAttachment: 'scroll'
      } : { backgroundColor: 'white' }}
    >
      {/* Content wrapper with semi-transparent background */}
      <div className={classNames('p-3', backgroundImage ? 'bg-white bg-opacity-70 relative z-10' : 'bg-white')}>
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