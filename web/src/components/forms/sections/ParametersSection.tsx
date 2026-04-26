import React, { useState, useEffect, useCallback, useRef, forwardRef } from 'react';
import { Settings, ChevronDown, ChevronRight, Share2 } from 'lucide-react';
import { SectionNavigationCallbacks } from '@/types/formSection';
import { CommitInput, CommitInputHandle } from '@/components/common/CommitInput';

import { ParameterDef, StateDef } from '@/types/wantType';

interface ExposeEntry {
  currentState?: string;
  param?: string;
  as?: string;
}

/**
 * Props for ParametersSection
 */
interface ParametersSectionProps {
  /** Current parameter values */
  parameters: Record<string, any>;
  /** Parameter definitions from want type (if available) */
  parameterDefinitions?: ParameterDef[];
  /** State definitions from want type (for expose configuration) */
  stateDefs?: StateDef[];
  /** Current expose entries */
  exposes?: ExposeEntry[];
  /** Callback when exposes change */
  onExposesChange?: (exposes: ExposeEntry[]) => void;
  /** Callback when parameters change */
  onChange: (parameters: Record<string, any>) => void;
  /** Whether the section is collapsed */
  isCollapsed: boolean;
  /** Callback to toggle collapsed state */
  onToggleCollapse: () => void;
  /** Navigation callbacks for moving between sections */
  navigationCallbacks: SectionNavigationCallbacks;
}

