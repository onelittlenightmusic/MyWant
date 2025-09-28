import { useState, useEffect } from 'react';
import { Plus, Minus, FileText, Code, Save } from 'lucide-react';
import { GenericRecipe } from '@/types/recipe';
import { useRecipeStore } from '@/stores/recipeStore';
import { YamlEditor } from '@/components/forms/YamlEditor';
import { CreateSidebar } from '@/components/layout/CreateSidebar';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';

interface RecipeModalProps {
  isOpen: boolean;
  onClose: () => void;
  recipe: GenericRecipe | null;
  mode: 'create' | 'edit';
}

export default function RecipeModal({
  isOpen,
  onClose,
  recipe,
  mode,
}: RecipeModalProps) {
  const { createRecipe, updateRecipe, loading } = useRecipeStore();

  const [formData, setFormData] = useState({
    name: '',
    description: '',
    version: '',
    custom_type: '',
    parameters: {} as Record<string, any>,
    wants: [
      {
        type: '',
        params: {} as Record<string, any>,
        using: [] as Record<string, string>[],
      },
    ],
  });

  const [parameterEntries, setParameterEntries] = useState<{ key: string; value: string; type: string }[]>([
    { key: '', value: '', type: 'string' },
  ]);

  const [viewMode, setViewMode] = useState<'form' | 'yaml'>('form');
  const [yamlContent, setYamlContent] = useState('');

  // Convert form data to YAML
  const formToYaml = () => {
    // Convert parameter entries back to object
    const parameters: Record<string, any> = {};
    parameterEntries.forEach(({ key, value, type }) => {
      if (key.trim()) {
        switch (type) {
          case 'number':
            parameters[key] = parseFloat(value) || 0;
            break;
          case 'boolean':
            parameters[key] = value.toLowerCase() === 'true';
            break;
          default:
            parameters[key] = value;
        }
      }
    });

    const recipeData = {
      recipe: {
        metadata: {
          name: formData.name,
          description: formData.description,
          version: formData.version,
          custom_type: formData.custom_type,
        },
        parameters,
        wants: formData.wants,
      },
    };

    // Convert to proper YAML format
    const yamlLines = [
      '# MyWant Recipe Configuration',
      '# This recipe can be used to generate want configurations with parameter substitution',
      '',
      'recipe:',
      '  metadata:',
      `    name: "${recipeData.recipe.metadata.name}"`,
    ];

    if (recipeData.recipe.metadata.description) {
      yamlLines.push(`    description: "${recipeData.recipe.metadata.description}"`);
    }
    if (recipeData.recipe.metadata.version) {
      yamlLines.push(`    version: "${recipeData.recipe.metadata.version}"`);
    }
    if (recipeData.recipe.metadata.custom_type) {
      yamlLines.push(`    custom_type: "${recipeData.recipe.metadata.custom_type}"`);
    }

    if (Object.keys(parameters).length > 0) {
      yamlLines.push('', '  parameters:');
      Object.entries(parameters).forEach(([key, value]) => {
        if (typeof value === 'string') {
          yamlLines.push(`    ${key}: "${value}"`);
        } else {
          yamlLines.push(`    ${key}: ${value}`);
        }
      });
    }

    yamlLines.push('', '  wants:');
    recipeData.recipe.wants.forEach((want) => {
      yamlLines.push(`    - type: "${want.type}"`);
      if (Object.keys(want.params).length > 0) {
        yamlLines.push('      params:');
        Object.entries(want.params).forEach(([key, value]) => {
          if (typeof value === 'string') {
            yamlLines.push(`        ${key}: "${value}"`);
          } else {
            yamlLines.push(`        ${key}: ${JSON.stringify(value)}`);
          }
        });
      }
      if (want.using && want.using.length > 0) {
        yamlLines.push('      using:');
        want.using.forEach((usingItem) => {
          yamlLines.push('        - ' + Object.entries(usingItem).map(([k, v]) => `${k}: "${v}"`).join(', '));
        });
      }
    });

    return yamlLines.join('\n');
  };

  // Convert YAML to form data
  const yamlToForm = (yaml: string) => {
    try {
      // Parse YAML structure manually for key information

      // Extract metadata information
      const nameMatch = yaml.match(/name:\s*["']?([^"'\n]*)["']?/);
      const descMatch = yaml.match(/description:\s*["']?([^"'\n]*)["']?/);
      const versionMatch = yaml.match(/version:\s*["']?([^"'\n]*)["']?/);
      const customTypeMatch = yaml.match(/custom_type:\s*["']?([^"'\n]*)["']?/);

      // Extract parameters section
      const parametersMatch = yaml.match(/parameters:\s*([\s\S]*?)(?=\n\s*\w+:|$)/);
      const extractedParams: Record<string, any> = {};
      const extractedParamEntries: { key: string; value: string; type: string }[] = [];

      if (parametersMatch) {
        const paramLines = parametersMatch[1].split('\n').filter(line => line.trim());
        paramLines.forEach(line => {
          const paramMatch = line.match(/^\s*([\w_]+):\s*(.+)$/);
          if (paramMatch) {
            const [, key, value] = paramMatch;
            const cleanValue = value.replace(/^["']|["']$/g, '');
            let paramType = 'string';

            if (/^\d+(\.\d+)?$/.test(cleanValue)) {
              paramType = 'number';
            } else if (/^(true|false)$/i.test(cleanValue)) {
              paramType = 'boolean';
            }

            extractedParams[key] = paramType === 'number' ? parseFloat(cleanValue) :
                                  paramType === 'boolean' ? cleanValue.toLowerCase() === 'true' :
                                  cleanValue;
            extractedParamEntries.push({ key, value: cleanValue, type: paramType });
          }
        });
      }

      // Extract wants section
      const wantsMatch = yaml.match(/wants:\s*([\s\S]*?)(?=\n\w+:|$)/);
      const extractedWants: any[] = [];

      if (wantsMatch) {
        const wantLines = wantsMatch[1].split('\n');
        let currentWant: any = null;
        let inParams = false;

        wantLines.forEach(line => {
          const trimmed = line.trim();
          if (trimmed.startsWith('- type:')) {
            if (currentWant) extractedWants.push(currentWant);
            const typeMatch = trimmed.match(/- type:\s*["']?([^"']*)["']?/);
            currentWant = {
              type: typeMatch ? typeMatch[1] : '',
              params: {},
              using: []
            };
            inParams = false;
          } else if (trimmed === 'params:' && currentWant) {
            inParams = true;
          } else if (trimmed === 'using:' && currentWant) {
            inParams = false;
          } else if (inParams && trimmed.includes(':')) {
            const paramMatch = trimmed.match(/^([\w_]+):\s*(.+)$/);
            if (paramMatch && currentWant) {
              const [, key, value] = paramMatch;
              const cleanValue = value.replace(/^["']|["']$/g, '');
              if (/^\d+(\.\d+)?$/.test(cleanValue)) {
                currentWant.params[key] = parseFloat(cleanValue);
              } else if (/^(true|false)$/i.test(cleanValue)) {
                currentWant.params[key] = cleanValue.toLowerCase() === 'true';
              } else {
                currentWant.params[key] = cleanValue;
              }
            }
          }
        });

        if (currentWant) extractedWants.push(currentWant);
      }

      // Update form data
      const newFormData = { ...formData };
      if (nameMatch) newFormData.name = nameMatch[1].trim();
      if (descMatch) newFormData.description = descMatch[1].trim();
      if (versionMatch) newFormData.version = versionMatch[1].trim();
      if (customTypeMatch) newFormData.custom_type = customTypeMatch[1].trim();

      newFormData.parameters = extractedParams;
      newFormData.wants = extractedWants.length > 0 ? extractedWants : [{ type: '', params: {}, using: [] }];

      setFormData(newFormData);
      setParameterEntries(extractedParamEntries.length > 0 ? extractedParamEntries : [{ key: '', value: '', type: 'string' }]);

    } catch (error) {
      console.error('Error parsing YAML:', error);
      // You could add user-facing error handling here
    }
  };

  // Handle view mode switch
  const handleViewModeSwitch = (newMode: 'form' | 'yaml') => {
    if (newMode === 'yaml' && viewMode === 'form') {
      // Switching to YAML - convert form to YAML
      setYamlContent(formToYaml());
    } else if (newMode === 'form' && viewMode === 'yaml') {
      // Switching to form - convert YAML to form
      yamlToForm(yamlContent);
    }
    setViewMode(newMode);
  };

  useEffect(() => {
    if (isOpen) {
      if (mode === 'edit' && recipe) {
        setFormData({
          name: recipe.recipe.metadata.name,
          description: recipe.recipe.metadata.description || '',
          version: recipe.recipe.metadata.version || '',
          custom_type: recipe.recipe.metadata.custom_type || '',
          parameters: recipe.recipe.parameters || {},
          wants: recipe.recipe.wants.length > 0 ? recipe.recipe.wants.map(want => ({
            type: want.type || want.metadata?.type || '',
            params: want.params || want.spec?.params || {},
            using: want.using || [],
          })) : [{ type: '', params: {} as Record<string, any>, using: [] as Record<string, string>[] }],
        });

        // Convert parameters to entries
        const params = recipe.recipe.parameters || {};
        const entries = Object.entries(params).map(([key, value]) => ({
          key,
          value: typeof value === 'string' ? value : JSON.stringify(value),
          type: typeof value === 'number' ? 'number' : typeof value === 'boolean' ? 'boolean' : 'string',
        }));
        setParameterEntries(entries.length > 0 ? entries : [{ key: '', value: '', type: 'string' }]);
      } else {
        // Reset form for create mode
        setFormData({
          name: '',
          description: '',
          version: '',
          custom_type: '',
          parameters: {},
          wants: [{ type: '', params: {} as Record<string, any>, using: [] as Record<string, string>[] }],
        });
        setParameterEntries([{ key: '', value: '', type: 'string' }]);
      }
    }
  }, [isOpen, recipe, mode]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    try {
      let recipeData: GenericRecipe;

      if (viewMode === 'yaml') {
        // Parse YAML content to create recipe data
        // First, try to convert YAML back to form data to validate
        yamlToForm(yamlContent);

        // Simple validation - check if required fields are present
        if (!yamlContent.includes('name:')) {
          throw new Error('Recipe name is required');
        }

        // For now, use form data after YAML parsing
        // In a production app, you'd use a proper YAML parser like js-yaml
        const parameters: Record<string, any> = {};
        parameterEntries.forEach(({ key, value, type }) => {
          if (key.trim()) {
            switch (type) {
              case 'number':
                parameters[key] = parseFloat(value) || 0;
                break;
              case 'boolean':
                parameters[key] = value.toLowerCase() === 'true';
                break;
              default:
                parameters[key] = value;
            }
          }
        });

        recipeData = {
          recipe: {
            metadata: {
              name: formData.name,
              description: formData.description,
              version: formData.version,
              custom_type: formData.custom_type,
            },
            parameters,
            wants: formData.wants.map(want => ({
              type: want.type,
              params: want.params,
              using: want.using,
            })),
          },
        };
      } else {
        // Convert parameter entries back to object
        const parameters: Record<string, any> = {};
        parameterEntries.forEach(({ key, value, type }) => {
          if (key.trim()) {
            switch (type) {
              case 'number':
                parameters[key] = parseFloat(value) || 0;
                break;
              case 'boolean':
                parameters[key] = value.toLowerCase() === 'true';
                break;
              default:
                parameters[key] = value;
            }
          }
        });

        recipeData = {
          recipe: {
            metadata: {
              name: formData.name,
              description: formData.description,
              version: formData.version,
              custom_type: formData.custom_type,
            },
            parameters,
            wants: formData.wants.map(want => ({
              type: want.type,
              params: want.params,
              using: want.using,
            })),
          },
        };
      }

      if (mode === 'create') {
        await createRecipe(recipeData);
      } else {
        await updateRecipe(formData.name, recipeData);
      }
      onClose();
    } catch (error) {
      console.error('Failed to save recipe:', error);
      // You could add user-facing error display here
      alert(`Failed to save recipe: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  };

  const addParameterEntry = () => {
    setParameterEntries([...parameterEntries, { key: '', value: '', type: 'string' }]);
  };

  const removeParameterEntry = (index: number) => {
    if (parameterEntries.length > 1) {
      setParameterEntries(parameterEntries.filter((_, i) => i !== index));
    }
  };

  const updateParameterEntry = (index: number, field: 'key' | 'value' | 'type', value: string) => {
    const updated = [...parameterEntries];
    updated[index][field] = value;
    setParameterEntries(updated);
  };

  const addWant = () => {
    setFormData({
      ...formData,
      wants: [...formData.wants, { type: '', params: {} as Record<string, any>, using: [] as Record<string, string>[] }],
    });
  };

  const removeWant = (index: number) => {
    if (formData.wants.length > 1) {
      const updated = formData.wants.filter((_, i) => i !== index);
      setFormData({ ...formData, wants: updated });
    }
  };

  const updateWant = (index: number, field: 'type' | 'params', value: any) => {
    const updated = [...formData.wants];
    updated[index][field] = value;
    setFormData({ ...formData, wants: updated });
  };

  return (
    <CreateSidebar
      isOpen={isOpen}
      onClose={onClose}
      title={mode === 'create' ? 'Create Recipe' : 'Edit Recipe'}
    >
      <div className="p-8">
        {/* Create/Update Button at Top */}
        <form onSubmit={handleSubmit}>
          <div className="mb-8">
            <button
              type="submit"
              disabled={loading || !formData.name}
              className="inline-flex items-center justify-center px-6 py-3 bg-primary-600 hover:bg-primary-700 focus:ring-primary-500 focus:ring-offset-2 text-white font-medium rounded-md transition duration-150 ease-in-out focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {loading ? (
                <>
                  <LoadingSpinner size="sm" color="white" className="mr-2" />
                  {mode === 'edit' ? 'Updating...' : 'Creating...'}
                </>
              ) : (
                <>
                  <Save className="h-5 w-5 mr-2" />
                  {mode === 'edit' ? 'Update Recipe' : 'Create Recipe'}
                </>
              )}
            </button>
          </div>

        {/* View Mode Toggle */}
        <div className="mb-8">
          <div className="flex items-center space-x-4">
            <span className="text-sm font-medium text-gray-700">Edit Mode:</span>
            <div className="flex rounded-lg bg-gray-100 p-1">
              <button
                type="button"
                onClick={() => handleViewModeSwitch('form')}
                className={`px-3 py-1 rounded-md text-sm font-medium transition-colors ${
                  viewMode === 'form'
                    ? 'bg-white text-blue-600 shadow-sm'
                    : 'text-gray-600 hover:text-gray-900'
                }`}
              >
                <FileText className="w-4 h-4 inline mr-1" />
                Form
              </button>
              <button
                type="button"
                onClick={() => handleViewModeSwitch('yaml')}
                className={`px-3 py-1 rounded-md text-sm font-medium transition-colors ${
                  viewMode === 'yaml'
                    ? 'bg-white text-blue-600 shadow-sm'
                    : 'text-gray-600 hover:text-gray-900'
                }`}
              >
                <Code className="w-4 h-4 inline mr-1" />
                YAML
              </button>
            </div>
          </div>
        </div>

        <div className="space-y-8">
          {viewMode === 'form' ? (
            <>
              {/* Basic Information */}
          <div className="space-y-4">
            <h3 className="text-lg font-medium text-gray-900">Basic Information</h3>

            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  Name *
                </label>
                <input
                  type="text"
                  required
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                  className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                  placeholder="recipe-name"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  Version
                </label>
                <input
                  type="text"
                  value={formData.version}
                  onChange={(e) => setFormData({ ...formData, version: e.target.value })}
                  className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                  placeholder="1.0.0"
                />
              </div>
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Description
              </label>
              <textarea
                value={formData.description}
                onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                rows={3}
                className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                placeholder="Describe what this recipe does..."
              />
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Custom Type
              </label>
              <input
                type="text"
                value={formData.custom_type}
                onChange={(e) => setFormData({ ...formData, custom_type: e.target.value })}
                className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                placeholder="custom target type name"
              />
            </div>
          </div>

          {/* Parameters */}
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <h3 className="text-lg font-medium text-gray-900">Parameters</h3>
              <button
                type="button"
                onClick={addParameterEntry}
                className="text-blue-600 hover:text-blue-800 flex items-center gap-1"
              >
                <Plus className="h-4 w-4" />
                Add Parameter
              </button>
            </div>

            {parameterEntries.map((entry, index) => (
              <div key={index} className="grid grid-cols-12 gap-2 items-end">
                <div className="col-span-4">
                  <input
                    type="text"
                    placeholder="Parameter name"
                    value={entry.key}
                    onChange={(e) => updateParameterEntry(index, 'key', e.target.value)}
                    className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                  />
                </div>
                <div className="col-span-4">
                  <input
                    type="text"
                    placeholder="Default value"
                    value={entry.value}
                    onChange={(e) => updateParameterEntry(index, 'value', e.target.value)}
                    className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                  />
                </div>
                <div className="col-span-3">
                  <select
                    value={entry.type}
                    onChange={(e) => updateParameterEntry(index, 'type', e.target.value)}
                    className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                  >
                    <option value="string">String</option>
                    <option value="number">Number</option>
                    <option value="boolean">Boolean</option>
                  </select>
                </div>
                <div className="col-span-1">
                  {parameterEntries.length > 1 && (
                    <button
                      type="button"
                      onClick={() => removeParameterEntry(index)}
                      className="text-red-600 hover:text-red-800 p-2"
                    >
                      <Minus className="h-4 w-4" />
                    </button>
                  )}
                </div>
              </div>
            ))}
          </div>

          {/* Wants */}
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <h3 className="text-lg font-medium text-gray-900">Wants</h3>
              <button
                type="button"
                onClick={addWant}
                className="text-blue-600 hover:text-blue-800 flex items-center gap-1"
              >
                <Plus className="h-4 w-4" />
                Add Want
              </button>
            </div>

            {formData.wants.map((want, index) => (
              <div key={index} className="border border-gray-200 rounded-lg p-4">
                <div className="flex items-center justify-between mb-4">
                  <h4 className="font-medium text-gray-900">Want {index + 1}</h4>
                  {formData.wants.length > 1 && (
                    <button
                      type="button"
                      onClick={() => removeWant(index)}
                      className="text-red-600 hover:text-red-800"
                    >
                      <Minus className="h-4 w-4" />
                    </button>
                  )}
                </div>

                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">
                      Type *
                    </label>
                    <input
                      type="text"
                      required
                      value={want.type}
                      onChange={(e) => updateWant(index, 'type', e.target.value)}
                      className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                      placeholder="sequence, queue, sink, etc."
                    />
                  </div>

                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">
                      Parameters (JSON)
                    </label>
                    <textarea
                      value={JSON.stringify(want.params, null, 2)}
                      onChange={(e) => {
                        try {
                          const params = JSON.parse(e.target.value);
                          updateWant(index, 'params', params);
                        } catch (error) {
                          // Invalid JSON, keep the text for editing
                        }
                      }}
                      rows={3}
                      className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 font-mono text-sm"
                      placeholder='{"count": 100}'
                    />
                  </div>
                </div>
              </div>
            ))}
          </div>

            </>
          ) : (
            /* YAML Editor View */
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <h3 className="text-lg font-medium text-gray-900">YAML Configuration</h3>
                <div className="text-sm text-gray-500">
                  Edit the complete recipe configuration in YAML format
                </div>
              </div>

              <YamlEditor
                value={yamlContent}
                onChange={setYamlContent}
                height="500px"
                placeholder={`# Recipe YAML configuration...\nrecipe:\n  metadata:\n    name: "my-recipe"\n  parameters:\n    count: 100\n  wants:\n    - type: "sequence"\n      params:\n        count: count`}
                className="border border-gray-300 rounded-md"
              />

              <div className="bg-blue-50 border border-blue-200 rounded-md p-4">
                <h4 className="text-sm font-medium text-blue-900 mb-2">YAML Format Guidelines:</h4>
                <ul className="text-sm text-blue-800 space-y-1 list-disc list-inside">
                  <li>Use proper YAML indentation (2 spaces)</li>
                  <li>Reference parameters by name (e.g., <code className="bg-blue-100 px-1 rounded">count: count</code>)</li>
                  <li>Include all required metadata fields (name, etc.)</li>
                  <li>Define wants with type, params, and optional using connections</li>
                </ul>
              </div>
            </div>
          )}

          </div>
        </form>
      </div>
    </CreateSidebar>
  );
}