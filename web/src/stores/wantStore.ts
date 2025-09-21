import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';
import { Want, WantDetails, WantResults, CreateWantRequest } from '@/types/want';
import { apiClient } from '@/api/client';

interface WantStore {
  // State
  wants: Want[];
  selectedWant: Want | null;
  selectedWantDetails: WantDetails | null;
  selectedWantResults: WantResults | null;
  loading: boolean;
  error: string | null;

  // Actions
  fetchWants: () => Promise<void>;
  createWant: (request: CreateWantRequest) => Promise<Want>;
  createWantFromYaml: (yaml: string, name?: string) => Promise<Want>;
  updateWant: (id: string, yaml: string) => Promise<void>;
  deleteWant: (id: string) => Promise<void>;
  selectWant: (want: Want | null) => void;
  fetchWantDetails: (id: string) => Promise<void>;
  fetchWantResults: (id: string) => Promise<void>;
  clearError: () => void;
  refreshWant: (id: string) => Promise<void>;
  suspendWant: (id: string) => Promise<void>;
  resumeWant: (id: string) => Promise<void>;
}

export const useWantStore = create<WantStore>()(
  subscribeWithSelector((set) => ({
    // Initial state
    wants: [],
    selectedWant: null,
    selectedWantDetails: null,
    selectedWantResults: null,
    loading: false,
    error: null,

    // Actions
    fetchWants: async () => {
      set({ loading: true, error: null });
      try {
        const wants = await apiClient.listWants();
        set({ wants, loading: false });
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to fetch wants',
          loading: false
        });
      }
    },

    createWant: async (request: CreateWantRequest) => {
      set({ loading: true, error: null });
      try {
        const want = await apiClient.createWant(request);
        set(state => ({
          wants: [...state.wants, want],
          loading: false
        }));
        return want;
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to create want',
          loading: false
        });
        throw error;
      }
    },

    createWantFromYaml: async (yaml: string, name?: string) => {
      set({ loading: true, error: null });
      try {
        const want = await apiClient.createWantFromYaml(yaml, name);
        set(state => ({
          wants: [...state.wants, want],
          loading: false
        }));
        return want;
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to create want',
          loading: false
        });
        throw error;
      }
    },

    updateWant: async (id: string, yaml: string) => {
      set({ loading: true, error: null });
      try {
        const updatedWant = await apiClient.updateWantFromYaml(id, yaml);
        set(state => ({
          wants: state.wants.map(w => w.id === id ? updatedWant : w),
          selectedWant: state.selectedWant?.id === id ? updatedWant : state.selectedWant,
          loading: false
        }));
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to update want',
          loading: false
        });
        throw error;
      }
    },

    deleteWant: async (id: string) => {
      set({ loading: true, error: null });
      try {
        await apiClient.deleteWant(id);
        set(state => ({
          wants: state.wants.filter(w => w.id !== id),
          selectedWant: state.selectedWant?.id === id ? null : state.selectedWant,
          selectedWantDetails: state.selectedWantDetails?.id === id ? null : state.selectedWantDetails,
          selectedWantResults: null,
          loading: false
        }));
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to delete want',
          loading: false
        });
        throw error;
      }
    },

    selectWant: (want: Want | null) => {
      set({
        selectedWant: want,
        selectedWantDetails: null,
        selectedWantResults: null
      });
    },

    fetchWantDetails: async (id: string) => {
      set({ loading: true, error: null });
      try {
        const details = await apiClient.getWant(id);
        set({
          selectedWantDetails: details,
          loading: false
        });
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to fetch want details',
          loading: false
        });
      }
    },

    fetchWantResults: async (id: string) => {
      set({ loading: true, error: null });
      try {
        const results = await apiClient.getWantResults(id);
        set({
          selectedWantResults: results,
          loading: false
        });
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to fetch want results',
          loading: false
        });
      }
    },

    refreshWant: async (id: string) => {
      try {
        const [want, status] = await Promise.all([
          apiClient.getWant(id),
          apiClient.getWantStatus(id)
        ]);

        set(state => ({
          wants: state.wants.map(w => w.id === id ? { ...w, status: status.status } : w),
          selectedWant: state.selectedWant?.id === id ? { ...state.selectedWant, status: status.status } : state.selectedWant,
          selectedWantDetails: { ...want, suspended: status.suspended }
        }));
      } catch (error) {
        console.error('Failed to refresh want:', error);
      }
    },

    clearError: () => {
      set({ error: null });
    },

    suspendWant: async (id: string) => {
      set({ loading: true, error: null });
      try {
        await apiClient.suspendWant(id);

        // Refresh the want status to get updated suspended state
        const status = await apiClient.getWantStatus(id);

        set(state => ({
          wants: state.wants.map(w => w.id === id ? { ...w, status: status.status } : w),
          selectedWant: state.selectedWant?.id === id ? { ...state.selectedWant, status: status.status } : state.selectedWant,
          selectedWantDetails: state.selectedWantDetails?.id === id ?
            { ...state.selectedWantDetails, suspended: status.suspended } :
            state.selectedWantDetails,
          loading: false
        }));
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to suspend want',
          loading: false
        });
        throw error;
      }
    },

    resumeWant: async (id: string) => {
      set({ loading: true, error: null });
      try {
        await apiClient.resumeWant(id);

        // Refresh the want status to get updated suspended state
        const status = await apiClient.getWantStatus(id);

        set(state => ({
          wants: state.wants.map(w => w.id === id ? { ...w, status: status.status } : w),
          selectedWant: state.selectedWant?.id === id ? { ...state.selectedWant, status: status.status } : state.selectedWant,
          selectedWantDetails: state.selectedWantDetails?.id === id ?
            { ...state.selectedWantDetails, suspended: status.suspended } :
            state.selectedWantDetails,
          loading: false
        }));
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to resume want',
          loading: false
        });
        throw error;
      }
    },
  }))
);

// Auto-refresh wants every 5 seconds
const startAutoRefresh = () => {
  setInterval(() => {
    const store = useWantStore.getState();
    if (store.wants.length > 0) {
      store.fetchWants();
    }
  }, 5000);
};

// Start auto-refresh when store is first used
let autoRefreshStarted = false;
useWantStore.subscribe((state) => {
  if (!autoRefreshStarted && state.wants.length > 0) {
    autoRefreshStarted = true;
    startAutoRefresh();
  }
});