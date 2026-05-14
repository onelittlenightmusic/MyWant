import React, { useState, useEffect, useCallback, useRef } from 'react';
import { StickyNote, ChevronDown, ChevronRight, Copy, Check, Eraser, SlidersHorizontal, Plus, Save, BarChart3, Radar, Type, X, KeyRound } from 'lucide-react';
import { apiClient } from '@/api/client';
import { useCardGridNavigation } from '@/hooks/useCardGridNavigation';
import { classNames } from '@/utils/helpers';
import { useDebugStore } from '@/stores/debugStore';
import { DetailsSidebar } from './DetailsSidebar';
import { ConfirmationBubble } from '@/components/notifications/ConfirmationBubble';
import { SummarySidebarContent, SummarySidebarContentProps } from '@/components/sidebar/SummarySidebarContent';
import { DisplayCard, AddCard, BLUE_SCHEME } from '@/components/forms/CardPrimitives';

// --- Shared state renderers (mirrors WantDetailsSidebar) ---

const CopyValueButton: React.FC<{ value: string }> = ({ value }) => {
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      onClick={e => { e.stopPropagation(); navigator.clipboard.writeText(value); setCopied(true); setTimeout(() => setCopied(false), 1500); }}
      title="Copy value"
      className="w-4 h-4 flex items-center justify-center text-gray-300 dark:text-gray-600 hover:text-blue-400 dark:hover:text-blue-500 transition-colors flex-shrink-0"
    >
      {copied ? <Check className="w-2.5 h-2.5 text-green-500" /> : <Copy className="w-2.5 h-2.5" />}
    </button>
  );
};

const CollapsibleArray: React.FC<{ label: string; items: any[]; depth: number }> = ({ label, items, depth }) => {
  const [isExpanded, setIsExpanded] = useState(false);
  return (
    <div className="space-y-1">
      <button
        onClick={() => setIsExpanded(!isExpanded)}
        className="flex items-center gap-2 font-medium text-gray-800 dark:text-gray-100 text-sm hover:text-gray-900 dark:hover:text-gray-300 py-1"
      >
        {isExpanded ? (
          <ChevronDown className="h-4 w-4 text-gray-500 dark:text-gray-400" />
        ) : (
          <ChevronRight className="h-4 w-4 text-gray-500 dark:text-gray-400" />
        )}
        {label}:
        {!isExpanded && <span className="text-xs text-gray-400 dark:text-gray-500 ml-1">Array({items.length})</span>}
      </button>
      {isExpanded && (
        <div className="ml-4 border-l border-gray-200 dark:border-gray-700 pl-3 space-y-2">
          {items.map((item, index) => (
            <ArrayItemRenderer key={index} item={item} index={index} depth={depth + 1} />
          ))}
        </div>
      )}
    </div>
  );
};

const ArrayItemRenderer: React.FC<{ item: any; index: number; depth: number }> = ({ item, index, depth }) => {
  const [isExpanded, setIsExpanded] = useState(false);
  const isNested = item !== null && typeof item === 'object';

  if (!isNested) {
    return (
      <div className="text-sm text-gray-700 dark:text-gray-200 font-mono ml-4">
        [{index}]: {String(item)}
      </div>
    );
  }

  return (
    <div className="border-l border-gray-300 dark:border-gray-600 pl-3 ml-2">
      <button
        onClick={() => setIsExpanded(!isExpanded)}
        className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200 hover:text-gray-900 dark:hover:text-gray-300 py-1"
      >
        {isExpanded ? (
          <ChevronDown className="h-4 w-4 text-gray-500 dark:text-gray-400" />
        ) : (
          <ChevronRight className="h-4 w-4 text-gray-500 dark:text-gray-400" />
        )}
        <span className="text-xs text-gray-500 dark:text-gray-400">[{index}]</span>
        {!isExpanded && (
          <span className="text-xs text-gray-400 dark:text-gray-500">
            {Array.isArray(item) ? `Array(${item.length})` : 'Object'}
          </span>
        )}
      </button>
      {isExpanded && (
        <div className="mt-2 ml-2 space-y-2">
          {renderKeyValuePairs(item, depth + 1)}
        </div>
      )}
    </div>
  );
};

