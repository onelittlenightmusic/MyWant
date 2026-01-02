import { create } from 'zustand';
import { apiClient } from '@/api/client';
import { GenericRecipe, RecipeListResponse } from '@/types/recipe';

interface RecipeState {
  recipes: GenericRecipe[];
  currentRecipe: GenericRecipe | null;
  loading: boolean;
  error: string | null;
}

interface RecipeActions {
  fetchRecipes: () => Promise<void>;
  fetchRecipe: (id: string) => Promise<void>;
  createRecipe: (recipe: GenericRecipe) => Promise<void>;
  updateRecipe: (id: string, recipe: GenericRecipe) => Promise<void>;
  deleteRecipe: (id: string) => Promise<void>;
  setCurrentRecipe: (recipe: GenericRecipe | null) => void;
  clearError: () => void;
}

interface RecipeStore extends RecipeState, RecipeActions {}

export const useRecipeStore = create<RecipeStore>((set, get) => ({
  // Initial state
  recipes: [],
  currentRecipe: null,
  loading: false,
  error: null,

  // Actions
  fetchRecipes: async () => {
    set({ loading: true, error: null });
    try {
      const data: RecipeListResponse = await apiClient.listRecipes();

      // Convert the object to array format
      const recipesArray = Object.entries(data).map(([id, recipe]) => ({
        ...recipe,
        id, // Add the ID to the recipe object for easier handling
      })) as GenericRecipe[];

      // Sort deterministically by name to ensure consistent ordering across fetches
      const sortedRecipes = [...recipesArray].sort((a, b) => {
        const nameA = a.recipe?.metadata?.name || (a as any).id || '';
        const nameB = b.recipe?.metadata?.name || (b as any).id || '';
        return nameA.localeCompare(nameB);
      });

      set({ recipes: sortedRecipes, loading: false });
    } catch (error) {
      set({
        error: error instanceof Error ? error.message : 'Failed to fetch recipes',
        loading: false,
      });
    }
  },

  fetchRecipe: async (id: string) => {
    set({ loading: true, error: null });
    try {
      const recipe = await apiClient.getRecipe(id);
      set({ currentRecipe: recipe, loading: false });
    } catch (error) {
      set({
        error: error instanceof Error ? error.message : 'Failed to fetch recipe',
        loading: false,
      });
    }
  },

  createRecipe: async (recipe: GenericRecipe) => {
    set({ loading: true, error: null });
    try {
      await apiClient.createRecipe(recipe);

      // Refresh the recipes list
      await get().fetchRecipes();

      set({ loading: false });
    } catch (error) {
      set({
        error: error instanceof Error ? error.message : 'Failed to create recipe',
        loading: false,
      });
    }
  },

  updateRecipe: async (id: string, recipe: GenericRecipe) => {
    set({ loading: true, error: null });
    try {
      await apiClient.updateRecipe(id, recipe);

      // Refresh the recipes list
      await get().fetchRecipes();

      set({ loading: false });
    } catch (error) {
      set({
        error: error instanceof Error ? error.message : 'Failed to update recipe',
        loading: false,
      });
    }
  },

  deleteRecipe: async (id: string) => {
    set({ loading: true, error: null });
    try {
      await apiClient.deleteRecipe(id);

      // Refresh the recipes list
      await get().fetchRecipes();

      set({ loading: false });
    } catch (error) {
      set({
        error: error instanceof Error ? error.message : 'Failed to delete recipe',
        loading: false,
      });
    }
  },

  setCurrentRecipe: (recipe: GenericRecipe | null) => {
    set({ currentRecipe: recipe });
  },

  clearError: () => {
    set({ error: null });
  },
}));