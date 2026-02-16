import React, { useState } from 'react';
import { Eye, Edit2, Trash2, MoreHorizontal, BookOpen, Play, Zap } from 'lucide-react';
import { GenericRecipe } from '@/types/recipe';
import { truncateText, classNames } from '@/utils/helpers';

interface RecipeCardProps {
  recipe: GenericRecipe;
  selected?: boolean;
  onView: (recipe: GenericRecipe) => void;
  onEdit: (recipe: GenericRecipe) => void;
  onDelete: (recipe: GenericRecipe) => void;
  onDeploy?: (recipe: GenericRecipe) => void;
  onDeployExample?: (recipe: GenericRecipe) => void;
  onSelect?: (recipe: GenericRecipe) => void;
  className?: string;
}

export const RecipeCard: React.FC<RecipeCardProps> = ({
  recipe,
  selected = false,
  onView,
  onEdit,
  onDelete,
  onDeploy,
  onDeployExample,
  onSelect,
  className
}) => {
  const [isDeploying, setIsDeploying] = useState(false);
  const recipeName = recipe.recipe.metadata.name || 'Unnamed Recipe';
  const description = recipe.recipe.metadata.description || '';
  const version = recipe.recipe.metadata.version || '';
  const wantsCount = recipe.recipe.wants?.length || 0;
  const parametersCount = recipe.recipe.parameters ? Object.keys(recipe.recipe.parameters).length : 0;
  const cardRef = React.useRef<HTMLDivElement>(null);

  // Focus the card when it's targeted by keyboard navigation
  React.useEffect(() => {
    if (selected && document.activeElement !== cardRef.current) {
      cardRef.current?.focus();
    }
  }, [selected]);

  const handleCardClick = () => {
    onSelect?.(recipe);

    // Smooth scroll the card into view after selection
    requestAnimationFrame(() => {
      setTimeout(() => {
        const selectedElement = document.querySelector('[data-keyboard-nav-selected="true"]');
        if (selectedElement && selectedElement instanceof HTMLElement) {
          selectedElement.scrollIntoView({ behavior: 'smooth', block: 'center' });
        }
      }, 0);
    });
  };

  const handleDeploy = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!onDeploy) return;

    setIsDeploying(true);
    try {
      await onDeploy(recipe);
    } finally {
      setIsDeploying(false);
    }
  };

  const handleDeployExample = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!onDeployExample) return;

    setIsDeploying(true);
    try {
      await onDeployExample(recipe);
    } finally {
      setIsDeploying(false);
    }
  };

  const hasExample = recipe.recipe.example?.wants && recipe.recipe.example.wants.length > 0;

  return (
    <div
      ref={cardRef}
      onClick={handleCardClick}
      tabIndex={0}
      data-keyboard-nav-selected={selected}
      data-keyboard-nav-id={recipeName}
      className={classNames(
        'card hover:shadow-md dark:hover:shadow-blue-900/20 transition-shadow duration-200 cursor-pointer group relative focus:outline-none focus:ring-2 focus:ring-blue-400 dark:focus:ring-blue-500 focus:ring-inset h-full flex flex-col min-h-[8rem] sm:min-h-[12.5rem]',
        selected ? 'border-blue-500 border-2' : 'border-gray-200 dark:border-gray-700',
        className || ''
      )}>
      {/* Header */}
      <div className="flex items-start justify-between mb-2 sm:mb-4">
        <div className="flex-1 min-w-0">
          <h3
            className="text-xs sm:text-lg font-semibold text-gray-900 dark:text-white truncate group-hover:text-primary-600 dark:group-hover:text-primary-400 transition-colors cursor-pointer flex items-center gap-1.5"
            onClick={() => onView(recipe)}
          >
            <BookOpen className="h-3 w-3 sm:h-4 sm:w-4 flex-shrink-0 text-indigo-500" />
            {truncateText(recipeName, 30)}
          </h3>
          <p className="text-[10px] sm:text-sm text-gray-500 dark:text-gray-400 mt-1 truncate">
            {wantsCount} wants · {parametersCount} params
          </p>
        </div>

        <div className="flex items-center space-x-1 sm:space-x-2 ml-1 sm:ml-2">
          {/* Deploy button */}
          {onDeploy && (
            <button
              onClick={handleDeploy}
              disabled={isDeploying}
              className="inline-flex items-center px-1.5 sm:px-2 py-0.5 sm:py-1 rounded-full text-[10px] sm:text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300 hover:bg-green-200 dark:hover:bg-green-900/50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
              title={isDeploying ? 'Deploying...' : 'Deploy'}
            >
              <Play className="h-3 w-3 sm:h-3.5 sm:w-3.5" />
            </button>
          )}

          {/* Actions menu */}
          <div className="relative group/menu">
            <button className="p-1 rounded-md text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-800">
              <MoreHorizontal className="h-4 w-4" />
            </button>

            <div className="absolute right-0 top-8 w-48 bg-white dark:bg-gray-800 rounded-md shadow-lg border border-gray-200 dark:border-gray-700 z-10 opacity-0 invisible group-hover/menu:opacity-100 group-hover/menu:visible transition-all duration-200">
              <div className="py-1">
                <button
                  onClick={() => onView(recipe)}
                  className="flex items-center w-full px-4 py-2 text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
                >
                  <Eye className="h-4 w-4 mr-2" />
                  View Details
                </button>

                <hr className="my-1 border-gray-200 dark:border-gray-700" />

                {onDeploy && (
                  <button
                    onClick={handleDeploy}
                    disabled={isDeploying}
                    className="flex items-center w-full px-4 py-2 text-sm text-green-600 dark:text-green-400 hover:bg-green-50 dark:hover:bg-green-900/30 disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    <Play className="h-4 w-4 mr-2" />
                    {isDeploying ? 'Deploying...' : 'Deploy'}
                  </button>
                )}

                {hasExample && onDeployExample && (
                  <button
                    onClick={handleDeployExample}
                    disabled={isDeploying}
                    className="flex items-center w-full px-4 py-2 text-sm text-blue-600 dark:text-blue-400 hover:bg-blue-50 dark:hover:bg-blue-900/30 disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    <Zap className="h-4 w-4 mr-2" />
                    {isDeploying ? 'Deploying...' : 'Deploy Example'}
                  </button>
                )}

                <hr className="my-1 border-gray-200 dark:border-gray-700" />

                <button
                  onClick={() => onEdit(recipe)}
                  className="flex items-center w-full px-4 py-2 text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
                >
                  <Edit2 className="h-4 w-4 mr-2" />
                  Edit
                </button>

                <button
                  onClick={() => onDelete(recipe)}
                  className="flex items-center w-full px-4 py-2 text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30"
                >
                  <Trash2 className="h-4 w-4 mr-2" />
                  Delete
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};