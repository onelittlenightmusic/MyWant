import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';
import { Want, WantDetails, WantResults, CreateWantRequest, UpdateWantRequest } from '@/types/want';
import { apiClient } from '@/api/client';

interface WantStore {
  // State
  wants: Want[];
  selectedWant: Want | null;
  selectedWantDetails: WantDetails | null;
  selectedWantResults: WantResults | null;
  loading: boolean;
  error: string | null;
  draggingWant: string | null;
  isOverTarget: boolean;

  // Actions
  fetchWants: () => Promise<void>;
  createWant: (request: CreateWantRequest) => Promise<Want>;
  updateWant: (id: string, request: UpdateWantRequest) => Promise<void>;
  deleteWant: (id: string) => Promise<void>;
  deleteWants: (ids: string[]) => Promise<void>;
  selectWant: (want: Want | null) => void;
  fetchWantDetails: (id: string) => Promise<void>;
  fetchWantResults: (id: string) => Promise<void>;
  clearError: () => void;
  refreshWant: (id: string) => Promise<void>;
  suspendWant: (id: string) => Promise<void>;
  resumeWant: (id: string) => Promise<void>;
  stopWant: (id: string) => Promise<void>;
  startWant: (id: string) => Promise<void>;
  suspendWants: (ids: string[]) => Promise<void>;
  resumeWants: (ids: string[]) => Promise<void>;
  stopWants: (ids: string[]) => Promise<void>;
  startWants: (ids: string[]) => Promise<void>;
  setDraggingWant: (wantId: string | null) => void;
  setIsOverTarget: (isOver: boolean) => void;
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
    draggingWant: null,
    isOverTarget: false,

    // Actions
    setDraggingWant: (wantId: string | null) => set({ draggingWant: wantId }),
    setIsOverTarget: (isOver: boolean) => set({ isOverTarget: isOver }),

