import React, { useState, useCallback, useRef, useEffect } from 'react';
import { Type, Hash, ToggleLeft, Link, Plus, X, Zap, List, LucideIcon, Globe, Rows3, Copy, Check, KeyRound } from 'lucide-react';
import { ParameterDef, StateDef } from '@/types/wantType';
import { SelectInput, SelectInputHandle } from '@/components/common/SelectInput';
import { CommitInput, CommitInputHandle } from '@/components/common/CommitInput';
import { useCardGridNavigation } from '@/hooks/useCardGridNavigation';
import { apiClient } from '@/api/client';
import { classNames } from '@/utils/helpers';
import { DisplayCard, AddCard, FormCard, CardScheme, BLUE_SCHEME, GREEN_SCHEME, TEAL_SCHEME, PURPLE_SCHEME, AMBER_SCHEME } from '@/components/forms/CardPrimitives';
import { NumberSliderInput } from '@/components/common/NumberSliderInput';

const COLS = 2;

function CopyButton({ value }: { value: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      onClick={e => {
        e.stopPropagation();
        navigator.clipboard.writeText(value);
        setCopied(true);
        setTimeout(() => setCopied(false), 1500);
      }}
      title="Copy value"
      className="w-4 h-4 flex items-center justify-center text-gray-300 dark:text-gray-600 hover:text-blue-400 dark:hover:text-blue-500 transition-colors"
    >
      {copied ? <Check className="w-2.5 h-2.5 text-green-500" /> : <Copy className="w-2.5 h-2.5" />}
    </button>
  );
}

/** Serialize an array item for display in a text input */
function serializeItem(item: any): string {
  if (item === null || item === undefined) return '';
  if (typeof item === 'object') return JSON.stringify(item);
  return String(item);
}

/** Parse a text input back to an array item value */
function parseItem(text: string): any {
  const t = text.trim();
  if (t === '') return '';
  try { return JSON.parse(t); } catch { return t; }
}

/** Returns the fromGlobalParam key if the value is a ParamRef, otherwise null */
function getParamRef(value: any): string | null {
  if (value && typeof value === 'object' && !Array.isArray(value) && typeof value.fromGlobalParam === 'string') {
    return value.fromGlobalParam;
  }
  return null;
}

export interface ParameterGridSectionProps {
  parameters: Record<string, any>;
  parameterDefinitions?: ParameterDef[];
  stateDefs?: StateDef[];
  originalParameters?: Record<string, any>;
  onChange: (params: Record<string, any>) => void;
  /** Whether this tab is currently active — enables grid keyboard/gamepad navigation */
  isActive: boolean;
  /** Forward LB/RB bumper tab navigation to parent (called by captureGamepad handler) */
  onTabForward?: () => void;
  onTabBackward?: () => void;
}

