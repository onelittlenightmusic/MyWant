import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';
import { apiClient } from '@/api/client';

interface LabelHistoryStore {
  // State
  labelKeys: string[];
  labelValues: Record<string, string[]>; // Map of key -> array of values
  loading: boolean;
  error: string | null;

  // Actions
  fetchLabels: () => Promise<void>;
  clearError: () => void;
}

export const useLabelHistoryStore = create<LabelHistoryStore>()(
  subscribeWithSelector((set) => ({
    // Initial state
    labelKeys: [],
    labelValues: {},
    loading: false,
    error: null,

    // Actions
    fetchLabels: async () => {
      set({ loading: true, error: null });
      try {
        const response = await apiClient.getLabels();
        set({
          labelKeys: response.labelKeys,
          labelValues: response.labelValues || {},
          loading: false
        });
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to fetch labels',
          loading: false
        });
      }
    },

    clearError: () => {
      set({ error: null });
    },
  }))
);

// Auto-refresh labels every 10 seconds when they exist
const startAutoRefresh = () => {
  setInterval(() => {
    const store = useLabelHistoryStore.getState();
    // Only refresh if we have labels and no error state
    if (store.labelKeys.length > 0 && !store.error) {
      store.fetchLabels();
    }
  }, 10000);
};

// Start auto-refresh when store is first used with labels
let autoRefreshStarted = false;
useLabelHistoryStore.subscribe((state) => {
  if (!autoRefreshStarted && state.labelKeys.length > 0) {
    autoRefreshStarted = true;
    startAutoRefresh();
  }
});
