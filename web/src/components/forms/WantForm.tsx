import React, { useState, useEffect } from 'react';
import { Save, Plus, X, Code, Edit3, Search, ChevronDown } from 'lucide-react';
import { Want, CreateWantRequest, UpdateWantRequest } from '@/types/want';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { ErrorDisplay } from '@/components/common/ErrorDisplay';
import { FormYamlToggle } from '@/components/common/FormYamlToggle';
import { CreateSidebar } from '@/components/layout/CreateSidebar';
import { YamlEditor } from './YamlEditor';
import { LabelAutocomplete } from './LabelAutocomplete';
import { LabelSelectorAutocomplete } from './LabelSelectorAutocomplete';
import { TypeRecipeSelector } from './TypeRecipeSelector';
import { validateYaml, stringifyYaml } from '@/utils/yaml';
import { generateWantName, generateUniqueWantName, isValidWantName } from '@/utils/nameGenerator';
import { addLabelToRegistry } from '@/utils/labelUtils';
import { useWantStore } from '@/stores/wantStore';
import { useWantTypeStore } from '@/stores/wantTypeStore';
import { useRecipeStore } from '@/stores/recipeStore';
import { ApiError } from '@/types/api';

interface WantFormProps {
  isOpen: boolean;
  onClose: () => void;
  editingWant?: Want | null;
}


