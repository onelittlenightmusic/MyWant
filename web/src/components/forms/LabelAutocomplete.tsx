import React, { useState, useEffect, useRef } from 'react';
import { useLabelHistoryStore } from '@/stores/labelHistoryStore';
import { ChevronDown, X } from 'lucide-react';
import { CommitInput, CommitInputHandle } from '@/components/common/CommitInput';

interface LabelAutocompleteProps {
  keyValue: string;
  valueValue: string;
  onKeyChange: (key: string) => void;
  onValueChange: (value: string) => void;
  onRemove: () => void;
  onLeftKey?: () => void;
}

export const LabelAutocomplete = React.forwardRef<CommitInputHandle, LabelAutocompleteProps>(({
  keyValue,
  valueValue,
  onKeyChange,
  onValueChange,
  onRemove,
  onLeftKey,
}, ref) => {
  const [keyOpen, setKeyOpen] = useState(false);
  const [valueOpen, setValueOpen] = useState(false);
  const [filteredKeys, setFilteredKeys] = useState<string[]>([]);
  const [filteredValues, setFilteredValues] = useState<string[]>([]);
  const keyInputRef = useRef<CommitInputHandle>(null);
  const valueInputRef = useRef<CommitInputHandle>(null);
  const keyContainerRef = useRef<HTMLDivElement>(null);
  const valueContainerRef = useRef<HTMLDivElement>(null);

  // Expose commit methods to parent
  React.useImperativeHandle(ref, () => ({
    commit: () => {
      keyInputRef.current?.commit();
      valueInputRef.current?.commit();
    },
    getValue: () => keyInputRef.current?.getValue() || '',
    focus: () => {
      keyInputRef.current?.focus();
    }
  }));

  const { labelKeys, labelValues, fetchLabels } = useLabelHistoryStore();

  // Fetch labels on mount
  useEffect(() => {
    fetchLabels();
  }, [fetchLabels]);

  // Filter keys based on input
  useEffect(() => {
    if (keyValue.length > 0) {
      const filtered = labelKeys.filter(key =>
        key.toLowerCase().includes(keyValue.toLowerCase())
      );
      setFilteredKeys(filtered);
      setKeyOpen(filtered.length > 0);
    } else {
      setFilteredKeys(labelKeys);
      setKeyOpen(false);
    }
  }, [keyValue, labelKeys]);

  // Filter values based on input and current key
  useEffect(() => {
    if (valueValue.length > 0) {
      const keyVals = keyValue && labelValues[keyValue] ? labelValues[keyValue] : [];
      const filtered = keyVals.filter(val =>
        val.toLowerCase().includes(valueValue.toLowerCase())
      );
      setFilteredValues(filtered);
      setValueOpen(filtered.length > 0);
    } else if (keyValue && labelValues[keyValue]) {
      setFilteredValues(labelValues[keyValue]);
      setValueOpen(false);
    } else {
      setFilteredValues([]);
      setValueOpen(false);
    }
  }, [valueValue, keyValue, labelValues]);

  // Close dropdowns when clicking outside
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      const target = e.target as Node;

      if (keyContainerRef.current && !keyContainerRef.current.contains(target)) {
        setKeyOpen(false);
      }

      if (valueContainerRef.current && !valueContainerRef.current.contains(target)) {
        setValueOpen(false);
      }
    };

    document.addEventListener('click', handleClickOutside);
    return () => document.removeEventListener('click', handleClickOutside);
  }, []);

  const handleKeySelect = (key: string) => {
    onKeyChange(key);
    setKeyOpen(false);
    // Reset value when key changes
    if (keyValue !== key) {
      onValueChange('');
    }
  };

  const handleValueSelect = (value: string) => {
    onValueChange(value);
    setValueOpen(false);
  };

  const handleKeyFocus = () => {
    if (labelKeys.length > 0) {
      setFilteredKeys(labelKeys);
      setKeyOpen(true);
    }
  };

  const handleValueFocus = () => {
    if (keyValue && labelValues[keyValue]) {
      setFilteredValues(labelValues[keyValue]);
      setValueOpen(true);
    }
  };

  return (
    <div className="flex gap-2">
      {/* Key Input */}
      <div ref={keyContainerRef} className="relative flex-1">
        <div className="relative">
          <CommitInput
            ref={keyInputRef}
            type="text"
            value={keyValue}
            onChange={(val) => onKeyChange(val)}
            onFocus={handleKeyFocus}
            onBlur={() => {
              // フォーカスが外れたら少し遅延してドロップダウンを閉じる
              // （ドロップダウンのボタンクリックを可能にするため）
              setTimeout(() => setKeyOpen(false), 200);
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                // We let CommitInput handle Enter for committing, 
                // but we might want to also focus the value input.
                // However, CommitInput's onChange will be called on Enter.
                // Let's see if we can chain it.
                setTimeout(() => valueInputRef.current?.focus(), 0);
              } else if (e.key === 'Tab') {
                // Tabキーでもフォーカスを値フィールドに移動
                e.preventDefault();
                valueInputRef.current?.focus();
              } else if (e.key === 'ArrowLeft' && onLeftKey) {
                e.preventDefault();
                onLeftKey();
              }
            }}
            className="pr-8"
            placeholder="Key"
          />
          {labelKeys.length > 0 && (
            <ChevronDown
              className="absolute right-2 top-1/2 transform -translate-y-1/2 w-4 h-4 text-gray-400 pointer-events-none"
            />
          )}
        </div>

        {keyOpen && filteredKeys.length > 0 && (
          <div className="absolute z-10 w-full mt-1 bg-white border border-gray-300 rounded-md shadow-lg max-h-48 overflow-y-auto">
            {filteredKeys.map((key, index) => (
              <button
                key={index}
                type="button"
                onClick={() => handleKeySelect(key)}
                className="w-full text-left px-3 py-2 hover:bg-blue-50 focus:outline-none focus:bg-blue-50 border-b border-gray-100 last:border-b-0"
              >
                <span className="text-sm text-gray-700 font-medium">{key}</span>
              </button>
            ))}
          </div>
        )}
      </div>

      {/* Value Input */}
      <div ref={valueContainerRef} className="relative flex-1">
        <div className="relative">
          <CommitInput
            ref={valueInputRef}
            type="text"
            value={valueValue}
            onChange={(val) => onValueChange(val)}
            onFocus={handleValueFocus}
            onBlur={() => {
              // フォーカスが外れたら少し遅延してドロップダウンを閉じる
              // （ドロップダウンのボタンクリックを可能にするため）
              setTimeout(() => setValueOpen(false), 200);
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                // Enterキーで値を確定（親でDoneボタンをクリック）
                // または外側をクリックしてドロップダウンを閉じる
                setValueOpen(false);
              } else if (e.key === 'ArrowLeft' && onLeftKey) {
                e.preventDefault();
                onLeftKey();
              }
            }}
            className="pr-8"
            placeholder="Value"
            disabled={!keyValue}
          />
          {keyValue && filteredValues.length > 0 && (
            <ChevronDown
              className="absolute right-2 top-1/2 transform -translate-y-1/2 w-4 h-4 text-gray-400 pointer-events-none"
            />
          )}
        </div>

        {valueOpen && filteredValues.length > 0 && (
          <div className="absolute z-10 w-full mt-1 bg-white border border-gray-300 rounded-md shadow-lg max-h-48 overflow-y-auto">
            {filteredValues.map((value, index) => (
              <button
                key={index}
                type="button"
                onClick={() => handleValueSelect(value)}
                className="w-full text-left px-3 py-2 hover:bg-blue-50 focus:outline-none focus:bg-blue-50 border-b border-gray-100 last:border-b-0"
              >
                <span className="text-sm text-gray-700">{value}</span>
              </button>
            ))}
          </div>
        )}
      </div>

      {/* Remove Button */}
      <button
        type="button"
        onClick={onRemove}
        className="text-red-600 hover:text-red-800 p-2"
        title="Remove this label"
      >
        <X className="w-4 h-4" />
      </button>
    </div>
  );
});

LabelAutocomplete.displayName = 'LabelAutocomplete';