const renderKeyValuePairs = (obj: any, depth: number = 0): React.ReactNode[] => {
  const items: React.ReactNode[] = [];

  if (obj === null || obj === undefined) {
    return [<span key="null" className="text-gray-500 dark:text-gray-400 italic">null</span>];
  }

  if (typeof obj !== 'object') {
    return [
      <span key="value" className="text-gray-700 dark:text-gray-200 font-mono break-all">
        {String(obj)}
      </span>
    ];
  }

  if (Array.isArray(obj)) {
    return [
      <div key="array" className="space-y-2">
        {obj.map((item, index) => (
          <ArrayItemRenderer key={index} item={item} index={index} depth={depth} />
        ))}
      </div>
    ];
  }

  Object.entries(obj).forEach(([key, value]) => {
    const isNested = value !== null && typeof value === 'object';
    const isArray = Array.isArray(value);

    if (isArray) {
      items.push(
        <CollapsibleArray key={key} label={key} items={value as any[]} depth={depth} />
      );
    } else if (isNested) {
      items.push(
        <div key={key} className="space-y-1">
          <div className="font-medium text-gray-800 dark:text-gray-100 text-xs sm:text-sm">{key}:</div>
          <div className="ml-2 sm:ml-4 border-l border-gray-200 dark:border-gray-700 pl-2 sm:pl-3">
            {renderKeyValuePairs(value, depth + 1)}
          </div>
        </div>
      );
    } else {
      const valueStr = String(value);
      const displayKey = key.length > 25 ? key.slice(0, 25) + '~' : key;
      const keyLength = displayKey.length;
      const valueLength = valueStr.length;
      const minDots = 3;
      const totalAvailableChars = 50;
      const dotsNeeded = Math.max(minDots, totalAvailableChars - keyLength - valueLength);
      const dots = '.'.repeat(Math.max(minDots, Math.min(dotsNeeded, 30)));

      items.push(
        <div key={key} className="flex justify-between items-center text-xs sm:text-sm gap-2 group/kv">
          <span className="text-gray-600 dark:text-gray-300 font-normal text-[10px] sm:text-xs whitespace-nowrap flex-shrink-0" title={key}>{displayKey}</span>
          <div className="flex items-center gap-1 min-w-0">
            <span className="text-gray-400 dark:text-gray-500 text-[10px] sm:text-xs flex-shrink-0">{dots}</span>
            <span className="text-gray-800 dark:text-gray-100 font-semibold text-sm sm:text-base truncate group-hover/kv:whitespace-normal group-hover/kv:break-all group-hover/kv:overflow-visible" title={valueStr}>{valueStr}</span>
            <CopyValueButton value={valueStr} />
          </div>
        </div>
      );
    }
  });

  return items;
};

// --- Settings (Global Parameters) tab ---

interface ParamRow {
  key: string;
  value: string;
}

interface SettingsTabProps {
  parameters: Record<string, unknown>;
  onUpdate: (parameters: Record<string, unknown>) => Promise<void>;
  loading?: boolean;
  isActive?: boolean;
  onTabForward?: () => void;
  onTabBackward?: () => void;
}

const SETTINGS_COLS = 2;

