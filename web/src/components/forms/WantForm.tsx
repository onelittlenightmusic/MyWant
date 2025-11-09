import React, { useState, useEffect } from 'react';
import { Save, Plus, X, Code, Edit3 } from 'lucide-react';
import { Want, CreateWantRequest, UpdateWantRequest } from '@/types/want';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { ErrorDisplay } from '@/components/common/ErrorDisplay';
import { CreateSidebar } from '@/components/layout/CreateSidebar';
import { YamlEditor } from './YamlEditor';
import { validateYaml, stringifyYaml } from '@/utils/yaml';
import { useWantStore } from '@/stores/wantStore';
import { ApiError } from '@/types/api';

interface WantFormProps {
  isOpen: boolean;
  onClose: () => void;
  editingWant?: Want | null;
}

// Sample configurations based on existing config files
const SAMPLE_CONFIGS = [
  {
    name: 'Queue System',
    description: 'Queue-based processing pipeline with generators, queues, and sinks',
    config: {
      metadata: {
        name: 'qnet-pipeline',
        type: 'wait time in queue system',
        labels: {
          role: 'qnet-target'
        }
      },
      spec: {
        recipe: 'recipes/qnet-pipeline.yaml',
        params: {
          prefix: 'qnet',
          primary_count: 1000,
          secondary_count: 800,
          primary_rate: 2.5,
          secondary_rate: 3.5,
          primary_service_time: 0.08,
          secondary_service_time: 0.10,
          final_service_time: 0.04
        }
      }
    }
  },
  {
    name: 'Fibonacci Sequence',
    description: 'Mathematical sequence generator with configurable parameters',
    config: {
      metadata: {
        name: 'fibonacci-sequence',
        type: 'fibonacci sequence',
        labels: {
          category: 'fibonacci'
        }
      },
      spec: {
        recipe: 'recipes/fibonacci-sequence.yaml',
        params: {
          count: 15,
          min_value: 1,
          max_value: 100
        }
      }
    }
  },
  {
    name: 'Travel Planner',
    description: 'Travel itinerary planning with hotels, restaurants, and coordination using agents',
    config: {
      metadata: {
        name: 'travel-planner',
        type: 'agent travel system',
        labels: {
          role: 'travel-planner'
        }
      },
      spec: {
        recipe: 'Travel Agent System',
        params: {
          prefix: 'vacation',
          display_name: 'One Day Travel Itinerary',
          restaurant_type: 'fine dining',
          hotel_type: 'luxury',
          buffet_type: 'international',
          dinner_duration: 2.0
        }
      }
    }
  },
  {
    name: 'Hierarchical Approval',
    description: 'Level 1 approval workflow',
    config: {
      metadata: {
        name: 'level1_approval',
        type: 'level 1 approval',
        labels: {
          role: 'approval-target',
          approval_level: '1'
        }
      },
      spec: {
        params: {
          approval_id: 'approval-001',
          coordinator_type: 'level1',
          level2_authority: 'senior_manager'
        }
      }
    }
  },
  {
    name: 'Dynamic Travel Change',
    description: 'Dynamic travel itinerary with flight booking and schedule changes',
    config: {
      metadata: {
        name: 'dynamic-travel-planner',
        type: 'dynamic travel change',
        labels: {
          role: 'dynamic-travel-planner'
        }
      },
      spec: {
        recipe: 'recipes/dynamic-travel-change.yaml',
        params: {
          prefix: 'dynamic-travel',
          display_name: 'Dynamic Travel Itinerary with Flight',
          flight_type: 'business class',
          departure_date: '2026-12-20',
          flight_duration: 12.0,
          restaurant_type: 'fine dining',
          hotel_type: 'luxury',
          buffet_type: 'international',
          dinner_duration: 2.0
        }
      }
    }
  }
];