    fetchWants: async () => {
      set({ loading: true, error: null });
      try {
        const wants = await apiClient.listWants();
        // Sort deterministically by ID to ensure consistent ordering across fetches
        const sortedWants = [...wants].sort((a, b) => {
          const idA = a.metadata?.id || a.id || '';
          const idB = b.metadata?.id || b.id || '';
          return idA.localeCompare(idB);
        });
        set({ wants: sortedWants, loading: false });
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

    updateWant: async (id: string, request: UpdateWantRequest) => {
      set({ loading: true, error: null });
      try {
        const updatedWant = await apiClient.updateWant(id, request);
        set(state => ({
          wants: state.wants.map(w => w.id === id ? updatedWant : w),
          selectedWant: state.selectedWant?.id === id ? updatedWant : state.selectedWant,
          selectedWantDetails: state.selectedWantDetails?.metadata?.id === id ? updatedWant : state.selectedWantDetails,
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
      // Optimistically update the status of the specific want to 'deleting'
      set(state => ({
        wants: state.wants.map(w =>
          (w.metadata?.id === id || w.id === id) ? { ...w, status: 'deleting' } : w
        ),
      }));

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
        // If deletion fails, revert the status or rely on next fetchWants to correct
        set(state => ({
          wants: state.wants.map(w =>
            (w.metadata?.id === id || w.id === id) ? { ...w, status: 'failed' } : w // Revert to a 'failed' status on error
          ),
          error: error instanceof Error ? error.message : 'Failed to delete want',
          loading: false
        }));
        throw error;
      }
    },

    deleteWants: async (ids: string[]) => {
      set({ loading: true, error: null });
      // Optimistically update the status of specific wants to 'deleting'
      set(state => ({
        wants: state.wants.map(w =>
          (ids.includes(w.metadata?.id || w.id || '')) ? { ...w, status: 'deleting' } : w
        ),
      }));

      try {
        await apiClient.deleteWants(ids);
        set(state => ({
          wants: state.wants.filter(w => !ids.includes(w.metadata?.id || w.id || '')),
          selectedWant: (state.selectedWant && ids.includes(state.selectedWant.metadata?.id || state.selectedWant.id || '')) ? null : state.selectedWant,
          selectedWantDetails: (state.selectedWantDetails && ids.includes(state.selectedWantDetails.metadata?.id || state.selectedWantDetails.id || '')) ? null : state.selectedWantDetails,
          selectedWantResults: (state.selectedWantDetails && ids.includes(state.selectedWantDetails.metadata?.id || state.selectedWantDetails.id || '')) ? null : state.selectedWantResults,
          loading: false
        }));
      } catch (error) {
        // If deletion fails, revert the status or rely on next fetchWants to correct
        set(state => ({
          wants: state.wants.map(w =>
            (ids.includes(w.metadata?.id || w.id || '')) ? { ...w, status: 'failed' } : w // Revert to a 'failed' status on error
          ),
          error: error instanceof Error ? error.message : 'Failed to delete wants',
          loading: false
        }));
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

        // Refresh the want status to get updated status
        const status = await apiClient.getWantStatus(id);

        set(state => ({
          wants: state.wants.map(w => (w.metadata?.id === id || w.id === id) ? { ...w, status: status.status } : w),
          selectedWant: (state.selectedWant?.metadata?.id === id || state.selectedWant?.id === id) ? { ...state.selectedWant, status: status.status } : state.selectedWant,
          selectedWantDetails: (state.selectedWantDetails?.metadata?.id === id || state.selectedWantDetails?.id === id) ?
            { ...state.selectedWantDetails, status: status.status } :
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

        // Refresh the want status to get updated status
        const status = await apiClient.getWantStatus(id);

        set(state => ({
          wants: state.wants.map(w => (w.metadata?.id === id || w.id === id) ? { ...w, status: status.status } : w),
          selectedWant: (state.selectedWant?.metadata?.id === id || state.selectedWant?.id === id) ? { ...state.selectedWant, status: status.status } : state.selectedWant,
          selectedWantDetails: (state.selectedWantDetails?.metadata?.id === id || state.selectedWantDetails?.id === id) ?
            { ...state.selectedWantDetails, status: status.status } :
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

    stopWant: async (id: string) => {
      set({ loading: true, error: null });
      try {
        await apiClient.stopWant(id);

        // Refresh the want status to get updated state
        const status = await apiClient.getWantStatus(id);

        set(state => ({
          wants: state.wants.map(w => (w.metadata?.id === id || w.id === id) ? { ...w, status: status.status } : w),
          selectedWant: (state.selectedWant?.metadata?.id === id || state.selectedWant?.id === id) ? { ...state.selectedWant, status: status.status } : state.selectedWant,
          selectedWantDetails: (state.selectedWantDetails?.metadata?.id === id || state.selectedWantDetails?.id === id) ?
            { ...state.selectedWantDetails, status: status.status } :
            state.selectedWantDetails,
          loading: false
        }));
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to stop want',
          loading: false
        });
        throw error;
      }
    },

    startWant: async (id: string) => {
      set({ loading: true, error: null });
      try {
        await apiClient.startWant(id);

        // Refresh the want status to get updated state
        const status = await apiClient.getWantStatus(id);

        set(state => ({
          wants: state.wants.map(w => (w.metadata?.id === id || w.id === id) ? { ...w, status: status.status } : w),
          selectedWant: (state.selectedWant?.metadata?.id === id || state.selectedWant?.id === id) ? { ...state.selectedWant, status: status.status } : state.selectedWant,
          selectedWantDetails: (state.selectedWantDetails?.metadata?.id === id || state.selectedWantDetails?.id === id) ?
            { ...state.selectedWantDetails, status: status.status } :
            state.selectedWantDetails,
          loading: false
        }));
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to start want',
          loading: false
        });
        throw error;
      }
    },

    suspendWants: async (ids: string[]) => {
      set({ loading: true, error: null });
      try {
        await apiClient.suspendWants(ids);
        set({ loading: false });
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to suspend wants',
          loading: false
        });
        throw error;
      }
    },

    resumeWants: async (ids: string[]) => {
      set({ loading: true, error: null });
      try {
        await apiClient.resumeWants(ids);
        set({ loading: false });
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to resume wants',
          loading: false
        });
        throw error;
      }
    },

    stopWants: async (ids: string[]) => {
      set({ loading: true, error: null });
      try {
        await apiClient.stopWants(ids);
        set({ loading: false });
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to stop wants',
          loading: false
        });
        throw error;
      }
    },

    startWants: async (ids: string[]) => {
      set({ loading: true, error: null });
      try {
        await apiClient.startWants(ids);
        set({ loading: false });
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to start wants',
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