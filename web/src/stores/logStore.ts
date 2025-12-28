import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';
import { LogEntry } from '@/types/api';
import { apiClient } from '@/api/client';

interface LogStore {
  logs: LogEntry[];
  loading: boolean;
  error: string | null;
  fetchLogs: () => Promise<void>;
  clearAllLogs: () => Promise<void>;
  clearError: () => void;
}

export const useLogStore = create<LogStore>()(
  subscribeWithSelector((set) => ({
    logs: [],
    loading: false,
    error: null,

    fetchLogs: async () => {
      set({ loading: true, error: null });
      try {
        const response = await apiClient.listLogs();
        set({ logs: response.logs, loading: false });
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to fetch logs',
          loading: false
        });
      }
    },

    clearAllLogs: async () => {
      set({ loading: true, error: null });
      try {
        await apiClient.clearLogs();
        set({ logs: [], loading: false });
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to clear logs',
          loading: false
        });
        throw error;
      }
    },

    clearError: () => {
      set({ error: null });
    },
  }))
);

// Auto-refresh logs every 30 seconds
const startAutoRefresh = () => {
  setInterval(() => {
    const store = useLogStore.getState();
    // Only refresh if no error state
    if (!store.error) {
      store.fetchLogs();
    }
  }, 30000);
};

// Start auto-refresh when store is first used with logs
let autoRefreshStarted = false;
useLogStore.subscribe((state) => {
  if (!autoRefreshStarted && state.logs.length > 0) {
    autoRefreshStarted = true;
    startAutoRefresh();
  }
});
