import React, { useState, useEffect, useRef, useMemo, useCallback, forwardRef, useImperativeHandle } from 'react';
import { Save, Plus, Heart, Bot, FolderOpen, Crown } from 'lucide-react';
import { Want, CreateWantRequest, UpdateWantRequest, WhenSpec } from '@/types/want';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { ErrorDisplay } from '@/components/common/ErrorDisplay';
import { FormYamlToggle } from '@/components/common/FormYamlToggle';
import { RightSidebar } from '@/components/layout/RightSidebar';
import { YamlEditor } from './YamlEditor';
import { LabelAutocomplete } from './LabelAutocomplete';
import { LabelSelectorAutocomplete } from './LabelSelectorAutocomplete';
import { WantInventoryPicker, WantInventoryPickerRef, WantSlot } from './WantInventoryPicker';
import { RecommendationSelector } from '@/components/interact/RecommendationSelector';
import { LabelsSection } from './sections/LabelsSection';
import { DependenciesSection } from './sections/DependenciesSection';
import { SchedulingSection } from './sections/SchedulingSection';
import { validateYaml, stringifyYaml } from '@/utils/yaml';
import { generateUniqueWantName, isValidWantName } from '@/utils/nameGenerator';
import { classNames } from '@/utils/helpers';
import { getBackgroundStyle } from '@/utils/backgroundStyles';
import { useWantStore } from '@/stores/wantStore';
import { useWantTypeStore } from '@/stores/wantTypeStore';
import { useRecipeStore } from '@/stores/recipeStore';
import { ApiError } from '@/types/api';
import { Recommendation, ConfigModifications } from '@/types/interact';
import { useInputActions } from '@/hooks/useInputActions';
import { ParameterGridSection } from './sections/ParameterGridSection';
import { ExposeSection, ExposeEntry } from './sections/ExposeSection';
import { useConfigStore } from '@/stores/configStore';
import { FormTab, FormTabBar, FORM_TABS } from './FormTabBar';
import { ParameterDef } from '@/types/wantType';

type WFTab = FormTab | 'add';
const WF_TABS: WFTab[] = [...FORM_TABS, 'add'];

export interface WantFormHandle {
  navigateInventory: (dir: 'up' | 'down' | 'left' | 'right') => void;
  confirmInventory: () => void;
}

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
  /** Authoritative form situation owned by Dashboard — drives input routing */
  formSituation?: 'closed' | 'type-selection' | 'fields' | 'select-mode' | 'batch-action';
  /** Called when WantForm transitions between phases (type selected / back) */
  onSituationChange?: (sit: 'type-selection' | 'fields') => void;
}


