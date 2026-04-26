import React, { useState, useEffect, useRef, useMemo } from 'react';
import { Save, Plus, Heart, X, Code, Edit3, ChevronDown, Clock, Bot, FolderOpen, Crown, Search } from 'lucide-react';
import { Want, CreateWantRequest, UpdateWantRequest, WhenSpec } from '@/types/want';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { ErrorDisplay } from '@/components/common/ErrorDisplay';
import { FormYamlToggle } from '@/components/common/FormYamlToggle';
import { RightSidebar } from '@/components/layout/RightSidebar';
import { YamlEditor } from './YamlEditor';
import { LabelAutocomplete } from './LabelAutocomplete';
import { LabelSelectorAutocomplete } from './LabelSelectorAutocomplete';
import { TypeRecipeSelector, TypeRecipeSelectorRef } from './TypeRecipeSelector';
import { WantInventoryPicker, WantSlot } from './WantInventoryPicker';
import { RecommendationSelector } from '@/components/interact/RecommendationSelector';
import { LabelsSection } from './sections/LabelsSection';
import { DependenciesSection } from './sections/DependenciesSection';
import { SchedulingSection } from './sections/SchedulingSection';
import { ParametersSection } from './sections/ParametersSection';
import { validateYaml, stringifyYaml } from '@/utils/yaml';
import { generateWantName, generateUniqueWantName, isValidWantName } from '@/utils/nameGenerator';
import { addLabelToRegistry } from '@/utils/labelUtils';
import { truncateText, classNames } from '@/utils/helpers';
import { getBackgroundStyle } from '@/utils/backgroundStyles';
import { useWantStore } from '@/stores/wantStore';
import { useWantTypeStore } from '@/stores/wantTypeStore';
import { useRecipeStore } from '@/stores/recipeStore';
import { ApiError } from '@/types/api';
import { Recommendation, ConfigModifications } from '@/types/interact';

interface WantFormProps {
  isOpen: boolean;
  onClose: () => void;
  editingWant?: Want | null;
  ownerWant?: Want | null;
  initialTypeId?: string;
  initialItemType?: 'want-type' | 'recipe';
  mode?: 'create' | 'edit' | 'recommendation';
  recommendations?: Recommendation[];
  selectedRecommendation?: Recommendation | null;
  onRecommendationSelect?: (rec: Recommendation) => void;
  onRecommendationDeploy?: (recId: string, modifications?: ConfigModifications) => void;
}