// Common want types and their default parameters
const WANT_TYPES = [
  { value: 'qnet numbers', label: 'Number Generator', defaultParams: { rate: 2.0, count: 100 } },
  { value: 'qnet queue', label: 'Processing Queue', defaultParams: { service_time: 0.1 } },
  { value: 'qnet sink', label: 'Result Collector', defaultParams: {} },
  { value: 'sequence', label: 'Sequence Generator', defaultParams: { count: 10, rate: 1.0 } },
  { value: 'travel_hotel', label: 'Hotel Booking', defaultParams: { hotel_type: 'luxury' } },
  { value: 'travel_restaurant', label: 'Restaurant Booking', defaultParams: { restaurant_type: 'fine dining' } },
  { value: 'dynamic travel change', label: 'Dynamic Travel Change', defaultParams: { prefix: 'dynamic-travel', display_name: 'Dynamic Travel Itinerary with Flight', flight_type: 'business class', departure_date: '2026-12-20', flight_duration: 12.0, restaurant_type: 'fine dining', hotel_type: 'luxury', buffet_type: 'international', dinner_duration: 2.0 } }
];

export const WantForm: React.FC<WantFormProps> = ({
  isOpen,
  onClose,
  editingWant
}) => {
  const { createWant, updateWant, loading, error } = useWantStore();

  // UI state
  const [editMode, setEditMode] = useState<'form' | 'yaml'>('form');
  const [showSamples, setShowSamples] = useState(false);
  const [isEditing, setIsEditing] = useState(false);
  const [validationError, setValidationError] = useState<string | null>(null);
  const [apiError, setApiError] = useState<ApiError | null>(null);

  // Form state
  const [name, setName] = useState('');
  const [type, setType] = useState('sequence');
  const [labels, setLabels] = useState<Record<string, string>>({});
  const [params, setParams] = useState<Record<string, unknown>>({});
  const [using, setUsing] = useState<Array<Record<string, string>>>([]);
  const [recipe, setRecipe] = useState('');

  // YAML state
  const [yamlContent, setYamlContent] = useState('');

  // Convert form data to want object
  const formToWantObject = () => ({
    metadata: {
      name: name.trim(),
      type: type.trim(),
      ...(Object.keys(labels).length > 0 && { labels })
    },
    spec: {
      ...(Object.keys(params).length > 0 && { params }),
      ...(using.length > 0 && { using }),
      ...(recipe.trim() && { recipe: recipe.trim() })
    }
  });

  // Convert want object to form data
  const wantObjectToForm = (want: Want) => {
    setName(want.metadata?.name || '');
    setType(want.metadata?.type || 'sequence');
    setLabels(want.metadata?.labels || {});
    setParams(want.spec?.params || {});
    setUsing(want.spec?.using || []);
    setRecipe(want.spec?.recipe || '');
  };

  // Update YAML when form data changes
  useEffect(() => {
    if (editMode === 'form') {
      const wantObject = formToWantObject();
      setYamlContent(stringifyYaml(wantObject));
    }
  }, [name, type, labels, params, using, recipe, editMode]);

  // Initialize form when editing
  useEffect(() => {
    if (editingWant) {
      setIsEditing(true);
      wantObjectToForm(editingWant);
      setYamlContent(stringifyYaml({
        metadata: editingWant.metadata,
        spec: editingWant.spec
      }));
    } else {
      resetForm();
    }
  }, [editingWant]);

  const resetForm = () => {
    setIsEditing(false);
    setEditMode('form');
    setShowSamples(false);
    setName('');
    setType('sequence');
    setLabels({});
    setParams(WANT_TYPES[0].defaultParams);
    setUsing([]);
    setRecipe('');
    setYamlContent(stringifyYaml({
      metadata: { name: '', type: 'sequence' },
      spec: { params: WANT_TYPES[0].defaultParams }
    }));
    setValidationError(null);
    setApiError(null);
  };

  const loadSampleConfig = (sample: typeof SAMPLE_CONFIGS[0]) => {
    const config = sample.config;

    // Check if this is a multi-want configuration (has 'wants' array)
    if ((config as any).wants && Array.isArray((config as any).wants)) {
      // For multi-want configs, just load the YAML representation
      // This handles samples like "Hierarchical Approval" that deploy multiple wants at once
      setYamlContent(stringifyYaml(config));
      setShowSamples(false);
      return;
    }

    // Single-want configuration handling
    setName(config.metadata.name);
    setType(config.metadata.type);
    const labels = config.metadata.labels || {};
    setLabels(Object.fromEntries(
      Object.entries(labels).filter(([_, value]) => value !== undefined)
    ));
    setParams(config.spec.params ? {...config.spec.params} : {});
    setUsing((config.spec as any).using || []);
    setRecipe(config.spec.recipe || '');
    setYamlContent(stringifyYaml(config));
    setShowSamples(false);
  };

  // Update default params when type changes
  useEffect(() => {
    const selectedType = WANT_TYPES.find(t => t.value === type);
    if (selectedType && !isEditing) {
      setParams(selectedType.defaultParams);
    }
  }, [type, isEditing]);

  const validateForm = (): boolean => {
    if (!name.trim()) {
      setValidationError('Want name is required');
      return false;
    }
    if (!type.trim()) {
      setValidationError('Want type is required');
      return false;
    }
    setValidationError(null);
    return true;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setApiError(null);

    try {
      let wantRequest: CreateWantRequest | UpdateWantRequest;

      if (editMode === 'yaml') {
        // Parse YAML to want object
        if (!yamlContent.trim()) {
          setValidationError('YAML content is required');
          return;
        }

        const yamlValidation = validateYaml(yamlContent);
        if (!yamlValidation.isValid) {
          setValidationError(`Invalid YAML: ${yamlValidation.error}`);
          return;
        }

        wantRequest = yamlValidation.data as CreateWantRequest | UpdateWantRequest;
      } else {
        // Use form data
        if (!validateForm()) {
          return;
        }
        wantRequest = formToWantObject();
      }

      if (isEditing && editingWant?.metadata?.id) {
        await updateWant(editingWant.metadata.id, wantRequest as UpdateWantRequest);
      } else {
        await createWant(wantRequest as CreateWantRequest);
      }

      onClose();
      resetForm();
    } catch (error) {
      console.error('Failed to save want:', error);
      setApiError(error as ApiError);
    }
  };

  const addLabel = () => {
    setLabels(prev => ({ ...prev, '': '' }));
  };

  const updateLabel = (oldKey: string, newKey: string, value: string) => {
    setLabels(prev => {
      const newLabels = { ...prev };
      if (oldKey !== newKey && oldKey in newLabels) {
        delete newLabels[oldKey];
      }
      if (newKey.trim()) {
        newLabels[newKey] = value;
      }
      return newLabels;
    });
  };

  const removeLabel = (key: string) => {
    setLabels(prev => {
      const newLabels = { ...prev };
      delete newLabels[key];
      return newLabels;
    });
  };

  const addParam = () => {
    setParams(prev => ({ ...prev, '': '' }));
  };

  const updateParam = (key: string, value: string) => {
    setParams(prev => {
      const newParams = { ...prev };
      if (key.trim()) {
        // Try to parse as number if possible, otherwise keep as string
        const numValue = Number(value);
        newParams[key] = !isNaN(numValue) && value.trim() !== '' ? numValue : value;
      }
      return newParams;
    });
  };

  const removeParam = (key: string) => {
    setParams(prev => {
      const newParams = { ...prev };
      delete newParams[key];
      return newParams;
    });
  };

  const addUsing = () => {
    setUsing(prev => [...prev, { '': '' }]);
  };

  const updateUsing = (index: number, key: string, value: string) => {
    setUsing(prev => prev.map((item, i) =>
      i === index ? { [key]: value } : item
    ));
  };

  const removeUsing = (index: number) => {
    setUsing(prev => prev.filter((_, i) => i !== index));
  };

  if (!isOpen) return null;

  return (
    <CreateSidebar isOpen={isOpen} onClose={onClose} title={isEditing ? 'Edit Want' : 'Create New Want'}>
      <form onSubmit={handleSubmit} className="space-y-6">
        {/* Form Actions - At the very top */}
        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className="px-3 py-1.5 text-sm border border-gray-300 text-gray-700 rounded-md hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={loading}
            className="flex items-center justify-center gap-1.5 bg-blue-600 text-white px-3 py-1.5 text-sm rounded-md hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {loading ? (
              <LoadingSpinner size="sm" />
            ) : (
              <>
                <Save className="w-3.5 h-3.5" />
                {isEditing ? 'Update' : 'Create'}
              </>
            )}
          </button>
        </div>

        {/* Sample Selector */}
        {!isEditing && (
          <div className="border-b border-gray-200 pb-4">
            <label className="block text-sm font-medium text-gray-700 mb-2">
              Sample Configurations
            </label>
            <select
              onChange={(e) => {
                const index = parseInt(e.target.value);
                if (!isNaN(index)) {
                  loadSampleConfig(SAMPLE_CONFIGS[index]);
                }
              }}
              className="w-full px-3 py-2 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent bg-white"
              defaultValue=""
            >
              <option value="" disabled>Select a sample configuration...</option>
              {SAMPLE_CONFIGS.map((sample, index) => (
                <option key={index} value={index}>
                  {sample.name} - {sample.description}
                </option>
              ))}
            </select>
          </div>
        )}

        {/* Mode Toggle */}
        <div className="border-b border-gray-200 pb-4">
          <div className="flex items-center justify-center space-x-1 bg-gray-100 rounded-lg p-1">
            <button
              type="button"
              onClick={() => setEditMode('form')}
              className={`flex items-center gap-2 px-4 py-2 rounded-md text-sm font-medium transition-colors ${
                editMode === 'form'
                  ? 'bg-white text-blue-600 shadow-sm'
                  : 'text-gray-600 hover:text-gray-900'
              }`}
            >
              <Edit3 className="w-4 h-4" />
              Form Editor
            </button>
            <button
              type="button"
              onClick={() => setEditMode('yaml')}
              className={`flex items-center gap-2 px-4 py-2 rounded-md text-sm font-medium transition-colors ${
                editMode === 'yaml'
                  ? 'bg-white text-blue-600 shadow-sm'
                  : 'text-gray-600 hover:text-gray-900'
              }`}
            >
              <Code className="w-4 h-4" />
              YAML Editor
            </button>
          </div>
        </div>

        {editMode === 'form' ? (
          <>
            {/* Want Name */}
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-2">
            Want Name *
          </label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
            placeholder="Enter want name"
            required
          />
        </div>

        {/* Want Type */}
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-2">
            Want Type *
          </label>
          <select
            value={type}
            onChange={(e) => setType(e.target.value)}
            className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
            required
          >
            {WANT_TYPES.map(wantType => (
              <option key={wantType.value} value={wantType.value}>
                {wantType.label}
              </option>
            ))}
          </select>
        </div>

        {/* Labels */}
        <div>
          <div className="flex items-center justify-between mb-2">
            <label className="block text-sm font-medium text-gray-700">
              Labels
            </label>
            <button
              type="button"
              onClick={addLabel}
              className="text-blue-600 hover:text-blue-800 text-sm flex items-center gap-1"
            >
              <Plus className="w-4 h-4" />
              Add Label
            </button>
          </div>
          <div className="space-y-2">
            {Object.entries(labels).map(([key, value], index) => (
              <div key={index} className="flex gap-2">
                <input
                  type="text"
                  value={key}
                  onChange={(e) => updateLabel(key, e.target.value, value)}
                  className="flex-1 px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                  placeholder="Key"
                />
                <input
                  type="text"
                  value={value}
                  onChange={(e) => updateLabel(key, key, e.target.value)}
                  className="flex-1 px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                  placeholder="Value"
                />
                <button
                  type="button"
                  onClick={() => removeLabel(key)}
                  className="text-red-600 hover:text-red-800 p-2"
                >
                  <X className="w-4 h-4" />
                </button>
              </div>
            ))}
          </div>
        </div>

        {/* Parameters */}
        <div>
          <div className="flex items-center justify-between mb-2">
            <label className="block text-sm font-medium text-gray-700">
              Parameters
            </label>
            <button
              type="button"
              onClick={addParam}
              className="text-blue-600 hover:text-blue-800 text-sm flex items-center gap-1"
            >
              <Plus className="w-4 h-4" />
              Add Parameter
            </button>
          </div>
          <div className="space-y-2">
            {Object.entries(params).map(([key, value], index) => (
              <div key={index} className="flex gap-2">
                <input
                  type="text"
                  value={key}
                  onChange={(e) => {
                    const newKey = e.target.value;
                    const newParams = { ...params };
                    if (key !== newKey) {
                      delete newParams[key];
                      if (newKey.trim()) {
                        newParams[newKey] = value;
                      }
                      setParams(newParams);
                    }
                  }}
                  className="flex-1 px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                  placeholder="Parameter name"
                />
                <input
                  type="text"
                  value={String(value)}
                  onChange={(e) => updateParam(key, e.target.value)}
                  className="flex-1 px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                  placeholder="Parameter value"
                />
                <button
                  type="button"
                  onClick={() => removeParam(key)}
                  className="text-red-600 hover:text-red-800 p-2"
                >
                  <X className="w-4 h-4" />
                </button>
              </div>
            ))}
          </div>
        </div>

        {/* Using (Dependencies) */}
        <div>
          <div className="flex items-center justify-between mb-2">
            <label className="block text-sm font-medium text-gray-700">
              Dependencies (using)
            </label>
            <button
              type="button"
              onClick={addUsing}
              className="text-blue-600 hover:text-blue-800 text-sm flex items-center gap-1"
            >
              <Plus className="w-4 h-4" />
              Add Dependency
            </button>
          </div>
          <div className="space-y-2">
            {using.map((usingItem, index) => (
              <div key={index} className="space-y-2 border border-gray-200 rounded-md p-3">
                {Object.entries(usingItem).map(([key, value], keyIndex) => (
                  <div key={keyIndex} className="flex gap-2">
                    <input
                      type="text"
                      value={key}
                      onChange={(e) => {
                        const newUsing = [...using];
                        const newItem = { ...newUsing[index] };
                        delete newItem[key];
                        if (e.target.value.trim()) {
                          newItem[e.target.value] = value;
                        }
                        newUsing[index] = newItem;
                        setUsing(newUsing);
                      }}
                      className="flex-1 px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                      placeholder="Selector key (e.g., role, category)"
                    />
                    <input
                      type="text"
                      value={value}
                      onChange={(e) => updateUsing(index, key, e.target.value)}
                      className="flex-1 px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                      placeholder="Selector value"
                    />
                  </div>
                ))}
                <button
                  type="button"
                  onClick={() => removeUsing(index)}
                  className="text-red-600 hover:text-red-800 text-sm flex items-center gap-1"
                >
                  <X className="w-4 h-4" />
                  Remove Dependency
                </button>
              </div>
            ))}
          </div>
        </div>

        {/* Recipe */}
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-2">
            Recipe (optional)
          </label>
          <input
            type="text"
            value={recipe}
            onChange={(e) => setRecipe(e.target.value)}
            className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
            placeholder="Recipe name"
          />
        </div>
          </>
        ) : (
          <>
            {/* YAML Editor */}
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Want Configuration (YAML)
              </label>
              <YamlEditor
                value={yamlContent}
                onChange={setYamlContent}
                placeholder="Enter want configuration in YAML format..."
              />
            </div>
          </>
        )}

        {/* Error Display */}
        {validationError && (
          <ErrorDisplay error={validationError} />
        )}

        {apiError && (
          <ErrorDisplay error={apiError} />
        )}

        {error && (
          <ErrorDisplay error={error} />
        )}
      </form>
    </CreateSidebar>
  );
};