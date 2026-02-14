import { create } from 'zustand';
import { ServerConfig } from '@/types/config';
import { apiClient } from '@/api/client';

interface ConfigStore {
  config: ServerConfig | null;
  loading: boolean;
  error: string | null;
  fetchConfig: () => Promise<void>;
  updateConfig: (updates: Partial<ServerConfig>) => Promise<void>;
  applyColorMode: (mode: 'light' | 'dark' | 'system') => void;
}

export const useConfigStore = create<ConfigStore>((set, get) => ({
  config: null,
  loading: false,
  error: null,

  fetchConfig: async () => {
    set({ loading: true, error: null });
    try {
      const config = await apiClient.getServerConfig();
      set({ config, loading: false });
      get().applyColorMode(config.color_mode);
    } catch (err) {
      console.error('Failed to fetch config:', err);
      const fallback: ServerConfig = {
        port: 8080,
        host: 'localhost',
        debug: false,
        header_position: 'top',
        color_mode: 'system'
      };
      set({ 
        error: err instanceof Error ? err.message : 'Failed to fetch config', 
        loading: false,
        config: fallback
      });
      get().applyColorMode(fallback.color_mode);
    }
  },

  updateConfig: async (updates) => {
    const current = get().config;
    if (!current) return;

    try {
      const updated = await apiClient.updateServerConfig({ ...current, ...updates });
      set({ config: updated });
      if (updates.color_mode) {
        get().applyColorMode(updates.color_mode);
      }
    } catch (err) {
      console.error('Failed to update config:', err);
      set({ error: err instanceof Error ? err.message : 'Failed to update config' });
    }
  },

  applyColorMode: (mode) => {
    const root = window.document.documentElement;
    const isDark = mode === 'dark' || (mode === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches);
    
    if (isDark) {
      root.classList.add('dark');
    } else {
      root.classList.remove('dark');
    }
  },
}));
