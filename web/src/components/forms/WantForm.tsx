import React, { useState, useEffect, useRef } from 'react';
import { Save, Plus, X, Code, Edit3, ChevronDown, Clock } from 'lucide-react';
import { Want, CreateWantRequest, UpdateWantRequest } from '@/types/want';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { ErrorDisplay } from '@/components/common/ErrorDisplay';
import { FormYamlToggle } from '@/components/common/FormYamlToggle';
import { RightSidebar } from '@/components/layout/RightSidebar';
import { YamlEditor } from './YamlEditor';
import { LabelAutocomplete } from './LabelAutocomplete';
import { LabelSelectorAutocomplete } from './LabelSelectorAutocomplete';
import { TypeRecipeSelector, TypeRecipeSelectorRef } from './TypeRecipeSelector';
import { LabelsSection } from './sections/LabelsSection';
import { DependenciesSection } from './sections/DependenciesSection';
import { SchedulingSection } from './sections/SchedulingSection';
import { ParametersSection } from './sections/ParametersSection';
import { validateYaml, stringifyYaml } from '@/utils/yaml';
import { generateWantName, generateUniqueWantName, isValidWantName } from '@/utils/nameGenerator';
import { addLabelToRegistry } from '@/utils/labelUtils';
import { truncateText } from '@/utils/helpers';
import { getBackgroundStyle } from '@/utils/backgroundStyles';
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

  // Ref for TypeRecipeSelector
  const typeSelectorRef = useRef<TypeRecipeSelectorRef>(null);

  // Refs for form fields navigation
  const nameInputRef = useRef<HTMLInputElement>(null);
  const paramsSectionRef = useRef<HTMLButtonElement>(null);
  const labelsSectionRef = useRef<HTMLButtonElement>(null);
  const dependenciesSectionRef = useRef<HTMLButtonElement>(null);
  const schedulingSectionRef = useRef<HTMLButtonElement>(null);
  const addButtonRef = useRef<HTMLButtonElement>(null);
  const lastFocusedFieldRef = useRef<HTMLElement | null>(null);

  // UI state
  const [editMode, setEditMode] = useState<'form' | 'yaml'>('form');
  const [isEditing, setIsEditing] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [validationError, setValidationError] = useState<string | null>(null);
  const [apiError, setApiError] = useState<ApiError | null>(null);
  const [wantTypeLoading, setWantTypeLoading] = useState(false);
  const [selectedTypeId, setSelectedTypeId] = useState<string | null>(null); // Selected want type or recipe ID
  const [selectedItemType, setSelectedItemType] = useState<'want-type' | 'recipe'>('want-type'); // Type of selected item
  const [userNameSuffix, setUserNameSuffix] = useState(''); // User-provided name suffix for auto generation
  const [collapsedSections, setCollapsedSections] = useState<Set<'parameters' | 'labels' | 'dependencies' | 'scheduling'>>(() => {
    // All sections collapsed by default
    return new Set(['parameters', 'labels', 'dependencies', 'scheduling']);
  });

  // Handle Tab key from editing fields (Name or Sections) to focus Add button
  const handleFieldTab = () => {
    lastFocusedFieldRef.current = document.activeElement as HTMLElement;
    addButtonRef.current?.focus();
  };

  // Handle Tab key from Add button to return to last focused field
  const handleAddButtonKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Tab') {
      e.preventDefault();
      if (lastFocusedFieldRef.current) {
        lastFocusedFieldRef.current.focus();
      } else {
        // Fallback to name input or first section
        if (nameInputRef.current) {
          nameInputRef.current.focus();
        } else {
          paramsSectionRef.current?.focus();
        }
      }
    }
  };

  // Form state
  const [name, setName] = useState('');
  const [type, setType] = useState('');
  const [labels, setLabels] = useState<Record<string, string>>({});
  const [params, setParams] = useState<Record<string, unknown>>({});
  const [using, setUsing] = useState<Array<Record<string, string>>>([]);
  const [when, setWhen] = useState<Array<{ at?: string; every: string }>>([]);

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

  // Keyboard shortcut: / to focus search
  useEffect(() => {
    if (!isOpen || editMode !== 'form') return;

    const handleKeyDown = (e: KeyboardEvent) => {
      // Don't intercept if user is typing in an input (except for the target search box)
      const target = e.target as HTMLElement;
      const isInputElement =
        target.tagName === 'INPUT' ||
        target.tagName === 'TEXTAREA' ||
        target.isContentEditable;

      // If typing in an input, skip (the search input will handle ESC itself)
      if (isInputElement) return;

      // Handle / to open search and focus
      if (e.key === '/' && !e.shiftKey && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault();
        // Focus search input
        setTimeout(() => {
          typeSelectorRef.current?.focusSearch();
        }, 0);
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, editMode]);

  // Handle arrow key navigation for form fields
  const handleArrowKeyNavigation = (e: React.KeyboardEvent, currentField: string) => {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;

    const fields = ['type', 'name', 'parameters', 'labels', 'dependencies', 'scheduling'];
    const currentIndex = fields.indexOf(currentField);

    if (e.key === 'ArrowDown' && currentIndex < fields.length - 1) {
      e.preventDefault();
      const nextField = fields[currentIndex + 1];
      switch (nextField) {
        case 'name':
          nameInputRef.current?.focus();
          break;
        case 'parameters':
          paramsSectionRef.current?.focus();
          break;
        case 'labels':
          labelsSectionRef.current?.focus();
          break;
        case 'dependencies':
          dependenciesSectionRef.current?.focus();
          break;
        case 'scheduling':
          schedulingSectionRef.current?.focus();
          break;
      }
    } else if (e.key === 'ArrowUp' && currentIndex > 0) {
      e.preventDefault();
      const prevField = fields[currentIndex - 1];
      switch (prevField) {
        case 'type':
          typeSelectorRef.current?.focus();
          break;
        case 'name':
          nameInputRef.current?.focus();
          break;
        case 'parameters':
          paramsSectionRef.current?.focus();
          break;
        case 'labels':
          labelsSectionRef.current?.focus();
          break;
        case 'dependencies':
          dependenciesSectionRef.current?.focus();
          break;
      }
    }
  };

  // Convert form data to want object
  const formToWantObject = () => {
    // Filter out using entries with empty keys
    const validUsing = using.filter(item => Object.keys(item)[0]?.trim());
    // Filter out when entries with empty every
    const validWhen = when.filter(item => item.every?.trim());

    return {
      metadata: {
        name: name.trim(),
        type: type.trim(),
        ...(Object.keys(labels).length > 0 && { labels })
      },
      spec: {
        ...(Object.keys(params).length > 0 && { params }),
        ...(validUsing.length > 0 && { using: validUsing }),
        ...(validWhen.length > 0 && { when: validWhen })
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
    setWhen(want.spec?.when || []);
  };

  // Update YAML when form data changes
  useEffect(() => {
    if (editMode === 'form') {
      const wantObject = formToWantObject();
      console.log('Updating YAML - wantObject:', wantObject, 'using state:', using);
      setYamlContent(stringifyYaml(wantObject));
    }
  }, [name, type, labels, params, using, when, editMode]);

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

  const toggleSection = (section: 'parameters' | 'labels' | 'dependencies' | 'scheduling') => {
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
    setWhen([]);
    setSelectedTypeId(null); // Reset selector state
    setSelectedItemType('want-type');
    setUserNameSuffix('');
    setYamlContent(stringifyYaml({
      metadata: { name: '', type: '' },
      spec: {}
    }));
    setValidationError(null);
    setApiError(null);
    setCollapsedSections(new Set(['parameters', 'labels', 'dependencies', 'scheduling']));
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
        setParams(firstExample.want?.spec?.params || {});
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
    if (selectedItemType === 'recipe' && !isEditing && type) {
      const selectedRecipe = recipes.find(r => r.recipe?.metadata?.custom_type === type);
      if (selectedRecipe && selectedRecipe.recipe?.parameters) {
        // Use recipe parameters directly as editable parameters
        setParams(selectedRecipe.recipe.parameters);
      } else {
        setParams({});
      }
    }
  }, [selectedItemType, type, recipes, isEditing]);

  const validateForm = (): boolean => {
    if (!name.trim()) {
      setValidationError('Want name is required');
      return false;
    }
    if (!type.trim()) {
      setValidationError('Want type or recipe is required');
      return false;
    }
    setValidationError(null);
    return true;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setApiError(null);
    setIsSubmitting(true);

    try {
      let wantRequest: CreateWantRequest | UpdateWantRequest;

      console.log('handleSubmit - editMode:', editMode, 'form state using:', using, 'labels:', labels, 'params:', params);

      if (editMode === 'yaml') {
        // Parse YAML to want object
        if (!yamlContent.trim()) {
          setValidationError('YAML content is required');
          setIsSubmitting(false);
          return;
        }

        const yamlValidation = validateYaml(yamlContent);
        if (!yamlValidation.isValid) {
          setValidationError(`Invalid YAML: ${yamlValidation.error}`);
          setIsSubmitting(false);
          return;
        }

        wantRequest = yamlValidation.data as CreateWantRequest | UpdateWantRequest;
      } else {
        // Use form data
        if (!validateForm()) {
          setIsSubmitting(false);
          return;
        }
        wantRequest = formToWantObject();
        console.log('handleSubmit - form mode, wantRequest:', wantRequest);
      }

      console.log('handleSubmit - final wantRequest:', wantRequest);

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
    } finally {
      setIsSubmitting(false);
    }
  };


  const isTypeSelected = !!type;
  const shouldGlowButton = isTypeSelected && !isEditing && selectedTypeId;

  const headerAction = (
    <div className="flex items-center gap-3">
      <FormYamlToggle
        mode={editMode}
        onModeChange={setEditMode}
      />
      <button
        ref={addButtonRef}
        type="submit"
        disabled={isSubmitting || (!isEditing && !isTypeSelected)}
        form="want-form"
        onKeyDown={handleAddButtonKeyDown}
        className={`flex items-center justify-center gap-1.5 bg-blue-600 text-white px-3 py-2 text-sm rounded-xl hover:bg-blue-700 focus:outline-none focus-glow disabled:opacity-50 disabled:cursor-not-allowed whitespace-nowrap ${
          shouldGlowButton ? '' : ''
        }`}
        style={shouldGlowButton ? { height: '2.4rem' } : { height: '2rem' }}
      >
        {isSubmitting ? (
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

  // Get background style based on selected want type
  // Only show background when a type is selected (not during selection/re-selection)
  const backgroundStyle = (selectedTypeId && selectedWantType)
    ? getBackgroundStyle(selectedWantType.metadata.name).style
    : undefined;

  return (
    <RightSidebar
      isOpen={isOpen}
      onClose={onClose}
      title={isEditing ? 'Edit Want' : 'New Want'}
      headerActions={headerAction}
      backgroundStyle={backgroundStyle}
    >
      <form id="want-form" onSubmit={handleSubmit} className="space-y-3">

        {editMode === 'form' ? (
          <>
            {/* Type/Recipe Selector */}
            <div>
              {!selectedTypeId && (
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  Select Want Type or Recipe *
                </label>
              )}
              <TypeRecipeSelector
                ref={typeSelectorRef}
                wantTypes={wantTypes}
                recipes={recipes}
                selectedId={selectedTypeId}
                showSearch={true}
                onSelect={(id, itemType) => {
                  setSelectedTypeId(id);
                  setSelectedItemType(itemType);
                  // Update type based on selection
                  setType(id);
                  // Auto-generate unique name that doesn't conflict with existing wants
                  const existingNames = new Set(wants?.map(w => w.metadata?.name) || []);
                  const generatedName = generateUniqueWantName(id, itemType, existingNames, userNameSuffix);
                  setName(generatedName);

                  // Auto-focus Want Name after selection
                  setTimeout(() => {
                    nameInputRef.current?.focus();
                  }, 0);
                }}
                onClear={() => {
                  setSelectedTypeId(null);
                  setSelectedItemType(null);
                  setType('');
                  setName('');
                }}
                onGenerateName={(id, itemType, suffix) => {
                  const existingNames = new Set(wants?.map(w => w.metadata?.name) || []);
                  return generateUniqueWantName(id, itemType, existingNames, suffix);
                }}
                onArrowDown={() => {
                  nameInputRef.current?.focus();
                }}
              />
            </div>

            {/* Show fields only when a want type or recipe is selected */}
            {selectedTypeId && (
              <>
                {/* Want Name with Auto-generation */}
                <div className="bg-blue-50 rounded-lg p-4">
                  <label className="block text-sm font-medium text-gray-700 mb-2">
                    Want Name *
                  </label>
                  <div className="space-y-2">
                    <input
                      ref={nameInputRef}
                      type="text"
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === 'Tab') {
                          e.preventDefault();
                          handleFieldTab();
                        } else {
                          handleArrowKeyNavigation(e, 'name');
                        }
                      }}
                      className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                      placeholder="Auto-generated or enter custom name"
                      required
                    />
                    <div className="text-xs text-gray-600">
                      <p className="mb-1">💡 Tip: Edit the name or use the selector's suffix option to customize auto-generation</p>
                      {!isValidWantName(name) && name.trim() && (
                        <p className="text-red-600">⚠️ Name contains invalid characters. Use only letters, numbers, hyphens, and underscores.</p>
                      )}
                    </div>
                  </div>
                </div>

                {/* Parameters Section */}
                <ParametersSection
                  ref={paramsSectionRef}
                  parameters={params}
                  parameterDefinitions={selectedWantType?.parameters}
                  onChange={setParams}
                  isCollapsed={collapsedSections.has('parameters')}
                  onToggleCollapse={() => toggleSection('parameters')}
                  navigationCallbacks={{
                    onNavigateUp: () => handleArrowKeyNavigation({ key: 'ArrowUp', preventDefault: () => {} } as any, 'parameters'),
                    onNavigateDown: () => handleArrowKeyNavigation({ key: 'ArrowDown', preventDefault: () => {} } as any, 'parameters'),
                    onTab: handleFieldTab,
                  }}
                />

                {/* Labels Section */}
                <LabelsSection
                  ref={labelsSectionRef}
                  labels={labels}
                  onChange={setLabels}
                  isCollapsed={collapsedSections.has('labels')}
                  onToggleCollapse={() => toggleSection('labels')}
                  navigationCallbacks={{
                    onNavigateUp: () => handleArrowKeyNavigation({ key: 'ArrowUp', preventDefault: () => {} } as any, 'labels'),
                    onNavigateDown: () => handleArrowKeyNavigation({ key: 'ArrowDown', preventDefault: () => {} } as any, 'labels'),
                    onTab: handleFieldTab,
                  }}
                />

                {/* Dependencies Section */}
                <DependenciesSection
                  ref={dependenciesSectionRef}
                  dependencies={using}
                  onChange={setUsing}
                  isCollapsed={collapsedSections.has('dependencies')}
                  onToggleCollapse={() => toggleSection('dependencies')}
                  navigationCallbacks={{
                    onNavigateUp: () => handleArrowKeyNavigation({ key: 'ArrowUp', preventDefault: () => {} } as any, 'dependencies'),
                    onNavigateDown: () => handleArrowKeyNavigation({ key: 'ArrowDown', preventDefault: () => {} } as any, 'dependencies'),
                    onTab: handleFieldTab,
                  }}
                />

                {/* Scheduling Section */}
                <SchedulingSection
                  ref={schedulingSectionRef}
                  schedules={when}
                  onChange={setWhen}
                  isCollapsed={collapsedSections.has('scheduling')}
                  onToggleCollapse={() => toggleSection('scheduling')}
                  navigationCallbacks={{
                    onNavigateUp: () => handleArrowKeyNavigation({ key: 'ArrowUp', preventDefault: () => {} } as any, 'scheduling'),
                    onNavigateDown: () => handleArrowKeyNavigation({ key: 'ArrowDown', preventDefault: () => {} } as any, 'scheduling'),
                    onTab: handleFieldTab,
                  }}
                />
              </>
            )}
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
    </RightSidebar>
  );
};