export const WantForm: React.FC<WantFormProps> = ({
  isOpen,
  onClose,
  editingWant,
  ownerWant = null,
  initialTypeId,
  initialItemType = 'want-type',
  mode = 'create',
  recommendations = [],
  selectedRecommendation = null,
  onRecommendationSelect,
  onRecommendationDeploy
}) => {
  const { wants, createWant, updateWant, fetchWants, loading, error } = useWantStore();
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
  const exampleMenuRef = useRef<HTMLDivElement>(null);

  const [showExampleMenu, setShowExampleMenu] = useState(false);

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
    // Parameters open by default, others collapsed
    return new Set(['labels', 'dependencies', 'scheduling']);
  });
  const [showAdvanced, setShowAdvanced] = useState(false);

  // Recommendation mode state
  const [selectedRecId, setSelectedRecId] = useState<string | null>(null);
  const isRecommendationMode = mode === 'recommendation';

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
  const [when, setWhen] = useState<WhenSpec[]>([]);
  const [exposes, setExposes] = useState<Array<{currentState?: string; param?: string; as?: string}>>([]);

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

  // Filter out system want types (custom_target, draft, owner)
  const userFacingWantTypes = useMemo(() => {
    return wantTypes.filter(wt => !wt.system_type);
  }, [wantTypes]);

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

  // Handle arrow key navigation for form fields based on DOM order
  const handleArrowKeyNavigation = (e: React.KeyboardEvent) => {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;

    // Find all focusable form elements within this container
    const currentTarget = e.currentTarget || (e as any).target;
    const container = currentTarget?.closest('.focusable-container');
    if (!container) return;

    const focusableElements = Array.from(container.querySelectorAll('.focusable-section-header')) as HTMLElement[];
    const currentIndex = focusableElements.indexOf(document.activeElement as HTMLElement);

    if (currentIndex === -1) {
      // If none focused, focus the first one on ArrowDown
      if (e.key === 'ArrowDown' && focusableElements.length > 0) {
        if (typeof e.preventDefault === 'function') e.preventDefault();
        focusableElements[0].focus();
      }
      return;
    }

    if (e.key === 'ArrowDown' && currentIndex < focusableElements.length - 1) {
      if (typeof e.preventDefault === 'function') e.preventDefault();
      focusableElements[currentIndex + 1].focus();
    } else if (e.key === 'ArrowUp' && currentIndex > 0) {
      if (typeof e.preventDefault === 'function') e.preventDefault();
      focusableElements[currentIndex - 1].focus();
    }
  };

  // Convert form data to want object
  const formToWantObject = () => {
    // Filter out using entries with empty keys
    const validUsing = using.filter(item => Object.keys(item)[0]?.trim());
    // Filter out when entries with neither every nor fromGlobalParam
    const validWhen = when.filter(item => item.every?.trim() || item.fromGlobalParam?.trim());

    const ownerName = ownerWant?.metadata?.name || '';
    const ownerId = ownerWant?.metadata?.id || ownerWant?.id || '';
    const ownerReferences = (ownerWant && ownerName && ownerId)
      ? [{ apiVersion: 'v1', kind: 'Want', name: ownerName, id: ownerId, controller: true, blockOwnerDeletion: true }]
      : (isEditing && editingWant?.metadata?.ownerReferences?.length ? editingWant.metadata.ownerReferences : undefined);

    return {
      metadata: {
        name: name.trim(),
        type: type.trim(),
        ...(Object.keys(labels).length > 0 && { labels }),
        ...(ownerReferences && { ownerReferences }),
      },
      spec: {
        ...(Object.keys(params).length > 0 && { params }),
        ...(validUsing.length > 0 && { using: validUsing }),
        ...(validWhen.length > 0 && { when: validWhen }),
        ...(exposes.length > 0 && { exposes })
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
    setExposes(want.spec?.exposes || []);
  };

  // Update YAML when form data changes
  useEffect(() => {
    if (editMode === 'form') {
      const wantObject = formToWantObject();
      console.log('Updating YAML - wantObject:', wantObject, 'using state:', using);
      setYamlContent(stringifyYaml(wantObject));
    }
  }, [name, type, labels, params, using, when, exposes, editMode, ownerWant]);

  // Initialize form when sidebar opens/closes
  useEffect(() => {
    if (!isOpen) {
      resetForm();
    } else if (initialTypeId && !editingWant) {
      setSelectedTypeId(initialTypeId);
      setSelectedItemType(initialItemType);
      setType(initialTypeId);
      const existingNames = new Set(wants?.map(w => w.metadata?.name) || []);
      setName(generateUniqueWantName(initialTypeId, initialItemType, existingNames, ''));
    }
  }, [isOpen, initialTypeId]);

  // Initialize form when editing
  useEffect(() => {
    if (editingWant) {
      setIsEditing(true);
      wantObjectToForm(editingWant);
      
      // Initialize selector state from want type/recipe
      const wType = editingWant.metadata?.type || '';
      if (wType) {
        // Try to determine if it's a recipe or want-type
        const isRecipe = recipes.some(r => r.recipe?.metadata?.custom_type === wType);
        setSelectedTypeId(wType);
        setSelectedItemType(isRecipe ? 'recipe' : 'want-type');
      }

      // Auto-show advanced if editing want has labels, deps, or scheduling
      const hasAdvanced =
        Object.keys(editingWant.metadata?.labels || {}).length > 0 ||
        (editingWant.spec?.using || []).length > 0 ||
        (editingWant.spec?.when || []).length > 0;
      setShowAdvanced(hasAdvanced);
      setYamlContent(stringifyYaml({
        metadata: editingWant.metadata,
        spec: editingWant.spec
      }));
    }
  }, [editingWant, recipes]);

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
    setExposes([]);
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
    setShowAdvanced(false);
  };

  // Close example menu on outside click
  useEffect(() => {
    if (!showExampleMenu) return;
    const handleClickOutside = (e: MouseEvent) => {
      if (exampleMenuRef.current && !exampleMenuRef.current.contains(e.target as Node)) {
        setShowExampleMenu(false);
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [showExampleMenu]);

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
      let hasParams = false;
      if (selectedWantType.examples && selectedWantType.examples.length > 0) {
        const firstExample = selectedWantType.examples[0];
        const exampleParams = firstExample.want?.spec?.params || {};
        setParams(exampleParams);
        hasParams = Object.keys(exampleParams).length > 0;
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
        hasParams = Object.keys(paramsFromExamples).length > 0;
      } else {
        setParams({});
      }
      // B: Auto-expand parameters section if type has params
      if (hasParams) {
        setCollapsedSections(prev => {
          const updated = new Set(prev);
          updated.delete('parameters');
          return updated;
        });
      }
    }
  }, [selectedWantType, isEditing, type, selectedItemType]);

  // Populate parameters from recipe definition when a recipe is selected
  useEffect(() => {
    if (selectedItemType === 'recipe' && !isEditing && type) {
      const selectedRecipe = recipes.find(r => r.recipe?.metadata?.custom_type === type);
      if (selectedRecipe && selectedRecipe.recipe?.parameters) {
        setParams(selectedRecipe.recipe.parameters);
        // B: Auto-expand parameters if recipe has params
        if (Object.keys(selectedRecipe.recipe.parameters).length > 0) {
          setCollapsedSections(prev => {
            const updated = new Set(prev);
            updated.delete('parameters');
            return updated;
          });
        }
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
      // Handle recommendation deployment differently
      if (isRecommendationMode && selectedRecId && onRecommendationDeploy) {
        const modifications: ConfigModifications = {
          parameterOverrides: params,
          disableWants: [] // Could add UI for this later
        };
        await onRecommendationDeploy(selectedRecId, modifications);
        onClose();
        resetForm();
        setIsSubmitting(false);
        return;
      }

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
        onClose();
        resetForm();
      } else {
        await createWant(wantRequest as CreateWantRequest);

        // Refresh at 500ms intervals for 10 seconds to capture status transitions quickly
        fetchWants().catch(console.error);
        const fastRefreshEnd = Date.now() + 10000;
        const fastRefreshInterval = setInterval(() => {
          if (Date.now() >= fastRefreshEnd) {
            clearInterval(fastRefreshInterval);
            return;
          }
          fetchWants().catch(console.error);
        }, 500);

        // Stay open and reset to inventory for next want
        resetForm();
      }
    } catch (error) {
      console.error('Failed to save want:', error);
      setApiError(error as ApiError);
    } finally {
      setIsSubmitting(false);
    }
  };


  const isTypeSelected = !!type;
  const shouldGlowButton = isTypeSelected && !isEditing && selectedTypeId;
  const [showFilter, setShowFilter] = useState(false);

  const headerAction = (
    <div className="flex items-stretch gap-0">
      {!isEditing && !!selectedTypeId && (
        <button
          type="button"
          onClick={() => setShowFilter(v => !v)}
          className={classNames(
            "flex flex-col items-center justify-center gap-0.5 px-3 h-full transition-all duration-150 focus:outline-none",
            showFilter
              ? "text-blue-600 dark:text-blue-400 bg-blue-50 dark:bg-blue-900/20"
              : "text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-white hover:bg-gray-100 dark:hover:bg-gray-800"
          )}
          title="Toggle search/filter"
        >
          <Search className="w-4 h-4" />
          <span className="text-[9px] font-bold uppercase tracking-tighter hidden sm:block">Filter</span>
        </button>
      )}
      <div className="flex items-center h-full px-2">
        <FormYamlToggle
          mode={editMode}
          onModeChange={setEditMode}
        />
      </div>
      <button
        ref={addButtonRef}
        type="submit"
        disabled={isSubmitting || (!isEditing && !isTypeSelected)}
        form="want-form"
        onKeyDown={handleAddButtonKeyDown}
        className={classNames(
          "flex flex-col items-center justify-center gap-0.5 px-4 h-full transition-all duration-150 flex-shrink-0 focus:outline-none",
          isSubmitting || (!isEditing && !isTypeSelected)
            ? "bg-gray-400/30 cursor-not-allowed grayscale opacity-50"
            : isEditing
              ? "bg-indigo-600/90 text-white hover:brightness-110 active:opacity-80"
              : isRecommendationMode
                ? "bg-purple-600/90 text-white hover:brightness-110 active:opacity-80"
                : "bg-green-600/90 text-white hover:brightness-110 active:opacity-80"
        )}
      >
        {isSubmitting ? (
          <LoadingSpinner size="sm" />
        ) : (
          <>
            <div className="w-5 h-5 flex items-center justify-center">
              {isEditing ? (
                <Save className="w-5 h-5" />
              ) : isRecommendationMode ? (
                <Plus className="w-5 h-5" />
              ) : (
                <span className="relative inline-flex flex-shrink-0">
                  <Heart className="w-4 h-4" />
                  <Plus className="w-2.5 h-2.5 absolute -top-1 -right-1" style={{ strokeWidth: 3 }} />
                </span>
              )}
            </div>
            <span className="text-white text-[9px] font-bold leading-none uppercase tracking-tighter hidden sm:block">
              {isRecommendationMode ? 'Deploy' : (isEditing ? 'Update' : 'Add')}
            </span>
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
      <form id="want-form" onSubmit={handleSubmit} className="space-y-3 focusable-container h-full flex flex-col">

        {editMode === 'form' ? (
          <>
            {/* Type/Recipe Selector or Recommendation Selector */}
            {isRecommendationMode ? (
              /* Recommendation Mode - always show selector or collapsed view */
              <div className={classNames(
                selectedRecId && selectedRecommendation ? "flex-shrink-0" : "flex-1 min-h-0 flex flex-col"
              )}>
                {selectedRecId && selectedRecommendation ? (
                  /* Collapsed view - show selected recommendation with Change button */
                  <button
                    type="button"
                    onClick={() => {
                      setSelectedRecId(null);
                      setSelectedTypeId(null);
                      setType('');
                      setName('');
                      setParams({});
                      setLabels({});
                      setUsing([]);
                      setWhen([]);
                    }}
                    className="w-full flex items-center justify-between p-4 bg-white dark:bg-gray-800 border-2 border-blue-300 dark:border-blue-600 rounded-lg hover:border-blue-400 dark:hover:border-blue-500 transition-colors group"
                  >
                    <div className="flex items-center gap-3">
                      <Bot className="w-5 h-5 text-blue-500" />
                      <div className="text-left">
                        <h4 className="font-medium text-gray-900 dark:text-gray-100">{selectedRecommendation.title}</h4>
                        <p className="text-xs text-gray-600 dark:text-gray-400 mt-1">{selectedRecommendation.approach}</p>
                      </div>
                    </div>
                    <span className="px-4 py-2 text-sm font-medium rounded-lg bg-blue-100 text-blue-700 transition-colors">
                      Change
                    </span>
                  </button>
                ) : (
                  /* Recommendation Selector */
                  <RecommendationSelector
                    recommendations={recommendations}
                    selectedId={selectedRecId}
                    onSelect={(rec) => {
                      setSelectedRecId(rec.id);
                      onRecommendationSelect?.(rec);

                      // Auto-populate form from recommendation
                      // The first want in the recommendation's config will be used as the primary want
                      if (rec.config && rec.config.wants && rec.config.wants.length > 0) {
                        const firstWant = rec.config.wants[0];
                        // Populate form fields from the first want
                        setName(firstWant.metadata?.name || '');
                        setType(firstWant.metadata?.type || '');
                        setLabels(firstWant.metadata?.labels || {});
                        setParams(firstWant.spec?.params || {});
                        setUsing(firstWant.spec?.using || []);
                        setWhen(firstWant.spec?.when || []);
                        setSelectedTypeId(firstWant.metadata?.type || null);
                      }
                    }}
                  />
                )}
              </div>
            ) : selectedTypeId ? (
              /* Type selected — slot header with Change button */
              (() => {
                const selWt = userFacingWantTypes.find(wt => wt.name === selectedTypeId);
                const selRec = recipes.find(r => r.recipe?.metadata?.custom_type === selectedTypeId);
                const title = selWt?.title || selRec?.recipe?.metadata?.name || selectedTypeId;
                const category = selWt?.category || selRec?.recipe?.metadata?.category;
                return (
                  <div className="flex-shrink-0 flex items-center gap-3 px-3 py-2 rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800/50">
                    <WantSlot
                      id={selectedTypeId}
                      itemType={selectedItemType}
                      category={category}
                      size={56}
                    />
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-semibold text-gray-900 dark:text-white truncate">{title}</p>
                      {category && (
                        <p className="text-xs text-gray-500 dark:text-gray-400 mt-0.5 capitalize">{category}</p>
                      )}
                      <p className="text-[10px] text-gray-400 dark:text-gray-500 mt-0.5">
                        {selectedItemType === 'recipe' ? '📦 Recipe' : '⚡ Want Type'}
                      </p>
                    </div>
                    {!isEditing && (
                      <button
                        type="button"
                        onClick={() => {
                          setSelectedTypeId(null);
                          setSelectedItemType('want-type');
                          setType('');
                          setName('');
                        }}
                        className="px-3 py-1.5 text-xs font-medium rounded-md bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-600 transition-colors flex-shrink-0"
                      >
                        Change
                      </button>
                    )}
                  </div>
                );
              })()
            ) : (
              /* No type selected — Minecraft-style inventory picker */
              <div className="flex-1 min-h-0">
                <WantInventoryPicker
                  wantTypes={userFacingWantTypes}
                  recipes={recipes}
                  onSelect={(id, itemType) => {
                    setSelectedTypeId(id);
                    setSelectedItemType(itemType);
                    setType(id);
                    const existingNames = new Set(wants?.map(w => w.metadata?.name) || []);
                    setName(generateUniqueWantName(id, itemType, existingNames, userNameSuffix));
                    setTimeout(() => nameInputRef.current?.focus(), 0);
                  }}
                />
              </div>
            )}

            {/* Show fields only when a want type or recipe is selected */}
            {selectedTypeId && (
              <>
                {/* Parameters Section */}
                <ParametersSection
                  ref={paramsSectionRef}
                  parameters={params}
                  parameterDefinitions={selectedWantType?.parameters}
                  stateDefs={selectedWantType?.state}
                  exposes={exposes}
                  onExposesChange={setExposes}
                  onChange={setParams}
                  isCollapsed={collapsedSections.has('parameters')}
                  onToggleCollapse={() => toggleSection('parameters')}
                  navigationCallbacks={{
                    onNavigateUp: (e) => e && handleArrowKeyNavigation(e),
                    onNavigateDown: (e) => e && handleArrowKeyNavigation(e),
                    onTab: handleFieldTab,
                  }}
                />

                {/* C: Advanced Settings Toggle */}
                <button
                  type="button"
                  onClick={() => setShowAdvanced(v => !v)}
                  className="w-full flex items-center justify-between px-3 py-1.5 text-xs text-gray-500 dark:text-gray-400 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors"
                >
                  <span className="flex items-center gap-1.5">
                    <ChevronDown className={`w-3.5 h-3.5 transition-transform ${showAdvanced ? 'rotate-180' : ''}`} />
                    Advanced (scheduling · labels · dependencies)
                  </span>
                  {(Object.keys(labels).length > 0 || using.length > 0 || when.length > 0) && (
                    <span className="text-blue-500 dark:text-blue-400 font-bold text-base leading-none">·</span>
                  )}
                </button>

                {showAdvanced && (
                  <>
                    {/* Scheduling Section */}
                    <SchedulingSection
                      ref={schedulingSectionRef}
                      schedules={when}
                      onChange={setWhen}
                      isCollapsed={collapsedSections.has('scheduling')}
                      onToggleCollapse={() => toggleSection('scheduling')}
                      navigationCallbacks={{
                        onNavigateUp: (e) => e && handleArrowKeyNavigation(e),
                        onNavigateDown: (e) => e && handleArrowKeyNavigation(e),
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
                        onNavigateUp: (e) => e && handleArrowKeyNavigation(e),
                        onNavigateDown: (e) => e && handleArrowKeyNavigation(e),
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
                        onNavigateUp: (e) => e && handleArrowKeyNavigation(e),
                        onNavigateDown: (e) => e && handleArrowKeyNavigation(e),
                        onTab: handleFieldTab,
                      }}
                    />
                  </>
                )}

                {/* Owner Section */}
                {ownerWant && (
                  <div className="flex items-center gap-2 px-3 py-2 rounded-md bg-amber-50 dark:bg-amber-900/20">
                    <Crown className="w-3.5 h-3.5 text-amber-500 dark:text-amber-400 flex-shrink-0" />
                    <span className="text-xs font-medium text-amber-700 dark:text-amber-300">Owner</span>
                    <span className="text-xs text-amber-800 dark:text-amber-200 font-mono truncate">{ownerWant.metadata?.name || ownerWant.id}</span>
                  </div>
                )}

                {/* Want Name with Auto-generation (at bottom) */}
                <div className="bg-blue-50 dark:bg-blue-900/20 rounded-lg p-3 sm:p-4">
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1 sm:mb-2">
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
                          handleArrowKeyNavigation(e);
                        }
                      }}
                      className="focusable-section-header w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent dark:bg-gray-800 dark:text-gray-100"
                      placeholder="Auto-generated or enter custom name"
                      required
                    />
                    <div className="text-xs text-gray-600 dark:text-gray-400">
                      <p className="mb-1">💡 Tip: Edit the name or use the selector's suffix option to customize auto-generation</p>
                      {!isValidWantName(name) && name.trim() && (
                        <p className="text-red-600">⚠️ Name contains invalid characters. Use only letters, numbers, hyphens, and underscores.</p>
                      )}
                    </div>
                  </div>
                </div>
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

        {/* Example loader button - bottom right, shown when want type or recipe with examples is selected */}
        {(() => {
          const recipeExamples = selectedItemType === 'recipe'
            ? (recipes.find(r => r.recipe?.metadata?.custom_type === type)?.recipe?.examples ?? [])
            : [];
          const wantTypeExamples = selectedItemType === 'want-type'
            ? (selectedWantType?.examples ?? [])
            : [];
          const hasAnyExamples = recipeExamples.length > 0 || wantTypeExamples.length > 0;

          if (!selectedTypeId || editMode !== 'form' || !hasAnyExamples) return null;

          return (
            <div className="sticky bottom-2 flex justify-end pointer-events-none">
              <div ref={exampleMenuRef} className="relative pointer-events-auto">
                {showExampleMenu && (
                  <div className="absolute bottom-full right-0 mb-2 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-lg z-50 min-w-[220px] max-h-64 overflow-y-auto">
                    <p className="px-3 py-1.5 text-xs font-medium text-gray-500 dark:text-gray-400 border-b border-gray-100 dark:border-gray-700 sticky top-0 bg-white dark:bg-gray-800">
                      Load Example
                    </p>
                    {wantTypeExamples.map((example, i) => (
                      <button
                        key={`wt-${i}`}
                        type="button"
                        onClick={() => {
                          setParams(example.want?.spec?.params || {});
                          setExposes(example.want?.spec?.exposes || []);
                          setShowExampleMenu(false);
                        }}
                        className="w-full text-left px-3 py-2 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
                      >
                        <p className="text-sm font-medium text-gray-700 dark:text-gray-200">{example.name}</p>
                        {example.description && (
                          <p className="text-xs text-gray-500 dark:text-gray-400 truncate mt-0.5">{example.description}</p>
                        )}
                      </button>
                    ))}
                    {recipeExamples.map((example, i) => (
                      <button
                        key={`re-${i}`}
                        type="button"
                        onClick={() => {
                          setParams(prev => ({ ...prev, ...example.params }));
                          setShowExampleMenu(false);
                        }}
                        className="w-full text-left px-3 py-2 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
                      >
                        <p className="text-sm font-medium text-gray-700 dark:text-gray-200">{example.name}</p>
                        {example.description && (
                          <p className="text-xs text-gray-500 dark:text-gray-400 truncate mt-0.5">{example.description}</p>
                        )}
                      </button>
                    ))}
                  </div>
                )}
                <button
                  type="button"
                  onClick={() => setShowExampleMenu(v => !v)}
                  className="p-2 rounded-full bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 shadow-md text-gray-500 dark:text-gray-400 hover:text-blue-600 dark:hover:text-blue-400 hover:border-blue-300 dark:hover:border-blue-600 transition-colors"
                  title="Load example"
                >
                  <FolderOpen className="w-4 h-4" />
                </button>
              </div>
            </div>
          );
        })()}
      </form>
    </RightSidebar>
  );
};