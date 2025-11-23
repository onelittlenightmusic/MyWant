import React, { useMemo, useEffect } from 'react';
import { GenericRecipe } from '@/types/recipe';
import { RecipeCard } from './RecipeCard';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';

interface RecipeGridProps {
  recipes: GenericRecipe[];
  loading: boolean;
  searchQuery?: string;
  selectedRecipe?: GenericRecipe | null;
  onViewRecipe: (recipe: GenericRecipe) => void;
  onEditRecipe: (recipe: GenericRecipe) => void;
  onDeleteRecipe: (recipe: GenericRecipe) => void;
  onSelectRecipe?: (recipe: GenericRecipe) => void;
  onGetFilteredRecipes?: (recipes: GenericRecipe[]) => void;
}

export const RecipeGrid: React.FC<RecipeGridProps> = ({
  recipes,
  loading,
  searchQuery = '',
  selectedRecipe,
  onViewRecipe,
  onEditRecipe,
  onDeleteRecipe,
  onSelectRecipe,
  onGetFilteredRecipes
}) => {
  const filteredRecipes = useMemo(() => {
    return recipes.filter(recipe => {
      // Search filter
      if (searchQuery) {
        const query = searchQuery.toLowerCase();
        const recipeName = recipe.recipe.metadata.name || '';
        const description = recipe.recipe.metadata.description || '';
        const customType = recipe.recipe.metadata.custom_type || '';

        const matchesSearch =
          recipeName.toLowerCase().includes(query) ||
          description.toLowerCase().includes(query) ||
          customType.toLowerCase().includes(query);

        if (!matchesSearch) return false;
      }

      return true;
    }).sort((a, b) => {
      // Sort by name to ensure consistent ordering
      const nameA = a.recipe.metadata.name || '';
      const nameB = b.recipe.metadata.name || '';
      return nameA.localeCompare(nameB);
    });
  }, [recipes, searchQuery]);

  // Notify parent of filtered recipes for keyboard navigation
  useEffect(() => {
    onGetFilteredRecipes?.(filteredRecipes);
  }, [filteredRecipes, onGetFilteredRecipes]);

  if (loading && recipes.length === 0) {
    return (
      <div className="flex items-center justify-center py-16">
        <LoadingSpinner size="lg" />
        <span className="ml-3 text-gray-600">Loading recipes...</span>
      </div>
    );
  }

  if (recipes.length === 0) {
    return (
      <div className="text-center py-16">
        <div className="mx-auto w-24 h-24 bg-gray-100 rounded-full flex items-center justify-center mb-4">
          <svg
            className="w-12 h-12 text-gray-400"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={1.5}
              d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.746 0 3.332.477 4.5 1.253v13C19.832 18.477 18.246 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"
            />
          </svg>
        </div>
        <h3 className="text-lg font-medium text-gray-900 mb-2">No recipes yet</h3>
        <p className="text-gray-600 mb-4">
          Get started by creating your first recipe template.
        </p>
      </div>
    );
  }

  if (filteredRecipes.length === 0) {
    return (
      <div className="text-center py-16">
        <div className="mx-auto w-24 h-24 bg-gray-100 rounded-full flex items-center justify-center mb-4">
          <svg
            className="w-12 h-12 text-gray-400"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={1.5}
              d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
            />
          </svg>
        </div>
        <h3 className="text-lg font-medium text-gray-900 mb-2">No matches found</h3>
        <p className="text-gray-600">
          No recipes match your current search criteria.
        </p>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6 items-start">
      {filteredRecipes.map((recipe, index) => (
        <div
          key={recipe.recipe.metadata.name || `recipe-${index}`}
          data-keyboard-nav-selected={selectedRecipe?.recipe.metadata.name === recipe.recipe.metadata.name}
        >
          <RecipeCard
            recipe={recipe}
            selected={selectedRecipe?.recipe.metadata.name === recipe.recipe.metadata.name}
            onView={onViewRecipe}
            onEdit={onEditRecipe}
            onDelete={onDeleteRecipe}
            onSelect={onSelectRecipe}
          />
        </div>
      ))}
    </div>
  );
};