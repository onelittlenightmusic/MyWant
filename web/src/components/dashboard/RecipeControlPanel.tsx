import React, { useState } from 'react';
import { Play, Edit2, Trash2, Download } from 'lucide-react';
import { GenericRecipe } from '@/types/recipe';
import { classNames } from '@/utils/helpers';
import { apiClient } from '@/api/client';
import { validateYaml } from '@/utils/yaml';

interface RecipeControlPanelProps {
  selectedRecipe: GenericRecipe | null;
  onEdit: (recipe: GenericRecipe) => void;
  onDelete: (recipe: GenericRecipe) => void;
  onDeploySuccess?: (message: string) => void;
  onDeployError?: (error: string) => void;
  loading?: boolean;
  sidebarMinimized?: boolean;
}

export const RecipeControlPanel: React.FC<RecipeControlPanelProps> = ({
  selectedRecipe,
  onEdit,
  onDelete,
  onDeploySuccess,
  onDeployError,
  loading = false,
  sidebarMinimized = false
}) => {
  const [deploying, setDeploying] = useState(false);

  // Check if recipe has example deployment
  const hasExample = selectedRecipe?.recipe.example?.wants &&
                     selectedRecipe.recipe.example.wants.length > 0;

  // Button enable/disable logic
  const canDeploy = selectedRecipe && hasExample && !loading && !deploying;
  const canEdit = selectedRecipe && !loading && !deploying;
  const canDelete = selectedRecipe && !loading && !deploying;

  const handleDeploy = async () => {
    if (!selectedRecipe || !hasExample) return;

    setDeploying(true);
    try {
      // Convert example wants to YAML format
      const yamlContent = convertWantsToYAML(selectedRecipe.recipe.example!.wants);

      // Parse YAML to get the want data
      const yamlValidation = validateYaml(yamlContent);
      if (!yamlValidation.isValid) {
        throw new Error(`Invalid YAML: ${yamlValidation.error}`);
      }

      const wantData = yamlValidation.data;

      // Handle both single want and multiple wants
      const wants = Array.isArray(wantData.wants) ? wantData.wants : [wantData];

      // Deploy each want
      for (const want of wants) {
        await apiClient.createWant(want);
      }

      onDeploySuccess?.(
        `Recipe "${selectedRecipe.recipe.metadata.name}" deployed successfully!`
      );
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Failed to deploy recipe';
      onDeployError?.(errorMessage);
      console.error('Deploy error:', error);
    } finally {
      setDeploying(false);
    }
  };

  const handleEdit = () => {
    if (selectedRecipe && canEdit) onEdit(selectedRecipe);
  };

  const handleDelete = () => {
    if (selectedRecipe && canDelete) onDelete(selectedRecipe);
  };

  return (
    <div className={classNames(
      "fixed bottom-0 right-0 bg-blue-50 border-t border-blue-200 shadow-lg z-30 transition-all duration-300 ease-in-out",
      sidebarMinimized ? "lg:left-20" : "lg:left-64",
      "left-0"
    )}>
      <div className="px-6 py-3">
        <div className="flex items-center justify-between">
          {/* Recipe Info */}
          <div className="flex-1 min-w-0">
            {selectedRecipe ? (
              <p className="text-sm font-medium text-gray-700">
                {selectedRecipe.recipe.metadata.name}
                {hasExample && (
                  <span className="ml-2 inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800">
                    Ready to Deploy
                  </span>
                )}
              </p>
            ) : (
              <p className="text-sm text-gray-500">No recipe selected</p>
            )}
          </div>

          {/* Control Buttons */}
          <div className="flex items-center space-x-2 flex-wrap gap-y-2 ml-4">
            {/* Deploy */}
            <button
              onClick={handleDeploy}
              disabled={!canDeploy}
              className={classNames(
                'flex items-center space-x-2 px-4 py-2 rounded-md text-sm font-medium transition-colors',
                canDeploy
                  ? 'bg-green-600 text-white hover:bg-green-700'
                  : 'bg-gray-100 text-gray-400 cursor-not-allowed'
              )}
              title={
                !selectedRecipe
                  ? 'No recipe selected'
                  : !hasExample
                  ? 'Recipe has no example deployment configuration'
                  : 'Deploy recipe with example configuration'
              }
            >
              <Play className="h-4 w-4" />
              <span>{deploying ? 'Deploying...' : 'Deploy'}</span>
            </button>

            {/* Edit */}
            <button
              onClick={handleEdit}
              disabled={!canEdit}
              className={classNames(
                'flex items-center space-x-2 px-4 py-2 rounded-md text-sm font-medium transition-colors',
                canEdit
                  ? 'bg-blue-600 text-white hover:bg-blue-700'
                  : 'bg-gray-100 text-gray-400 cursor-not-allowed'
              )}
              title={canEdit ? 'Edit recipe' : 'No recipe selected'}
            >
              <Edit2 className="h-4 w-4" />
              <span>Edit</span>
            </button>

            {/* Download */}
            <button
              onClick={() => {
                if (selectedRecipe) {
                  downloadRecipeYAML(selectedRecipe);
                }
              }}
              disabled={!selectedRecipe}
              className={classNames(
                'flex items-center space-x-2 px-4 py-2 rounded-md text-sm font-medium transition-colors',
                selectedRecipe
                  ? 'bg-purple-600 text-white hover:bg-purple-700'
                  : 'bg-gray-100 text-gray-400 cursor-not-allowed'
              )}
              title={selectedRecipe ? 'Download recipe as YAML' : 'No recipe selected'}
            >
              <Download className="h-4 w-4" />
              <span>Download</span>
            </button>

            {/* Delete */}
            <button
              onClick={handleDelete}
              disabled={!canDelete}
              className={classNames(
                'flex items-center space-x-2 px-4 py-2 rounded-md text-sm font-medium transition-colors border',
                canDelete
                  ? 'border-gray-300 text-gray-700 hover:bg-gray-50'
                  : 'border-gray-200 text-gray-400 cursor-not-allowed'
              )}
              title={canDelete ? 'Delete recipe' : 'No recipe selected'}
            >
              <span>Delete</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};

/**
 * Convert recipe wants to YAML format for deployment
 */
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
