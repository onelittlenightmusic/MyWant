import React, { useState, useEffect } from 'react';
import { Plus, BookOpen, Edit2, Trash2, Eye, Menu } from 'lucide-react';
import { useRecipeStore } from '@/stores/recipeStore';
import { GenericRecipe } from '@/types/recipe';
import RecipeModal from '@/components/modals/RecipeModal';
import RecipeDetailsModal from '@/components/modals/RecipeDetailsModal';
import { Sidebar } from '@/components/layout/Sidebar';

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
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showEditModal, setShowEditModal] = useState(false);
  const [showDetailsModal, setShowDetailsModal] = useState(false);
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const [selectedRecipe, setSelectedRecipe] = useState<GenericRecipe | null>(null);

  useEffect(() => {
    fetchRecipes();
  }, [fetchRecipes]);

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
    setShowDetailsModal(true);
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

  const formatParametersCount = (params?: Record<string, any>) => {
    if (!params) return 0;
    return Object.keys(params).length;
  };

  return (
    <div className="min-h-screen bg-gray-50 flex">
      {/* Mobile sidebar toggle */}
      <div className="lg:hidden fixed top-4 left-4 z-40">
        <button
          onClick={() => setSidebarOpen(true)}
          className="p-2 rounded-md bg-white shadow-md border border-gray-200 text-gray-600 hover:text-gray-900"
        >
          <Menu className="h-5 w-5" />
        </button>
      </div>

      {/* Sidebar */}
      <Sidebar
        isOpen={sidebarOpen}
        onClose={() => setSidebarOpen(false)}
      />

      {/* Main content */}
      <div className="flex-1 lg:ml-0 flex flex-col">
        {/* Custom Header for Recipes */}
        <header className="bg-white border-b border-gray-200 px-6 py-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center space-x-4">
              <h1 className="text-2xl font-bold text-gray-900 flex items-center gap-2">
                <BookOpen className="h-6 w-6" />
                Recipe Manager
              </h1>
              <div className="text-sm text-gray-500">
                {recipes.length} recipe{recipes.length !== 1 ? 's' : ''}
              </div>
            </div>

            <div className="flex items-center space-x-3">
              <button
                onClick={() => fetchRecipes()}
                disabled={loading}
                className="flex items-center space-x-2 px-3 py-2 text-gray-600 hover:text-gray-900 border border-gray-300 rounded-md hover:bg-gray-50 disabled:opacity-50"
              >
                {loading ? (
                  <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-gray-600"></div>
                ) : (
                  <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                  </svg>
                )}
                <span>Refresh</span>
              </button>

              <button
                onClick={handleCreateRecipe}
                className="flex items-center space-x-2 px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2"
              >
                <Plus className="h-4 w-4" />
                <span>Create Recipe</span>
              </button>
            </div>
          </div>
        </header>

        {/* Main content area */}
        <main className="flex-1 p-6">
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

          {/* Recipes Grid */}
          {recipes.length === 0 ? (
            <div className="text-center py-12">
              <BookOpen className="mx-auto h-12 w-12 text-gray-400" />
              <h3 className="mt-2 text-sm font-medium text-gray-900">No recipes</h3>
              <p className="mt-1 text-sm text-gray-500">
                Get started by creating a new recipe template.
              </p>
              <div className="mt-6">
                <button
                  onClick={handleCreateRecipe}
                  className="bg-blue-600 hover:bg-blue-700 text-white px-4 py-2 rounded-lg flex items-center gap-2 mx-auto transition-colors"
                >
                  <Plus className="h-4 w-4" />
                  Create Recipe
                </button>
              </div>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
              {recipes.map((recipe) => (
                <div
                  key={recipe.recipe.metadata.name}
                  className="bg-white border border-gray-200 rounded-lg p-6 hover:shadow-lg transition-shadow"
                >
                  <div className="flex items-start justify-between mb-4">
                    <div>
                      <h3 className="text-lg font-semibold text-gray-900">
                        {recipe.recipe.metadata.name}
                      </h3>
                      <p className="text-sm text-gray-500">
                        {recipe.recipe.metadata.description || 'No description'}
                      </p>
                    </div>
                  </div>

                  <div className="space-y-2 mb-4">
                    <div className="flex justify-between text-sm">
                      <span className="text-gray-500">Wants:</span>
                      <span className="text-gray-900">{recipe.recipe.wants.length}</span>
                    </div>
                    <div className="flex justify-between text-sm">
                      <span className="text-gray-500">Parameters:</span>
                      <span className="text-gray-900">
                        {formatParametersCount(recipe.recipe.parameters)}
                      </span>
                    </div>
                    {recipe.recipe.metadata.version && (
                      <div className="flex justify-between text-sm">
                        <span className="text-gray-500">Version:</span>
                        <span className="text-gray-900">{recipe.recipe.metadata.version}</span>
                      </div>
                    )}
                  </div>

                  <div className="flex items-center justify-between pt-4 border-t border-gray-200">
                    <div className="flex space-x-2">
                      <button
                        onClick={() => handleViewRecipe(recipe)}
                        className="text-blue-600 hover:text-blue-800 p-1"
                        title="View Details"
                      >
                        <Eye className="h-4 w-4" />
                      </button>
                      <button
                        onClick={() => handleEditRecipe(recipe)}
                        className="text-yellow-600 hover:text-yellow-800 p-1"
                        title="Edit Recipe"
                      >
                        <Edit2 className="h-4 w-4" />
                      </button>
                      <button
                        onClick={() => handleDeleteRecipe(recipe)}
                        className="text-red-600 hover:text-red-800 p-1"
                        title="Delete Recipe"
                      >
                        <Trash2 className="h-4 w-4" />
                      </button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}

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

          <RecipeDetailsModal
            isOpen={showDetailsModal}
            onClose={() => setShowDetailsModal(false)}
            recipe={selectedRecipe}
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

        </main>
      </div>
    </div>
  );
}