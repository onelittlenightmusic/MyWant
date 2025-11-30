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

        // Convert API response format to the expected labelValues format
        // API returns: { labelValues: { key: [{ value: string, owners: [], users: [] }, ...] } }
        // We need: { key: [string, ...] }
        const convertedLabelValues: Record<string, string[]> = {};

        if (response.labelValues) {
          for (const [key, valuesArray] of Object.entries(response.labelValues)) {
            if (Array.isArray(valuesArray)) {
              convertedLabelValues[key] = (valuesArray as any[]).map(item =>
                typeof item === 'string' ? item : item.value
              ).filter(Boolean);
            }
          }
        }

        set({
          labelKeys: response.labelKeys,
          labelValues: convertedLabelValues,
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

// Auto-refresh is disabled to prevent excessive API calls
// The component that uses the labels should handle periodic refresh if needed
