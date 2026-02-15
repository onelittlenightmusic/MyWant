import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';
import { Want, WantDetails, WantResults, CreateWantRequest, UpdateWantRequest } from '@/types/want';
import { apiClient } from '@/api/client';

interface DraggingTemplate {
  id: string;
  type: 'want-type' | 'recipe';
  name: string;
}

interface WantStore {
  // State
  wants: Want[];
  selectedWant: Want | null;
  selectedWantDetails: WantDetails | null;
  selectedWantResults: WantResults | null;
  loading: boolean;
  error: string | null;
  draggingWant: string | null;
  draggingTemplate: DraggingTemplate | null;
  isOverTarget: boolean;
  highlightedLabel: { key: string; value: string } | null;
  blinkingWantId: string | null;
  isInitialLoad: boolean; // Track if this is the first load

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
  setDraggingTemplate: (template: DraggingTemplate | null) => void;
  setIsOverTarget: (isOver: boolean) => void;
  setHighlightedLabel: (label: { key: string; value: string } | null) => void;
  setBlinkingWantId: (wantId: string | null) => void;
  reorderWant: (id: string, previousWantId?: string, nextWantId?: string) => Promise<void>;
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
    draggingTemplate: null,
    isOverTarget: false,
    highlightedLabel: null,
    blinkingWantId: null,
    isInitialLoad: true,

    // Actions
    setDraggingWant: (wantId: string | null) => set({ draggingWant: wantId }),
    setDraggingTemplate: (template: DraggingTemplate | null) => set({ draggingTemplate: template }),
    setIsOverTarget: (isOver: boolean) => set({ isOverTarget: isOver }),
    setHighlightedLabel: (label: { key: string; value: string } | null) => {
      set({ highlightedLabel: label });
      // Automatically clear after a short delay so the animation can be re-triggered
      if (label) {
        setTimeout(() => {
          set({ highlightedLabel: null });
        }, 2000);
      }
    },
    setBlinkingWantId: (wantId: string | null) => {
      set({ blinkingWantId: wantId });
      if (wantId) {
        setTimeout(() => {
          set({ blinkingWantId: null });
        }, 2000);
      }
    },

    reorderWant: async (id: string, previousWantId?: string, nextWantId?: string) => {
      set({ loading: true, error: null });
      try {
        await apiClient.updateWantOrder(id, { previousWantId, nextWantId });
        // After reordering, we need to fetch wants again to get the updated order keys
        const wants = await apiClient.listWants();
        
        // Use sorting logic respecting orderKey
        const sortedWants = [...wants].sort((a, b) => {
          const keyA = a.metadata?.orderKey || a.metadata?.id || '';
          const keyB = b.metadata?.orderKey || b.metadata?.id || '';
          return keyA.localeCompare(keyB);
        });

        set({ 
          wants: sortedWants,
          loading: false 
        });
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to reorder want',
          loading: false
        });
        throw error;
      }
    },

    fetchWants: async () => {
      const currentState = useWantStore.getState();

      // Only show loading on initial load
      if (currentState.isInitialLoad) {
        set({ loading: true, error: null });
      }

      try {
        const wants = await apiClient.listWants();
        // Sort by orderKey for consistent ordering that respects reordering
        const sortedWants = [...wants].sort((a, b) => {
          const keyA = a.metadata?.orderKey || a.metadata?.id || '';
          const keyB = b.metadata?.orderKey || b.metadata?.id || '';
          return keyA.localeCompare(keyB);
        });

        // Compare hashes to detect changes (ID-based comparison)
        const currentWants = currentState.wants;

        // Build a map of current wants by ID for efficient lookup
        const currentWantsMap = new Map(
          currentWants.map(w => [w.metadata?.id || w.id || '', w])
        );

        // Check if there are any changes
        const hasChanges = sortedWants.length !== currentWants.length ||
          sortedWants.some((newWant) => {
            const wantId = newWant.metadata?.id || newWant.id || '';
            const oldWant = currentWantsMap.get(wantId);

            // If old want doesn't exist, it's a new want
            if (!oldWant) return true;

            // If either hash is missing/empty, consider it as changed
            if (!newWant.hash || !oldWant.hash) return true;

            // Compare hashes
            return newWant.hash !== oldWant.hash;
          });

        // Only update state if there are actual changes
        if (hasChanges || currentState.isInitialLoad) {
          set({
            wants: sortedWants,
            loading: false,
            isInitialLoad: false
          });
        } else {
          // No changes, just update loading state if it was set
          if (currentState.isInitialLoad) {
            set({ loading: false, isInitialLoad: false });
          }
        }
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to fetch wants',
          loading: false,
          isInitialLoad: false
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
          wants: state.wants.filter(w => w.metadata?.id !== id && w.id !== id),
          selectedWant: (state.selectedWant?.metadata?.id === id || state.selectedWant?.id === id) ? null : state.selectedWant,
          selectedWantDetails: (state.selectedWantDetails?.metadata?.id === id || state.selectedWantDetails?.id === id) ? null : state.selectedWantDetails,
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