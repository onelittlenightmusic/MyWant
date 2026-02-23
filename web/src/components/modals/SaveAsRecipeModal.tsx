import React, { useState, useEffect } from 'react';
import { Database } from 'lucide-react';
import { BaseModal } from './BaseModal';
import { RecipeMetadata, StateDef, WantRecipeAnalysis } from '@/types/recipe';
import { Want } from '@/types/want';

interface SaveAsRecipeModalProps {
  isOpen: boolean;
  want: Want | null;
  analysis: WantRecipeAnalysis | null;
  onClose: () => void;
  onSave: (metadata: RecipeMetadata, state: StateDef[]) => Promise<void>;
  loading?: boolean;
}

const CATEGORY_OPTIONS = ['general', 'approval', 'travel', 'mathematics', 'queue'];

export const SaveAsRecipeModal: React.FC<SaveAsRecipeModalProps> = ({
  isOpen,
  want,
  analysis,
  onClose,
  onSave,
  loading = false,
}) => {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [version, setVersion] = useState('1.0.0');
  const [customType, setCustomType] = useState('');
  const [category, setCategory] = useState('general');
  const [isSaving, setIsSaving] = useState(false);

  // Populate form when want/analysis changes
  useEffect(() => {
    if (want && isOpen) {
      setName(analysis?.suggestedMetadata?.name || `${want.metadata?.name || 'want'}-recipe`);
      setDescription('');
      setVersion('1.0.0');
      setCustomType('');
      setCategory('general');
    }
  }, [want, analysis, isOpen]);

  const handleSave = async () => {
    if (!name.trim()) return;
    setIsSaving(true);
    try {
      const metadata: RecipeMetadata = {
        name: name.trim(),
        description: description.trim() || undefined,
        version: version.trim() || '1.0.0',
        custom_type: customType.trim() || undefined,
        category: category || undefined,
      };
      await onSave(metadata, analysis?.recommendedState || []);
    } finally {
      setIsSaving(false);
    }
  };

  const footer = (
    <div className="flex justify-end gap-3">
      <button
        onClick={onClose}
        disabled={isSaving || loading}
        className="px-4 py-2 text-sm font-medium text-gray-700 dark:text-gray-200 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700 disabled:opacity-50 transition-colors"
      >
        Cancel
      </button>
      <button
        onClick={handleSave}
        disabled={!name.trim() || isSaving || loading}
        className="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50 transition-colors"
      >
        {isSaving ? 'Saving...' : 'Save Recipe'}
      </button>
    </div>
  );

  return (
    <BaseModal
      isOpen={isOpen}
      onClose={onClose}
      title="Save as Recipe"
      footer={footer}
      size="md"
    >
      <div className="space-y-4">
        {/* Metadata fields */}
        <div>
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
            Name <span className="text-red-500">*</span>
          </label>
          <input
            type="text"
            value={name}
            onChange={e => setName(e.target.value)}
            className="w-full px-3 py-2 text-sm border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
            placeholder="my-workflow-recipe"
          />
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
            Description
          </label>
          <input
            type="text"
            value={description}
            onChange={e => setDescription(e.target.value)}
            className="w-full px-3 py-2 text-sm border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
            placeholder="Optional description"
          />
        </div>

        <div className="flex gap-3">
          <div className="flex-1">
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Version
            </label>
            <input
              type="text"
              value={version}
              onChange={e => setVersion(e.target.value)}
              className="w-full px-3 py-2 text-sm border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
              placeholder="1.0.0"
            />
          </div>
          <div className="flex-1">
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Category
            </label>
            <select
              value={category}
              onChange={e => setCategory(e.target.value)}
              className="w-full px-3 py-2 text-sm border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
            >
              {CATEGORY_OPTIONS.map(opt => (
                <option key={opt} value={opt}>{opt}</option>
              ))}
            </select>
          </div>
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
            Custom Type
          </label>
          <input
            type="text"
            value={customType}
            onChange={e => setCustomType(e.target.value)}
            className="w-full px-3 py-2 text-sm border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
            placeholder="e.g. my-workflow"
          />
        </div>

        {/* State Fields Section */}
        <div className="border border-gray-200 dark:border-gray-700 rounded-lg p-4 bg-gray-50 dark:bg-gray-800/50">
          <div className="flex items-center gap-2 mb-2">
            <Database className="h-4 w-4 text-gray-500 dark:text-gray-400" />
            <h4 className="text-sm font-medium text-gray-700 dark:text-gray-300">
              State Fields
            </h4>
          </div>
          {analysis && analysis.childCount > 0 && analysis.recommendedState.length > 0 ? (
            <>
              <p className="text-xs text-gray-500 dark:text-gray-400 mb-3">
                These fields are auto-detected from child wants' capabilities and will be added to the recipe's state.
              </p>
              <ul className="space-y-1">
                {analysis.recommendedState.map(field => (
                  <li key={field.name} className="flex items-start gap-2 text-xs">
                    <span className="font-mono text-blue-600 dark:text-blue-400 font-medium">{field.name}</span>
                    {field.description && (
                      <span className="text-gray-500 dark:text-gray-400">— {field.description}</span>
                    )}
                  </li>
                ))}
              </ul>
            </>
          ) : analysis && analysis.childCount === 0 ? (
            <p className="text-xs text-gray-500 dark:text-gray-400">
              No child wants found. State fields will be empty.
            </p>
          ) : (
            <p className="text-xs text-gray-500 dark:text-gray-400">
              No state fields detected.
            </p>
          )}
        </div>
      </div>
    </BaseModal>
  );
};
