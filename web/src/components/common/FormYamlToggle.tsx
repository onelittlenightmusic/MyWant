import React from 'react';
import { Code, Edit3 } from 'lucide-react';

interface FormYamlToggleProps {
  mode: 'form' | 'yaml';
  onModeChange: (mode: 'form' | 'yaml') => void;
}

/**
 * Shared toggle component for switching between form and YAML editor modes
 * Minimal control module with just the toggle buttons
 */
export const FormYamlToggle: React.FC<FormYamlToggleProps> = ({
  mode,
  onModeChange
}) => {
  return (
    <div className="flex items-center justify-center space-x-1 bg-gray-100 rounded-lg p-1">
      <button
        type="button"
        onClick={() => onModeChange('form')}
        className={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
          mode === 'form'
            ? 'bg-white text-blue-600 shadow-sm'
            : 'text-gray-600 hover:text-gray-900'
        }`}
        title="Edit using form"
      >
        <Edit3 className="w-4 h-4" />
        Form
      </button>
      <button
        type="button"
        onClick={() => onModeChange('yaml')}
        className={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
          mode === 'yaml'
            ? 'bg-white text-blue-600 shadow-sm'
            : 'text-gray-600 hover:text-gray-900'
        }`}
        title="Edit as YAML"
      >
        <Code className="w-4 h-4" />
        YAML
      </button>
    </div>
  );
};
