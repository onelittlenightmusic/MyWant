import React, { useState, useEffect } from 'react';
import { Save, Plus, X, Code, Edit3 } from 'lucide-react';
import { Want, CreateWantRequest, UpdateWantRequest } from '@/types/want';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { ErrorDisplay } from '@/components/common/ErrorDisplay';
import { CreateSidebar } from '@/components/layout/CreateSidebar';
import { YamlEditor } from './YamlEditor';
import { LabelAutocomplete } from './LabelAutocomplete';
import { LabelSelectorAutocomplete } from './LabelSelectorAutocomplete';
import { validateYaml, stringifyYaml } from '@/utils/yaml';
import { useWantStore } from '@/stores/wantStore';
import { useWantTypeStore } from '@/stores/wantTypeStore';
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
      wants: [
        {
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
      ]
    }
  }
];


export const WantForm: React.FC<WantFormProps> = ({
  isOpen,
  onClose,
  editingWant
}) => {
  const { createWant, updateWant, loading, error } = useWantStore();
  const { wantTypes, selectedWantType, fetchWantTypes, getWantType } = useWantTypeStore();

  // UI state
  const [editMode, setEditMode] = useState<'form' | 'yaml'>('form');
  const [isEditing, setIsEditing] = useState(false);
  const [validationError, setValidationError] = useState<string | null>(null);
  const [apiError, setApiError] = useState<ApiError | null>(null);
  const [wantTypeLoading, setWantTypeLoading] = useState(false);
  const [editingLabelKey, setEditingLabelKey] = useState<string | null>(null);
  const [editingLabelDraft, setEditingLabelDraft] = useState<{ key: string; value: string }>({ key: '', value: '' }); // Temporary draft for label being edited
  const [editingUsingIndex, setEditingUsingIndex] = useState<number | null>(null);
  const [editingUsingDraft, setEditingUsingDraft] = useState<{ key: string; value: string }>({ key: '', value: '' }); // Temporary draft for dependency being edited

  // Form state
  const [name, setName] = useState('');
  const [type, setType] = useState('');
  const [labels, setLabels] = useState<Record<string, string>>({});
  const [params, setParams] = useState<Record<string, unknown>>({});
  const [using, setUsing] = useState<Array<Record<string, string>>>([]);
  const [recipe, setRecipe] = useState('');

  // YAML state
  const [yamlContent, setYamlContent] = useState('');

  // Fetch want types on component mount
  useEffect(() => {
    if (isOpen && wantTypes.length === 0) {
      fetchWantTypes();
    }
  }, [isOpen, wantTypes.length, fetchWantTypes]);

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

  // Initialize form when sidebar opens/closes
  useEffect(() => {
    if (!isOpen) {
      resetForm();
    }
  }, [isOpen]);

  // Initialize form when editing
  useEffect(() => {
    if (editingWant) {
      setIsEditing(true);
      wantObjectToForm(editingWant);
      setYamlContent(stringifyYaml({
        metadata: editingWant.metadata,
        spec: editingWant.spec
      }));
    }
  }, [editingWant]);

  const resetForm = () => {
    setIsEditing(false);
    setEditMode('form');
    setName('');
    setType('');
    setLabels({});
    setParams({});
    setUsing([]);
    setRecipe('');
    setYamlContent(stringifyYaml({
      metadata: { name: '', type: '' },
      spec: {}
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
      return;
    }

    // Single-want configuration handling
    if (config.metadata) {
      setName(config.metadata.name);
      setType(config.metadata.type);
      const labels = config.metadata.labels || {};
      setLabels(Object.fromEntries(
        Object.entries(labels).filter(([_, value]) => value !== undefined)
      ));
    }
    if (config.spec) {
      setParams(config.spec.params ? {...config.spec.params} : {});
      setUsing((config.spec as any).using || []);
      setRecipe(config.spec.recipe || '');
    }
    setYamlContent(stringifyYaml(config));
  };

  // Update form when want type is selected
  useEffect(() => {
    if (type && !isEditing) {
      setWantTypeLoading(true);
      getWantType(type).finally(() => {
        setWantTypeLoading(false);
      });
    }
  }, [type, isEditing, getWantType]);

  // Populate parameters from want type examples when selectedWantType changes
  useEffect(() => {
    if (selectedWantType && !isEditing && type === selectedWantType.metadata.name) {
      // Get the first example if available, use parameter examples otherwise
      if (selectedWantType.examples && selectedWantType.examples.length > 0) {
        const firstExample = selectedWantType.examples[0];
        setParams(firstExample.params || {});
      } else if (selectedWantType.parameters && selectedWantType.parameters.length > 0) {
        // Fallback: use parameter examples
        const paramsFromExamples: Record<string, unknown> = {};
        selectedWantType.parameters.forEach(param => {
          if (param.example !== undefined) {
            paramsFromExamples[param.name] = param.example;
          } else if (param.default !== undefined) {
            paramsFromExamples[param.name] = param.default;
          }
        });
        setParams(paramsFromExamples);
      } else {
        setParams({});
      }
    }
  }, [selectedWantType, isEditing, type]);

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
    setEditingLabelKey(''); // Start editing the new label immediately
    setEditingLabelDraft({ key: '', value: '' }); // Initialize draft state
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
    setEditingUsingIndex(using.length); // Start editing the new dependency
    setEditingUsingDraft({ key: '', value: '' });
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

  const headerAction = (
    <button
      type="submit"
      disabled={loading}
      form="want-form"
      className="flex items-center justify-center gap-1.5 bg-blue-600 text-white px-3 py-1.5 text-sm rounded-md hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed whitespace-nowrap"
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
  );

  return (
    <CreateSidebar
      isOpen={isOpen}
      onClose={onClose}
      title={isEditing ? 'Edit Want' : 'Create New Want'}
      headerAction={headerAction}
    >
      <form id="want-form" onSubmit={handleSubmit} className="space-y-6">

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
            className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent disabled:bg-gray-100 disabled:cursor-not-allowed"
            required
            disabled={wantTypes.length === 0 || wantTypeLoading}
          >
            <option value="">Select a want type...</option>
            {wantTypes.map(wantType => (
              <option key={wantType.name} value={wantType.name}>
                {wantType.title} ({wantType.category})
              </option>
            ))}
          </select>
          {wantTypeLoading && (
            <div className="mt-2 flex items-center gap-2 text-sm text-blue-600">
              <div className="animate-spin rounded-full h-3 w-3 border-b-2 border-blue-600"></div>
              Loading want type details...
            </div>
          )}
        </div>

        {/* Labels */}
        <div className="bg-gray-50 rounded-lg p-6">
          <div className="flex items-center justify-between mb-4">
            <h4 className="text-base font-medium text-gray-900">Labels</h4>
            {editingLabelKey === null && (
              <button
                type="button"
                onClick={() => addLabel()}
                className="text-blue-600 hover:text-blue-800 text-sm font-medium"
              >
                +
              </button>
            )}
          </div>

          {/* Display existing labels as styled chips */}
          {Object.entries(labels).length > 0 && (
            <div className="flex flex-wrap gap-2 mb-4">
              {Object.entries(labels).map(([key, value]) => {
                // Don't show chip if this label is being edited
                if (editingLabelKey === key) return null;

                return (
                  <button
                    key={key}
                    type="button"
                    onClick={() => {
                      setEditingLabelKey(key);
                      setEditingLabelDraft({ key, value });
                    }}
                    className="inline-flex items-center px-3 py-1.5 rounded-full text-sm font-medium bg-blue-100 text-blue-800 hover:bg-blue-200 transition-colors cursor-pointer"
                  >
                    {key}: {value}
                    <X
                      className="w-3 h-3 ml-2 hover:text-blue-900"
                      onClick={(e) => {
                        e.stopPropagation();
                        removeLabel(key);
                      }}
                    />
                  </button>
                );
              })}
            </div>
          )}

          {/* Label input form - shown when editing or adding new label */}
          {editingLabelKey !== null && (
            <div className="space-y-3 pt-4 border-t border-gray-200">
              <LabelAutocomplete
                keyValue={editingLabelDraft.key}
                valueValue={editingLabelDraft.value}
                onKeyChange={(newKey) => setEditingLabelDraft(prev => ({ ...prev, key: newKey }))}
                onValueChange={(newValue) => setEditingLabelDraft(prev => ({ ...prev, value: newValue }))}
                onRemove={() => {
                  removeLabel(editingLabelKey);
                  setEditingLabelKey(null);
                  setEditingLabelDraft({ key: '', value: '' });
                }}
              />
              <div className="flex gap-2">
                <button
                  type="button"
                  onClick={() => {
                    // Only update if draft has a key value
                    if (editingLabelDraft.key.trim()) {
                      updateLabel(editingLabelKey === '__new__' ? '' : editingLabelKey, editingLabelDraft.key, editingLabelDraft.value);
                    }
                    setEditingLabelKey(null);
                    setEditingLabelDraft({ key: '', value: '' });
                  }}
                  className="px-3 py-1.5 bg-blue-600 text-white text-sm rounded-md hover:bg-blue-700"
                >
                  Save
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setEditingLabelKey(null);
                    setEditingLabelDraft({ key: '', value: '' });
                  }}
                  className="px-3 py-1.5 border border-gray-300 text-gray-700 text-sm rounded-md hover:bg-gray-100"
                >
                  Cancel
                </button>
              </div>
            </div>
          )}
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
              disabled={!type}
            >
              <Plus className="w-4 h-4" />
              Add Parameter
            </button>
          </div>

          {/* Parameter definitions hint from want type */}
          {selectedWantType && selectedWantType.parameters && selectedWantType.parameters.length > 0 && (
            <div className="mb-4 p-3 bg-blue-50 border border-blue-200 rounded-md">
              <p className="text-sm font-medium text-blue-900 mb-2">Available Parameters:</p>
              <div className="space-y-1">
                {selectedWantType.parameters.map((param) => (
                  <div key={param.name} className="text-xs text-blue-800">
                    <span className="font-medium">{param.name}</span>
                    {param.required && <span className="text-red-600 ml-1">*</span>}
                    <span className="text-blue-700 ml-1">({param.type})</span>
                    {param.description && <span className="text-blue-600 ml-1">- {param.description}</span>}
                  </div>
                ))}
              </div>
            </div>
          )}

          <div className="space-y-2">
            {Object.entries(params).map(([key, value], index) => (
              <div key={index} className="space-y-1">
                <div className="flex gap-2">
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
                {/* Show parameter help text from want type definition */}
                {selectedWantType && selectedWantType.parameters && (
                  (() => {
                    const paramDef = selectedWantType.parameters.find(p => p.name === key);
                    return paramDef ? (
                      <p className="text-xs text-gray-600 ml-3">{paramDef.description}</p>
                    ) : null;
                  })()
                )}
              </div>
            ))}
          </div>
        </div>

        {/* Using (Dependencies) */}
        <div className="bg-gray-50 rounded-lg p-6">
          <div className="flex items-center justify-between mb-4">
            <h4 className="text-base font-medium text-gray-900">Dependencies (using)</h4>
            {editingUsingIndex === null && (
              <button
                type="button"
                onClick={addUsing}
                className="text-blue-600 hover:text-blue-800 text-sm font-medium"
              >
                +
              </button>
            )}
          </div>

          {/* Display existing dependencies as styled chips */}
          {using.length > 0 && (
            <div className="flex flex-wrap gap-2 mb-4">
              {using.map((usingItem, index) => {
                // Don't show chip if this dependency is being edited
                if (editingUsingIndex === index) return null;

                return Object.entries(usingItem).map(([key, value], keyIndex) => (
                  <button
                    key={`${index}-${keyIndex}`}
                    type="button"
                    onClick={() => {
                      setEditingUsingIndex(index);
                      setEditingUsingDraft({ key, value });
                    }}
                    className="inline-flex items-center px-3 py-1.5 rounded-full text-sm font-medium bg-blue-100 text-blue-800 hover:bg-blue-200 transition-colors cursor-pointer"
                  >
                    {key}: {value}
                    <X
                      className="w-3 h-3 ml-2 hover:text-blue-900"
                      onClick={(e) => {
                        e.stopPropagation();
                        removeUsing(index);
                      }}
                    />
                  </button>
                ));
              })}
            </div>
          )}

          {/* Dependency input form - shown when editing or adding new dependency */}
          {editingUsingIndex !== null && (
            <div className="space-y-3 pt-4 border-t border-gray-200">
              <LabelSelectorAutocomplete
                keyValue={editingUsingDraft.key}
                valuValue={editingUsingDraft.value}
                onKeyChange={(newKey) => {
                  setEditingUsingDraft(prev => ({ ...prev, key: newKey }));
                }}
                onValueChange={(newValue) => {
                  setEditingUsingDraft(prev => ({ ...prev, value: newValue }));
                }}
                onRemove={() => {
                  if (editingUsingIndex >= 0) {
                    removeUsing(editingUsingIndex);
                  }
                  setEditingUsingIndex(null);
                  setEditingUsingDraft({ key: '', value: '' });
                }}
              />
              <div className="flex gap-2">
                <button
                  type="button"
                  onClick={() => {
                    // Confirm the changes
                    if (editingUsingDraft.key.trim()) {
                      const newUsing = [...using];
                      if (editingUsingIndex < newUsing.length) {
                        const newItem = { ...newUsing[editingUsingIndex] };
                        // Get the original key from the current item
                        const originalKey = Object.keys(newItem)[0];
                        if (originalKey) {
                          delete newItem[originalKey];
                        }
                        newItem[editingUsingDraft.key] = editingUsingDraft.value;
                        newUsing[editingUsingIndex] = newItem;
                      } else {
                        // Adding new dependency
                        newUsing.push({ [editingUsingDraft.key]: editingUsingDraft.value });
                      }
                      setUsing(newUsing);
                    }
                    setEditingUsingIndex(null);
                    setEditingUsingDraft({ key: '', value: '' });
                  }}
                  className="px-3 py-1.5 bg-blue-600 text-white text-sm rounded-md hover:bg-blue-700"
                >
                  Save
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setEditingUsingIndex(null);
                    setEditingUsingDraft({ key: '', value: '' });
                  }}
                  className="px-3 py-1.5 border border-gray-300 text-gray-700 text-sm rounded-md hover:bg-gray-100"
                >
                  Cancel
                </button>
              </div>
            </div>
          )}
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