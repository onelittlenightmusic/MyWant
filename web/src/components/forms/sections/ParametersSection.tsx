import React, { useState, useEffect, useCallback, useRef, forwardRef } from 'react';
import { Settings, ChevronDown, ChevronRight, Plus, X, Link } from 'lucide-react';
import { SectionNavigationCallbacks } from '@/types/formSection';
import { CommitInput, CommitInputHandle } from '@/components/common/CommitInput';
import { SelectInput, SelectInputHandle } from '@/components/common/SelectInput';
import { ParameterDef, StateDef } from '@/types/wantType';
import { apiClient } from '@/api/client';
import { useInputActions } from '@/hooks/useInputActions';

interface ExposeEntry {
  currentState?: string;
  param?: string;
  as?: string;
}

interface ParametersSectionProps {
  parameters: Record<string, any>;
  parameterDefinitions?: ParameterDef[];
  stateDefs?: StateDef[];
  exposes?: ExposeEntry[];
  onExposesChange?: (exposes: ExposeEntry[]) => void;
  onChange: (parameters: Record<string, any>) => void;
  isCollapsed: boolean;
  onToggleCollapse: () => void;
  navigationCallbacks: SectionNavigationCallbacks;
  /** When true, hides the collapsible header (for use inside tabs) */
  hideHeader?: boolean;
}