export const WantForm: React.FC<WantFormProps> = ({
  isOpen,
  onClose,
  editingWant
}) => {
  const { wants, createWant, updateWant, loading, error } = useWantStore();
  const { wantTypes, selectedWantType, fetchWantTypes, getWantType } = useWantTypeStore();
  const { recipes, fetchRecipes } = useRecipeStore();

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
  const [selectedTypeId, setSelectedTypeId] = useState<string | null>(null); // Selected want type or recipe ID
  const [selectedItemType, setSelectedItemType] = useState<'want-type' | 'recipe'>('want-type'); // Type of selected item
  const [userNameSuffix, setUserNameSuffix] = useState(''); // User-provided name suffix for auto generation
  const [showSearch, setShowSearch] = useState(false); // Show/hide search filter
  const [collapsedSections, setCollapsedSections] = useState<Set<'parameters' | 'labels' | 'dependencies'>>(() => {
    // All sections collapsed by default
    return new Set(['parameters', 'labels', 'dependencies']);
  });

  // Form state
  const [name, setName] = useState('');
  const [type, setType] = useState('');
  const [labels, setLabels] = useState<Record<string, string>>({});
  const [params, setParams] = useState<Record<string, unknown>>({});
  const [using, setUsing] = useState<Array<Record<string, string>>>([]);
  const [recipe, setRecipe] = useState('');

  // YAML state
  const [yamlContent, setYamlContent] = useState('');

  // Fetch want types and recipes on component mount
  useEffect(() => {
    if (isOpen) {
      if (wantTypes.length === 0) {
        fetchWantTypes();
      }
      if (recipes.length === 0) {
        fetchRecipes();
      }
    }
  }, [isOpen, wantTypes.length, recipes.length, fetchWantTypes, fetchRecipes]);

  // Convert form data to want object
  const formToWantObject = () => {
    // Filter out using entries with empty keys
    const validUsing = using.filter(item => Object.keys(item)[0]?.trim());

    return {
      metadata: {
        name: name.trim(),
        type: type.trim(),
        ...(Object.keys(labels).length > 0 && { labels })
      },
      spec: {
        ...(Object.keys(params).length > 0 && { params }),
        ...(validUsing.length > 0 && { using: validUsing })
      }
    };
  };

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
      console.log('Updating YAML - wantObject:', wantObject, 'using state:', using);
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

  const toggleSection = (section: 'parameters' | 'labels' | 'dependencies') => {
    setCollapsedSections(prev => {
      const updated = new Set(prev);
      if (updated.has(section)) {
        updated.delete(section);
      } else {
        updated.add(section);
      }
      return updated;
    });
  };

  const resetForm = () => {
    setIsEditing(false);
    setEditMode('form');
    setName('');
    setType('');
    setLabels({});
    setParams({});
    setUsing([]);
    setRecipe('');
    setSelectedTypeId(null); // Reset selector state
    setSelectedItemType('want-type');
    setUserNameSuffix('');
    setShowSearch(false);
    setYamlContent(stringifyYaml({
      metadata: { name: '', type: '' },
      spec: {}
    }));
    setValidationError(null);
    setApiError(null);
    setCollapsedSections(new Set(['parameters', 'labels', 'dependencies']));
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
    // Skip if editing, or if a recipe is selected (recipe params handled separately)
    if (selectedItemType === 'recipe') {
      return;
    }

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
  }, [selectedWantType, isEditing, type, selectedItemType]);

  // Populate parameters from recipe definition when a recipe is selected
  useEffect(() => {
    if (selectedItemType === 'recipe' && !isEditing && recipe) {
      const selectedRecipe = recipes.find(r => r.recipe?.metadata?.custom_type === recipe);
      if (selectedRecipe && selectedRecipe.recipe?.parameters) {
        // Use recipe parameters directly
        setParams(selectedRecipe.recipe.parameters);
      } else {
        setParams({});
      }
    }
  }, [selectedItemType, recipe, recipes, isEditing]);

  const validateForm = (): boolean => {
    if (!name.trim()) {
      setValidationError('Want name is required');
      return false;
    }
    if (!type.trim() && !recipe.trim()) {
      setValidationError('Either want type or recipe is required');
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

  const updateLabel = async (oldKey: string, newKey: string, value: string) => {
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

    // Register the label in the global registry
    if (newKey.trim() && value.trim()) {
      await addLabelToRegistry(newKey.trim(), value.trim());
    }
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

  const isTypeSelected = !!type || !!recipe;
  const shouldGlowButton = isTypeSelected && !isEditing && selectedTypeId;

  const headerAction = (
    <div className="flex items-center gap-3">
      <FormYamlToggle
        mode={editMode}
        onModeChange={setEditMode}
      />
      <button
        type="submit"
        disabled={loading || (!isEditing && !isTypeSelected)}
        form="want-form"
        className={`flex items-center justify-center gap-1.5 bg-blue-600 text-white px-3 py-2 text-sm rounded-md hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed whitespace-nowrap transition-all ${
          shouldGlowButton ? 'glow-button' : ''
        }`}
        style={shouldGlowButton ? { height: '2.4rem' } : { height: '2rem' }}
      >
        {loading ? (
          <LoadingSpinner size="sm" />
        ) : (
          <>
            <Save className="w-3.5 h-3.5" />
            {isEditing ? 'Update' : 'Add'}
          </>
        )}
      </button>
    </div>
  );

  return (
    <CreateSidebar
      isOpen={isOpen}
      onClose={onClose}
      title={isEditing ? 'Edit Want' : 'New Want'}
      headerAction={headerAction}
    >
      <form id="want-form" onSubmit={handleSubmit} className="space-y-6">

        {editMode === 'form' ? (
          <>
            {/* Type/Recipe Selector */}
            <div>
              <div className="flex items-center justify-between mb-2">
                <label className="block text-sm font-medium text-gray-700">
                  Select Want Type or Recipe *
                </label>
                <button
                  type="button"
                  onClick={() => setShowSearch(!showSearch)}
                  className="p-1 hover:bg-gray-100 rounded transition-colors"
                  title="Filter want types and recipes"
                >
                  <Search className={`w-4 h-4 ${showSearch ? 'text-blue-500' : 'text-gray-500'}`} />
                </button>
              </div>
              <TypeRecipeSelector
                wantTypes={wantTypes}
                recipes={recipes}
                selectedId={selectedTypeId}
                showSearch={showSearch}
                onSelect={(id, itemType) => {
                  setSelectedTypeId(id);
                  setSelectedItemType(itemType);
                  // Update type and recipe fields based on selection
                  if (itemType === 'want-type') {
                    setType(id);
                    setRecipe('');
                  } else {
                    // For recipes, set both type (custom_type) and recipe field
                    setType(id); // Set type to recipe's custom_type
                    setRecipe(id); // Set recipe to the recipe custom_type/name
                  }
                  // Auto-generate unique name that doesn't conflict with existing wants
                  const existingNames = new Set(wants?.map(w => w.metadata?.name) || []);
                  const generatedName = generateUniqueWantName(id, itemType, existingNames, userNameSuffix);
                  setName(generatedName);
                }}
                onGenerateName={(id, itemType, suffix) => {
                  const existingNames = new Set(wants?.map(w => w.metadata?.name) || []);
                  return generateUniqueWantName(id, itemType, existingNames, suffix);
                }}
              />
            </div>

            {/* Want Name with Auto-generation */}
            <div className="bg-blue-50 rounded-lg p-4">
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Want Name *
              </label>
              <div className="space-y-2">
                <input
                  type="text"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                  placeholder="Auto-generated or enter custom name"
                  required
                />
                {selectedTypeId && (
                  <div className="text-xs text-gray-600">
                    <p className="mb-1">💡 Tip: Edit the name or use the selector's suffix option to customize auto-generation</p>
                    {!isValidWantName(name) && name.trim() && (
                      <p className="text-red-600">⚠️ Name contains invalid characters. Use only letters, numbers, hyphens, and underscores.</p>
                    )}
                  </div>
                )}
              </div>
            </div>

            {/* Parameters - Collapsible Section */}
            <div className="border border-gray-200 rounded-lg">
              <button
                type="button"
                onClick={() => toggleSection('parameters')}
                className="w-full flex items-center justify-between p-4 hover:bg-gray-50 transition-colors"
              >
                <div className="flex items-center gap-3">
                  <ChevronDown
                    className={`w-5 h-5 text-gray-600 transition-transform ${
                      collapsedSections.has('parameters') ? '-rotate-90' : ''
                    }`}
                  />
                  <h4 className="text-base font-medium text-gray-900">Parameters</h4>
                </div>
                {collapsedSections.has('parameters') && Object.entries(params).length > 0 && (
                  <div className="text-sm text-gray-600 text-right flex-1 mr-2">
                    {Object.entries(params).map(([key, value]) => (
                      <div key={key} className="text-gray-500">
                        <span className="font-medium">"{key}"</span> is <span className="font-medium">"{String(value)}"</span>
                      </div>
                    ))}
                  </div>
                )}
              </button>

              {!collapsedSections.has('parameters') && (
                <div className="border-t border-gray-200 p-4 space-y-4">
                  {/* Parameter definitions hint from want type */}
                  {selectedWantType && selectedWantType.parameters && selectedWantType.parameters.length > 0 && (
                    <div className="p-3 bg-blue-50 border border-blue-200 rounded-md">
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
                          {selectedWantType && selectedWantType.parameters && selectedWantType.parameters.length > 0 ? (
                            <select
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
                            >
                              <option value="">Select parameter...</option>
                              {selectedWantType.parameters.map((param) => (
                                <option key={param.name} value={param.name}>
                                  {param.name} {param.required ? '*' : ''} ({param.type})
                                </option>
                              ))}
                            </select>
                          ) : (
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
                          )}
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

                  <div className="flex gap-2 pt-2 border-t border-gray-200">
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
                </div>
              )}
            </div>

            {/* Labels - Collapsible Section */}
            <div className="border border-gray-200 rounded-lg">
              <button
                type="button"
                onClick={() => toggleSection('labels')}
                className="w-full flex items-center justify-between p-4 hover:bg-gray-50 transition-colors"
              >
                <div className="flex items-center gap-3">
                  <ChevronDown
                    className={`w-5 h-5 text-gray-600 transition-transform ${
                      collapsedSections.has('labels') ? '-rotate-90' : ''
                    }`}
                  />
                  <h4 className="text-base font-medium text-gray-900">Labels</h4>
                </div>
                {collapsedSections.has('labels') && Object.entries(labels).length > 0 && (
                  <div className="text-sm text-gray-600 text-right flex-1 mr-2">
                    {Object.entries(labels).map(([key, value]) => (
                      <div key={key} className="text-gray-500">
                        <span className="font-medium">"{key}"</span> is <span className="font-medium">"{value}"</span>
                      </div>
                    ))}
                  </div>
                )}
              </button>

              {!collapsedSections.has('labels') && (
                <div className="border-t border-gray-200 p-4 space-y-4">
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

                  <div className="flex gap-2 pt-2 border-t border-gray-200">
                    {editingLabelKey === null && (
                      <button
                        type="button"
                        onClick={() => addLabel()}
                        className="text-blue-600 hover:text-blue-800 text-sm font-medium flex items-center gap-1"
                      >
                        <Plus className="w-4 h-4" />
                        Add Label
                      </button>
                    )}
                  </div>
                </div>
              )}
            </div>

            {/* Dependencies - Collapsible Section */}
            <div className="border border-gray-200 rounded-lg">
              <button
                type="button"
                onClick={() => toggleSection('dependencies')}
                className="w-full flex items-center justify-between p-4 hover:bg-gray-50 transition-colors"
              >
                <div className="flex items-center gap-3">
                  <ChevronDown
                    className={`w-5 h-5 text-gray-600 transition-transform ${
                      collapsedSections.has('dependencies') ? '-rotate-90' : ''
                    }`}
                  />
                  <h4 className="text-base font-medium text-gray-900">Dependencies (using)</h4>
                </div>
                {collapsedSections.has('dependencies') && using.length > 0 && (
                  <div className="text-sm text-gray-600 text-right flex-1 mr-2">
                    {using.map((usingItem, index) => {
                      const [key, value] = Object.entries(usingItem)[0];
                      return (
                        <div key={index} className="text-gray-500">
                          <span className="font-medium">"{key}"</span> is <span className="font-medium">"{value}"</span>
                        </div>
                      );
                    })}
                  </div>
                )}
              </button>

              {!collapsedSections.has('dependencies') && (
                <div className="border-t border-gray-200 p-4 space-y-4">
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

                  <div className="flex gap-2 pt-2 border-t border-gray-200">
                    {editingUsingIndex === null && (
                      <button
                        type="button"
                        onClick={addUsing}
                        className="text-blue-600 hover:text-blue-800 text-sm font-medium flex items-center gap-1"
                      >
                        <Plus className="w-4 h-4" />
                        Add Dependency
                      </button>
                    )}
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