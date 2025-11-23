import React, { useState, useEffect, useRef } from 'react';
import { useLabelHistoryStore } from '@/stores/labelHistoryStore';
import { ChevronDown } from 'lucide-react';

interface LabelKeyAutocompleteProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
}

export const LabelKeyAutocomplete: React.FC<LabelKeyAutocompleteProps> = ({
  value,
  onChange,
  placeholder = 'Key'
}) => {
  const [isOpen, setIsOpen] = useState(false);
  const [filteredKeys, setFilteredKeys] = useState<string[]>([]);
  const inputRef = useRef<HTMLInputElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  const { labelKeys, fetchLabels } = useLabelHistoryStore();

  // Fetch labels on mount
  useEffect(() => {
    fetchLabels();
  }, [fetchLabels]);

  // Filter labels based on input
  useEffect(() => {
    if (value.length > 0) {
      const filtered = labelKeys.filter(key =>
        key.toLowerCase().includes(value.toLowerCase())
      );
      setFilteredKeys(filtered);
      setIsOpen(filtered.length > 0);
    } else {
      setFilteredKeys(labelKeys);
      setIsOpen(false);
    }
  }, [value, labelKeys]);

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      const target = e.target as Node;
      if (containerRef.current && !containerRef.current.contains(target)) {
        setIsOpen(false);
      }
    };

    // Use click instead of mousedown to allow buttons to handle their onClick first
    document.addEventListener('click', handleClickOutside);
    return () => document.removeEventListener('click', handleClickOutside);
  }, []);

  const handleSelect = (key: string) => {
    onChange(key);
    setIsOpen(false);
  };

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    onChange(e.target.value);
  };

  const handleFocus = () => {
    if (labelKeys.length > 0) {
      setFilteredKeys(labelKeys);
      setIsOpen(true);
    }
  };

  return (
    <div ref={containerRef} className="relative w-full">
      <div className="relative">
        <input
          ref={inputRef}
          type="text"
          value={value}
          onChange={handleInputChange}
          onFocus={handleFocus}
          className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent pr-8"
          placeholder={placeholder}
        />
        {labelKeys.length > 0 && (
          <ChevronDown
            className="absolute right-2 top-1/2 transform -translate-y-1/2 w-4 h-4 text-gray-400 pointer-events-none"
          />
        )}
      </div>

      {isOpen && filteredKeys.length > 0 && (
        <div className="absolute z-10 w-full mt-1 bg-white border border-gray-300 rounded-md shadow-lg max-h-48 overflow-y-auto">
          {filteredKeys.map((key, index) => (
            <button
              key={index}
              type="button"
              onClick={() => handleSelect(key)}
              className="w-full text-left px-3 py-2 hover:bg-blue-50 focus:outline-none focus:bg-blue-50 border-b border-gray-100 last:border-b-0"
            >
              <span className="text-sm text-gray-700">{key}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
};
