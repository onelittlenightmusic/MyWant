import { create } from 'zustand';
import { apiClient } from '@/api/client';
import {
  WantTypeListItem,
  WantTypeDefinition,
  WantTypeFilters,
} from '@/types/wantType';

interface WantTypeStore {
  // State
  wantTypes: WantTypeListItem[];
  selectedWantType: WantTypeDefinition | null;
  categories: string[];
  patterns: string[];
  loading: boolean;
  error: string | null;
  filters: WantTypeFilters;
  pendingRequest?: string; // Track pending want type request

  // Actions
  fetchWantTypes: () => Promise<void>;
  getWantType: (name: string) => Promise<void>;
  setSelectedWantType: (wantType: WantTypeDefinition | null) => void;
  setFilters: (filters: Partial<WantTypeFilters>) => void;
  clearFilters: () => void;
  clearError: () => void;

  // Computed
  getFilteredWantTypes: () => WantTypeListItem[];
  getCategories: () => string[];
  getPatterns: () => string[];
}

export const useWantTypeStore = create<WantTypeStore>((set, get) => ({
  // Initial state
  wantTypes: [],
  selectedWantType: null,
  categories: [],
  patterns: [],
  loading: false,
  error: null,
  filters: {},
  pendingRequest: undefined,

  // Fetch all want types with optional filters
  fetchWantTypes: async () => {
    set({ loading: true, error: null });
    try {
      const { filters } = get();
      const response = await apiClient.listWantTypes(
        filters.category,
        filters.pattern
      );

      // Extract unique categories and patterns
      const categories = new Set<string>();
      const patterns = new Set<string>();

      response.wantTypes.forEach(wt => {
        categories.add(wt.category);
        patterns.add(wt.pattern);
      });

      // Sort deterministically by name to ensure consistent ordering across fetches
      const sortedWantTypes = [...response.wantTypes].sort((a, b) => {
        const nameA = a.name || '';
        const nameB = b.name || '';
        return nameA.localeCompare(nameB);
      });

      set({
        wantTypes: sortedWantTypes,
        categories: Array.from(categories).sort(),
        patterns: Array.from(patterns).sort(),
        loading: false,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to fetch want types';
      set({
        error: message,
        loading: false,
        wantTypes: [],
      });
    }
  },

  // Fetch detailed want type - prevent duplicate concurrent requests
  getWantType: async (name: string) => {
    const current = get();

    // Skip if there's already a pending request for this want type
    if (current.pendingRequest === name) {
      return;
    }

    set({ pendingRequest: name, error: null });
    try {
      const response = await apiClient.getWantType(name);
      set({
        selectedWantType: response,
        pendingRequest: undefined,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to fetch want type details';
      set({
        error: message,
        selectedWantType: null,
        pendingRequest: undefined,
      });
    }
  },

  // Set selected want type
  setSelectedWantType: (wantType: WantTypeDefinition | null) => {
    set({ selectedWantType: wantType });
  },

  // Update filters and refetch
  setFilters: (newFilters: Partial<WantTypeFilters>) => {
    const currentFilters = get().filters;
    set({ filters: { ...currentFilters, ...newFilters } });
    // Trigger refetch with new filters
    setTimeout(() => {
      get().fetchWantTypes();
    }, 0);
  },

  // Clear all filters
  clearFilters: () => {
    set({ filters: {} });
    setTimeout(() => {
      get().fetchWantTypes();
    }, 0);
  },

  // Clear error message
  clearError: () => {
    set({ error: null });
  },

  // Get filtered want types based on search term
  getFilteredWantTypes: () => {
    const { wantTypes, filters } = get();
    let filtered = wantTypes;

    if (filters.searchTerm) {
      const term = filters.searchTerm.toLowerCase();
      filtered = filtered.filter(
        wt =>
          wt.name.toLowerCase().includes(term) ||
          wt.title.toLowerCase().includes(term)
      );
    }

    return filtered;
  },

  // Get unique categories
  getCategories: () => {
    const { categories } = get();
    return categories;
  },

  // Get unique patterns
  getPatterns: () => {
    const { patterns } = get();
    return patterns;
  },
}));
