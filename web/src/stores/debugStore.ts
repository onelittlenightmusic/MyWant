import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { POLLING_INTERVAL_MS } from '@/constants/polling';

interface DebugState {
  pollingIntervalMs: number;
  setPollingIntervalMs: (ms: number) => void;
}

export const useDebugStore = create<DebugState>()(
  persist(
    (set) => ({
      pollingIntervalMs: POLLING_INTERVAL_MS,
      setPollingIntervalMs: (ms) => set({ pollingIntervalMs: ms }),
    }),
    { name: 'mywant-debug' }
  )
);
