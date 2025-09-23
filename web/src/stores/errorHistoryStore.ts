import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';
import { ErrorHistoryEntry } from '@/types/api';
import { apiClient } from '@/api/client';

interface ErrorHistoryStore {
  // State
  errors: ErrorHistoryEntry[];
  selectedError: ErrorHistoryEntry | null;
  loading: boolean;
  error: string | null;

  // Actions
  fetchErrorHistory: () => Promise<void>;
  getErrorEntry: (id: string) => Promise<void>;
  updateErrorEntry: (id: string, updates: { resolved?: boolean; notes?: string }) => Promise<void>;
  deleteErrorEntry: (id: string) => Promise<void>;
  selectError: (error: ErrorHistoryEntry | null) => void;
  clearError: () => void;
  markAsResolved: (id: string) => Promise<void>;
  addNotes: (id: string, notes: string) => Promise<void>;
}

export const useErrorHistoryStore = create<ErrorHistoryStore>()(
  subscribeWithSelector((set, get) => ({
    // Initial state
    errors: [],
    selectedError: null,
    loading: false,
    error: null,

    // Actions
    fetchErrorHistory: async () => {
      set({ loading: true, error: null });
      try {
        const response = await apiClient.listErrorHistory();
        set({ errors: response.errors, loading: false });
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to fetch error history',
          loading: false
        });
      }
    },

    getErrorEntry: async (id: string) => {
      set({ loading: true, error: null });
      try {
        const errorEntry = await apiClient.getErrorHistoryEntry(id);
        set({ selectedError: errorEntry, loading: false });
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to fetch error entry',
          loading: false
        });
      }
    },

    updateErrorEntry: async (id: string, updates: { resolved?: boolean; notes?: string }) => {
      set({ loading: true, error: null });
      try {
        const updatedError = await apiClient.updateErrorHistoryEntry(id, updates);

        // Update the error in the list
        set(state => ({
          errors: state.errors.map(e => e.id === id ? updatedError : e),
          selectedError: state.selectedError?.id === id ? updatedError : state.selectedError,
          loading: false
        }));
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to update error entry',
          loading: false
        });
        throw error;
      }
    },

    deleteErrorEntry: async (id: string) => {
      set({ loading: true, error: null });
      try {
        await apiClient.deleteErrorHistoryEntry(id);

        // Remove the error from the list
        set(state => ({
          errors: state.errors.filter(e => e.id !== id),
          selectedError: state.selectedError?.id === id ? null : state.selectedError,
          loading: false
        }));
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to delete error entry',
          loading: false
        });
        throw error;
      }
    },

    selectError: (error: ErrorHistoryEntry | null) => {
      set({ selectedError: error });
    },

    clearError: () => {
      set({ error: null });
    },

    markAsResolved: async (id: string) => {
      await get().updateErrorEntry(id, { resolved: true });
    },

    addNotes: async (id: string, notes: string) => {
      await get().updateErrorEntry(id, { notes });
    },
  }))
);

// Auto-refresh error history every 30 seconds
const startAutoRefresh = () => {
  setInterval(() => {
    const store = useErrorHistoryStore.getState();
    // Only refresh if we have errors and no error state
    if (store.errors.length > 0 && !store.error) {
      store.fetchErrorHistory();
    }
  }, 30000);
};

// Start auto-refresh when store is first used
let autoRefreshStarted = false;
useErrorHistoryStore.subscribe((state) => {
  if (!autoRefreshStarted && state.errors.length > 0) {
    autoRefreshStarted = true;
    startAutoRefresh();
  }
});