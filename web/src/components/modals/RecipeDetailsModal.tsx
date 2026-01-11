import { BookOpen, Settings, List, X } from 'lucide-react';
import { GenericRecipe } from '@/types/recipe';
import { truncateText } from '@/utils/helpers';
import { BaseModal } from './BaseModal';

interface RecipeDetailsModalProps {
  isOpen: boolean;
  onClose: () => void;
  recipe: GenericRecipe | null;
}

export default function RecipeDetailsModal({
  isOpen,
  onClose,
  recipe,
}: RecipeDetailsModalProps) {
  if (!recipe) return null;

  const formatParameterValue = (value: any) => {
    if (typeof value === 'object') {
      return JSON.stringify(value, null, 2);
    }
    return String(value);
  };

  const formatWantParams = (params: any) => {
    if (!params || Object.keys(params).length === 0) {
      return 'No parameters';
    }
    return JSON.stringify(params, null, 2);
  };

  const footer = (
    <div className="flex justify-end">
      <button
        onClick={onClose}
        className="px-6 py-2 text-sm font-bold text-gray-700 bg-white border border-gray-200 rounded-xl hover:bg-gray-50 transition-colors shadow-sm"
      >
        Close
      </button>
    </div>
  );

  return (
    <BaseModal
      isOpen={isOpen}
      onClose={onClose}
      title={recipe.recipe.metadata.name}
      footer={footer}
      size="lg"
    >
      <div className="space-y-8 max-h-[70vh] overflow-y-auto pr-2 custom-scrollbar">
        {/* Metadata */}
        <div>
          <h3 className="text-lg font-bold text-gray-900 mb-4 flex items-center gap-2">
            <Settings className="h-5 w-5 text-blue-600" />
            Metadata
          </h3>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div className="bg-gray-50 p-4 rounded-xl border border-gray-100">
              <dt className="text-xs font-bold text-gray-400 uppercase tracking-wider mb-1">Name</dt>
              <dd className="text-sm text-gray-900 font-mono">
                {recipe.recipe.metadata.name}
              </dd>
            </div>

            {recipe.recipe.metadata.version && (
              <div className="bg-gray-50 p-4 rounded-xl border border-gray-100">
                <dt className="text-xs font-bold text-gray-400 uppercase tracking-wider mb-1">Version</dt>
                <dd className="text-sm text-gray-900 font-mono">
                  {recipe.recipe.metadata.version}
                </dd>
              </div>
            )}

            {recipe.recipe.metadata.custom_type && (
              <div className="bg-gray-50 p-4 rounded-xl border border-gray-100 sm:col-span-2">
                <dt className="text-xs font-bold text-gray-400 uppercase tracking-wider mb-1">Custom Type</dt>
                <dd className="text-sm text-gray-900 font-bold">
                  {recipe.recipe.metadata.custom_type}
                </dd>
              </div>
            )}
          </div>
        </div>

        {/* Parameters */}
        {recipe.recipe.parameters && Object.keys(recipe.recipe.parameters).length > 0 && (
          <div>
            <h3 className="text-lg font-bold text-gray-900 mb-4 flex items-center gap-2">
              <List className="h-5 w-5 text-green-600" />
              Parameters
            </h3>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {Object.entries(recipe.recipe.parameters).map(([key, value]) => (
                <div key={key} className="border border-gray-100 rounded-xl p-4 bg-white shadow-sm">
                  <dt className="text-sm font-bold text-gray-700 mb-2">{key}</dt>
                  <dd className="text-xs text-blue-600 font-mono bg-blue-50 p-3 rounded-lg border border-blue-100 overflow-x-auto">
                    {formatParameterValue(value)}
                  </dd>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Wants */}
        <div>
          <h3 className="text-lg font-bold text-gray-900 mb-4 flex items-center gap-2">
            <BookOpen className="h-5 w-5 text-purple-600" />
            Wants ({recipe.recipe.wants?.length || 0})
          </h3>
          <div className="space-y-4">
            {recipe.recipe.wants?.map((want, index) => (
              <div key={index} className="border border-gray-100 rounded-2xl p-5 bg-gray-50">
                <div className="flex items-center justify-between mb-4">
                  <h4 className="font-bold text-gray-900">
                    Want {index + 1}
                    {(want.metadata?.name || want.name) && (
                      <span className="text-gray-400 font-medium ml-2 text-sm">
                        ({want.metadata?.name || want.name})
                      </span>
                    )}
                  </h4>
                  <span className="text-xs font-bold bg-white text-blue-600 border border-blue-100 px-3 py-1 rounded-full shadow-sm">
                    {want.type || want.metadata?.type || 'Unknown type'}
                  </span>
                </div>

                <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
                  {/* Parameters */}
                  <div>
                    <h5 className="text-xs font-bold text-gray-400 uppercase tracking-wider mb-2">Parameters</h5>
                    <pre className="text-xs text-gray-700 bg-white p-4 rounded-xl border border-gray-200 overflow-x-auto font-mono leading-relaxed shadow-inner">
                      {formatWantParams(want.params || want.spec?.params)}
                    </pre>
                  </div>

                  {/* Right Column: Using, Labels, Requirements */}
                  <div className="space-y-4">
                    {/* Using selectors */}
                    {want.using && want.using.length > 0 && (
                      <div>
                        <h5 className="text-xs font-bold text-gray-400 uppercase tracking-wider mb-2">Using Selectors</h5>
                        <div className="space-y-2">
                          {want.using.map((selector, selectorIndex) => (
                            <div key={selectorIndex} className="text-xs bg-blue-50 p-2 rounded-lg border border-blue-100">
                              {Object.entries(selector).map(([key, value]) => (
                                <div key={key} className="flex justify-between py-0.5">
                                  <span className="text-blue-700 font-medium">{key}</span>
                                  <span className="text-blue-900 font-bold font-mono">{value}</span>
                                </div>
                              ))}
                            </div>
                          ))}
                        </div>
                      </div>
                    )}

                    {/* Labels */}
                    {((want.metadata?.labels && Object.keys(want.metadata.labels).length > 0) ||
                      (want.labels && Object.keys(want.labels).length > 0)) && (
                      <div>
                        <h5 className="text-xs font-bold text-gray-400 uppercase tracking-wider mb-2">Labels</h5>
                        <div className="flex flex-wrap gap-1">
                          {Object.entries(want.metadata?.labels || want.labels || {}).map(([key, value]) => (
                            <span key={key} className="text-[10px] font-bold bg-green-100 text-green-700 border border-green-200 px-2 py-0.5 rounded-md shadow-sm">
                              {truncateText(`${key}=${value}`, 25)}
                            </span>
                          ))}
                        </div>
                      </div>
                    )}
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </BaseModal>
  );
}