// Cyan and indigo don't have CardScheme presets (param-grid only), so define inline.
const CYAN_SCHEME: CardScheme = {
  cardBg:      'bg-cyan-50/70 dark:bg-cyan-900/15',
  cardHover:   'hover:shadow hover:bg-cyan-100/70 dark:hover:bg-cyan-900/25',
  bgIconColor: 'text-cyan-400 dark:text-cyan-600',
  iconColor:   'text-cyan-500 dark:text-cyan-400',
  formBorder:  'border-cyan-300 dark:border-cyan-600',
  formBg:      'bg-cyan-50/30 dark:bg-cyan-900/10',
  saveColor:   'text-cyan-600 dark:text-cyan-400 hover:bg-cyan-50 dark:hover:bg-cyan-900/30',
  addBorder:   'border-cyan-200 dark:border-cyan-800/50 hover:border-cyan-400 dark:hover:border-cyan-600',
  addIcon:     'text-cyan-300 dark:text-cyan-700 group-hover:text-cyan-500 dark:group-hover:text-cyan-400',
};
const INDIGO_SCHEME: CardScheme = {
  cardBg:      'bg-indigo-50/70 dark:bg-indigo-900/15',
  cardHover:   'hover:shadow hover:bg-indigo-100/70 dark:hover:bg-indigo-900/25',
  bgIconColor: 'text-indigo-400 dark:text-indigo-600',
  iconColor:   'text-indigo-500 dark:text-indigo-400',
  formBorder:  'border-indigo-300 dark:border-indigo-600',
  formBg:      'bg-indigo-50/30 dark:bg-indigo-900/10',
  saveColor:   'text-indigo-600 dark:text-indigo-400 hover:bg-indigo-50 dark:hover:bg-indigo-900/30',
  addBorder:   'border-indigo-200 dark:border-indigo-800/50 hover:border-indigo-400 dark:hover:border-indigo-600',
  addIcon:     'text-indigo-300 dark:text-indigo-700 group-hover:text-indigo-500 dark:group-hover:text-indigo-400',
};
const ORANGE_SCHEME: CardScheme = {
  cardBg:      'bg-orange-50/70 dark:bg-orange-900/15',
  cardHover:   'hover:shadow hover:bg-orange-100/70 dark:hover:bg-orange-900/25',
  bgIconColor: 'text-orange-400 dark:text-orange-600',
  iconColor:   'text-orange-500 dark:text-orange-400',
  formBorder:  'border-orange-300 dark:border-orange-600',
  formBg:      'bg-orange-50/30 dark:bg-orange-900/10',
  saveColor:   'text-orange-600 dark:text-orange-400 hover:bg-orange-50 dark:hover:bg-orange-900/30',
  addBorder:   'border-orange-200 dark:border-orange-800/50 hover:border-orange-400 dark:hover:border-orange-600',
  addIcon:     'text-orange-300 dark:text-orange-700 group-hover:text-orange-500 dark:group-hover:text-orange-400',
};

interface TypeStyle {
  scheme: CardScheme;
  BgIcon: LucideIcon;
}

function getTypeStyle(type: string, hasEnum: boolean): TypeStyle {
  if (hasEnum) return { scheme: CYAN_SCHEME, BgIcon: List };
  switch (type) {
    case 'array':    return { scheme: INDIGO_SCHEME, BgIcon: Rows3 };
    case 'int':
    case 'float64':  return { scheme: ORANGE_SCHEME, BgIcon: Hash };
    case 'bool':     return { scheme: GREEN_SCHEME,  BgIcon: ToggleLeft };
    case 'want_type':return { scheme: PURPLE_SCHEME, BgIcon: Zap };
    default:         return { scheme: BLUE_SCHEME,   BgIcon: Type };
  }
}

function TypeIcon({ type, hasEnum, colorClass }: { type: string; hasEnum: boolean; colorClass: string }) {
  const cls = `w-2.5 h-2.5 ${colorClass}`;
  if (hasEnum) return <List className={cls} />;
  switch (type) {
    case 'array':
      return <Rows3 className={cls} />;
    case 'int':
    case 'float64':
      return <Hash className={cls} />;
    case 'bool':
      return <ToggleLeft className={cls} />;
    case 'want_type':
      return <Zap className={cls} />;
    default:
      return <Type className={cls} />;
  }
}

