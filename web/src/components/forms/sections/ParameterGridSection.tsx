import React, { useState, useCallback, useRef, useEffect } from 'react';
import { Type, Hash, ToggleLeft, Link, Plus, X } from 'lucide-react';
import { ParameterDef, StateDef } from '@/types/wantType';
import { SelectInput, SelectInputHandle } from '@/components/common/SelectInput';
import { CommitInput, CommitInputHandle } from '@/components/common/CommitInput';
import { useInputActions } from '@/hooks/useInputActions';
import { apiClient } from '@/api/client';
import { classNames } from '@/utils/helpers';

const COLS = 2;

interface ExposeEntry {
  currentState?: string;
  param?: string;
  as?: string;
}

export interface ParameterGridSectionProps {
  parameters: Record<string, any>;
  parameterDefinitions?: ParameterDef[];
  stateDefs?: StateDef[];
  exposes?: ExposeEntry[];
  onExposesChange?: (exposes: ExposeEntry[]) => void;
  originalParameters?: Record<string, any>;
  onChange: (params: Record<string, any>) => void;
  /** Whether this tab is currently active — enables grid keyboard/gamepad navigation */
  isActive: boolean;
  /** Forward LB/RB bumper tab navigation to parent (called by captureGamepad handler) */
  onTabForward?: () => void;
  onTabBackward?: () => void;
}

function TypeIcon({ type }: { type: string }) {
  const cls = 'w-2.5 h-2.5 text-gray-400 dark:text-gray-500';
  switch (type) {
    case 'int':
    case 'float64':
      return <Hash className={cls} />;
    case 'bool':
      return <ToggleLeft className={cls} />;
    default:
      return <Type className={cls} />;
  }
}

