import React from 'react';
import { Eye, Edit2, Trash2, MoreHorizontal, BookOpen } from 'lucide-react';
import { GenericRecipe } from '@/types/recipe';
import { truncateText, classNames } from '@/utils/helpers';

interface RecipeCardProps {
  recipe: GenericRecipe;
  selected?: boolean;
  onView: (recipe: GenericRecipe) => void;
  onEdit: (recipe: GenericRecipe) => void;
  onDelete: (recipe: GenericRecipe) => void;
  className?: string;
}

export const RecipeCard: React.FC<RecipeCardProps> = ({
  recipe,
  selected = false,
  onView,
  onEdit,
  onDelete,
  className
}) => {
  const recipeName = recipe.recipe.metadata.name || 'Unnamed Recipe';
  const description = recipe.recipe.metadata.description || '';
  const version = recipe.recipe.metadata.version || '';
  const wantsCount = recipe.recipe.wants.length;
  const parametersCount = recipe.recipe.parameters ? Object.keys(recipe.recipe.parameters).length : 0;

  const formatParametersCount = (params?: Record<string, any>) => {
    if (!params) return 0;
    return Object.keys(params).length;
  };

  return (
    <div className={classNames(
      'card hover:shadow-md transition-shadow duration-200 cursor-pointer group relative',
      selected ? 'border-blue-500 border-2' : 'border-gray-200',
      className || ''
    )}>
      {/* Header */}
      <div className="flex items-start justify-between mb-4">
        <div className="flex-1 min-w-0">
          <h3
            className="text-lg font-semibold text-gray-900 truncate group-hover:text-primary-600 transition-colors cursor-pointer"
            onClick={() => onView(recipe)}
          >
            {truncateText(recipeName, 30)}
          </h3>
          <p className="text-sm text-gray-500 mt-1">
            {description ? truncateText(description, 60) : 'No description'}
          </p>
          {version && (
            <p className="text-xs text-gray-400 mt-1">
              Version: {version}
            </p>
          )}
        </div>

        <div className="flex items-center space-x-2">
          {/* Recipe type icon */}
          <div className="flex items-center space-x-1" title="Recipe template">
            <BookOpen className="h-4 w-4 text-blue-600" />
          </div>

          {/* Actions menu */}
          <div className="relative group/menu">
            <button className="p-1 rounded-md text-gray-400 hover:text-gray-600 hover:bg-gray-100">
              <MoreHorizontal className="h-4 w-4" />
            </button>

            <div className="absolute right-0 top-8 w-48 bg-white rounded-md shadow-lg border border-gray-200 z-10 opacity-0 invisible group-hover/menu:opacity-100 group-hover/menu:visible transition-all duration-200">
              <div className="py-1">
                <button
                  onClick={() => onView(recipe)}
                  className="flex items-center w-full px-4 py-2 text-sm text-gray-700 hover:bg-gray-100"
                >
                  <Eye className="h-4 w-4 mr-2" />
                  View Details
                </button>

                <button
                  onClick={() => onEdit(recipe)}
                  className="flex items-center w-full px-4 py-2 text-sm text-gray-700 hover:bg-gray-100"
                >
                  <Edit2 className="h-4 w-4 mr-2" />
                  Edit
                </button>

                <hr className="my-1" />

                <button
                  onClick={() => onDelete(recipe)}
                  className="flex items-center w-full px-4 py-2 text-sm text-red-600 hover:bg-red-50"
                >
                  <Trash2 className="h-4 w-4 mr-2" />
                  Delete
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Recipe Stats */}
      <div className="space-y-2 mb-4">
        <div className="flex justify-between text-sm">
          <span className="text-gray-500">Wants:</span>
          <span className="text-gray-900 font-medium">{wantsCount}</span>
        </div>
        <div className="flex justify-between text-sm">
          <span className="text-gray-500">Parameters:</span>
          <span className="text-gray-900 font-medium">{parametersCount}</span>
        </div>
      </div>

      {/* Custom Type Badge (if available) */}
      {recipe.recipe.metadata.custom_type && (
        <div className="mb-4">
          <span className="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium bg-purple-100 text-purple-800">
            {recipe.recipe.metadata.custom_type}
          </span>
        </div>
      )}

      {/* Recipe Summary */}
      <div className="pt-4 border-t border-gray-200">
        <p className="text-xs text-gray-600">
          Template with {wantsCount} want{wantsCount !== 1 ? 's' : ''} and {parametersCount} parameter{parametersCount !== 1 ? 's' : ''}
        </p>
      </div>
    </div>
  );
};