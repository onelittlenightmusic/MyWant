import React, { useState, useEffect, useRef } from 'react';
import { useLabelHistoryStore } from '@/stores/labelHistoryStore';
import { ChevronDown, X } from 'lucide-react';

interface LabelSelectorAutocompleteProps {
  keyValue: string;
  valuValue: string;
  onKeyChange: (key: string) => void;
  onValueChange: (value: string) => void;
  onRemove: () => void;
}

export const LabelSelectorAutocomplete: React.FC<LabelSelectorAutocompleteProps> = ({
  keyValue,
  valuValue,
  onKeyChange,
  onValueChange,
  onRemove,
}) => {
  const [keyOpen, setKeyOpen] = useState(false);
  const [valueOpen, setValueOpen] = useState(false);
  const [filteredKeys, setFilteredKeys] = useState<string[]>([]);
  const [filteredValues, setFilteredValues] = useState<string[]>([]);
  const [keySelectedIndex, setKeySelectedIndex] = useState(-1);
  const [valueSelectedIndex, setValueSelectedIndex] = useState(-1);
  const keyInputRef = useRef<HTMLInputElement>(null);
  const valueInputRef = useRef<HTMLInputElement>(null);
  const keyContainerRef = useRef<HTMLDivElement>(null);
  const valueContainerRef = useRef<HTMLDivElement>(null);

  const { labelKeys, labelValues, fetchLabels } = useLabelHistoryStore();

  // Fetch labels on mount only
  useEffect(() => {
    fetchLabels();
  }, []);

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
    setKeySelectedIndex(-1);
  }, [keyValue, labelKeys]);

  // Filter values based on input and current key
  useEffect(() => {
    if (valuValue.length > 0) {
      const keyValues = keyValue && labelValues[keyValue] ? labelValues[keyValue] : [];
      const filtered = keyValues.filter(val =>
        val.toLowerCase().includes(valuValue.toLowerCase())
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
    setValueSelectedIndex(-1);
  }, [valuValue, keyValue, labelValues]);

  // Close dropdowns when clicking outside
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      const target = e.target as HTMLElement;

      // Don't close if click is on a button (Save/Cancel)
      if (target.closest('button')) {
        return;
      }

      // Only close key dropdown if click is outside the key container
      if (keyContainerRef.current && !keyContainerRef.current.contains(target)) {
        setKeyOpen(false);
      }

      // Only close value dropdown if click is outside the value container
      if (valueContainerRef.current && !valueContainerRef.current.contains(target)) {
        setValueOpen(false);
      }
    };

    // Use click instead of mousedown to allow buttons to handle their onClick first
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
    // Auto-focus value input after key selection
    setTimeout(() => {
      valueInputRef.current?.focus();
    }, 0);
  };

  const handleValueSelect = (value: string) => {
    onValueChange(value);
    setValueOpen(false);
    // Move focus back to key input after value selection
    setTimeout(() => {
      keyInputRef.current?.focus();
    }, 0);
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
    <div className="space-y-2 border border-gray-200 rounded-md p-3">
      <div className="flex gap-2">
        {/* Key Input */}
        <div ref={keyContainerRef} className="relative flex-1">
          <div className="relative">
            <input
              ref={keyInputRef}
              type="text"
              value={keyValue}
              onChange={(e) => onKeyChange(e.target.value)}
              onFocus={handleKeyFocus}
              onKeyDown={(e) => {
                if (e.key === 'ArrowDown') {
                  e.preventDefault();
                  if (keyOpen && filteredKeys.length > 0) {
                    setKeySelectedIndex(Math.min(keySelectedIndex + 1, filteredKeys.length - 1));
                  }
                } else if (e.key === 'ArrowUp') {
                  e.preventDefault();
                  if (keyOpen && keySelectedIndex > 0) {
                    setKeySelectedIndex(keySelectedIndex - 1);
                  }
                } else if (e.key === 'Enter') {
                  e.preventDefault();
                  if (keyOpen && keySelectedIndex >= 0 && filteredKeys[keySelectedIndex]) {
                    handleKeySelect(filteredKeys[keySelectedIndex]);
                  } else {
                    valueInputRef.current?.focus();
                  }
                } else if (e.key === 'Tab') {
                  e.preventDefault();
                  if (keyOpen && keySelectedIndex >= 0 && filteredKeys[keySelectedIndex]) {
                    handleKeySelect(filteredKeys[keySelectedIndex]);
                  } else {
                    valueInputRef.current?.focus();
                  }
                }
              }}
              className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent pr-8"
              placeholder="Selector key (e.g., role, category)"
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
                  className={`w-full text-left px-3 py-2 focus:outline-none border-b border-gray-100 last:border-b-0 ${
                    index === keySelectedIndex
                      ? 'bg-blue-500 text-white'
                      : 'hover:bg-blue-50'
                  }`}
                >
                  <span className={`text-sm font-medium ${index === keySelectedIndex ? 'text-white' : 'text-gray-700'}`}>{key}</span>
                </button>
              ))}
            </div>
          )}
        </div>

        {/* Value Input */}
        <div ref={valueContainerRef} className="relative flex-1">
          <div className="relative">
            <input
              ref={valueInputRef}
              type="text"
              value={valuValue}
              onChange={(e) => onValueChange(e.target.value)}
              onFocus={handleValueFocus}
              onKeyDown={(e) => {
                if (e.key === 'ArrowDown') {
                  e.preventDefault();
                  if (valueOpen && filteredValues.length > 0) {
                    setValueSelectedIndex(Math.min(valueSelectedIndex + 1, filteredValues.length - 1));
                  }
                } else if (e.key === 'ArrowUp') {
                  e.preventDefault();
                  if (valueOpen && valueSelectedIndex > 0) {
                    setValueSelectedIndex(valueSelectedIndex - 1);
                  }
                } else if (e.key === 'Enter') {
                  e.preventDefault();
                  if (valueOpen && valueSelectedIndex >= 0 && filteredValues[valueSelectedIndex]) {
                    handleValueSelect(filteredValues[valueSelectedIndex]);
                  } else {
                    setValueOpen(false);
                  }
                } else if (e.key === 'Tab') {
                  e.preventDefault();
                  if (valueOpen && valueSelectedIndex >= 0 && filteredValues[valueSelectedIndex]) {
                    handleValueSelect(filteredValues[valueSelectedIndex]);
                  } else {
                    setValueOpen(false);
                  }
                }
              }}
              className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent pr-8"
              placeholder="Selector value"
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
                  className={`w-full text-left px-3 py-2 focus:outline-none border-b border-gray-100 last:border-b-0 ${
                    index === valueSelectedIndex
                      ? 'bg-blue-500 text-white'
                      : 'hover:bg-blue-50'
                  }`}
                >
                  <span className={`text-sm ${index === valueSelectedIndex ? 'text-white' : 'text-gray-700'}`}>{value}</span>
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
          title="Remove this dependency"
        >
          <X className="w-4 h-4" />
        </button>
      </div>
    </div>
  );
};
