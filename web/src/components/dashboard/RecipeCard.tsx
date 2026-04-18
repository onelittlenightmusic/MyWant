import React, { useState } from 'react';
import { Eye, Edit2, Trash2, MoreHorizontal, BookOpen, Play, Zap } from 'lucide-react';
import { GenericRecipe } from '@/types/recipe';
import { truncateText, classNames } from '@/utils/helpers';
import { getBackgroundStyle, getBackgroundOverlayClass } from '@/utils/backgroundStyles';

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
  const backgroundStyle = getBackgroundStyle(recipe.recipe.metadata.custom_type, true);

  return (
    <div
      ref={cardRef}
      onClick={handleCardClick}
      tabIndex={0}
      data-keyboard-nav-selected={selected}
      data-keyboard-nav-id={recipeName}
      className={classNames(
        'card hover:shadow-md dark:hover:shadow-blue-900/20 transition-all duration-300 cursor-pointer group relative overflow-hidden focus:outline-none focus:ring-2 focus:ring-blue-400 dark:focus:ring-blue-500 focus:ring-inset h-full flex flex-col min-h-[6rem] sm:min-h-[10rem]',
        selected ? 'border-blue-500 border-2 shadow-lg scale-[1.02] z-10' : 'border-gray-200 dark:border-gray-700',
        className || ''
      )}
      style={backgroundStyle.style}
    >
      {/* Background overlay */}
      <div className={getBackgroundOverlayClass()}></div>

      {/* Content Area */}
      <div className="relative z-10 px-3 sm:px-6 pb-3 pt-3 order-1 flex-1">
        <p className="text-[10px] sm:text-sm text-gray-500 dark:text-gray-400 mt-1 truncate">
          {wantsCount} wants · {parametersCount} params
        </p>
        {description && (
          <p className="text-[10px] sm:text-xs text-gray-500 dark:text-gray-400 mt-2 line-clamp-2">
            {description}
          </p>
        )}
      </div>

      {/* Header (Title Area) - Moved to bottom to match WantCard */}
      <div className="relative z-20 order-2 mt-auto">
        <div className={classNames(
          "backdrop-blur-[2px] transition-colors duration-200 px-3 sm:px-6 py-1.5 flex items-center justify-between",
          selected ? "bg-blue-100/90 dark:bg-blue-900/70" : "bg-white/60 dark:bg-gray-900/70"
        )}>
          <div className="flex-1 min-w-0">
            <h3
              className="text-[9px] sm:text-[13px] font-semibold text-gray-900 dark:text-gray-100 truncate group-hover:text-primary-600 dark:group-hover:text-primary-400 transition-colors cursor-pointer flex items-center gap-1.5"
              onClick={(e) => { e.stopPropagation(); onView(recipe); }}
            >
              <BookOpen className="h-2 w-2 sm:h-3.5 sm:w-3.5 flex-shrink-0 text-indigo-500" />
              {truncateText(recipeName, 30)}
            </h3>
          </div>

          <div className="flex items-center space-x-1 sm:space-x-2 ml-1 sm:ml-2">
            {/* Version Badge */}
            {version && (
              <span className="px-1.5 py-0.5 rounded-full bg-gray-100 dark:bg-gray-800 text-[8px] sm:text-[10px] text-gray-600 dark:text-gray-400">
                v{version}
              </span>
            )}

            {/* Deploy button */}
            {onDeploy && (
              <button
                onClick={handleDeploy}
                disabled={isDeploying}
                className="inline-flex items-center px-1.5 sm:px-2 py-0.5 sm:py-1 rounded-full text-[8px] sm:text-[10px] font-medium bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300 hover:bg-green-200 dark:hover:bg-green-900/50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                title={isDeploying ? 'Deploying...' : 'Deploy'}
              >
                <Play className="h-3 w-3 sm:h-3.5 sm:w-3.5" />
              </button>
            )}

            {/* Actions menu */}
            <div className="relative group/menu">
              <button 
                className="p-1 rounded-md text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-800"
                onClick={(e) => e.stopPropagation()}
              >
                <MoreHorizontal className="h-3.5 w-3.5" />
              </button>

              <div className="absolute right-0 bottom-full mb-2 w-48 bg-white dark:bg-gray-800 rounded-md shadow-lg border border-gray-200 dark:border-gray-700 z-10 opacity-0 invisible group-hover/menu:opacity-100 group-hover/menu:visible transition-all duration-200">
                <div className="py-1">
                  <button
                    onClick={(e) => { e.stopPropagation(); onView(recipe); }}
                    className="flex items-center w-full px-4 py-2 text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
                  >
                    <Eye className="h-4 w-4 mr-2" />
                    View Details
                  </button>

                  <hr className="my-1 border-gray-200 dark:border-gray-700" />

                  {onDeploy && (
                    <button
                      onClick={(e) => { e.stopPropagation(); handleDeploy(e); }}
                      disabled={isDeploying}
                      className="flex items-center w-full px-4 py-2 text-sm text-green-600 dark:text-green-400 hover:bg-green-50 dark:hover:bg-green-900/30 disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                      <Play className="h-4 w-4 mr-2" />
                      {isDeploying ? 'Deploying...' : 'Deploy'}
                    </button>
                  )}

                  {hasExample && onDeployExample && (
                    <button
                      onClick={(e) => { e.stopPropagation(); handleDeployExample(e); }}
                      disabled={isDeploying}
                      className="flex items-center w-full px-4 py-2 text-sm text-blue-600 dark:text-blue-400 hover:bg-blue-50 dark:hover:bg-blue-900/30 disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                      <Zap className="h-4 w-4 mr-2" />
                      {isDeploying ? 'Deploying...' : 'Deploy Example'}
                    </button>
                  )}

                  <hr className="my-1 border-gray-200 dark:border-gray-700" />

                  <button
                    onClick={(e) => { e.stopPropagation(); onEdit(recipe); }}
                    className="flex items-center w-full px-4 py-2 text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
                  >
                    <Edit2 className="h-4 w-4 mr-2" />
                    Edit
                  </button>

                  <button
                    onClick={(e) => { e.stopPropagation(); onDelete(recipe); }}
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
    </div>
  );
};