/**
 * ParametersSection - Section for managing want parameters
 *
 * Unlike other sections, this doesn't use chips. Parameters are auto-populated
 * from want type definitions and displayed as inline inputs.
 */
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
}, ref) => {
  const [showOptionalParams, setShowOptionalParams] = useState(false);
  const headerRef = useRef<HTMLButtonElement>(null);
  const firstInputRef = useRef<CommitInputHandle>(null);
  const sectionRef = useRef<HTMLDivElement>(null);
  const wasCollapsedOnMouseDown = useRef<boolean>(false);

  // Merge forwarded ref with local ref
  const mergedRef = useCallback((node: HTMLButtonElement | null) => {
    headerRef.current = node;
    if (typeof ref === 'function') {
      ref(node);
    } else if (ref) {
      (ref as React.MutableRefObject<HTMLButtonElement | null>).current = node;
    }
  }, [ref]);

  /**
   * Handle Click - only toggle if it wasn't just expanded by focus
   */
  const handleClick = useCallback((e: React.MouseEvent) => {
    if (wasCollapsedOnMouseDown.current && !isCollapsed) {
      return;
    }
    onToggleCollapse();
  }, [isCollapsed, onToggleCollapse]);

  /**
   * Handle Mouse Down - capture state before focus fires
   */
  const handleMouseDown = useCallback(() => {
    wasCollapsedOnMouseDown.current = isCollapsed;
  }, [isCollapsed]);

  /**
   * Update a parameter value, converting type if necessary
   */
  const handleUpdateParam = useCallback((key: string, value: string, paramType: string) => {
    let typedValue: any = value;
    if (value === '') { // Treat empty string as undefined for optional params, or default
      typedValue = undefined;
    } else {
      switch (paramType) {
        case 'float64':
        case 'int':
          typedValue = parseFloat(value);
          if (isNaN(typedValue)) typedValue = undefined; // Set to undefined if invalid number
          break;
        case 'bool':
          typedValue = value.toLowerCase() === 'true';
          break;
        // Add other types as needed
      }
    }

    const newParams = { ...parameters };
    if (typedValue === undefined) {
      delete newParams[key]; // Remove if value is empty/invalid for optional params
    } else {
      newParams[key] = typedValue;
    }
    onChange(newParams);
  }, [parameters, onChange]);

  /**
   * Remove a parameter (for recipes without definitions)
   */
  const handleRemoveParam = useCallback((key: string) => {
    const newParams = { ...parameters };
    delete newParams[key];
    onChange(newParams);
  }, [parameters, onChange]);

  /**
   * Add a new parameter (for recipes without definitions)
   */
  const handleAddParam = useCallback((key: string, value: any) => {
    onChange({
      ...parameters,
      [key]: value
    });
  }, [parameters, onChange]);

  /**
   * Handle header keyboard navigation
   */
  const handleHeaderKeyDown = useCallback((e: React.KeyboardEvent<HTMLButtonElement>) => {
    // Right arrow - expand and focus first input (if exists)
    if (e.key === 'ArrowRight') {
      e.preventDefault();

      // Expand if collapsed
      if (isCollapsed) {
        onToggleCollapse();
      }

      // Focus first input if exists (after expansion animation)
      setTimeout(() => {
        firstInputRef.current?.focus();
      }, 100);
    }
    // Left arrow - collapse section
    else if (e.key === 'ArrowLeft') {
      e.preventDefault();
      if (!isCollapsed) {
        onToggleCollapse();
      }
    }
    // 'a' key - do nothing for parameters (they're auto-populated)
    else if (e.key === 'a' && !e.shiftKey && !e.ctrlKey && !e.metaKey && !e.altKey) {
      e.preventDefault();
      // Parameters are auto-populated from want type, so 'a' does nothing
    }
    // Up arrow - navigate to previous section
    else if (e.key === 'ArrowUp') {
      e.preventDefault();
      navigationCallbacks.onNavigateUp(e);
    }
    // Down arrow - navigate to next section
    else if (e.key === 'ArrowDown') {
      e.preventDefault();
      navigationCallbacks.onNavigateDown(e);
    }
    // Enter/Space - toggle collapse
    else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onToggleCollapse();
    }
    // Tab - custom navigation (e.g. to Add button)
    else if (e.key === 'Tab' && navigationCallbacks.onTab) {
      e.preventDefault();
      navigationCallbacks.onTab();
    }
  }, [isCollapsed, onToggleCollapse, navigationCallbacks]);

  /**
   * Get filtered parameters based on optional params toggle
   */
  const filteredParams = parameterDefinitions?.filter(
    param => param.required || showOptionalParams || (parameters[param.name] !== undefined) // Always show if already has a value
  ) || [];

  const [expandedExposeFields, setExpandedExposeFields] = useState<Set<string>>(
    () => new Set(exposes.filter(e => e.currentState && e.as).map(e => e.currentState!))
  );

  // Sync expanded fields when exposes are set from outside (e.g. loading an example)
  useEffect(() => {
    setExpandedExposeFields(prev => {
      const next = new Set(prev);
      exposes.forEach(e => {
        if (e.currentState && e.as) next.add(e.currentState);
      });
      return next;
    });
  }, [exposes]);

  /** Toggle expose input visibility for a state field */
  const toggleExposeField = useCallback((stateKey: string) => {
    setExpandedExposeFields(prev => {
      const next = new Set(prev);
      if (next.has(stateKey)) {
        next.delete(stateKey);
        // Clear the expose entry when collapsing
        if (onExposesChange) {
          onExposesChange(exposes.filter(e => e.currentState !== stateKey));
        }
      } else {
        next.add(stateKey);
      }
      return next;
    });
  }, [exposes, onExposesChange]);

  /** Update expose-as key for a state field */
  const handleExposeAsChange = useCallback((stateKey: string, asValue: string) => {
    if (!onExposesChange) return;
    const next = exposes.filter(e => e.currentState !== stateKey);
    if (asValue.trim()) {
      next.push({ currentState: stateKey, as: asValue.trim() });
    }
    onExposesChange(next);
  }, [exposes, onExposesChange]);

  const currentStateFields = stateDefs?.filter(s => s.name !== 'final_result') || [];

  return (
    <div ref={sectionRef} className="space-y-2">
      {/* Section Header */}
      <button
        ref={mergedRef}
        type="button"
        onClick={handleClick}
        onMouseDown={handleMouseDown}
        onFocus={() => {
          // ヘッダーにフォーカスが当たった時、折りたたまれていれば自動的に展開
          if (isCollapsed) {
            onToggleCollapse();
          }
        }}
        onBlur={(e) => {
          // フォーカスがセクション内の要素に移った場合は閉じない
          const relatedTarget = e.relatedTarget as Node;
          if (relatedTarget && sectionRef.current?.contains(relatedTarget)) {
            return;
          }
          // ヘッダーからフォーカスが外れた時、展開されていれば自動的に折りたたむ
          if (!isCollapsed) {
            onToggleCollapse();
          }
        }}
        onKeyDown={handleHeaderKeyDown}
        className={`
          focusable-section-header
          w-full text-left px-3 py-2 rounded-lg
          transition-all duration-200 focus:outline-none
          relative bg-blue-50 dark:bg-blue-900/20 hover:bg-blue-100 dark:hover:bg-blue-900/30

          before:absolute before:left-0 before:top-0
          before:bottom-0 before:w-1 before:rounded-l-md
          before:opacity-0 before:transition-opacity
          focus:before:opacity-100 before:bg-blue-500
        `}
        aria-expanded={!isCollapsed}
        aria-label="Parameters section - Press Right to focus inputs, Up/Down to navigate sections"
      >
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            {isCollapsed ? (
              <ChevronRight className="w-4 h-4 text-gray-400 dark:text-gray-500" />
            ) : (
              <ChevronDown className="w-4 h-4 text-gray-400 dark:text-gray-500" />
            )}
            <Settings className="w-3.5 h-3.5 text-blue-500 dark:text-blue-400" />
            <h3 className="text-sm font-medium text-gray-700 dark:text-gray-200">
              Parameters
            </h3>
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
                <span className="text-xs text-gray-400 dark:text-gray-500">
                  +{Object.entries(parameters).length - 3}
                </span>
              )}
            </div>
          )}
        </div>
      </button>

      {/* Section Content (Expanded) */}
      {!isCollapsed && (
        <div className="pl-2 sm:pl-4 space-y-2 sm:space-y-3">
          {parameterDefinitions && parameterDefinitions.length > 0 ? (
            <>
              {/* Parameters from want type definition */}
              <div className="space-y-1 sm:space-y-2">
                {filteredParams.map((param, index) => {
                  // Determine input type based on param.type
                  let inputType: string = 'text';
                  let inputValue: string | boolean = '';

                  // Get current value, falling back to example, then default
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
                      // For boolean, default to false if no value present
                      if (currentValue === undefined && param.example === undefined && param.default === undefined) {
                          inputValue = false;
                      } else {
                          inputValue = Boolean(currentValue ?? param.example ?? param.default);
                      }
                      break;
                    // Add other types as needed
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
                        {inputType === 'checkbox' ? (
                           <input
                            type="checkbox"
                            checked={inputValue as boolean}
                            onChange={(e) => handleUpdateParam(param.name, String(e.target.checked), param.type)}
                            className="flex-1 w-full h-5 w-5 text-blue-600 focus:ring-blue-500 border-gray-300 rounded"
                          />
                        ) : (
                          <CommitInput
                            ref={index === 0 ? firstInputRef : undefined}
                            type={inputType}
                            value={inputValue as string}
                            onChange={(val) => handleUpdateParam(param.name, val, param.type)}
                            onKeyDown={(e) => {
                              if (e.key === 'Escape') {
                                e.preventDefault();
                                headerRef.current?.focus();
                              }
                            }}
                            onBlur={(e) => {
                              // フォーカスがセクション外に移った場合のみセクションを閉じる
                              const relatedTarget = e.relatedTarget as Node;
                              if (!relatedTarget || !sectionRef.current?.contains(relatedTarget)) {
                                if (!isCollapsed) {
                                  onToggleCollapse();
                                }
                              }
                            }}
                            className="flex-1 w-full"
                            placeholder={param.description || 'Enter value'}
                            multiline={inputType === 'text'}
                          />
                        )}
                      </div>
                      {param.description && (
                        <p className="text-[10px] sm:text-xs text-gray-600 dark:text-gray-400 sm:ml-32">{param.description}</p>
                      )}
                    </div>
                  );
                })}
              </div>

              {/* Show Optional Parameters Toggle */}
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
            <>
              {/* Parameters from recipe (no definitions) - show as editable key-value pairs */}
              <div className="space-y-2">
                <p className="text-sm text-gray-600 italic">
                  Recipe parameters (editable)
                </p>
                {Object.entries(parameters).map(([key, value], index) => (
                  <div key={key} className="flex gap-2 items-start">
                    <div className="w-32 flex-shrink-0">
                      <label className="block text-sm font-medium text-gray-700 dark:text-gray-200 pt-2">
                        {key}
                      </label>
                    </div>
                    <CommitInput
                      ref={index === 0 ? firstInputRef : undefined}
                      type="text"
                      value={String(value)}
                      onChange={(val) => handleUpdateParam(key, val, 'string')} // Default to string for recipe params
                      onKeyDown={(e) => {
                        if (e.key === 'Escape') {
                          e.preventDefault();
                          headerRef.current?.focus();
                        }
                      }}
                      onBlur={(e) => {
                        // フォーカスがセクション外に移った場合のみセクションを閉じる
                        const relatedTarget = e.relatedTarget as Node;
                        if (!relatedTarget || !sectionRef.current?.contains(relatedTarget)) {
                          if (!isCollapsed) {
                            onToggleCollapse();
                          }
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
            </>
          ) : (
            <div className="text-sm text-gray-500 italic">
              No parameters defined for this want type.
            </div>
          )}

          {/* State Exposures */}
          {currentStateFields.length > 0 && onExposesChange && (
            <div className="pt-3 border-t border-gray-100 dark:border-gray-700 space-y-1">
              {currentStateFields.map(s => {
                const configuredAs = exposes.find(e => e.currentState === s.name)?.as || '';
                const isExpanded = expandedExposeFields.has(s.name);
                return (
                  <div key={s.name} className="group flex items-center gap-2 min-h-[28px]">
                    {/* State field name */}
                    <span className="w-32 flex-shrink-0 text-xs font-mono text-gray-500 dark:text-gray-400 truncate" title={s.name}>
                      {s.name}
                    </span>

                    {/* Expose toggle icon */}
                    <button
                      type="button"
                      onClick={() => toggleExposeField(s.name)}
                      title={isExpanded ? 'Remove expose' : 'Expose as global key'}
                      className={`
                        flex-shrink-0 p-1 rounded transition-all duration-150
                        ${isExpanded || configuredAs
                          ? 'text-purple-500 dark:text-purple-400 bg-purple-50 dark:bg-purple-900/20'
                          : 'text-gray-300 dark:text-gray-600 opacity-0 group-hover:opacity-100 hover:text-purple-400 hover:bg-purple-50 dark:hover:bg-purple-900/20'
                        }
                      `}
                    >
                      <Share2 className="w-3 h-3" />
                    </button>

                    {/* Expandable input */}
                    {isExpanded && (
                      <div className="flex items-center gap-1.5 flex-1 animate-in fade-in slide-in-from-left-1 duration-150">
                        <span className="text-xs text-gray-300 dark:text-gray-600">→</span>
                        <input
                          autoFocus
                          type="text"
                          value={configuredAs}
                          onChange={e => handleExposeAsChange(s.name, e.target.value)}
                          placeholder="global key name"
                          className="flex-1 text-xs px-2 py-0.5 rounded-md border border-purple-200 dark:border-purple-700 bg-white dark:bg-gray-800 text-gray-700 dark:text-gray-200 placeholder-gray-300 dark:placeholder-gray-600 focus:outline-none focus:ring-1 focus:ring-purple-400 focus:border-purple-400"
                        />
                      </div>
                    )}

                    {/* Show configured value when collapsed */}
                    {!isExpanded && configuredAs && (
                      <span className="text-xs text-purple-500 dark:text-purple-400 font-mono">→ {configuredAs}</span>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </div>
      )}
    </div>
  );
});

ParametersSection.displayName = 'ParametersSection';
