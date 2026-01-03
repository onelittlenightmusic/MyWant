import React, { useState, useCallback, useRef, forwardRef } from 'react';
import { Settings, ChevronDown, ChevronRight } from 'lucide-react';
import { SectionNavigationCallbacks } from '@/types/formSection';

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
  const firstInputRef = useRef<HTMLInputElement>(null);

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
      navigationCallbacks.onNavigateUp();
    }
    // Down arrow - navigate to next section
    else if (e.key === 'ArrowDown') {
      e.preventDefault();
      navigationCallbacks.onNavigateDown();
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
    <div className="space-y-2">
      {/* Section Header */}
      <button
        ref={mergedRef}
        type="button"
        onClick={onToggleCollapse}
        onKeyDown={handleHeaderKeyDown}
        className={`
          w-full text-left px-4 py-3 rounded-lg border-2
          transition-all duration-200 focus:outline-none
          relative bg-blue-50 border-blue-200 hover:bg-blue-100

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
              <ChevronRight className="w-5 h-5 text-gray-500" />
            ) : (
              <ChevronDown className="w-5 h-5 text-gray-500" />
            )}

            {/* Section Icon */}
            <Settings className="w-5 h-5 text-gray-700" />

            {/* Section Title */}
            <h3 className="text-lg font-semibold text-gray-900">
              Parameters
            </h3>

            {/* Parameter Count Badge */}
            {Object.keys(parameters).length > 0 && (
              <span className="px-2 py-0.5 text-xs font-medium rounded-full bg-blue-100 text-blue-700">
                {Object.keys(parameters).length}
              </span>
            )}
          </div>

          {/* Collapsed Summary */}
          {isCollapsed && Object.entries(parameters).length > 0 && (
            <div className="flex flex-wrap gap-2 ml-4">
              {Object.entries(parameters).slice(0, 3).map(([key, value]) => (
                <span key={key} className="text-xs bg-blue-100 text-blue-700 px-2 py-1 rounded-full">
                  {key}: {String(value)}
                </span>
              ))}
              {Object.entries(parameters).length > 3 && (
                <span className="text-xs text-gray-500">
                  +{Object.entries(parameters).length - 3} more
                </span>
              )}
            </div>
          )}
        </div>
      </button>

      {/* Section Content (Expanded) */}
      {!isCollapsed && (
        <div className="pl-4 space-y-3">
          {parameterDefinitions && parameterDefinitions.length > 0 ? (
            <>
              {/* Parameters from want type definition */}
              <div className="space-y-2">
                {filteredParams.map((param, index) => (
                  <div key={param.name} className="space-y-1">
                    <div className="flex gap-2 items-start">
                      <div className="w-32 flex-shrink-0">
                        <label className="block text-sm font-medium text-gray-700 pt-2">
                          {param.name}
                          {param.required && <span className="text-red-600 ml-1">*</span>}
                        </label>
                      </div>
                      <input
                        ref={index === 0 ? firstInputRef : undefined}
                        type="text"
                        value={String(parameters[param.name] || '')}
                        onChange={(e) => handleUpdateParam(param.name, e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === 'Escape') {
                            e.preventDefault();
                            headerRef.current?.focus();
                          }
                        }}
                        className="flex-1 px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent text-sm"
                        placeholder={param.description || 'Enter value'}
                      />
                    </div>
                    {param.description && (
                      <p className="text-xs text-gray-600 ml-32">{param.description}</p>
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
                    <input
                      ref={index === 0 ? firstInputRef : undefined}
                      type="text"
                      value={String(value)}
                      onChange={(e) => handleUpdateParam(key, e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === 'Escape') {
                          e.preventDefault();
                          headerRef.current?.focus();
                        }
                      }}
                      className="flex-1 px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent text-sm"
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
