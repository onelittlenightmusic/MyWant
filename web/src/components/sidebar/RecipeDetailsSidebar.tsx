import React, { useState } from 'react';
import { BookOpen, Settings, List, FileText, Play, Edit2, Download, Trash2, Zap } from 'lucide-react';
import { GenericRecipe } from '@/types/recipe';
import { classNames } from '@/utils/helpers';
import { apiClient } from '@/api/client';
import { validateYaml } from '@/utils/yaml';
import {
  TabContent,
  TabSection,
  TabGrid,
  InfoRow,
  EmptyState,
} from './DetailsSidebar';

interface RecipeDetailsSidebarProps {
  recipe: GenericRecipe | null;
  onDeploy?: (recipe: GenericRecipe) => Promise<void>;
  onDeployExample?: (recipe: GenericRecipe) => Promise<void>;
  onEdit?: (recipe: GenericRecipe) => void;
  onDelete?: (recipe: GenericRecipe) => void;
  onDeploySuccess?: (message: string) => void;
  onDeployError?: (error: string) => void;
  loading?: boolean;
}

type TabType = 'overview' | 'parameters' | 'wants' | 'results';

export const RecipeDetailsSidebar: React.FC<RecipeDetailsSidebarProps> = ({
  recipe,
  onDeploy,
  onDeployExample,
  onEdit,
  onDelete,
  onDeploySuccess,
  onDeployError,
  loading = false
}) => {
  const [activeTab, setActiveTab] = useState<TabType>('overview');
  const [deploying, setDeploying] = useState(false);

  // Check if recipe has example deployment
  const hasExample = recipe?.recipe.example?.wants &&
                     recipe.recipe.example.wants.length > 0;

  // Button enable/disable logic
  const canDeploy = recipe && !loading && !deploying;
  const canDeployExample = recipe && hasExample && !loading && !deploying;
  const canEdit = recipe && !loading && !deploying;
  const canDelete = recipe && !loading && !deploying;

  const handleDeployClick = async () => {
    if (!recipe) return;

    setDeploying(true);
    try {
      if (onDeploy) {
        await onDeploy(recipe);
      } else {
        // Default: deploy recipe with its parameters
        throw new Error('Deploy handler not provided');
      }
      onDeploySuccess?.(`Recipe "${recipe.recipe.metadata.name}" deployed successfully!`);
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Failed to deploy recipe';
      onDeployError?.(errorMessage);
      console.error('Deploy error:', error);
    } finally {
      setDeploying(false);
    }
  };

  const handleDeployExampleClick = async () => {
    if (!recipe || !hasExample) return;

    setDeploying(true);
    try {
      if (onDeployExample) {
        await onDeployExample(recipe);
      } else {
        // Default deployment logic for example
        const yamlContent = convertWantsToYAML(recipe.recipe.example!.wants);
        const yamlValidation = validateYaml(yamlContent);
        if (!yamlValidation.isValid) {
          throw new Error(`Invalid YAML: ${yamlValidation.error}`);
        }
        const wantData = yamlValidation.data;
        const wants = Array.isArray(wantData.wants) ? wantData.wants : [wantData];
        for (const want of wants) {
          await apiClient.createWant(want);
        }
      }
      onDeploySuccess?.(`Recipe example "${recipe.recipe.metadata.name}" deployed successfully!`);
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Failed to deploy recipe example';
      onDeployError?.(errorMessage);
      console.error('Deploy example error:', error);
    } finally {
      setDeploying(false);
    }
  };

  const handleEditClick = () => {
    if (recipe && canEdit && onEdit) onEdit(recipe);
  };

  const handleDeleteClick = () => {
    if (recipe && canDelete && onDelete) onDelete(recipe);
  };

  const handleDownloadClick = () => {
    if (recipe) {
      downloadRecipeYAML(recipe);
    }
  };

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
      {/* Control Panel Buttons - Icon Only, Minimal Height */}
      {recipe && (
        <div className="flex-shrink-0 border-b border-gray-200 px-4 py-2 flex gap-1 justify-center">
          {/* Deploy */}
          <button
            onClick={handleDeployClick}
            disabled={!canDeploy}
            title={
              !recipe
                ? 'No recipe selected'
                : 'Deploy recipe with parameters'
            }
            className={classNames(
              'p-2 rounded-md transition-colors',
              canDeploy && !deploying
                ? 'bg-green-100 text-green-600 hover:bg-green-200'
                : 'bg-gray-100 text-gray-400 cursor-not-allowed'
            )}
          >
            <Play className="h-4 w-4" />
          </button>

          {/* Deploy Example (only if example exists) */}
          {hasExample && (
            <button
              onClick={handleDeployExampleClick}
              disabled={!canDeployExample}
              title={canDeployExample ? 'Deploy recipe example' : 'Recipe has no example'}
              className={classNames(
                'p-2 rounded-md transition-colors',
                canDeployExample && !deploying
                  ? 'bg-blue-100 text-blue-600 hover:bg-blue-200'
                  : 'bg-gray-100 text-gray-400 cursor-not-allowed'
              )}
            >
              <Zap className="h-4 w-4" />
            </button>
          )}

          {/* Edit */}
          <button
            onClick={handleEditClick}
            disabled={!canEdit}
            title={canEdit ? 'Edit recipe' : 'Cannot edit'}
            className={classNames(
              'p-2 rounded-md transition-colors',
              canEdit
                ? 'bg-blue-100 text-blue-600 hover:bg-blue-200'
                : 'bg-gray-100 text-gray-400 cursor-not-allowed'
            )}
          >
            <Edit2 className="h-4 w-4" />
          </button>

          {/* Download */}
          <button
            onClick={handleDownloadClick}
            disabled={!recipe}
            title={recipe ? 'Download recipe as YAML' : 'No recipe selected'}
            className={classNames(
              'p-2 rounded-md transition-colors',
              recipe
                ? 'bg-purple-100 text-purple-600 hover:bg-purple-200'
                : 'bg-gray-100 text-gray-400 cursor-not-allowed'
            )}
          >
            <Download className="h-4 w-4" />
          </button>

          {/* Delete */}
          <button
            onClick={handleDeleteClick}
            disabled={!canDelete}
            title={canDelete ? 'Delete recipe' : 'Cannot delete'}
            className={classNames(
              'p-2 rounded-md transition-colors',
              canDelete
                ? 'bg-red-100 text-red-600 hover:bg-red-200'
                : 'bg-gray-100 text-gray-400 cursor-not-allowed'
            )}
          >
            <Trash2 className="h-4 w-4" />
          </button>
        </div>
      )}

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
        {Object.entries(recipe.recipe.parameters).map(([key, value]) => {
          const description = recipe.recipe.parameter_descriptions?.[key];
          return (
            <TabSection key={key} title={key}>
              <div className="space-y-3">
                {description && (
                  <div>
                    <p className="text-xs text-gray-600 mb-1">Description:</p>
                    <p className="text-sm text-gray-700">{description}</p>
                  </div>
                )}
                <div>
                  <p className="text-xs text-gray-600 mb-2">Default Value:</p>
                  <pre className="text-xs text-gray-800 bg-gray-50 p-3 rounded border overflow-x-auto whitespace-pre-wrap">
                    {formatParameterValue(value)}
                  </pre>
                </div>
              </div>
            </TabSection>
          );
        })}
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
              {Object.entries(labels as Record<string, any>).map(([key, value]) => (
                <span key={key} className="text-xs bg-green-100 text-green-800 px-2 py-1 rounded border border-green-300">
                  {key}={value as any}
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

// Helper functions for recipe deployment and download
function convertWantsToYAML(wants: any[]): string {
  const config = { wants };
  return convertToYAML(config);
}

/**
 * Convert object to YAML string
 */
function convertToYAML(obj: any, indent = 0): string {
  const spaces = ' '.repeat(indent);
  let result = '';

  if (Array.isArray(obj)) {
    obj.forEach((item) => {
      if (typeof item === 'object' && item !== null) {
        // For array items that are objects, format as YAML list items
        const itemYaml = convertToYAML(item, indent + 2);
        result += `${spaces}- ${itemYaml.substring(indent + 2)}`;
      } else {
        result += `${spaces}- ${item}\n`;
      }
    });
  } else if (typeof obj === 'object' && obj !== null) {
    // Filter out null, undefined, and empty values
    const entries = Object.entries(obj).filter(([, value]) => {
      if (value === null || value === undefined) return false;
      if (typeof value === 'object' && Object.keys(value).length === 0 && !Array.isArray(value)) return false;
      return true;
    });

    entries.forEach(([key, value]) => {
      result += `${spaces}${key}`;
      if (Array.isArray(value)) {
        result += `:\n${convertToYAML(value, indent + 2)}`;
      } else if (typeof value === 'object' && value !== null) {
        result += `:\n${convertToYAML(value, indent + 2)}`;
      } else if (typeof value === 'string') {
        result += `: "${value}"\n`;
      } else if (typeof value === 'boolean' || typeof value === 'number') {
        result += `: ${value}\n`;
      } else {
        result += `: ${JSON.stringify(value)}\n`;
      }
    });
  }

  return result;
}

/**
 * Download recipe as YAML file
 */
function downloadRecipeYAML(recipe: GenericRecipe): void {
  const recipeName = recipe.recipe.metadata.name || 'recipe';
  const yamlContent = `recipe:\n${convertToYAML(recipe.recipe, 2)}`;

  const element = document.createElement('a');
  element.setAttribute(
    'href',
    `data:text/yaml;charset=utf-8,${encodeURIComponent(yamlContent)}`
  );
  element.setAttribute('download', `${recipeName}.yaml`);
  element.style.display = 'none';

  document.body.appendChild(element);
  element.click();
  document.body.removeChild(element);
}