export const WantForm = forwardRef<WantFormHandle, WantFormProps>(function WantForm({
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
  onRecommendationDeploy,
  formSituation,
  onSituationChange,
}, ref) {
  const { wants, createWant, updateWant, fetchWants, loading, error } = useWantStore();
  const { wantTypes, selectedWantType, fetchWantTypes, getWantType } = useWantTypeStore();
  const { recipes, fetchRecipes } = useRecipeStore();

  const inventoryPickerRef = useRef<WantInventoryPickerRef>(null);

  // Refs for form fields navigation
  // Expose inventory navigation to Dashboard so it can route arrow keys directly
  // from its own situation-based handler, independent of focus state.
  useImperativeHandle(ref, () => ({
    navigateInventory: (dir) => { inventoryPickerRef.current?.navigate(dir); },
    confirmInventory:  ()    => { inventoryPickerRef.current?.confirmFocused(); },
  }));

  const changeButtonRef = useRef<HTMLButtonElement>(null);
  const nameInputRef = useRef<HTMLInputElement>(null);
  const labelsSectionRef = useRef<HTMLButtonElement>(null);
  const dependenciesSectionRef = useRef<HTMLButtonElement>(null);
  const schedulingSectionRef = useRef<HTMLButtonElement>(null);
  const addButtonRef = useRef<HTMLButtonElement>(null);
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
  const [activeFormTab, setActiveFormTab] = useState<WFTab>('params');
  const [addButtonFocused, setAddButtonFocused] = useState(false);

  // Recommendation mode state
  const [selectedRecId, setSelectedRecId] = useState<string | null>(null);
  const isRecommendationMode = mode === 'recommendation';

  // Cycle through form tabs including the Add button as the final stop
  const navigateTab = useCallback((forward: boolean) => {
    setActiveFormTab(prev => {
      const idx = WF_TABS.indexOf(prev);
      const next = forward
        ? (idx + 1) % WF_TABS.length
        : (idx - 1 + WF_TABS.length) % WF_TABS.length;
      return WF_TABS[next];
    });
  }, []);

  // Form state
  const [name, setName] = useState('');
  const [type, setType] = useState('');
  const [labels, setLabels] = useState<Record<string, string>>({});
  const [params, setParams] = useState<Record<string, unknown>>({});
  const [using, setUsing] = useState<Array<Record<string, string>>>([]);
  const [when, setWhen] = useState<WhenSpec[]>([]);
  const [exposes, setExposes] = useState<ExposeEntry[]>([]);

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
          inventoryPickerRef.current?.focusSearch();
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

      // Auto-switch to Labels tab if editing want has labels; otherwise stay on Params
      const hasLabels = Object.keys(editingWant.metadata?.labels || {}).length > 0;
      if (hasLabels) setActiveFormTab('labels');
      setYamlContent(stringifyYaml({
        metadata: editingWant.metadata,
        spec: editingWant.spec
      }));
    }
  }, [editingWant, recipes]);

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
    setSelectedTypeId(null);
    setSelectedItemType('want-type');
    setUserNameSuffix('');
    setYamlContent(stringifyYaml({
      metadata: { name: '', type: '' },
      spec: {}
    }));
    setValidationError(null);
    setApiError(null);
    setActiveFormTab('params');
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
      // Switch to params tab if type has params
      if (hasParams) setActiveFormTab('params');
    }
  }, [selectedWantType, isEditing, type, selectedItemType]);

  // Populate parameters from recipe definition when a recipe is selected
  useEffect(() => {
    if (selectedItemType === 'recipe' && !isEditing && type) {
      const selectedRecipe = recipes.find(r => r.recipe?.metadata?.custom_type === type);
      if (selectedRecipe && selectedRecipe.recipe?.parameters) {
        setParams(selectedRecipe.recipe.parameters);
        if (Object.keys(selectedRecipe.recipe.parameters).length > 0) setActiveFormTab('params');
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


  // ── Gamepad input for Add Want sidebar ───────────────────────────────────────
  // When Dashboard passes formSituation, use it as the authoritative source.
  // It is set synchronously by Dashboard before WantForm re-renders, so it is
  // never stale.  Fall back to internal computation when prop is absent.
  const isTypeSelectionPhase = formSituation
    ? formSituation === 'type-selection'
    : (isOpen && !selectedTypeId && editMode === 'form');

  // Notify Dashboard when the phase transitions (type selected / back / reset).
  useEffect(() => {
    if (!onSituationChange || !isOpen) return;
    onSituationChange(selectedTypeId ? 'fields' : 'type-selection');
  }, [selectedTypeId, isOpen]); // eslint-disable-line react-hooks/exhaustive-deps

  // Auto-focus the primary element when the active tab changes
  useEffect(() => {
    if (!selectedTypeId) return;
    if (activeFormTab === 'name') {
      setTimeout(() => nameInputRef.current?.focus(), 50);
    } else if (activeFormTab === 'add') {
      setTimeout(() => addButtonRef.current?.focus(), 50);
    }
    // 'params': auto-highlighted by ParameterGridSection (isActive prop drives it)
  }, [activeFormTab, selectedTypeId]);

  // Confirm action: type phase → select focused item; params phase → Enter/click on active element
  const handleGamepadConfirm = useCallback(() => {
    if (isTypeSelectionPhase) {
      inventoryPickerRef.current?.confirmFocused();
    } else {
      const el = document.activeElement as HTMLElement | null;
      if (!el) return;
      if (el.tagName === 'BUTTON' || el.getAttribute('role') === 'button') {
        el.click();
      } else {
        el.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }));
      }
    }
  }, [isTypeSelectionPhase]);

  // Cancel action: params phase → Escape on active input or close form; type phase → close form
  const handleGamepadCancel = useCallback(() => {
    const el = document.activeElement as HTMLElement | null;
    if (el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA')) {
      el.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }));
    } else {
      onClose();
    }
  }, [onClose]);

  // Dashboard owns arrow-key routing during type-selection phase via wantFormRef.
  // Fields phase: L/R Bumper / Tab cycles form tabs; confirm/cancel as before.
  useInputActions({
    enabled: isOpen && editMode === 'form',
    captureInput: !isTypeSelectionPhase,
    ignoreWhenInputFocused: false,
    ignoreWhenInSidebar: false,
    onConfirm: handleGamepadConfirm,
    onCancel: handleGamepadCancel,
    onTabForward:  !isTypeSelectionPhase ? () => navigateTab(true)  : undefined,
    onTabBackward: !isTypeSelectionPhase ? () => navigateTab(false) : undefined,
  });

  // Tab badge counts — shown on each tab button
  const tabBadges = useMemo((): Record<FormTab, string | number | null> => {
    const unfilledRequired = selectedWantType?.parameters
      ? selectedWantType.parameters.filter(p => p.required && params[p.name] === undefined).length
      : 0;
    return {
      name: (!name.trim() && !!selectedTypeId) ? '!' : null,
      params: unfilledRequired > 0 ? `★${unfilledRequired}` : null,
      labels: Object.keys(labels).length > 0 ? Object.keys(labels).length : null,
      schedule: when.length > 0 ? when.length : null,
      deps: using.length > 0 ? using.length : null,
      expose: exposes.length > 0 ? exposes.length : null,
    };
  }, [selectedWantType, params, name, selectedTypeId, labels, when, using]);

  const isTypeSelected = !!type;
  const shouldGlowButton = isTypeSelected && !isEditing && selectedTypeId;
  const config = useConfigStore(state => state.config);
  const isBottom = config?.header_position === 'bottom';

  const allExamples = useMemo(() => {
    const recipeExamples = selectedItemType === 'recipe'
      ? (recipes.find(r => r.recipe?.metadata?.custom_type === type)?.recipe?.examples ?? [])
      : [];
    const wantTypeExamples = selectedItemType === 'want-type'
      ? (selectedWantType?.examples ?? [])
      : [];
    return [
      ...wantTypeExamples.map((ex, i) => ({
        key: `wt-${i}`, name: ex.name, description: ex.description,
        onLoad: () => { setParams(ex.want?.spec?.params || {}); setExposes(ex.want?.spec?.exposes || []); },
      })),
      ...recipeExamples.map((ex, i) => ({
        key: `re-${i}`, name: ex.name, description: ex.description,
        onLoad: () => setParams(prev => ({ ...prev, ...ex.params })),
      })),
    ];
  }, [selectedItemType, type, recipes, selectedWantType]);

  // Synthesize ParameterDef[] from recipe parameters so the grid card format is used.
  const recipeParamDefs = useMemo((): ParameterDef[] | undefined => {
    if (selectedItemType !== 'recipe') return undefined;
    const selectedRecipe = recipes.find(r => r.recipe?.metadata?.custom_type === type);
    if (!selectedRecipe?.recipe?.parameters) return undefined;
    const descs = selectedRecipe.recipe.parameter_descriptions ?? {};
    const types = selectedRecipe.recipe.parameter_types ?? {};
    return Object.entries(selectedRecipe.recipe.parameters).map(([name, defaultValue]) => {
      let paramType = types[name] ?? 'string';
      if (!types[name]) {
        if (Array.isArray(defaultValue)) paramType = 'array';
        else if (typeof defaultValue === 'boolean') paramType = 'bool';
        else if (typeof defaultValue === 'number') paramType = Number.isInteger(defaultValue) ? 'int' : 'float64';
      }
      return {
        name,
        type: paramType,
        description: descs[name] ?? '',
        required: false,
        default: defaultValue,
        example: defaultValue,
      } satisfies ParameterDef;
    });
  }, [selectedItemType, type, recipes]);

  const headerAction = (
    <div className="flex items-stretch gap-0">
      {!isEditing && !!selectedTypeId && allExamples.length > 0 && (
        <div ref={exampleMenuRef} className="relative flex items-stretch">
          <button
            type="button"
            tabIndex={-1}
            onClick={() => setShowExampleMenu(v => !v)}
            className={classNames(
              "flex flex-col items-center justify-center gap-0.5 px-3 h-full transition-all duration-150 focus:outline-none",
              showExampleMenu
                ? "text-blue-600 dark:text-blue-400 bg-blue-50 dark:bg-blue-900/20"
                : "text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-white hover:bg-gray-100 dark:hover:bg-gray-800"
            )}
            title="Load example"
          >
            <FolderOpen className="w-4 h-4" />
            <span className="text-[9px] font-bold uppercase tracking-tighter hidden sm:block">Example</span>
          </button>
          {showExampleMenu && (
            <div className="absolute bottom-full right-0 mb-1 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-lg z-50 min-w-48 max-h-48 overflow-y-auto">
              {allExamples.map(ex => (
                <button
                  key={ex.key}
                  type="button"
                  onClick={() => { ex.onLoad(); setShowExampleMenu(false); }}
                  className="w-full text-left px-3 py-2 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
                >
                  <p className="text-sm font-medium text-gray-700 dark:text-gray-200">{ex.name}</p>
                  {ex.description && (
                    <p className="text-xs text-gray-500 dark:text-gray-400 truncate mt-0.5">{ex.description}</p>
                  )}
                </button>
              ))}
            </div>
          )}
        </div>
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

        onFocus={() => setAddButtonFocused(true)}
        onBlur={() => setAddButtonFocused(false)}
        className={classNames(
          "sidebar-focus-ring flex flex-col items-center justify-center gap-0.5 px-4 h-full transition-all duration-150 flex-shrink-0",
          isSubmitting || (!isEditing && !isTypeSelected)
            ? "bg-gray-400/30 cursor-not-allowed grayscale opacity-50"
            : isEditing
              ? "bg-indigo-600/90 text-white hover:brightness-110 active:opacity-80"
              : isRecommendationMode
                ? "bg-purple-600/90 text-white hover:brightness-110 active:opacity-80"
                : addButtonFocused
                  ? "bg-blue-500 text-white active:opacity-80"
                  : "bg-gray-700 text-white hover:bg-gray-600 active:opacity-80"
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
            {/* ── Phase 1: Type/Recipe selector or Recommendation selector ── */}
            {isRecommendationMode ? (
              <div className={classNames(
                selectedRecId && selectedRecommendation ? 'flex-shrink-0' : 'flex-1 min-h-0 flex flex-col'
              )}>
                {selectedRecId && selectedRecommendation ? (
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
                    <span className="px-4 py-2 text-sm font-medium rounded-lg bg-blue-100 text-blue-700 transition-colors">Change</span>
                  </button>
                ) : (
                  <RecommendationSelector
                    recommendations={recommendations}
                    selectedId={selectedRecId}
                    onSelect={(rec) => {
                      setSelectedRecId(rec.id);
                      onRecommendationSelect?.(rec);
                      if (rec.config?.wants?.length) {
                        const fw = rec.config.wants[0];
                        setName(fw.metadata?.name || '');
                        setType(fw.metadata?.type || '');
                        setLabels(fw.metadata?.labels || {});
                        setParams(fw.spec?.params || {});
                        setUsing(fw.spec?.using || []);
                        setWhen(fw.spec?.when || []);
                        setSelectedTypeId(fw.metadata?.type || null);
                      }
                    }}
                  />
                )}
              </div>
            ) : !selectedTypeId ? (
              /* Inventory picker (no type selected yet) */
              <div className="flex-1 min-h-0">
                <WantInventoryPicker
                  ref={inventoryPickerRef}
                  wantTypes={userFacingWantTypes}
                  recipes={recipes}
                  onSelect={(id, itemType) => {
                    setSelectedTypeId(id);
                    setSelectedItemType(itemType);
                    setType(id);
                    const existingNames = new Set(wants?.map(w => w.metadata?.name) || []);
                    setName(generateUniqueWantName(id, itemType, existingNames, userNameSuffix));
                    setActiveFormTab('params');
                  }}
                />
              </div>
            ) : (
              /* ── Phase 2: Type selected — compact slot + tab layout ── */
              (() => {
                const selWt = userFacingWantTypes.find(wt => wt.name === selectedTypeId);
                const selRec = recipes.find(r => r.recipe?.metadata?.custom_type === selectedTypeId);
                const slotTitle = selWt?.title || selRec?.recipe?.metadata?.name || selectedTypeId;
                const slotCategory = selWt?.category || selRec?.recipe?.metadata?.category;
                const noopNav = { onNavigateUp: () => {}, onNavigateDown: () => {} };
                const tabNav = {
                  ...noopNav,
                  onTab: () => navigateTab(true),
                  onTabBack: () => navigateTab(false),
                };

                return (
                  <>
                    {/* Compact type slot header */}
                    <div className="flex-shrink-0 flex items-center gap-2 px-2 py-1.5 rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800/50">
                      <WantSlot
                        id={selectedTypeId}
                        itemType={selectedItemType}
                        category={slotCategory}
                        size={40}
                      />
                      <div className="flex-1 min-w-0">
                        <p className="text-xs font-semibold text-gray-900 dark:text-white truncate">{slotTitle}</p>
                        {slotCategory && (
                          <p className="text-[10px] text-gray-400 dark:text-gray-500 capitalize">{slotCategory}</p>
                        )}
                      </div>
                      {!isEditing && (
                        <button
                          ref={changeButtonRef}
                          type="button"
                          onClick={() => {
                            setSelectedTypeId(null);
                            setSelectedItemType('want-type');
                            setType('');
                            setName('');
                          }}
                          className="sidebar-focus-ring px-2 py-1 text-[10px] font-medium rounded-md bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 text-gray-500 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-600 transition-colors flex-shrink-0"
                        >
                          Change
                        </button>
                      )}
                    </div>

                    {/* ── Tab bar — order-2 when isBottom so content floats above ── */}
                    <div className={isBottom ? 'order-2' : ''}>
                      <FormTabBar
                        activeTab={activeFormTab}
                        onTabChange={(tab) => setActiveFormTab(tab)}
                        badges={tabBadges}
                        isBottom={isBottom}
                      />
                    </div>

                    {/* ── Tab content (scrollable) — order-1 when isBottom ── */}
                    <div className={classNames(
                      'flex-1 min-h-0 overflow-y-auto space-y-3 pr-0.5',
                      isBottom ? 'order-1' : ''
                    )}>

                      {/* NAME tab */}
                      {activeFormTab === 'name' && (
                        <div className="space-y-3 pt-1">
                          <div>
                            <label className="block text-xs font-medium text-gray-600 dark:text-gray-400 mb-1.5">
                              Want Name <span className="text-red-500">*</span>
                            </label>
                            <input
                              ref={nameInputRef}
                              type="text"
                              value={name}
                              onChange={(e) => setName(e.target.value)}
                              className="sidebar-focus-ring w-full px-3 py-2 text-sm border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 dark:text-gray-100 focus:border-blue-400 dark:focus:border-blue-500 transition-colors"
                              placeholder="Auto-generated or enter custom name"
                              required
                            />
                            {!isValidWantName(name) && name.trim() && (
                              <p className="mt-1 text-xs text-red-600">
                                Invalid characters — use only letters, numbers, hyphens, underscores.
                              </p>
                            )}
                          </div>

                          {/* Owner */}
                          {ownerWant && (
                            <div className="flex items-center gap-2 px-3 py-2 rounded-md bg-amber-50 dark:bg-amber-900/20">
                              <Crown className="w-3.5 h-3.5 text-amber-500 flex-shrink-0" />
                              <span className="text-xs font-medium text-amber-700 dark:text-amber-300">Owner</span>
                              <span className="text-xs text-amber-800 dark:text-amber-200 font-mono truncate">
                                {ownerWant.metadata?.name || ownerWant.id}
                              </span>
                            </div>
                          )}

                        </div>
                      )}

                      {/* PARAMS tab — RPG-style grid cards */}
                      {activeFormTab === 'params' && (
                        <div className="pt-1">
                          <ParameterGridSection
                            parameters={params}
                            parameterDefinitions={recipeParamDefs ?? selectedWantType?.parameters}
                            stateDefs={selectedWantType?.state}
                            onChange={setParams}
                            isActive={activeFormTab === 'params'}
                            onTabForward={() => navigateTab(true)}
                            onTabBackward={() => navigateTab(false)}
                          />
                        </div>
                      )}

                      {/* LABELS tab */}
                      {activeFormTab === 'labels' && (
                        <LabelsSection
                          ref={labelsSectionRef}
                          labels={labels}
                          onChange={setLabels}
                          isCollapsed={false}
                          onToggleCollapse={() => {}}
                          navigationCallbacks={tabNav}
                          hideHeader={true}
                        />
                      )}

                      {/* SCHEDULE tab */}
                      {activeFormTab === 'schedule' && (
                        <SchedulingSection
                          ref={schedulingSectionRef}
                          schedules={when}
                          onChange={setWhen}
                          isCollapsed={false}
                          onToggleCollapse={() => {}}
                          navigationCallbacks={tabNav}
                          hideHeader={true}
                        />
                      )}

                      {/* DEPS tab */}
                      {activeFormTab === 'deps' && (
                        <DependenciesSection
                          ref={dependenciesSectionRef}
                          dependencies={using}
                          onChange={setUsing}
                          isCollapsed={false}
                          onToggleCollapse={() => {}}
                          navigationCallbacks={tabNav}
                          hideHeader={true}
                        />
                      )}

                      {/* EXPOSE tab */}
                      {activeFormTab === 'expose' && (
                        <ExposeSection
                          exposes={exposes}
                          onExposesChange={setExposes}
                          stateDefs={selectedWantType?.state}
                        />
                      )}
                    </div>
                  </>
                );
              })()
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
});