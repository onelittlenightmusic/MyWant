import { X, BookOpen, Settings, List } from 'lucide-react';
import { GenericRecipe } from '@/types/recipe';
import { truncateText } from '@/utils/helpers';

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
  if (!isOpen || !recipe) return null;

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

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
      <div className="bg-white rounded-lg max-w-4xl w-full max-h-[90vh] overflow-y-auto">
        {/* Header */}
        <div className="p-6 border-b border-gray-200">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <BookOpen className="h-6 w-6 text-blue-600" />
              <div>
                <h2 className="text-xl font-semibold text-gray-900">
                  {recipe.recipe.metadata.name}
                </h2>
                {recipe.recipe.metadata.description && (
                  <p className="text-sm text-gray-600 mt-1">
                    {recipe.recipe.metadata.description}
                  </p>
                )}
              </div>
            </div>
            <button
              onClick={onClose}
              className="text-gray-500 hover:text-gray-700"
            >
              <X className="h-5 w-5" />
            </button>
          </div>
        </div>

        <div className="p-6 space-y-8">
          {/* Metadata */}
          <div>
            <h3 className="text-lg font-medium text-gray-900 mb-4 flex items-center gap-2">
              <Settings className="h-5 w-5" />
              Metadata
            </h3>
            <div className="grid grid-cols-2 gap-6">
              <div>
                <dt className="text-sm font-medium text-gray-500">Name</dt>
                <dd className="text-sm text-gray-900 mt-1 font-mono">
                  {recipe.recipe.metadata.name}
                </dd>
              </div>

              {recipe.recipe.metadata.version && (
                <div>
                  <dt className="text-sm font-medium text-gray-500">Version</dt>
                  <dd className="text-sm text-gray-900 mt-1 font-mono">
                    {recipe.recipe.metadata.version}
                  </dd>
                </div>
              )}

              {recipe.recipe.metadata.custom_type && (
                <div className="col-span-2">
                  <dt className="text-sm font-medium text-gray-500">Custom Type</dt>
                  <dd className="text-sm text-gray-900 mt-1">
                    {recipe.recipe.metadata.custom_type}
                  </dd>
                </div>
              )}
            </div>
          </div>

          {/* Parameters */}
          {recipe.recipe.parameters && Object.keys(recipe.recipe.parameters).length > 0 && (
            <div>
              <h3 className="text-lg font-medium text-gray-900 mb-4">Parameters</h3>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                {Object.entries(recipe.recipe.parameters).map(([key, value]) => (
                  <div key={key} className="border border-gray-200 rounded-lg p-4">
                    <dt className="text-sm font-medium text-gray-500 mb-2">{key}</dt>
                    <dd className="text-sm text-gray-900 font-mono bg-gray-50 p-2 rounded">
                      {formatParameterValue(value)}
                    </dd>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Wants */}
          <div>
            <h3 className="text-lg font-medium text-gray-900 mb-4 flex items-center gap-2">
              <List className="h-5 w-5" />
              Wants ({recipe.recipe.wants?.length || 0})
            </h3>
            <div className="space-y-4">
              {recipe.recipe.wants?.map((want, index) => (
                <div key={index} className="border border-gray-200 rounded-lg p-4">
                  <div className="flex items-center justify-between mb-3">
                    <h4 className="font-medium text-gray-900">
                      Want {index + 1}
                      {(want.metadata?.name || want.name) && (
                        <span className="text-gray-500 font-normal ml-2">
                          ({want.metadata?.name || want.name})
                        </span>
                      )}
                    </h4>
                    <span className="text-xs bg-blue-100 text-blue-800 px-2 py-1 rounded-full">
                      {want.type || want.metadata?.type || 'Unknown type'}
                    </span>
                  </div>

                  <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
                    {/* Parameters */}
                    <div>
                      <h5 className="text-sm font-medium text-gray-500 mb-2">Parameters</h5>
                      <pre className="text-xs text-gray-900 bg-gray-50 p-3 rounded border overflow-x-auto">
                        {formatWantParams(want.params || want.spec?.params)}
                      </pre>
                    </div>

                    {/* Using selectors */}
                    {want.using && want.using.length > 0 && (
                      <div>
                        <h5 className="text-sm font-medium text-gray-500 mb-2">Using Selectors</h5>
                        <div className="space-y-1">
                          {want.using.map((selector, selectorIndex) => (
                            <div key={selectorIndex} className="text-xs bg-gray-50 p-2 rounded border">
                              {Object.entries(selector).map(([key, value]) => (
                                <div key={key} className="flex justify-between">
                                  <span className="text-gray-600">{key}:</span>
                                  <span className="text-gray-900 font-mono">{value}</span>
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
                        <h5 className="text-sm font-medium text-gray-500 mb-2">Labels</h5>
                        <div className="flex flex-wrap gap-1">
                          {Object.entries(want.metadata?.labels || want.labels || {}).map(([key, value]) => {
                            const labelText = `${key}=${value}`;
                            const displayText = truncateText(labelText, 20);
                            return (
                              <span key={key} className="text-xs bg-green-100 text-green-800 px-2 py-1 rounded" title={labelText.length > 20 ? labelText : undefined}>
                                {displayText}
                              </span>
                            );
                          })}
                        </div>
                      </div>
                    )}

                    {/* Requirements */}
                    {want.requires && want.requires.length > 0 && (
                      <div>
                        <h5 className="text-sm font-medium text-gray-500 mb-2">Requirements</h5>
                        <div className="flex flex-wrap gap-1">
                          {want.requires.map((req, reqIndex) => (
                            <span key={reqIndex} className="text-xs bg-yellow-100 text-yellow-800 px-2 py-1 rounded">
                              {req}
                            </span>
                          ))}
                        </div>
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Result Configuration */}
          {recipe.recipe.result && recipe.recipe.result.length > 0 && (
            <div>
              <h3 className="text-lg font-medium text-gray-900 mb-4">Result Configuration</h3>
              <div className="space-y-2">
                {recipe.recipe.result.map((result, index) => (
                  <div key={index} className="border border-gray-200 rounded-lg p-4">
                    <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                      <div>
                        <dt className="text-xs font-medium text-gray-500">Want Name</dt>
                        <dd className="text-sm text-gray-900 font-mono">{result.want_name}</dd>
                      </div>
                      <div>
                        <dt className="text-xs font-medium text-gray-500">Stat Name</dt>
                        <dd className="text-sm text-gray-900 font-mono">{result.stat_name}</dd>
                      </div>
                      {result.description && (
                        <div>
                          <dt className="text-xs font-medium text-gray-500">Description</dt>
                          <dd className="text-sm text-gray-900">{result.description}</dd>
                        </div>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="p-6 border-t border-gray-200">
          <div className="flex justify-end">
            <button
              onClick={onClose}
              className="px-4 py-2 bg-gray-600 text-white rounded-md hover:bg-gray-700 focus:outline-none focus:ring-2 focus:ring-gray-500"
            >
              Close
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}