export const ParameterGridSection: React.FC<ParameterGridSectionProps> = ({
  parameters,
  parameterDefinitions,
  stateDefs,
  exposes = [],
  onExposesChange,
  originalParameters = {},
  onChange,
  isActive,
  onTabForward,
  onTabBackward,
}) => {
  const [focusedIndex, setFocusedIndex] = useState(-1);
  const [showOptional, setShowOptional] = useState(false);
  const [globalParams, setGlobalParams] = useState<Record<string, unknown>>({});
  const [globalOverrides, setGlobalOverrides] = useState<Record<string, boolean>>({});
  const [addingExpose, setAddingExpose] = useState(false);
  const [newExposeState, setNewExposeState] = useState('');
  const [newExposeAs, setNewExposeAs] = useState('');

  const focusedIndexRef = useRef(-1);
  focusedIndexRef.current = focusedIndex;

  const inputRefs = useRef<Array<CommitInputHandle | SelectInputHandle | null>>([]);

  const filteredParams = (parameterDefinitions ?? []).filter(
    p => p.required || showOptional || parameters[p.name] !== undefined
  );
  const filteredParamsRef = useRef(filteredParams);
  filteredParamsRef.current = filteredParams;

  const currentStateFields = stateDefs?.filter(s => s.name !== 'final_result') ?? [];

  useEffect(() => {
    const hasGlobal = parameterDefinitions?.some(p => p.defaultGlobalParameter);
    if (!hasGlobal) return;
    apiClient.getGlobalParameters()
      .then(res => setGlobalParams(res.parameters ?? {}))
      .catch(() => {});
  }, [parameterDefinitions]);

  useEffect(() => {
    if (!parameterDefinitions) return;
    setGlobalOverrides(prev => {
      const next = { ...prev };
      for (const p of parameterDefinitions) {
        if (p.defaultGlobalParameter && !(p.name in next)) {
          next[p.name] = p.name in parameters;
        }
      }
      return next;
    });
  }, [parameterDefinitions]); // intentionally excludes parameters to avoid loop

  // Auto-highlight first card when tab becomes active
  useEffect(() => {
    if (isActive && focusedIndex < 0 && filteredParams.length > 0) {
      setFocusedIndex(0);
    }
    if (!isActive) setFocusedIndex(-1);
  }, [isActive]); // eslint-disable-line react-hooks/exhaustive-deps

  // 2D grid D-pad navigation between cards.
  // captureGamepad: true — owns all gamepad events (LB/RB forwarded to parent via onTabForward/Backward).
  // ignoreWhenInputFocused: true — with captureGamepad the guard is bypassed anyway, but
  // keyboard bubble phase is also skipped, so only gamepad D-pad reaches onNavigate.
  useInputActions({
    enabled: isActive && focusedIndex >= 0,
    captureGamepad: true,
    ignoreWhenInputFocused: true,
    ignoreWhenInSidebar: false,
    onTabForward: onTabForward,
    onTabBackward: onTabBackward,
    onNavigate: (dir) => {
      // Blur any focused input first so the value is committed before moving
      const el = document.activeElement as HTMLElement | null;
      if (el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT')) {
        el.blur();
      }

      const i = focusedIndexRef.current;
      const total = filteredParamsRef.current.length;
      const col = i % COLS;
      const row = Math.floor(i / COLS);

      if (dir === 'left' && col > 0) setFocusedIndex(i - 1);
      else if (dir === 'right' && col < COLS - 1 && i + 1 < total) setFocusedIndex(i + 1);
      else if (dir === 'up' && row > 0) setFocusedIndex(i - COLS);
      else if (dir === 'down' && i + COLS < total) setFocusedIndex(i + COLS);
      // At grid edge: no-op; LB/RB bumpers (tab-forward/backward) handle tab switching
    },
    onConfirm: () => {
      const target = inputRefs.current[focusedIndexRef.current];
      if (target && 'focus' in target) target.focus();
    },
    // Escape / B — two levels:
    //   Level 1: input is focused → blur it, stay on card
    //   Level 2: card highlighted, no input focused → exit grid, return to parent tab navigation
    onCancel: () => {
      const el = document.activeElement as HTMLElement | null;
      if (el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT')) {
        el.blur();
      } else {
        setFocusedIndex(-1);
      }
    },
  });

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
    const next = { ...parameters };
    if (typedValue === undefined) delete next[key];
    else next[key] = typedValue;
    onChange(next);
  }, [parameters, onChange]);

  const handleToggleGlobal = useCallback((paramName: string) => {
    setGlobalOverrides(prev => {
      const nowOverride = !prev[paramName];
      const next = { ...prev, [paramName]: nowOverride };
      if (!nowOverride) {
        const newParams = { ...parameters };
        delete newParams[paramName];
        onChange(newParams);
      }
      return next;
    });
  }, [parameters, onChange]);

  const handleAddExpose = useCallback(() => {
    if (!onExposesChange || !newExposeState || !newExposeAs.trim()) return;
    const next = exposes.filter(e => e.currentState !== newExposeState);
    next.push({ currentState: newExposeState, as: newExposeAs.trim() });
    onExposesChange(next);
    setNewExposeState('');
    setNewExposeAs('');
    setAddingExpose(false);
  }, [exposes, onExposesChange, newExposeState, newExposeAs]);

  // Recipe mode (no parameterDefinitions): render simple list
  if (!parameterDefinitions || parameterDefinitions.length === 0) {
    const entries = Object.entries(parameters);
    if (entries.length === 0) {
      return (
        <div className="flex flex-col items-center justify-center py-12 text-gray-400 dark:text-gray-600 gap-2">
          <Type className="w-8 h-8 opacity-40" />
          <p className="text-sm">No parameters defined</p>
        </div>
      );
    }
    return (
      <div className="space-y-2">
        <p className="text-xs text-gray-500 dark:text-gray-400 italic mb-2">Recipe parameters</p>
        {entries.map(([key, value], index) => (
          <div key={key} className="flex items-center gap-2">
            <label className="w-28 text-xs font-medium text-gray-700 dark:text-gray-300 truncate flex-shrink-0">
              {key}
            </label>
            <CommitInput
              ref={el => { inputRefs.current[index] = el as CommitInputHandle | null; }}
              type="text"
              value={String(value)}
              onChange={val => handleUpdateParam(key, val, 'string')}
              className="flex-1"
              placeholder="value"
              multiline
            />
          </div>
        ))}
      </div>
    );
  }

  const optionalCount = parameterDefinitions.filter(p => !p.required).length;

  return (
    <div className="space-y-3">
      {filteredParams.length > 0 ? (
        <div className="grid grid-cols-2 gap-2">
          {filteredParams.map((param, index) => {
            const hasEnum = !!(param.validation?.enum?.length);
            const isCheckbox = param.type === 'bool';
            const isNumber = param.type === 'int' || param.type === 'float64';
            const hasGlobal = !!param.defaultGlobalParameter;
            const isOverride = globalOverrides[param.name] ?? false;
            const globalValue = hasGlobal ? globalParams[param.defaultGlobalParameter!] : undefined;
            const currentValue = parameters[param.name];
            const isFocused = focusedIndex === index;

            const origVal = originalParameters[param.name] ?? param.default ?? param.example;
            const isModified = currentValue !== undefined &&
              String(currentValue) !== String(origVal ?? '');

            const isEmpty = (!hasGlobal || isOverride) &&
              (currentValue === undefined || currentValue === '');

            let displayValue = '';
            if (currentValue !== undefined) displayValue = String(currentValue);
            else if (param.example !== undefined) displayValue = String(param.example);
            else if (param.default !== undefined) displayValue = String(param.default);

            return (
              <div
                key={param.name}
                onClick={() => setFocusedIndex(index)}
                className={classNames(
                  'relative rounded-xl border-2 p-2.5 transition-all duration-150 cursor-pointer',
                  isFocused
                    ? 'border-blue-400 dark:border-blue-500 shadow-[0_0_0_3px_rgba(59,130,246,0.22)] bg-white dark:bg-gray-800'
                    : isModified
                      ? 'border-amber-300 dark:border-amber-600 bg-amber-50/30 dark:bg-amber-900/10 hover:border-amber-400'
                      : param.required && isEmpty
                        ? 'border-red-200 dark:border-red-800/60 bg-red-50/20 dark:bg-red-900/10 hover:border-red-300'
                        : 'border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800/50 hover:border-gray-300 dark:hover:border-gray-600'
                )}
              >
                {/* Card header */}
                <div className="flex items-center justify-between mb-1.5">
                  <span className="text-[11px] font-semibold text-gray-600 dark:text-gray-300 truncate leading-none">
                    {param.name}
                    {param.required && <span className="text-red-500 ml-0.5">★</span>}
                  </span>
                  <div className="flex items-center gap-0.5 ml-1 flex-shrink-0">
                    {isModified && (
                      <span className="w-1.5 h-1.5 rounded-full bg-amber-400" title="Modified" />
                    )}
                    <TypeIcon type={param.type} />
                  </div>
                </div>

                {/* Input area */}
                {hasGlobal && !isOverride ? (
                  <span className="block text-sm text-purple-500 dark:text-purple-400 italic truncate">
                    {globalValue !== undefined
                      ? String(globalValue)
                      : <span className="text-gray-300 dark:text-gray-600 not-italic text-xs">global</span>
                    }
                  </span>
                ) : hasEnum ? (
                  <SelectInput
                    ref={el => { inputRefs.current[index] = el; }}
                    value={displayValue}
                    onChange={val => handleUpdateParam(param.name, val, param.type)}
                    options={(param.validation!.enum! as string[]).map(v => ({ value: String(v) }))}
                    className="w-full"
                  />
                ) : isCheckbox ? (
                  <input
                    type="checkbox"
                    checked={Boolean(currentValue ?? param.default ?? false)}
                    onChange={e => handleUpdateParam(param.name, String(e.target.checked), 'bool')}
                    className="w-5 h-5 text-blue-600 border-gray-300 rounded mt-0.5"
                  />
                ) : (
                  <CommitInput
                    ref={el => { inputRefs.current[index] = el as CommitInputHandle | null; }}
                    type={isNumber ? 'number' : 'text'}
                    value={displayValue}
                    onChange={val => handleUpdateParam(param.name, val, param.type)}
                    placeholder={param.description || '—'}
                    className="w-full"
                    multiline={false}
                  />
                )}

                {/* Global param toggle button */}
                {hasGlobal && (
                  <button
                    type="button"
                    onClick={e => { e.stopPropagation(); handleToggleGlobal(param.name); }}
                    title={isOverride
                      ? `Custom value (click to use global: ${param.defaultGlobalParameter})`
                      : `Using global: ${param.defaultGlobalParameter} (click to override)`
                    }
                    className={classNames(
                      'mt-1 text-[9px] flex items-center gap-0.5 px-1 py-0.5 rounded-full transition-colors',
                      isOverride
                        ? 'bg-amber-100 text-amber-600 dark:bg-amber-900/30 dark:text-amber-400 hover:bg-amber-200'
                        : 'bg-purple-100 text-purple-600 dark:bg-purple-900/30 dark:text-purple-400 hover:bg-purple-200'
                    )}
                  >
                    <Link className="w-2 h-2" />
                    {isOverride ? 'custom' : param.defaultGlobalParameter}
                  </button>
                )}

                {/* Glow bottom bar when focused */}
                {isFocused && (
                  <div className="absolute bottom-0 left-1/4 right-1/4 h-0.5 rounded-full bg-blue-400 dark:bg-blue-500" />
                )}
              </div>
            );
          })}
        </div>
      ) : (
        <div className="flex flex-col items-center justify-center py-8 text-gray-400 dark:text-gray-600 gap-2">
          <Type className="w-6 h-6 opacity-40" />
          <p className="text-xs">No parameters to show</p>
        </div>
      )}

      {/* Optional params toggle */}
      {optionalCount > 0 && (
        <button
          type="button"
          onClick={() => setShowOptional(v => !v)}
          className="text-xs text-blue-500 dark:text-blue-400 flex items-center gap-1 px-2 py-0.5 rounded hover:bg-blue-50 dark:hover:bg-blue-900/20 transition-colors"
        >
          {showOptional ? '▼ Hide' : '▶ Show'} optional ({optionalCount})
        </button>
      )}

      {/* State Exposures */}
      {currentStateFields.length > 0 && onExposesChange && (
        <div className="pt-3 border-t border-gray-100 dark:border-gray-700 space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-[10px] font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500">
              Expose
            </span>
            <button
              type="button"
              onClick={() => { setAddingExpose(v => !v); setNewExposeState(''); setNewExposeAs(''); }}
              className="text-[10px] flex items-center gap-0.5 text-purple-500 hover:text-purple-700 dark:text-purple-400 px-1.5 py-0.5 rounded hover:bg-purple-50 dark:hover:bg-purple-900/20 transition-colors"
            >
              <Plus className="w-2.5 h-2.5" /> Add
            </button>
          </div>

          {exposes.filter(e => e.currentState && e.as).length > 0 && (
            <div className="flex flex-wrap gap-1">
              {exposes.filter(e => e.currentState && e.as).map(e => (
                <span
                  key={e.currentState}
                  className="inline-flex items-center gap-1 text-[10px] bg-purple-50 dark:bg-purple-900/20 text-purple-700 dark:text-purple-300 border border-purple-200 dark:border-purple-700 px-1.5 py-0.5 rounded-full"
                >
                  <span className="font-mono">{e.currentState}</span>
                  <span className="text-purple-400 dark:text-purple-500">→</span>
                  <span className="font-mono">{e.as}</span>
                  <button
                    type="button"
                    onClick={() => onExposesChange(exposes.filter(ex => ex.currentState !== e.currentState))}
                    className="ml-0.5 text-purple-400 hover:text-purple-600 dark:hover:text-purple-200"
                    title="Remove expose"
                  >
                    <X className="w-2 h-2" />
                  </button>
                </span>
              ))}
            </div>
          )}

          {addingExpose && (
            <div className="flex items-center gap-1.5">
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
                className="text-xs px-2 py-1 flex-1 rounded-md border border-purple-200 dark:border-purple-700 bg-white dark:bg-gray-800 text-gray-700 dark:text-gray-200 placeholder-gray-300 dark:placeholder-gray-600 focus:outline-none focus:ring-1 focus:ring-purple-400"
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
  );
};

ParameterGridSection.displayName = 'ParameterGridSection';
