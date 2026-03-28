import React, { useState, useEffect, useCallback } from 'react';
import { StickyNote, ChevronDown, ChevronRight, Copy, Check, Eraser, Settings, Plus, Trash2, Save } from 'lucide-react';
import { apiClient } from '@/api/client';
import { DetailsSidebar } from './DetailsSidebar';
import { ConfirmationBubble } from '@/components/notifications/ConfirmationBubble';

// --- Shared state renderers (mirrors WantDetailsSidebar) ---

const CopyValueButton: React.FC<{ value: string }> = ({ value }) => {
  const [copied, setCopied] = useState(false);
  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation();
    navigator.clipboard.writeText(value);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };
  return (
    <button
      onClick={handleCopy}
      className="opacity-0 group-hover/kv:opacity-100 flex-shrink-0 p-0.5 rounded hover:bg-gray-200 dark:hover:bg-gray-700 transition-opacity"
      title="Copy value"
    >
      {copied ? <Check className="w-3.5 h-3.5 text-green-500" /> : <Copy className="w-3.5 h-3.5 text-gray-400" />}
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

const SettingsTab: React.FC = () => {
  const [rows, setRows] = useState<ParamRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchParams = useCallback(async () => {
    try {
      const res = await apiClient.getGlobalParameters();
      const params = res.parameters || {};
      setRows(
        Object.entries(params).map(([key, value]) => ({
          key,
          value: typeof value === 'object' ? JSON.stringify(value) : String(value ?? ''),
        }))
      );
    } catch (e) {
      console.error('Failed to fetch global parameters:', e);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchParams();
  }, [fetchParams]);

  const handleKeyChange = (index: number, newKey: string) => {
    setRows(prev => prev.map((r, i) => i === index ? { ...r, key: newKey } : r));
  };

  const handleValueChange = (index: number, newValue: string) => {
    setRows(prev => prev.map((r, i) => i === index ? { ...r, value: newValue } : r));
  };

  const handleAddRow = () => {
    setRows(prev => [...prev, { key: '', value: '' }]);
  };

  const handleDeleteRow = (index: number) => {
    setRows(prev => prev.filter((_, i) => i !== index));
  };

  const handleSave = async () => {
    setSaving(true);
    setError(null);
    try {
      const parameters: Record<string, unknown> = {};
      for (const row of rows) {
        const k = row.key.trim();
        if (!k) continue;
        // Try to parse as JSON for structured values, otherwise keep as string
        try {
          parameters[k] = JSON.parse(row.value);
        } catch {
          parameters[k] = row.value;
        }
      }
      await apiClient.updateGlobalParameters(parameters);
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
    } catch (e: any) {
      setError(e?.message || 'Failed to save parameters');
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return <div className="text-center py-12 text-gray-500 dark:text-gray-400">Loading parameters...</div>;
  }

  return (
    <div className="space-y-3">
      <div className="border border-gray-200 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-800 bg-opacity-50 overflow-hidden p-3 sm:p-4">
        <div className="flex items-center justify-between mb-3">
          <h4 className="text-sm sm:text-base font-medium text-gray-900 dark:text-white">Global Parameters</h4>
          <div className="flex items-center gap-1.5">
            <button
              onClick={handleAddRow}
              className="flex items-center gap-1 px-2 py-1 text-xs rounded-md border border-gray-300 dark:border-gray-600 text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
              title="Add parameter"
            >
              <Plus className="h-3 w-3" />
              Add
            </button>
            <button
              onClick={handleSave}
              disabled={saving}
              className="flex items-center gap-1 px-2 py-1 text-xs rounded-md bg-blue-600 hover:bg-blue-700 text-white transition-colors disabled:opacity-50"
              title="Save parameters"
            >
              {saved ? <Check className="h-3 w-3" /> : <Save className="h-3 w-3" />}
              {saved ? 'Saved' : saving ? 'Saving...' : 'Save'}
            </button>
          </div>
        </div>

        {error && (
          <div className="mb-2 px-2 py-1.5 rounded bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-400 text-xs">
            {error}
          </div>
        )}

        {rows.length === 0 ? (
          <div className="text-center py-6">
            <Settings className="h-8 w-8 text-gray-400 mx-auto mb-2" />
            <p className="text-xs text-gray-500 dark:text-gray-400">No parameters yet</p>
            <p className="text-xs text-gray-400 dark:text-gray-500 mt-1">Click "Add" to create a parameter</p>
          </div>
        ) : (
          <div className="space-y-2">
            {rows.map((row, index) => (
              <div key={index} className="flex items-center gap-2">
                <input
                  type="text"
                  value={row.key}
                  onChange={e => handleKeyChange(index, e.target.value)}
                  placeholder="key"
                  className="w-2/5 px-2 py-1 text-xs rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-1 focus:ring-blue-500"
                />
                <input
                  type="text"
                  value={row.value}
                  onChange={e => handleValueChange(index, e.target.value)}
                  placeholder="value"
                  className="flex-1 px-2 py-1 text-xs rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-1 focus:ring-blue-500"
                />
                <button
                  onClick={() => handleDeleteRow(index)}
                  className="flex-shrink-0 p-1 rounded text-gray-400 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 transition-colors"
                  title="Delete"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              </div>
            ))}
          </div>
        )}

        <p className="mt-3 text-[10px] text-gray-400 dark:text-gray-500">
          Saved to ~/.mywant/parameters.yaml · JSON values are parsed automatically
        </p>
      </div>
    </div>
  );
};

// --- GlobalStateSidebar component ---

const SECTION_CONTAINER_CLASS = 'border border-gray-200 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-800 bg-opacity-50 overflow-hidden p-3 sm:p-4';

const TABS = [
  { id: 'results', label: 'Results', icon: StickyNote },
  { id: 'settings', label: 'Settings', icon: Settings },
];

export const GlobalStateSidebar: React.FC = () => {
  const [activeTab, setActiveTab] = useState('results');
  const [globalState, setGlobalState] = useState<Record<string, unknown>>({});
  const [timestamp, setTimestamp] = useState<string>('');
  const [loading, setLoading] = useState(true);
  const [showClearConfirmation, setShowClearConfirmation] = useState(false);

  const fetchGlobalState = useCallback(async () => {
    try {
      const res = await apiClient.getGlobalState();
      setGlobalState(res.state || {});
      setTimestamp(res.timestamp);
    } catch (e) {
      console.error('Failed to fetch global state:', e);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchGlobalState();
    const interval = setInterval(fetchGlobalState, 3000);
    return () => clearInterval(interval);
  }, [fetchGlobalState]);

  const handleClearGlobalState = async () => {
    try {
      await apiClient.deleteGlobalState();
      await fetchGlobalState();
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
        subtitle={subtitleText}
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
        badge={
          <div className="flex items-center justify-between w-full">
            <StickyNote className="h-5 w-5 text-green-500" />
            {activeTab === 'results' && hasState && (
              <button
                onClick={() => setShowClearConfirmation(true)}
                className="p-1.5 rounded-md text-gray-400 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 transition-colors"
                title="Clear all memo data"
              >
                <Eraser className="h-4 w-4" />
              </button>
            )}
          </div>
        }
        tabs={TABS}
        defaultTab="results"
        onTabChange={setActiveTab}
      >
        <div className="px-3 sm:px-4 py-4 sm:py-8 h-full overflow-y-auto">
          {activeTab === 'results' && (
            loading ? (
              <div className="text-center py-12 text-gray-500 dark:text-gray-400">Loading memo...</div>
            ) : hasState ? (
              <div className="space-y-2">
                <div className={SECTION_CONTAINER_CLASS}>
                  <h4 className="text-sm sm:text-base font-medium text-gray-900 dark:text-white mb-2 sm:mb-4">Memo</h4>
                  <div className="space-y-3 sm:space-y-4">
                    {renderKeyValuePairs(globalState)}
                  </div>
                </div>
              </div>
            ) : (
              <div className="text-center py-12">
                <StickyNote className="h-12 w-12 text-gray-400 mx-auto mb-4" />
                <p className="text-gray-500 dark:text-gray-400">No memo data</p>
                <p className="text-xs text-gray-400 dark:text-gray-500 mt-2">
                  Memo will appear here once wants store values via StoreGlobalState
                </p>
              </div>
            )
          )}

          {activeTab === 'settings' && <SettingsTab />}
        </div>
    </DetailsSidebar>
  );
};
