import React from 'react';
import { Code, Edit3 } from 'lucide-react';

interface FormYamlToggleProps {
  mode: 'form' | 'yaml';
  onModeChange: (mode: 'form' | 'yaml') => void;
  yamlContent?: string;
  title?: string;
  showPreview?: boolean;
}

/**
 * Shared toggle component for switching between form and YAML editor modes
 * Optionally displays YAML content as preview
 */
export const FormYamlToggle: React.FC<FormYamlToggleProps> = ({
  mode,
  onModeChange,
  yamlContent = '',
  title,
  showPreview = true
}) => {
  return (
    <div className={showPreview ? 'border-b border-gray-200 pb-4' : ''}>
      {/* Toggle Buttons */}
      <div className="flex items-center justify-between">
        {title && <h3 className="text-sm font-medium text-gray-700">{title}</h3>}
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
      </div>

      {/* YAML Preview - Optional */}
      {showPreview && (
        <div className="bg-gray-50 rounded border border-gray-200 p-3">
          <pre className="text-xs text-gray-700 overflow-x-auto max-h-32">
            {yamlContent || '# No content'}
          </pre>
        </div>
      )}
    </div>
  );
};