export const ParameterGridSection: React.FC<ParameterGridSectionProps> = ({
  parameters,
  parameterDefinitions,
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
  const [wantTypeOptions, setWantTypeOptions] = useState<{ value: string }[]>([]);
  // Cache for both modes per param — survives toggling back and forth
  const [paramCache, setParamCache] = useState<Record<string, { literal: any; ref: string }>>({});
  const [pendingKey, setPendingKey] = useState<string | null>(null);

  const focusedIndexRef = useRef(-1);
  focusedIndexRef.current = focusedIndex;

  const inputRefs = useRef<Array<CommitInputHandle | SelectInputHandle | null>>([]);

  const filteredParams = (parameterDefinitions ?? []).filter(
    p => p.required || showOptional || parameters[p.name] !== undefined
  );
  const filteredParamsRef = useRef(filteredParams);
  filteredParamsRef.current = filteredParams;

  const extraParamKeysRef = useRef<string[]>([]);

  useEffect(() => {
    apiClient.getGlobalParameters()
      .then(res => setGlobalParams(res.parameters ?? {}))
      .catch(() => {});
  }, []);

  useEffect(() => {
    const hasWantType = parameterDefinitions?.some(p => p.type === 'want_type');
    if (!hasWantType) return;
    apiClient.listWantTypes()
      .then(res => setWantTypeOptions((res.wantTypes ?? []).map(wt => ({ value: wt.name }))))
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
    if (isActive && focusedIndex < 0 && filteredParams.length > 0) setFocusedIndex(0);
    if (!isActive) setFocusedIndex(-1);
  }, [isActive, filteredParams.length]); // eslint-disable-line react-hooks/exhaustive-deps

  const { gridProps } = useCardGridNavigation({
    count: filteredParamsRef.current.length + extraParamKeysRef.current.length,
    cols: COLS,
    isActive,
    focusedIndex,
    setFocusedIndex,
    onConfirm: (i) => {
      const t = inputRefs.current[i];
      if (t && 'focus' in t) (t as { focus: () => void }).focus();
    },
    onTabForward,
    onTabBackward,
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
    if (typedValue !== undefined) {
      setParamCache(c => ({ ...c, [key]: { ...c[key], literal: typedValue } }));
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

  const handleToggleParamRef = useCallback((paramName: string, currentRef: string | null) => {
    const next = { ...parameters };
    if (currentRef !== null) {
      // ref → literal: save current ref key, restore cached literal
      setParamCache(c => ({ ...c, [paramName]: { ...c[paramName], ref: currentRef } }));
      const cached = paramCache[paramName]?.literal;
      if (cached !== undefined) next[paramName] = cached;
      else next[paramName] = '';
    } else {
      // literal → ref: save current literal, restore cached ref key
      const literalNow = parameters[paramName];
      setParamCache(c => ({ ...c, [paramName]: { ...c[paramName], literal: literalNow } }));
      const cachedRef = paramCache[paramName]?.ref ?? '';
      next[paramName] = { fromGlobalParam: cachedRef };
    }
    onChange(next);
  }, [parameters, paramCache, onChange]);

  const handleUpdateParamRef = useCallback((paramName: string, globalKey: string) => {
    // Keep the ParamRef object even when key is empty — deletion is only done via handleToggleParamRef
    setParamCache(c => ({ ...c, [paramName]: { ...c[paramName], ref: globalKey } }));
    const next = { ...parameters, [paramName]: { fromGlobalParam: globalKey } };
    onChange(next);
  }, [parameters, onChange]);

  const optionalCount = (parameterDefinitions ?? []).filter(p => !p.required).length;
  const definedParamNames = new Set((parameterDefinitions ?? []).map(p => p.name));
  const extraParamKeys = Object.keys(parameters).filter(k => !definedParamNames.has(k));
  extraParamKeysRef.current = extraParamKeys;

  return (
    <div className="space-y-3">
      <div {...gridProps} className="grid grid-cols-2 gap-2 outline-none">
        {filteredParams.map((param, index) => {
            const hasEnum = !!(param.validation?.enum?.length);
            const isArray = param.type === 'array';
            const isCheckbox = param.type === 'bool';
            const isNumber = param.type === 'int' || param.type === 'float64';
            const isWantType = param.type === 'want_type';
            const sliderMin = param.validation?.min ?? 0;
            const sliderMax = param.validation?.max ?? 100;
            const isSlider = isNumber && param.validation?.min !== undefined && param.validation?.max !== undefined;
            const sliderStep = param.type === 'int' ? 1 : Math.max(0.001, (sliderMax - sliderMin) / 100);
            const hasGlobal = !!param.defaultGlobalParameter;
            const isOverride = globalOverrides[param.name] ?? false;
            const globalValue = hasGlobal ? globalParams[param.defaultGlobalParameter!] : undefined;
            const currentValue = parameters[param.name];
            const isFocused = focusedIndex === index;

            // Want-level fromGlobalParam reference
            const paramRef = getParamRef(currentValue);
            const isParamRef = paramRef !== null;

            const origVal = originalParameters[param.name] ?? param.default ?? param.example;
            const isModified = currentValue !== undefined &&
              !isParamRef &&
              String(currentValue) !== String(origVal ?? '');

            const isEmpty = !isParamRef && (!hasGlobal || isOverride) &&
              (currentValue === undefined || currentValue === '');

            let displayValue = '';
            if (currentValue !== undefined && !isParamRef) displayValue = String(currentValue);
            else if (param.example !== undefined) displayValue = String(param.example);
            else if (param.default !== undefined) displayValue = String(param.default);

            const typeStyle = getTypeStyle(param.type, hasEnum);
            const { scheme: ts, BgIcon } = typeStyle;

            // ── Array card (full width) ──────────────────────────────────────
            if (isArray) {
              const items: any[] = Array.isArray(currentValue) ? currentValue : [];
              const handleItemChange = (i: number, text: string) => {
                const next = [...items];
                next[i] = parseItem(text);
                onChange({ ...parameters, [param.name]: next });
              };
              const handleItemRemove = (i: number) => {
                const next = items.filter((_, idx) => idx !== i);
                onChange({ ...parameters, [param.name]: next });
              };
              const handleItemAdd = () => {
                // If existing items are objects, add an empty object; otherwise add empty string
                const template = items.length > 0 && typeof items[0] === 'object' && !Array.isArray(items[0])
                  ? {} : '';
                onChange({ ...parameters, [param.name]: [...items, template] });
              };
              return (
                <DisplayCard
                  key={param.name}
                  className={classNames(
                    'col-span-2 relative rounded-xl p-2.5 transition-all duration-150 cursor-pointer shadow-sm',
                    isFocused
                      ? 'shadow-md ring-2 ring-blue-400/40 bg-white dark:bg-gray-800'
                      : `${ts.cardBg} ${ts.cardHover}`
                  )}
                  BgIcon={BgIcon}
                  bgIconColor={ts.bgIconColor}
                  showBgIcon={true}
                  showFocusBar={isFocused}
                  onClick={() => setFocusedIndex(index)}
                  headerLeft={
                    <>
                      <KeyRound className={classNames('w-2.5 h-2.5 flex-shrink-0', ts.iconColor)} />
                      <span className="text-[11px] font-semibold text-gray-600 dark:text-gray-300 truncate leading-none">
                        {param.name}{param.required && <span className="text-red-500 ml-0.5">★</span>}
                      </span>
                      <span className="text-[9px] text-gray-400 dark:text-gray-500 ml-1">{items.length} items</span>
                    </>
                  }
                >
                  {/* Item sub-cards */}
                  <div className="grid grid-cols-2 gap-1.5">
                    {items.map((item, i) => {
                      const isObj = item !== null && typeof item === 'object' && !Array.isArray(item);
                      return (
                        <div
                          key={i}
                          onClick={e => e.stopPropagation()}
                          className="relative rounded-lg border border-indigo-200 dark:border-indigo-700/60 bg-white/70 dark:bg-gray-800/70 p-2 flex flex-col gap-1"
                        >
                          {/* Sub-card header */}
                          <div className="flex items-center justify-between mb-0.5">
                            <span className="text-[9px] font-semibold text-indigo-400 dark:text-indigo-500">#{i + 1}</span>
                            <button
                              type="button"
                              onClick={() => handleItemRemove(i)}
                              className="text-gray-300 dark:text-gray-600 hover:text-red-400 dark:hover:text-red-500 transition-colors"
                            >
                              <X className="w-3 h-3" />
                            </button>
                          </div>
                          {/* Sub-card body: key-value pairs for objects, single input for primitives */}
                          {isObj ? (
                            <div className="space-y-1">
                              {Object.entries(item as Record<string, any>).map(([k, v]) => (
                                <div key={k} className="flex items-center gap-1">
                                  <span className="text-[9px] text-gray-400 dark:text-gray-500 truncate w-16 flex-shrink-0">{k}</span>
                                  <CommitInput
                                    type="text"
                                    value={String(v ?? '')}
                                    onChange={val => {
                                      const updated = { ...(item as Record<string, any>), [k]: parseItem(val) };
                                      handleItemChange(i, JSON.stringify(updated));
                                    }}
                                    className="flex-1"
                                    multiline={false}
                                  />
                                </div>
                              ))}
                              {/* Add key button */}
                              <button
                                type="button"
                                onClick={() => {
                                  const key = prompt('Key name:');
                                  if (!key) return;
                                  const updated = { ...(item as Record<string, any>), [key]: '' };
                                  handleItemChange(i, JSON.stringify(updated));
                                }}
                                className="text-[9px] text-indigo-400 hover:text-indigo-600 dark:text-indigo-500 flex items-center gap-0.5"
                              >
                                <Plus className="w-2.5 h-2.5" /> key
                              </button>
                            </div>
                          ) : (
                            <CommitInput
                              type="text"
                              value={serializeItem(item)}
                              onChange={text => handleItemChange(i, text)}
                              placeholder="value"
                              className="w-full"
                              multiline={false}
                            />
                          )}
                        </div>
                      );
                    })}
                    {/* Add item placeholder card */}
                    <button
                      type="button"
                      onClick={e => { e.stopPropagation(); handleItemAdd(); }}
                      className="flex flex-col items-center justify-center gap-1 rounded-lg border-2 border-dashed border-indigo-200 dark:border-indigo-700/60 hover:border-indigo-400 dark:hover:border-indigo-500 transition-colors group min-h-[4rem] bg-transparent"
                    >
                      <Plus className="w-4 h-4 text-indigo-300 dark:text-indigo-600 group-hover:text-indigo-500 dark:group-hover:text-indigo-400 transition-colors" />
                      <span className="text-[9px] text-indigo-300 dark:text-indigo-600 group-hover:text-indigo-500 dark:group-hover:text-indigo-400 transition-colors">Add item</span>
                    </button>
                  </div>
                </DisplayCard>
              );
            }

            // ── Normal card ──────────────────────────────────────────────────
            return (
              <DisplayCard
                key={param.name}
                className={classNames(
                  'relative rounded-xl p-2.5 transition-all duration-150 cursor-pointer shadow-sm',
                  isFocused
                    ? 'shadow-md ring-2 ring-blue-400/40 bg-white dark:bg-gray-800'
                    : isModified
                      ? 'bg-amber-50/60 dark:bg-amber-900/20 shadow hover:shadow-md hover:bg-amber-100/60 dark:hover:bg-amber-900/30'
                      : param.required && isEmpty
                        ? 'bg-red-50/40 dark:bg-red-900/15 shadow-sm hover:shadow hover:bg-red-100/40 dark:hover:bg-red-900/25'
                        : `${ts.cardBg} ${ts.cardHover}`
                )}
                BgIcon={BgIcon}
                bgIconColor={ts.bgIconColor}
                showBgIcon={true}
                showFocusBar={isFocused}
                onClick={() => setFocusedIndex(index)}
                headerLeft={
                  <>
                    <KeyRound className={classNames('w-2.5 h-2.5 flex-shrink-0', isFocused || isModified ? 'text-gray-400 dark:text-gray-500' : ts.iconColor)} />
                    <span className="text-[11px] font-semibold text-gray-600 dark:text-gray-300 truncate leading-none">
                      {param.name}{param.required && <span className="text-red-500 ml-0.5">★</span>}
                    </span>
                  </>
                }
                headerRight={
                  <>
                    {isModified && (
                      <span className="w-1.5 h-1.5 rounded-full bg-amber-400" title="Modified" />
                    )}
                    {!hasGlobal && (
                      <button
                        type="button"
                        onClick={e => { e.stopPropagation(); handleToggleParamRef(param.name, paramRef); }}
                        title={isParamRef ? 'Using fromGlobalParam — click to use literal value' : 'Use fromGlobalParam'}
                        className={classNames(
                          'w-4 h-4 flex items-center justify-center rounded transition-colors',
                          isParamRef
                            ? 'text-teal-600 dark:text-teal-400 bg-teal-100 dark:bg-teal-900/40'
                            : 'text-gray-300 dark:text-gray-600 hover:text-teal-500 dark:hover:text-teal-400'
                        )}
                      >
                        <Globe className="w-2.5 h-2.5" />
                      </button>
                    )}
                  </>
                }
              >
                {/* Input area */}
                {isSlider && !isParamRef && (!hasGlobal || isOverride) ? (
                  <NumberSliderInput
                    value={typeof currentValue === 'number' ? currentValue : (typeof param.default === 'number' ? param.default : sliderMin)}
                    min={sliderMin}
                    max={sliderMax}
                    step={sliderStep}
                    onChange={v => handleUpdateParam(param.name, String(v), param.type)}
                  />
                ) : (
                <div className="flex items-center gap-1">
                  <TypeIcon type={param.type} hasEnum={hasEnum} colorClass={classNames('opacity-70 flex-shrink-0', isFocused || isModified ? 'text-gray-400 dark:text-gray-500' : ts.iconColor)} />
                  <div className="flex-1 min-w-0">
                    {isParamRef ? (
                      <SelectInput
                        ref={el => { inputRefs.current[index] = el; }}
                        value={paramRef ?? ''}
                        onChange={val => handleUpdateParamRef(param.name, val)}
                        options={[
                          { value: '', label: '— select global param —' },
                          ...Object.keys(globalParams).map(k => ({ value: k })),
                        ]}
                        className="w-full"
                        transparent
                      />
                    ) : hasGlobal && !isOverride ? (
                      <span className="block text-sm text-purple-500 dark:text-purple-400 italic truncate">
                        {globalValue !== undefined
                          ? String(globalValue)
                          : <span className="text-gray-300 dark:text-gray-600 not-italic text-xs">global</span>
                        }
                      </span>
                    ) : isWantType ? (
                      <SelectInput
                        ref={el => { inputRefs.current[index] = el; }}
                        value={displayValue}
                        onChange={val => handleUpdateParam(param.name, val, param.type)}
                        options={[{ value: '', label: '— select type —' }, ...wantTypeOptions]}
                        className="w-full"
                        transparent
                      />
                    ) : hasEnum ? (
                      <SelectInput
                        ref={el => { inputRefs.current[index] = el; }}
                        value={displayValue}
                        onChange={val => handleUpdateParam(param.name, val, param.type)}
                        options={(param.validation!.enum! as string[]).map(v => ({ value: String(v) }))}
                        className="w-full"
                        transparent
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
                        transparent
                      />
                    )}
                  </div>
                  {!isParamRef && currentValue !== undefined && currentValue !== '' && (
                    <CopyButton value={String(currentValue)} />
                  )}
                </div>
                )}

                {/* Type-level global param toggle button */}
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

              </DisplayCard>
            );
          })}
        {/* Extra custom params not in parameterDefinitions */}
        {extraParamKeys.map((key, ei) => {
          const extraIndex = filteredParams.length + ei;
          const isExtraFocused = focusedIndex === extraIndex;
          return (
            <DisplayCard
              key={`extra-${key}`}
              className={classNames(
                'relative rounded-xl p-2.5 transition-all duration-150 cursor-pointer shadow-sm',
                isExtraFocused
                  ? 'shadow-md ring-2 ring-blue-400/40 bg-white dark:bg-gray-800'
                  : `${BLUE_SCHEME.cardBg} ${BLUE_SCHEME.cardHover}`
              )}
              BgIcon={Type}
              bgIconColor={BLUE_SCHEME.bgIconColor}
              showBgIcon={true}
              showFocusBar={isExtraFocused}
              onClick={() => setFocusedIndex(extraIndex)}
              headerLeft={
                <>
                  <KeyRound className="w-2.5 h-2.5 flex-shrink-0 text-blue-400 dark:text-blue-500" />
                  <span className="text-[11px] font-semibold text-gray-600 dark:text-gray-300 truncate leading-none">{key}</span>
                </>
              }
              headerRight={
                <button
                  type="button"
                  onClick={e => { e.stopPropagation(); const next = { ...parameters }; delete next[key]; onChange(next); }}
                  className="w-4 h-4 flex items-center justify-center text-gray-300 dark:text-gray-600 hover:text-red-400 dark:hover:text-red-500 transition-colors"
                >
                  <X className="w-2.5 h-2.5" />
                </button>
              }
            >
              <div className="flex items-center gap-1">
                <Type className="w-2.5 h-2.5 flex-shrink-0 text-blue-400 dark:text-blue-500 opacity-70" />
                <CommitInput
                  ref={el => { inputRefs.current[extraIndex] = el as CommitInputHandle | null; }}
                  type="text"
                  value={String(parameters[key] ?? '')}
                  onChange={val => onChange({ ...parameters, [key]: val })}
                  placeholder="value"
                  className="flex-1 min-w-0"
                  multiline={false}
                  transparent
                />
                {parameters[key] !== undefined && parameters[key] !== '' && (
                  <CopyButton value={String(parameters[key])} />
                )}
              </div>
            </DisplayCard>
          );
        })}
        {/* Add param dashed card */}
        {pendingKey === null ? (
          <AddCard
            borderClass={BLUE_SCHEME.addBorder}
            iconClass={BLUE_SCHEME.addIcon}
            label="Add param"
            onClick={() => setPendingKey('')}
          />
        ) : (
          <FormCard
            borderClass={BLUE_SCHEME.formBorder}
            bgClass={BLUE_SCHEME.formBg}
            saveColorClass={BLUE_SCHEME.saveColor}
            header={<><Type className="w-2.5 h-2.5 text-blue-400" /><span className="text-[10px] text-blue-500 dark:text-blue-400 font-medium">New parameter</span></>}
            onSave={() => { if (pendingKey.trim() && !definedParamNames.has(pendingKey.trim())) { onChange({ ...parameters, [pendingKey.trim()]: '' }); setPendingKey(null); } }}
            onCancel={() => setPendingKey(null)}
            saveDisabled={!pendingKey.trim() || definedParamNames.has(pendingKey.trim())}
          >
            <input
              autoFocus
              value={pendingKey}
              onChange={e => setPendingKey(e.target.value)}
              onKeyDown={e => {
                if (e.key === 'Enter' && pendingKey.trim() && !definedParamNames.has(pendingKey.trim())) {
                  onChange({ ...parameters, [pendingKey.trim()]: '' });
                  setPendingKey(null);
                }
                if (e.key === 'Escape') setPendingKey(null);
              }}
              placeholder="key name"
              className="w-full text-xs px-2 py-1 rounded border border-blue-200 dark:border-blue-700 bg-white dark:bg-gray-800 text-gray-700 dark:text-gray-200 placeholder-gray-300 dark:placeholder-gray-600 focus:outline-none focus:ring-1 focus:ring-blue-400"
            />
          </FormCard>
        )}
      </div>

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

    </div>
  );
};

ParameterGridSection.displayName = 'ParameterGridSection';