const SettingsTab: React.FC<SettingsTabProps> = ({
  parameters, onUpdate, loading, isActive, onTabForward, onTabBackward,
}) => {
  const [rows, setRows] = useState<ParamRow[]>([]);
  const [isEditing, setIsEditing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [focusedIndex, setFocusedIndex] = useState(-1);
  const rowsRef = useRef<ParamRow[]>([]);
  rowsRef.current = rows;
  const valueRefs = useRef<Array<HTMLInputElement | null>>([]);

  // Auto-focus first card when this tab becomes active
  useEffect(() => {
    if (isActive && focusedIndex < 0 && rowsRef.current.length > 0) setFocusedIndex(0);
    if (!isActive) setFocusedIndex(-1);
  }, [isActive]); // eslint-disable-line react-hooks/exhaustive-deps

  const { gridProps } = useCardGridNavigation({
    count: rows.length,
    cols: SETTINGS_COLS,
    isActive: isActive !== false,
    focusedIndex,
    setFocusedIndex,
    onConfirm: (i) => valueRefs.current[i]?.focus(),
    onTabForward,
    onTabBackward,
  });

  // Update local rows when parameters prop changes, but only if not currently editing
  useEffect(() => {
    if (!isEditing) {
      setRows(
        Object.entries(parameters).map(([key, value]) => ({
          key,
          value: typeof value === 'object' ? JSON.stringify(value) : String(value ?? ''),
        }))
      );
    }
  }, [parameters, isEditing]);

  const handleKeyChange = (index: number, newKey: string) => {
    setRows(prev => prev.map((r, i) => i === index ? { ...r, key: newKey } : r));
  };

  const handleValueChange = (index: number, newValue: string) => {
    setRows(prev => prev.map((r, i) => i === index ? { ...r, value: newValue } : r));
  };

  const handleAddRow = () => {
    setRows(prev => [...prev, { key: '', value: '' }]);
    setIsEditing(true);
  };

  const handleDeleteRow = (index: number) => {
    setRows(prev => prev.filter((_, i) => i !== index));
    setIsEditing(true);
  };

  const handleSave = async () => {
    setSaving(true);
    setError(null);
    try {
      const newParams: Record<string, unknown> = {};
      for (const row of rows) {
        const k = row.key.trim();
        if (!k) continue;
        try {
          newParams[k] = JSON.parse(row.value);
        } catch {
          newParams[k] = row.value;
        }
      }
      await onUpdate(newParams);
      setSaved(true);
      setIsEditing(false);
      setTimeout(() => setSaved(false), 2000);
    } catch (e: any) {
      setError(e?.message || 'Failed to save parameters');
    } finally {
      setSaving(false);
    }
  };

  const handleCancel = () => {
    setRows(
      Object.entries(parameters).map(([key, value]) => ({
        key,
        value: typeof value === 'object' ? JSON.stringify(value) : String(value ?? ''),
      }))
    );
    setIsEditing(false);
    setError(null);
  };

  if (loading && Object.keys(parameters).length === 0) {
    return <div className="text-center py-12 text-gray-500 dark:text-gray-400">Loading parameters...</div>;
  }

  return (
    <div className="space-y-3">
      <div className="border border-gray-200 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-800 bg-opacity-50 overflow-hidden p-3 sm:p-4">
        {/* Header */}
        <div className="flex items-center justify-between mb-3">
          <div className="flex items-center gap-2">
            <h4 className="text-sm sm:text-base font-medium text-gray-900 dark:text-white">Global Parameters</h4>
            {loading && <div className="w-3 h-3 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />}
          </div>
          {isEditing && (
            <div className="flex items-center gap-1.5">
              <button
                onClick={handleCancel}
                className="flex items-center gap-1 px-2 py-1 text-xs rounded-md border border-gray-300 dark:border-gray-600 text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={handleSave}
                disabled={saving}
                className="flex items-center gap-1 px-2 py-1 text-xs rounded-md bg-blue-600 hover:bg-blue-700 text-white transition-colors disabled:opacity-50"
              >
                {saved ? <Check className="h-3 w-3" /> : <Save className="h-3 w-3" />}
                {saved ? 'Saved' : saving ? 'Saving...' : 'Save'}
              </button>
            </div>
          )}
        </div>

        {error && (
          <div className="mb-2 px-2 py-1.5 rounded bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-400 text-xs">
            {error}
          </div>
        )}

        {/* Card grid */}
        <div {...gridProps} className="grid grid-cols-2 gap-2 outline-none">
          {rows.map((row, index) => (
            <DisplayCard
              key={index}
              className={classNames(
                'relative rounded-xl p-2.5 transition-all duration-150 cursor-pointer shadow-sm',
                focusedIndex === index
                  ? 'shadow-md ring-2 ring-blue-400/40 bg-white dark:bg-gray-800'
                  : `${BLUE_SCHEME.cardBg} ${BLUE_SCHEME.cardHover}`
              )}
              BgIcon={Type}
              bgIconColor={BLUE_SCHEME.bgIconColor}
              showBgIcon={focusedIndex !== index}
              showFocusBar={focusedIndex === index}
              onClick={() => setFocusedIndex(index)}
              headerLeft={
                <>
                  <KeyRound className={classNames('w-2.5 h-2.5 flex-shrink-0', BLUE_SCHEME.iconColor)} />
                  <input
                    type="text"
                    value={row.key}
                    onChange={e => { handleKeyChange(index, e.target.value); setIsEditing(true); }}
                    placeholder="key"
                    className="text-[11px] font-semibold bg-transparent text-gray-600 dark:text-gray-300 w-full outline-none truncate placeholder-gray-300 dark:placeholder-gray-600"
                  />
                </>
              }
              headerRight={
                <button
                  onClick={e => { e.stopPropagation(); handleDeleteRow(index); }}
                  className="opacity-0 group-hover:opacity-100 w-4 h-4 flex items-center justify-center text-gray-300 dark:text-gray-600 hover:text-red-400 dark:hover:text-red-500 transition-all"
                >
                  <X className="w-2.5 h-2.5" />
                </button>
              }
            >
              <div className="flex items-center gap-1">
                <Type className="w-2.5 h-2.5 flex-shrink-0 text-blue-400 dark:text-blue-500 opacity-70" />
                <input
                  ref={el => { valueRefs.current[index] = el; }}
                  type="text"
                  value={row.value}
                  onChange={e => { handleValueChange(index, e.target.value); setIsEditing(true); }}
                  placeholder="value"
                  className="flex-1 min-w-0 text-xs px-1.5 py-1 rounded border border-blue-100 dark:border-blue-900/40 bg-white/80 dark:bg-gray-900/40 text-gray-700 dark:text-gray-200 placeholder-gray-300 dark:placeholder-gray-600 focus:outline-none focus:ring-1 focus:ring-blue-400"
                />
                {row.value && <CopyValueButton value={row.value} />}
              </div>
            </DisplayCard>
          ))}
          {/* Add param dashed card */}
          <AddCard
            borderClass={BLUE_SCHEME.addBorder}
            iconClass={BLUE_SCHEME.addIcon}
            label="Add param"
            onClick={handleAddRow}
          />
        </div>

        <p className="mt-3 text-[10px] text-gray-400 dark:text-gray-500">
          Saved to ~/.mywant/parameters.yaml · JSON values are parsed automatically
        </p>
      </div>
    </div>
  );
};

// --- GlobalStateSidebar component ---

const SECTION_CONTAINER_CLASS = 'border border-gray-200 dark:border-gray-700 rounded-lg bg-gray-100 dark:bg-gray-900 overflow-hidden p-2 sm:p-4';

const TABS = [
  { id: 'results', label: 'Memo', icon: StickyNote },
  { id: 'stats', label: 'Stats', icon: BarChart3 },
  { id: 'settings', label: 'Params', icon: SlidersHorizontal },
];

interface GlobalStateSidebarProps {
  summaryProps?: SummarySidebarContentProps;
  radarMode?: boolean;
  onRadarModeToggle?: () => void;
}

export const GlobalStateSidebar: React.FC<GlobalStateSidebarProps> = ({ summaryProps, radarMode, onRadarModeToggle }) => {
  const pollingIntervalMs = useDebugStore(state => state.pollingIntervalMs);

  const [activeTab, setActiveTab] = useState('results');
  const [globalState, setGlobalState] = useState<Record<string, unknown>>({});
  const [globalParams, setGlobalParams] = useState<Record<string, unknown>>({});
  const [timestamp, setTimestamp] = useState<string>('');
  const [loading, setLoading] = useState(true);
  const [paramsLoading, setParamsLoading] = useState(true);
  const [showClearConfirmation, setShowClearConfirmation] = useState(false);
  const stateETagRef = useRef<string | undefined>(undefined);
  const paramsETagRef = useRef<string | undefined>(undefined);

  const fetchData = useCallback(async () => {
    try {
      const [stateResult, paramsResult] = await Promise.all([
        apiClient.getGlobalStateConditional(stateETagRef.current),
        apiClient.getGlobalParametersConditional(paramsETagRef.current),
      ]);
      if (stateResult.data !== null) {
        setGlobalState(stateResult.data.state || {});
        setTimestamp(stateResult.data.timestamp);
        stateETagRef.current = stateResult.etag;
      }
      if (paramsResult.data !== null) {
        setGlobalParams(paramsResult.data.parameters || {});
        paramsETagRef.current = paramsResult.etag;
      }
    } catch (e) {
      console.error('Failed to fetch global data:', e);
    } finally {
      setLoading(false);
      setParamsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchData();
    const interval = setInterval(fetchData, pollingIntervalMs);
    return () => clearInterval(interval);
  }, [fetchData, pollingIntervalMs]);

  const handleUpdateParameters = async (parameters: Record<string, unknown>) => {
    await apiClient.updateGlobalParameters(parameters);
    setGlobalParams(parameters); // Optimistic update
  };

  const handleClearGlobalState = async () => {
    try {
      await apiClient.deleteGlobalState();
      setGlobalState({});
    } catch (e) {
      console.error('Failed to clear global state:', e);
    } finally {
      setShowClearConfirmation(false);
    }
  };

  const hasState = Object.keys(globalState).length > 0;
  const subtitleText = timestamp
    ? `Updated: ${new Date(timestamp).toLocaleTimeString()}`
    : undefined;

  return (
    <DetailsSidebar
        title="Memo"
        headerOverlay={
          <ConfirmationBubble
            isVisible={showClearConfirmation}
            onConfirm={handleClearGlobalState}
            onCancel={() => setShowClearConfirmation(false)}
            onDismiss={() => setShowClearConfirmation(false)}
            title="Clear Memo"
            layout="header-overlay"
          />
        }
        tabs={TABS}
        defaultTab="results"
        onTabChange={setActiveTab}
      >
        <div className="flex flex-col h-full">
          {/* Action Bar / Status Info */}
          <div className="flex-shrink-0 px-3 sm:px-6 py-1.5 sm:py-2 flex items-center justify-between border-b border-gray-100 dark:border-gray-800 bg-gray-50/30 dark:bg-gray-900/30">
            <span className="text-[10px] sm:text-xs text-gray-500 dark:text-gray-400 font-medium italic">
              {subtitleText || 'Loading...'}
            </span>
            {activeTab === 'results' && hasState && (
              <button
                onClick={() => setShowClearConfirmation(true)}
                className="p-1 sm:p-1.5 rounded-md text-gray-400 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 transition-all flex items-center gap-1.5 group"
                title="Clear all memo data"
              >
                <Eraser className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
                <span className="text-[10px] font-bold uppercase tracking-tighter hidden sm:block">Clear</span>
              </button>
            )}
          </div>

          <div className="flex-1 px-3 sm:px-4 pt-0 pb-3 sm:py-4 h-full overflow-y-auto">
            {activeTab === 'results' && (
              loading ? (
                <div className="text-center py-12 text-gray-500 dark:text-gray-400">
                  <div className="w-8 h-8 border-4 border-blue-500 border-t-transparent rounded-full animate-spin mx-auto mb-4" />
                  Loading memo...
                </div>
              ) : hasState ? (
                <div className="space-y-2">
                  <div className={SECTION_CONTAINER_CLASS}>
                    <h4 className="text-xs sm:text-base font-medium text-gray-900 dark:text-white mb-1 sm:mb-3">Memo</h4>
                    <div className="space-y-0.5 sm:space-y-2">
                      {renderKeyValuePairs(globalState)}
                    </div>
                  </div>
                </div>
              ) : (
                <div className="text-center py-12">
                  <StickyNote className="h-12 w-12 text-gray-400 mx-auto mb-4" />
                  <p className="text-gray-500 dark:text-gray-400">No memo data</p>
                  <p className="text-xs text-gray-400 dark:text-gray-500 mt-2 px-6">
                    Memo will appear here once wants store values via StoreGlobalState
                  </p>
                </div>
              )
            )}

            {activeTab === 'stats' && summaryProps && (
              <SummarySidebarContent {...summaryProps} />
            )}

            {activeTab === 'settings' && (
              <div className="space-y-4">
                {/* Radar toggle */}
                {onRadarModeToggle && (
                  <div className="flex items-center justify-between border border-gray-200 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-800 bg-opacity-50 px-3 py-2">
                    <div className="flex items-center gap-2">
                      <Radar className="h-4 w-4 text-orange-500" />
                      <span className="text-sm font-medium text-gray-900 dark:text-white">Correlation Radar</span>
                    </div>
                    <button
                      onClick={onRadarModeToggle}
                      className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors focus:outline-none ${radarMode ? 'bg-orange-500' : 'bg-gray-200 dark:bg-gray-700'}`}
                    >
                      <span className={`inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform ${radarMode ? 'translate-x-6' : 'translate-x-1'}`} />
                    </button>
                  </div>
                )}
                <SettingsTab
                  parameters={globalParams}
                  onUpdate={handleUpdateParameters}
                  loading={paramsLoading}
                  isActive={activeTab === 'settings'}
                  onTabForward={() => {
                    const idx = TABS.findIndex(t => t.id === activeTab);
                    setActiveTab(TABS[(idx + 1) % TABS.length].id);
                  }}
                  onTabBackward={() => {
                    const idx = TABS.findIndex(t => t.id === activeTab);
                    setActiveTab(TABS[(idx - 1 + TABS.length) % TABS.length].id);
                  }}
                />
              </div>
            )}
          </div>
        </div>
    </DetailsSidebar>
  );
};
