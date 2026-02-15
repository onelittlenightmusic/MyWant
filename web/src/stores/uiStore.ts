import { create } from 'zustand';
import { persist } from 'zustand/middleware';

interface UIStore {
  sidebarMinimized: boolean;
  setSidebarMinimized: (minimized: boolean) => void;
  toggleSidebarMinimized: () => void;
}

export const useUIStore = create<UIStore>()(
  persist(
    (set) => ({
      sidebarMinimized: true,
      setSidebarMinimized: (minimized) => set({ sidebarMinimized: minimized }),
      toggleSidebarMinimized: () => set((state) => ({ sidebarMinimized: !state.sidebarMinimized })),
    }),
    {
      name: 'mywant-ui-storage',
    }
  )
);