export const ParametersSection = forwardRef<HTMLButtonElement, ParametersSectionProps>(({
  parameters,
  parameterDefinitions,
  stateDefs,
  exposes = [],
  onExposesChange,
  onChange,
  isCollapsed,
  onToggleCollapse,
  navigationCallbacks,
  hideHeader = false,
}, ref) => {
  const [showOptionalParams, setShowOptionalParams] = useState(false);
  const [headerFocused, setHeaderFocused] = useState(false);
  const headerRef = useRef<HTMLButtonElement>(null);
  const firstInputRef = useRef<CommitInputHandle>(null);
  const sectionRef = useRef<HTMLDivElement>(null);
  const wasCollapsedOnMouseDown = useRef<boolean>(false);

  // Refs for each param input (CommitInput or SelectInput)
  const paramInputRefs = useRef<Array<CommitInputHandle | SelectInputHandle | null>>([]);

  // Currently focused param index (-1 = no param focused); drives gamepad navigation
  const [focusedParamIndex, setFocusedParamIndex] = useState(-1);
  // Stable ref so useInputActions callback never goes stale
  const focusedParamIndexRef = useRef(-1);
  focusedParamIndexRef.current = focusedParamIndex;

  // Stable refs for header → param navigation callbacks (avoid stale closures)
  const filteredParamsLengthRef = useRef(0);
  const isCollapsedRef = useRef(isCollapsed);
  isCollapsedRef.current = isCollapsed;
  const onToggleCollapseRef = useRef(onToggleCollapse);
  onToggleCollapseRef.current = onToggleCollapse;

  // Global parameters cache for showing defaultGlobalParameter hints
  const [globalParams, setGlobalParams] = useState<Record<string, unknown>>({});
  // Which params are overriding the global default (key = param name, value = true means override)
  const [globalOverrides, setGlobalOverrides] = useState<Record<string, boolean>>({});

  // Expose add form
  const [addingExpose, setAddingExpose] = useState(false);
  const [newExposeState, setNewExposeState] = useState('');
  const [newExposeAs, setNewExposeAs] = useState('');

  // Helper: focus the param input at given index
  const focusParamAt = useCallback((index: number) => {
    const target = paramInputRefs.current[index];
    if (!target) return;
    if ('focus' in target && typeof target.focus === 'function') target.focus();
  }, []);

  // Tab / Shift+Tab on the section header — routes through useInputActions so
  // gamepad L/R bumpers use the same onTabForward/Backward callbacks.
  useInputActions({
    enabled: headerFocused,
    captureTab: true,
    ignoreWhenInputFocused: false,
    ignoreWhenInSidebar: false,
    onTabForward:  navigationCallbacks.onTab,
    onTabBackward: navigationCallbacks.onTabBack,
  });

  // D-pad/arrow navigation from header into param inputs.
  // Down → first param, Up → last param.
  // captureGamepad so D-pad doesn't simultaneously move want-card focus.
  useInputActions({
    enabled: headerFocused,
    captureGamepad: true,
    ignoreWhenInputFocused: false,
    ignoreWhenInSidebar: false,
    onNavigate: (dir) => {
      const count = filteredParamsLengthRef.current;
      if (count === 0) return;
      if (dir === 'down') {
        if (isCollapsedRef.current) { onToggleCollapseRef.current(); setTimeout(() => focusParamAt(0), 100); }
        else focusParamAt(0);
      } else if (dir === 'up') {
        if (isCollapsedRef.current) { onToggleCollapseRef.current(); setTimeout(() => focusParamAt(count - 1), 100); }
        else focusParamAt(count - 1);
      }
    },
  });

  // Gamepad navigation via common handler (captureGamepad so D-pad doesn't also move want cards)
  useInputActions({
    enabled: focusedParamIndex >= 0,
    captureGamepad: true,
    ignoreWhenInputFocused: false,
    ignoreWhenInSidebar: false,
    onNavigate: (direction) => {
      const current = focusedParamIndexRef.current;
      if (direction === 'up') {
        if (current > 0) {
          focusParamAt(current - 1);
        } else {
          // First param → move to section header
          headerRef.current?.focus();
        }
      } else if (direction === 'down') {
        const next = paramInputRefs.current[current + 1];
        if (next) {
          focusParamAt(current + 1);
        }
        // Last param: let focus stay; section blur handles section-level nav
      }
    },
    onCancel: () => {
      headerRef.current?.focus();
    },
  });

  // Merge forwarded ref with local ref
  const mergedRef = useCallback((node: HTMLButtonElement | null) => {
    headerRef.current = node;
    if (typeof ref === 'function') {
      ref(node);
    } else if (ref) {
      (ref as React.MutableRefObject<HTMLButtonElement | null>).current = node;
    }
  }, [ref]);

  // Fetch global params when any param has defaultGlobalParameter
  useEffect(() => {
    const hasGlobalParam = parameterDefinitions?.some(p => p.defaultGlobalParameter);
    if (!hasGlobalParam) return;
    apiClient.getGlobalParameters().then(res => {
      setGlobalParams(res.parameters ?? {});
    }).catch(() => {});
  }, [parameterDefinitions]);

  // Initialize globalOverrides: if param already has explicit value set, mark as override
  useEffect(() => {
    if (!parameterDefinitions) return;
    setGlobalOverrides(prev => {
      const next = { ...prev };
      for (const p of parameterDefinitions) {
        if (p.defaultGlobalParameter && !(p.name in next)) {
          // If parameter already has an explicit value, start as override mode
          next[p.name] = p.name in parameters;
        }
      }
      return next;
    });
  }, [parameterDefinitions]); // intentionally exclude `parameters` to avoid loop

  const handleClick = useCallback((e: React.MouseEvent) => {
    if (wasCollapsedOnMouseDown.current && !isCollapsed) return;
    onToggleCollapse();
  }, [isCollapsed, onToggleCollapse]);

  const handleMouseDown = useCallback(() => {
    wasCollapsedOnMouseDown.current = isCollapsed;
  }, [isCollapsed]);

  const handleUpdateParam = useCallback((key: string, value: string, paramType: string) => {
    let typedValue: any = value;
    if (value === '') {
      typedValue = undefined;
    } else {
      switch (paramType) {
        case 'float64':
        case 'int':
          typedValue = parseFloat(value);
          if (isNaN(typedValue)) typedValue = undefined;
          break;
        case 'bool':
          typedValue = value.toLowerCase() === 'true';
          break;
      }
    }
    const newParams = { ...parameters };
    if (typedValue === undefined) {
      delete newParams[key];
    } else {
      newParams[key] = typedValue;
    }
    onChange(newParams);
  }, [parameters, onChange]);

  const handleRemoveParam = useCallback((key: string) => {
    const newParams = { ...parameters };
    delete newParams[key];
    onChange(newParams);
  }, [parameters, onChange]);

  const handleAddParam = useCallback((key: string, value: any) => {
    onChange({ ...parameters, [key]: value });
  }, [parameters, onChange]);

  // Toggle whether a param overrides its global default
  const handleToggleGlobalOverride = useCallback((paramName: string, paramDef: ParameterDef) => {
    setGlobalOverrides(prev => {
      const nowOverride = !prev[paramName];
      const next = { ...prev, [paramName]: nowOverride };
      if (!nowOverride) {
        // Switching to "use global param" — remove explicit value
        const newParams = { ...parameters };
        delete newParams[paramName];
        onChange(newParams);
      }
      return next;
    });
  }, [parameters, onChange]);

  const handleHeaderKeyDown = useCallback((e: React.KeyboardEvent<HTMLButtonElement>) => {
    if (e.key === 'ArrowRight' || e.key === 'ArrowDown') {
      e.preventDefault();
      const count = filteredParamsLengthRef.current;
      if (count > 0) {
        if (isCollapsed) onToggleCollapse();
        setTimeout(() => focusParamAt(0), isCollapsed ? 100 : 0);
      }
    } else if (e.key === 'ArrowLeft') {
      e.preventDefault();
      if (!isCollapsed) onToggleCollapse();
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      const count = filteredParamsLengthRef.current;
      if (count > 0) {
        if (isCollapsed) onToggleCollapse();
        setTimeout(() => focusParamAt(count - 1), isCollapsed ? 100 : 0);
      }
    } else if (e.key === 'a' && !e.shiftKey && !e.ctrlKey && !e.metaKey && !e.altKey) {
      e.preventDefault();
    } else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onToggleCollapse();
    }
  }, [isCollapsed, onToggleCollapse, focusParamAt]);

  // Navigate between param inputs with arrow keys (keyboard path; gamepad uses useInputActions above)
  const handleParamInputKeyDown = useCallback((e: React.KeyboardEvent, index: number) => {
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (index > 0) {
        focusParamAt(index - 1);
      } else {
        headerRef.current?.focus();
      }
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (paramInputRefs.current[index + 1]) {
        focusParamAt(index + 1);
      }
    } else if (e.key === 'Escape') {
      e.preventDefault();
      headerRef.current?.focus();
    }
  }, [focusParamAt]);

  const filteredParams = parameterDefinitions?.filter(
    param => param.required || showOptionalParams || (parameters[param.name] !== undefined)
  ) || [];
  filteredParamsLengthRef.current = filteredParams.length;

  const currentStateFields = stateDefs?.filter(s => s.name !== 'final_result') || [];

  // Expose helpers
  const handleRemoveExpose = useCallback((stateKey: string) => {
    if (!onExposesChange) return;
    onExposesChange(exposes.filter(e => e.currentState !== stateKey));
  }, [exposes, onExposesChange]);

  const handleAddExpose = useCallback(() => {
    if (!onExposesChange || !newExposeState || !newExposeAs.trim()) return;
    const next = exposes.filter(e => e.currentState !== newExposeState);
    next.push({ currentState: newExposeState, as: newExposeAs.trim() });
    onExposesChange(next);
    setNewExposeState('');
    setNewExposeAs('');
    setAddingExpose(false);
  }, [exposes, onExposesChange, newExposeState, newExposeAs]);

  return (
    <div ref={sectionRef} className="space-y-2">
      {/* Section Header — hidden when hideHeader=true */}
      {!hideHeader && (
      <button
        ref={mergedRef}
        type="button"
        onClick={handleClick}
        onMouseDown={handleMouseDown}
        onFocus={() => { setHeaderFocused(true); if (isCollapsed) onToggleCollapse(); }}
        onBlur={(e) => {
          setHeaderFocused(false);
          const relatedTarget = e.relatedTarget as Node;
          if (relatedTarget && sectionRef.current?.contains(relatedTarget)) return;
          if (!isCollapsed) onToggleCollapse();
        }}
        onKeyDown={handleHeaderKeyDown}
        className="sidebar-section-btn sidebar-focus-ring focusable-section-header w-full text-left px-3 py-2 rounded-lg transition-all duration-200 relative bg-gray-100 dark:bg-gray-800 hover:bg-gray-200 dark:hover:bg-gray-700"
        aria-expanded={!isCollapsed}
        aria-label="Parameters section"
      >
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            {isCollapsed ? (
              <ChevronRight className="w-4 h-4 text-gray-400 dark:text-gray-500" />
            ) : (
              <ChevronDown className="w-4 h-4 text-gray-400 dark:text-gray-500" />
            )}
            <Settings className="w-3.5 h-3.5 text-blue-500 dark:text-blue-400" />
            <h3 className="text-sm font-medium text-gray-700 dark:text-gray-200">Parameters</h3>
            {Object.keys(parameters).length > 0 && (
              <span className="px-1.5 py-0.5 text-xs rounded-full bg-blue-100 dark:bg-blue-900/30 text-blue-600 dark:text-blue-400">
                {Object.keys(parameters).length}
              </span>
            )}
          </div>
          {isCollapsed && Object.entries(parameters).length > 0 && (
            <div className="flex flex-wrap gap-1.5 ml-4">
              {Object.entries(parameters).slice(0, 3).map(([key, value]) => (
                <span key={key} className="text-xs bg-blue-100 dark:bg-blue-900/30 text-blue-600 dark:text-blue-400 px-2 py-0.5 rounded-full">
                  {key}: {String(value)}
                </span>
              ))}
              {Object.entries(parameters).length > 3 && (
                <span className="text-xs text-gray-400 dark:text-gray-500">+{Object.entries(parameters).length - 3}</span>
              )}
            </div>
          )}
        </div>
      </button>
      )}

      {/* Section Content — always shown when hideHeader=true */}
      {(hideHeader || !isCollapsed) && (
        <div className="pl-2 sm:pl-4 space-y-2 sm:space-y-3">
          {parameterDefinitions && parameterDefinitions.length > 0 ? (
            <>
              <div className="space-y-1 sm:space-y-2">
                {filteredParams.map((param, index) => {
                  const hasEnum = param.validation?.enum && param.validation.enum.length > 0;
                  const hasGlobalParam = !!param.defaultGlobalParameter;
                  const isOverride = globalOverrides[param.name] ?? false;
                  const globalValue = hasGlobalParam ? globalParams[param.defaultGlobalParameter!] : undefined;

                  let inputType = 'text';
                  let inputValue: string | boolean = '';
                  const currentValue = parameters[param.name];
                  if (currentValue !== undefined) {
                    inputValue = String(currentValue);
                  } else if (param.example !== undefined) {
                    inputValue = String(param.example);
                  } else if (param.default !== undefined) {
                    inputValue = String(param.default);
                  }

                  switch (param.type) {
                    case 'float64':
                    case 'int':
                      inputType = 'number';
                      break;
                    case 'bool':
                      inputType = 'checkbox';
                      if (currentValue === undefined && param.example === undefined && param.default === undefined) {
                        inputValue = false;
                      } else {
                        inputValue = Boolean(currentValue ?? param.example ?? param.default);
                      }
                      break;
                  }

                  return (
                    <div key={param.name} className="space-y-0.5 sm:space-y-1">
                      <div className="flex flex-col sm:flex-row gap-1 sm:gap-2 items-start sm:items-center">
                        <div className="w-full sm:w-32 flex-shrink-0">
                          <label className="block text-xs sm:text-sm font-medium text-gray-700 dark:text-gray-200 pt-1 sm:pt-2">
                            {param.name}
                            {param.required && <span className="text-red-600 ml-1">*</span>}
                          </label>
                        </div>

                        {/* Global param toggle */}
                        {hasGlobalParam && (
                          <button
                            type="button"
                            onClick={() => handleToggleGlobalOverride(param.name, param)}
                            title={isOverride ? `Using custom value (click to use global param: ${param.defaultGlobalParameter})` : `Using global param: ${param.defaultGlobalParameter} (click to override)`}
                            className={`flex-shrink-0 flex items-center gap-1 text-xs px-1.5 py-0.5 rounded transition-colors ${
                              isOverride
                                ? 'bg-amber-100 dark:bg-amber-900/30 text-amber-600 dark:text-amber-400 hover:bg-amber-200 dark:hover:bg-amber-900/50'
                                : 'bg-purple-100 dark:bg-purple-900/30 text-purple-600 dark:text-purple-400 hover:bg-purple-200 dark:hover:bg-purple-900/50'
                            }`}
                          >
                            <Link className="w-3 h-3" />
                            {isOverride ? 'custom' : param.defaultGlobalParameter}
                          </button>
                        )}

                        {/* Input: hidden when using global param (not override) */}
                        {(!hasGlobalParam || isOverride) && (
                          <>
                            {hasEnum ? (
                              <SelectInput
                                ref={el => { paramInputRefs.current[index] = el; }}
                                value={String(inputValue)}
                                onChange={val => handleUpdateParam(param.name, val, param.type)}
                                options={(param.validation!.enum! as string[]).map(v => ({ value: String(v) }))}
                                onFocus={() => setFocusedParamIndex(index)}
                                onKeyDown={e => handleParamInputKeyDown(e, index)}
                                onBlur={(e) => {
                                  const relatedTarget = e.relatedTarget as Node;
                                  if (!relatedTarget || !sectionRef.current?.contains(relatedTarget)) {
                                    setFocusedParamIndex(-1);
                                    if (!isCollapsed) onToggleCollapse();
                                  }
                                }}
                                className="flex-1 w-full"
                              />
                            ) : inputType === 'checkbox' ? (
                              <input
                                type="checkbox"
                                checked={inputValue as boolean}
                                onChange={(e) => handleUpdateParam(param.name, String(e.target.checked), param.type)}
                                onFocus={() => setFocusedParamIndex(index)}
                                onBlur={() => setFocusedParamIndex(-1)}
                                className="flex-1 w-full h-5 w-5 text-blue-600 focus:ring-blue-500 border-gray-300 rounded"
                              />
                            ) : (
                              <CommitInput
                                ref={el => {
                                  paramInputRefs.current[index] = el as CommitInputHandle | null;
                                  if (index === 0) (firstInputRef as React.MutableRefObject<CommitInputHandle | null>).current = el;
                                }}
                                type={inputType}
                                value={inputValue as string}
                                onChange={(val) => handleUpdateParam(param.name, val, param.type)}
                                onKeyDown={(e) => handleParamInputKeyDown(e, index)}
                                onFocus={() => setFocusedParamIndex(index)}
                                onBlur={(e) => {
                                  const relatedTarget = e.relatedTarget as Node;
                                  if (!relatedTarget || !sectionRef.current?.contains(relatedTarget)) {
                                    setFocusedParamIndex(-1);
                                    if (!isCollapsed) onToggleCollapse();
                                  }
                                }}
                                className="flex-1 w-full"
                                placeholder={param.description || 'Enter value'}
                                multiline={inputType === 'text'}
                              />
                            )}
                          </>
                        )}

                        {/* Global param value preview (when not overriding) */}
                        {hasGlobalParam && !isOverride && (
                          <span className="flex-1 text-sm text-gray-400 dark:text-gray-500 italic truncate" title={`Global param value: ${String(globalValue ?? '(not set)')}`}>
                            {globalValue !== undefined ? String(globalValue) : <span className="text-gray-300 dark:text-gray-600">(not set)</span>}
                          </span>
                        )}
                      </div>
                      {param.description && (
                        <p className="text-[10px] sm:text-xs text-gray-600 dark:text-gray-400 sm:ml-32">{param.description}</p>
                      )}
                    </div>
                  );
                })}
              </div>

              {parameterDefinitions.some(param => !param.required) && (
                <div className="pt-2">
                  <button
                    type="button"
                    onClick={() => setShowOptionalParams(!showOptionalParams)}
                    className="text-blue-500 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300 text-xs flex items-center gap-1 focus:outline-none rounded px-2 py-1 hover:bg-blue-50 dark:hover:bg-blue-900/20"
                  >
                    <ChevronDown className={`w-4 h-4 transition-transform ${showOptionalParams ? '' : '-rotate-90'}`} />
                    {showOptionalParams ? 'Hide' : 'Show'} Optional Parameters ({parameterDefinitions.filter(p => !p.required).length})
                  </button>
                </div>
              )}
            </>
          ) : Object.keys(parameters).length > 0 ? (
            <div className="space-y-2">
              <p className="text-sm text-gray-600 italic">Recipe parameters (editable)</p>
              {Object.entries(parameters).map(([key, value], index) => (
                <div key={key} className="flex gap-2 items-start">
                  <div className="w-32 flex-shrink-0">
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-200 pt-2">{key}</label>
                  </div>
                  <CommitInput
                    ref={el => {
                      paramInputRefs.current[index] = el as CommitInputHandle | null;
                      if (index === 0) (firstInputRef as React.MutableRefObject<CommitInputHandle | null>).current = el;
                    }}
                    type="text"
                    value={String(value)}
                    onChange={(val) => handleUpdateParam(key, val, 'string')}
                    onKeyDown={(e) => handleParamInputKeyDown(e, index)}
                    onBlur={(e) => {
                      const relatedTarget = e.relatedTarget as Node;
                      if (!relatedTarget || !sectionRef.current?.contains(relatedTarget)) {
                        if (!isCollapsed) onToggleCollapse();
                      }
                    }}
                    className="flex-1"
                    placeholder="Enter value"
                    multiline
                  />
                  <button
                    type="button"
                    onClick={() => handleRemoveParam(key)}
                    className="px-2 py-2 text-red-600 hover:text-red-800 hover:bg-red-50 rounded-md transition-colors focus:outline-none focus:ring-2 focus:ring-red-300"
                    title="Remove parameter"
                  >
                    ×
                  </button>
                </div>
              ))}
            </div>
          ) : (
            <div className="text-sm text-gray-500 italic">No parameters defined for this want type.</div>
          )}

          {/* State Exposures — label-style */}
          {currentStateFields.length > 0 && onExposesChange && (
            <div className="pt-3 border-t border-gray-100 dark:border-gray-700">
              <div className="flex items-center justify-between mb-1.5">
                <span className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wide">Expose</span>
                <button
                  type="button"
                  onClick={() => { setAddingExpose(v => !v); setNewExposeState(''); setNewExposeAs(''); }}
                  className="text-xs flex items-center gap-0.5 text-purple-500 hover:text-purple-700 dark:text-purple-400 dark:hover:text-purple-300 px-1.5 py-0.5 rounded hover:bg-purple-50 dark:hover:bg-purple-900/20 transition-colors"
                >
                  <Plus className="w-3 h-3" />
                  Add
                </button>
              </div>

              {/* Existing exposes as label chips */}
              {exposes.length > 0 && (
                <div className="flex flex-wrap gap-1.5 mb-2">
                  {exposes.filter(e => e.currentState && e.as).map(e => (
                    <span
                      key={e.currentState}
                      className="inline-flex items-center gap-1 text-xs bg-purple-50 dark:bg-purple-900/20 text-purple-700 dark:text-purple-300 border border-purple-200 dark:border-purple-700 px-2 py-0.5 rounded-full"
                    >
                      <span className="font-mono">{e.currentState}</span>
                      <span className="text-purple-400 dark:text-purple-500">→</span>
                      <span className="font-mono">{e.as}</span>
                      <button
                        type="button"
                        onClick={() => handleRemoveExpose(e.currentState!)}
                        className="ml-0.5 text-purple-400 hover:text-purple-600 dark:hover:text-purple-200 transition-colors"
                        title="Remove expose"
                      >
                        <X className="w-2.5 h-2.5" />
                      </button>
                    </span>
                  ))}
                </div>
              )}

              {/* Add expose form */}
              {addingExpose && (
                <div className="flex items-center gap-2 mt-1">
                  <SelectInput
                    value={newExposeState}
                    onChange={setNewExposeState}
                    options={[
                      { value: '', label: 'state field' },
                      ...currentStateFields
                        .filter(s => !exposes.some(e => e.currentState === s.name))
                        .map(s => ({ value: s.name })),
                    ]}
                    className="flex-1"
                  />
                  <span className="text-xs text-gray-400">→</span>
                  <input
                    type="text"
                    value={newExposeAs}
                    onChange={e => setNewExposeAs(e.target.value)}
                    onKeyDown={e => {
                      if (e.key === 'Enter') { e.preventDefault(); handleAddExpose(); }
                      if (e.key === 'Escape') { e.preventDefault(); setAddingExpose(false); }
                    }}
                    placeholder="global key"
                    className="text-xs px-2 py-1 rounded-md border border-purple-200 dark:border-purple-700 bg-white dark:bg-gray-800 text-gray-700 dark:text-gray-200 placeholder-gray-300 dark:placeholder-gray-600 focus:outline-none focus:ring-1 focus:ring-purple-400 flex-1"
                  />
                  <button
                    type="button"
                    onClick={handleAddExpose}
                    disabled={!newExposeState || !newExposeAs.trim()}
                    className="text-xs px-2 py-1 rounded-md bg-purple-500 text-white hover:bg-purple-600 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                  >
                    Add
                  </button>
                </div>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
});

ParametersSection.displayName = 'ParametersSection';
