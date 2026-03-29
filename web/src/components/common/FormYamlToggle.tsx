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
    <div className="flex items-center justify-end space-x-0.5 bg-gray-100 dark:bg-gray-800 rounded-2xl p-0.5">
      <button
        type="button"
        onClick={() => onModeChange('form')}
        className={`flex items-center gap-1 px-1.5 py-0.5 rounded-xl text-[11px] font-medium transition-colors ${
          mode === 'form'
            ? 'bg-white dark:bg-gray-700 text-blue-600 dark:text-blue-400 shadow-sm'
            : 'text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-200'
        }`}
        title="Edit using form"
      >
        <Edit3 className="w-2.5 h-2.5" />
        {mode === 'form' && 'Form'}
      </button>
      <button
        type="button"
        onClick={() => onModeChange('yaml')}
        className={`flex items-center gap-1 px-1.5 py-0.5 rounded-xl text-[11px] font-medium transition-colors ${
          mode === 'yaml'
            ? 'bg-white dark:bg-gray-700 text-blue-600 dark:text-blue-400 shadow-sm'
            : 'text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-200'
        }`}
        title="Edit as YAML"
      >
        <Code className="w-2.5 h-2.5" />
        {mode === 'yaml' && 'YAML'}
      </button>
    </div>
  );
};
