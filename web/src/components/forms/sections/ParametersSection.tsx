import React, { useState, useCallback, useRef, forwardRef } from 'react';
import { Settings, ChevronDown, ChevronRight } from 'lucide-react';
import { SectionNavigationCallbacks } from '@/types/formSection';
import { CommitInput, CommitInputHandle } from '@/components/common/CommitInput';

/**
 * Parameter definition from want type
 */
export interface ParameterDefinition {
  name: string;
  required: boolean;
  description?: string;
}

/**
 * Props for ParametersSection
 */
interface ParametersSectionProps {
  /** Current parameter values */
  parameters: Record<string, any>;
  /** Parameter definitions from want type (if available) */
  parameterDefinitions?: ParameterDefinition[];
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
   * Update a parameter value
   */
  const handleUpdateParam = useCallback((key: string, value: any) => {
    onChange({
      ...parameters,
      [key]: value
    });
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
    param => param.required || showOptionalParams
  ) || [];

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
          w-full text-left px-3 sm:px-4 py-2 sm:py-3 rounded-lg border-2
          transition-all duration-200 focus:outline-none
          relative bg-blue-50 dark:bg-blue-900/20 border-blue-200 dark:border-blue-800 hover:bg-blue-100 dark:hover:bg-blue-900/30

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
            {/* Collapse/Expand Icon */}
            {isCollapsed ? (
              <ChevronRight className="w-4 h-4 sm:w-5 sm:h-5 text-gray-500 dark:text-gray-400" />
            ) : (
              <ChevronDown className="w-4 h-4 sm:w-5 sm:h-5 text-gray-500 dark:text-gray-400" />
            )}

            {/* Section Icon */}
            <Settings className="w-4 h-4 sm:w-5 sm:h-5 text-gray-700 dark:text-gray-200" />

            {/* Section Title */}
            <h3 className="text-base sm:text-lg font-semibold text-gray-900 dark:text-white">
              Parameters
            </h3>

            {/* Parameter Count Badge */}
            {Object.keys(parameters).length > 0 && (
              <span className="px-2 py-0.5 text-xs font-medium rounded-full bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400">
                {Object.keys(parameters).length}
              </span>
            )}
          </div>

          {/* Collapsed Summary */}
          {isCollapsed && Object.entries(parameters).length > 0 && (
            <div className="flex flex-wrap gap-2 ml-4">
              {Object.entries(parameters).slice(0, 3).map(([key, value]) => (
                <span key={key} className="text-xs bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400 px-2 py-1 rounded-full">
                  {key}: {String(value)}
                </span>
              ))}
              {Object.entries(parameters).length > 3 && (
                <span className="text-xs text-gray-500 dark:text-gray-400">
                  +{Object.entries(parameters).length - 3} more
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
                {filteredParams.map((param, index) => (
                  <div key={param.name} className="space-y-0.5 sm:space-y-1">
                    <div className="flex flex-col sm:flex-row gap-1 sm:gap-2 items-start sm:items-center">
                      <div className="w-full sm:w-32 flex-shrink-0">
                        <label className="block text-xs sm:text-sm font-medium text-gray-700 pt-1 sm:pt-2">
                          {param.name}
                          {param.required && <span className="text-red-600 ml-1">*</span>}
                        </label>
                      </div>
                      <CommitInput
                        ref={index === 0 ? firstInputRef : undefined}
                        type="text"
                        value={String(parameters[param.name] || '')}
                        onChange={(val) => handleUpdateParam(param.name, val)}
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
                      />
                    </div>
                    {param.description && (
                      <p className="text-[10px] sm:text-xs text-gray-600 sm:ml-32">{param.description}</p>
                    )}
                  </div>
                ))}
              </div>

              {/* Show Optional Parameters Toggle */}
              {parameterDefinitions.some(param => !param.required) && (
                <div className="pt-2 border-t border-gray-200">
                  <button
                    type="button"
                    onClick={() => setShowOptionalParams(!showOptionalParams)}
                    className="text-blue-600 hover:text-blue-800 text-sm flex items-center gap-1 focus:outline-none focus:ring-2 focus:ring-blue-300 rounded px-2 py-1"
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
                      <label className="block text-sm font-medium text-gray-700 pt-2">
                        {key}
                      </label>
                    </div>
                    <CommitInput
                      ref={index === 0 ? firstInputRef : undefined}
                      type="text"
                      value={String(value)}
                      onChange={(val) => handleUpdateParam(key, val)}
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
        </div>
      )}
    </div>
  );
});

ParametersSection.displayName = 'ParametersSection';
