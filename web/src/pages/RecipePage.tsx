import { useState, useEffect } from 'react';
import { Plus, Check, AlertCircle } from 'lucide-react';
import { useRecipeStore } from '@/stores/recipeStore';
import { GenericRecipe } from '@/types/recipe';
import { useKeyboardNavigation } from '@/hooks/useKeyboardNavigation';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import RecipeModal from '@/components/modals/RecipeModal';
import { RecipeDetailsSidebar } from '@/components/sidebar/RecipeDetailsSidebar';
import { RightSidebar } from '@/components/layout/RightSidebar';
import { Layout } from '@/components/layout/Layout';
import { Header } from '@/components/layout/Header';
import { RecipeGrid } from '@/components/dashboard/RecipeGrid';
import { RecipeControlPanel } from '@/components/dashboard/RecipeControlPanel';
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

  // UI State
  const [sidebarMinimized, setSidebarMinimized] = useState(false); // Start expanded, auto-collapse on mouse leave
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showEditModal, setShowEditModal] = useState(false);
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const [selectedRecipe, setSelectedRecipe] = useState<GenericRecipe | null>(null);
  const [notification, setNotification] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [filteredRecipes, setFilteredRecipes] = useState<GenericRecipe[]>([]);
  const [searchQuery, setSearchQuery] = useState('');

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

  const handleCreateRecipe = () => {
    setSelectedRecipe(null);
    setShowCreateModal(true);
  };

  const handleEditRecipe = (recipe: GenericRecipe) => {
    setSelectedRecipe(recipe);
    setShowEditModal(true);
  };

  const handleViewRecipe = (recipe: GenericRecipe) => {
    setSelectedRecipe(recipe);
  };

  const handleDeleteRecipe = (recipe: GenericRecipe) => {
    setSelectedRecipe(recipe);
    setShowDeleteModal(true);
  };

  const confirmDeleteRecipe = async () => {
    if (selectedRecipe) {
      await deleteRecipe(selectedRecipe.recipe.metadata.name);
      setShowDeleteModal(false);
      setSelectedRecipe(null);
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
    enabled: !showCreateModal && !showEditModal && filteredRecipes.length > 0
  });

  // Handle ESC key to close details sidebar and deselect
  const handleEscapeKey = () => {
    if (selectedRecipe) {
      setSelectedRecipe(null);
    }
  };

  useEscapeKey({
    onEscape: handleEscapeKey,
    enabled: !!selectedRecipe
  });

  return (
    <Layout
      sidebarMinimized={sidebarMinimized}
      onSidebarMinimizedChange={setSidebarMinimized}
    >
      {/* Header */}
      <Header
        onCreateWant={handleCreateRecipe}
        searchQuery={searchQuery}
        onSearchChange={setSearchQuery}
        title="Recipes"
        createButtonLabel="Create Recipe"
        itemCount={recipes.length}
        itemLabel="recipe"
        searchPlaceholder="Search recipes by name..."
        onRefresh={() => fetchRecipes()}
        loading={loading}
      />

      {/* Main content area with sidebar-aware layout */}
      <main className="flex-1 flex overflow-hidden bg-gray-50">
        {/* Left content area - main dashboard */}
        <div className="flex-1 overflow-y-auto">
          <div className="p-6 pb-24">
          {/* Loading State */}
          {loading && (
            <div className="flex items-center justify-center h-64">
              <div className="text-gray-500">Loading recipes...</div>
            </div>
          )}

          {/* Error Message */}
          {error && (
            <div className="mb-6 p-4 bg-red-50 border border-red-200 rounded-md">
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
                  <p className="text-sm text-red-700">{error}</p>
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

            {/* Error Message */}
            {error && (
              <div className="mb-6 p-4 bg-red-50 border border-red-200 rounded-md">
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
                    <p className="text-sm text-red-700">{error}</p>
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
              onSelectRecipe={setSelectedRecipe}
              onGetFilteredRecipes={setFilteredRecipes}
            />
          </div>
        </div>

        {/* Right sidebar area - reserved for statistics (hidden when sidebar is open) */}
        <div className={`w-[480px] bg-white border-l border-gray-200 overflow-y-auto transition-opacity duration-300 ease-in-out ${selectedRecipe ? 'opacity-0 pointer-events-none' : 'opacity-100'}`}>
          <div className="p-6 space-y-6">
            <div>
              <h3 className="text-lg font-semibold text-gray-900 mb-4">Statistics</h3>
              <div>
                <RecipeStatsOverview recipes={recipes} loading={loading} />
              </div>
            </div>

            {/* Filters section */}
            <div>
              <h3 className="text-lg font-semibold text-gray-900 mb-4">Search</h3>
              <RecipeFilters
                searchQuery={searchQuery}
                onSearchChange={setSearchQuery}
              />
            </div>
          </div>
        </div>
      </main>

      {/* Modals */}
      <RecipeModal
        isOpen={showCreateModal}
        onClose={() => setShowCreateModal(false)}
        recipe={null}
        mode="create"
      />

      <RecipeModal
        isOpen={showEditModal}
        onClose={() => setShowEditModal(false)}
        recipe={selectedRecipe}
        mode="edit"
      />

      {/* Delete Confirmation Modal */}
      {showDeleteModal && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
          <div className="bg-white rounded-lg max-w-md w-full p-6">
            <h3 className="text-lg font-semibold text-gray-900 mb-4">Delete Recipe</h3>
            <p className="text-gray-600 mb-6">
              Are you sure you want to delete the recipe "{selectedRecipe?.recipe.metadata.name}"? This action cannot be undone.
            </p>
            <div className="flex justify-end space-x-3">
              <button
                onClick={() => setShowDeleteModal(false)}
                className="px-4 py-2 text-gray-700 border border-gray-300 rounded-md hover:bg-gray-50"
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
            ? 'bg-green-50 text-green-800 border border-green-200'
            : 'bg-red-50 text-red-800 border border-red-200'
        )}>
          {notification.type === 'success' ? (
            <Check className="h-5 w-5" />
          ) : (
            <AlertCircle className="h-5 w-5" />
          )}
          <span className="text-sm font-medium">{notification.message}</span>
        </div>
      )}

      {/* Recipe Control Panel */}
      <RecipeControlPanel
        selectedRecipe={selectedRecipe}
        onEdit={handleEditRecipe}
        onDelete={handleDeleteRecipe}
        onDeploySuccess={(message) => setNotification({ message, type: 'success' })}
        onDeployError={(error) => setNotification({ message: error, type: 'error' })}
        loading={loading}
        sidebarMinimized={sidebarMinimized}
      />

      {/* Right Sidebar for Recipe Details */}
      <RightSidebar
        isOpen={!!selectedRecipe}
        onClose={() => setSelectedRecipe(null)}
        title={selectedRecipe ? selectedRecipe.recipe.metadata.name : undefined}
      >
        <RecipeDetailsSidebar
          recipe={selectedRecipe}
          onDeploy={async (recipe) => {
            try {
              // Implementation handled by RecipeDetailsSidebar
            } catch (error) {
              setNotification({
                message: error instanceof Error ? error.message : 'Deployment failed',
                type: 'error'
              });
            }
          }}
          onEdit={handleEditRecipe}
          onDelete={handleDeleteRecipe}
          onDeploySuccess={(message) => setNotification({ message, type: 'success' })}
          onDeployError={(error) => setNotification({ message: error, type: 'error' })}
          loading={loading}
        />
      </RightSidebar>
    </Layout>
  );
}