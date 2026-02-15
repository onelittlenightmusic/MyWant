import { useState, useEffect } from 'react';
import { Plus, Check, AlertCircle } from 'lucide-react';
import { useRecipeStore } from '@/stores/recipeStore';
import { useWantStore } from '@/stores/wantStore';
import { useUIStore } from '@/stores/uiStore';
import { GenericRecipe } from '@/types/recipe';
import { useKeyboardNavigation } from '@/hooks/useKeyboardNavigation';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { useRightSidebarExclusivity } from '@/hooks/useRightSidebarExclusivity';
import RecipeModal from '@/components/modals/RecipeModal';
import { RecipeDetailsSidebar } from '@/components/sidebar/RecipeDetailsSidebar';
import { RightSidebar } from '@/components/layout/RightSidebar';
import { Header } from '@/components/layout/Header';
import { RecipeGrid } from '@/components/dashboard/RecipeGrid';
import { RecipeStatsOverview } from '@/components/dashboard/RecipeStatsOverview';
import { RecipeFilters } from '@/components/dashboard/RecipeFilters';
import { classNames } from '@/utils/helpers';

export default function RecipePage() {
  const {
    recipes,
    loading,
    error,
    fetchRecipes,
    deleteRecipe,
    clearError,
  } = useRecipeStore();

  const {
    createWant,
  } = useWantStore();

  // UI State
  const sidebar = useRightSidebarExclusivity<GenericRecipe>();
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const [editingRecipe, setEditingRecipe] = useState<GenericRecipe | null>(null);
  const [notification, setNotification] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [filteredRecipes, setFilteredRecipes] = useState<GenericRecipe[]>([]);
  const [searchQuery, setSearchQuery] = useState('');

  // Map hook state to modal visibility
  const showCreateModal = sidebar.showForm && !editingRecipe;
  const showEditModal = sidebar.showForm && !!editingRecipe;

  // For backward compatibility
  const selectedRecipe = sidebar.selectedItem;

  useEffect(() => {
    fetchRecipes();
  }, [fetchRecipes]);

  // Auto-dismiss notifications after 5 seconds
  useEffect(() => {
    if (notification) {
      const timer = setTimeout(() => {
        setNotification(null);
      }, 5000);
      return () => clearTimeout(timer);
    }
  }, [notification]);

  // Clear editing recipe when Edit modal closes
  useEffect(() => {
    if (!showEditModal) {
      setEditingRecipe(null);
    }
  }, [showEditModal]);

  const handleCreateRecipe = () => {
    setEditingRecipe(null);
    sidebar.openForm();
  };

  const handleEditRecipe = (recipe: GenericRecipe) => {
    setEditingRecipe(recipe);
    sidebar.openForm();
  };

  const handleViewRecipe = (recipe: GenericRecipe) => {
    sidebar.selectItem(recipe);
  };

  const handleDeleteRecipe = (recipe: GenericRecipe) => {
    sidebar.selectItem(recipe);
    setShowDeleteModal(true);
  };

  const confirmDeleteRecipe = async () => {
    if (selectedRecipe) {
      await deleteRecipe(selectedRecipe.recipe.metadata.name);
      setShowDeleteModal(false);
      sidebar.clearSelection();
    }
  };

  const handleDeployRecipe = async (recipe: GenericRecipe) => {
    try {
      const customType = recipe.recipe.metadata.custom_type || 'unknown';
      const recipeFileName = customType.toLowerCase().replace(/\s+/g, '-');

      // Create a want that references the recipe
      await createWant({
        metadata: {
          name: recipeFileName,
          type: customType,
          labels: {},
        },
        spec: {
          recipe: `yaml/recipes/${recipeFileName}.yaml`,
          params: recipe.recipe.parameters || {},
        },
      });
      setNotification({
        message: `Recipe "${recipe.recipe.metadata.name}" deployed successfully!`,
        type: 'success',
      });
    } catch (err) {
      setNotification({
        message: `Failed to deploy recipe: ${err instanceof Error ? err.message : 'Unknown error'}`,
        type: 'error',
      });
    }
  };

  const handleDeployRecipeExample = async (recipe: GenericRecipe) => {
    try {
      if (!recipe.recipe.example || !recipe.recipe.example.wants || recipe.recipe.example.wants?.length === 0) {
        throw new Error('No example configuration available for this recipe');
      }

      // Deploy each want from the example
      for (const exampleWant of recipe.recipe.example.wants) {
        const metadata = exampleWant.metadata || {};
        // Ensure required fields
        if (!metadata.name || !metadata.type) {
          throw new Error('Example want must have name and type');
        }

        await createWant({
          metadata: {
            name: metadata.name,
            type: metadata.type,
            labels: metadata.labels || {},
          },
          spec: exampleWant.spec || {},
        });
      }

      setNotification({
        message: `Recipe example "${recipe.recipe.metadata.name}" deployed successfully!`,
        type: 'success',
      });
    } catch (err) {
      setNotification({
        message: `Failed to deploy recipe example: ${err instanceof Error ? err.message : 'Unknown error'}`,
        type: 'error',
      });
    }
  };

  // Keyboard navigation
  const currentRecipeIndex = selectedRecipe
    ? filteredRecipes.findIndex(r => r.recipe.metadata.name === selectedRecipe.recipe.metadata.name)
    : -1;

  const handleKeyboardNavigate = (index: number) => {
    if (index >= 0 && index < filteredRecipes.length) {
      const recipe = filteredRecipes[index];
      handleViewRecipe(recipe);
    }
  };

  useKeyboardNavigation({
    itemCount: filteredRecipes.length,
    currentIndex: currentRecipeIndex,
    onNavigate: handleKeyboardNavigate,
    enabled: !sidebar.showForm && filteredRecipes.length > 0
  });

  // Handle ESC key to close details sidebar and deselect
  const handleEscapeKey = () => {
    if (selectedRecipe) {
      sidebar.clearSelection();
    }
  };

  useEscapeKey({
    onEscape: handleEscapeKey,
    enabled: !!selectedRecipe
  });

    return (
      <>
        {/* Header */}
        <Header
          onCreateWant={handleCreateRecipe}
          title="Recipes"
          createButtonLabel="Add Recipe"
          itemCount={recipes.length}
          itemLabel="recipe"
          showSummary={sidebar.showSummary}
          onSummaryToggle={sidebar.toggleSummary}
        />
  
        {/* Main content area with sidebar-aware layout */}
        <main className="flex-1 flex overflow-hidden bg-gray-50 dark:bg-gray-950 lg:mr-[480px] mr-0">
          {/* Left content area - main dashboard */}
          <div className="flex-1 overflow-y-auto">
            <div className="p-6 pb-24">
            {/* Loading State */}
            {loading && (
              <div className="flex items-center justify-center h-64">
                <div className="text-gray-500 dark:text-gray-400">Loading recipes...</div>
              </div>
            )}
  
            {/* Error Message */}
            {error && (
              <div className="mb-6 p-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md">
                <div className="flex items-center">
                  <div className="flex-shrink-0">
                    <svg
                      className="h-5 w-5 text-red-400"
                      viewBox="0 0 20 20"
                      fill="currentColor"
                    >
                      <path
                        fillRule="evenodd"
                        d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z"
                        clipRule="evenodd"
                      />
                    </svg>
                  </div>
                  <div className="ml-3">
                    <p className="text-sm text-red-700 dark:text-red-300">{error}</p>
                  </div>
                  <div className="ml-auto">
                    <button
                      onClick={clearError}
                      className="text-red-400 hover:text-red-600"
                    >
                      <svg className="h-4 w-4" viewBox="0 0 20 20" fill="currentColor">
                        <path
                          fillRule="evenodd"
                          d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
                          clipRule="evenodd"
                        />
                      </svg>
                      </button>
                    </div>
                  </div>
                </div>
              )}
  
              {/* Recipes Grid */}
              <RecipeGrid
                recipes={recipes}
                loading={loading}
                selectedRecipe={selectedRecipe}
                onViewRecipe={handleViewRecipe}
                onEditRecipe={handleEditRecipe}
                onDeleteRecipe={handleDeleteRecipe}
                onDeployRecipe={handleDeployRecipe}
                onDeployRecipeExample={handleDeployRecipeExample}
                onSelectRecipe={sidebar.selectItem}
                onGetFilteredRecipes={setFilteredRecipes}
                searchQuery={searchQuery}
              />
            </div>
          </div>
        </main>
  
        {/* Summary Sidebar */}
        <RightSidebar
          isOpen={sidebar.showSummary}
          onClose={sidebar.closeSummary}
          title="Summary"
        >
          <div className="space-y-6">
            <div>
              <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Statistics</h3>
              <div>
                <RecipeStatsOverview recipes={recipes} loading={loading} />
              </div>
            </div>
  
            {/* Filters section */}
            <div>
              <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Search</h3>
              <RecipeFilters
                searchQuery={searchQuery}
                onSearchChange={setSearchQuery}
              />
            </div>
          </div>
        </RightSidebar>
  
        {/* Modals */}
        <RecipeModal
          isOpen={showCreateModal}
          onClose={() => {
            sidebar.closeForm();
          }}
          recipe={null}
          mode="create"
        />
  
        <RecipeModal
          isOpen={showEditModal}
          onClose={() => {
            sidebar.closeForm();
            setEditingRecipe(null);
          }}
          recipe={editingRecipe}
          mode="edit"
        />
  
        {/* Delete Confirmation Modal */}
        {showDeleteModal && (
          <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
            <div className="bg-white dark:bg-gray-800 rounded-lg max-w-md w-full p-6">
              <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Delete Recipe</h3>
              <p className="text-gray-600 dark:text-gray-300 mb-6">
                Are you sure you want to delete the recipe "{selectedRecipe?.recipe.metadata.name}"? This action cannot be undone.
              </p>
              <div className="flex justify-end space-x-3">
                <button
                  onClick={() => setShowDeleteModal(false)}
                  className="px-4 py-2 text-gray-700 dark:text-gray-300 border border-gray-300 dark:border-gray-600 rounded-md hover:bg-gray-50 dark:hover:bg-gray-700"
                >
                  Cancel
                </button>
                <button
                  onClick={confirmDeleteRecipe}
                  className="px-4 py-2 bg-red-600 text-white rounded-md hover:bg-red-700"
                >
                  Delete
                </button>
              </div>
            </div>
          </div>
        )}
  
        {/* Notification Toast */}
        {notification && (
          <div className={classNames(
            'fixed top-4 right-4 px-4 py-3 rounded-md shadow-lg flex items-center space-x-2 z-50 animate-fade-in',
            notification.type === 'success'
              ? 'bg-green-50 text-green-800 border border-green-200 dark:bg-green-900/20 dark:text-green-300 dark:border-green-800'
              : 'bg-red-50 text-red-800 border border-red-200 dark:bg-red-900/20 dark:text-red-300 dark:border-red-800'
          )}>
            {notification.type === 'success' ? (
              <Check className="h-5 w-5" />
            ) : (
              <AlertCircle className="h-5 w-5" />
            )}
            <span className="text-sm font-medium">{notification.message}</span>
          </div>
        )}
  
        {/* Right Sidebar for Recipe Details */}
        <RightSidebar
          isOpen={!!selectedRecipe}
          onClose={sidebar.clearSelection}
          title={selectedRecipe ? selectedRecipe.recipe.metadata.name : undefined}
        >
          <RecipeDetailsSidebar
            recipe={selectedRecipe}
            onDeploy={handleDeployRecipe}
            onDeployExample={handleDeployRecipeExample}
            onEdit={handleEditRecipe}
            onDelete={handleDeleteRecipe}
            onDeploySuccess={(message) => setNotification({ message, type: 'success' })}
            onDeployError={(error) => setNotification({ message: error, type: 'error' })}
            loading={loading}
          />
        </RightSidebar>
      </>
    );}