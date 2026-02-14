import { create } from 'zustand';
import { ServerConfig } from '@/types/config';
import { apiClient } from '@/api/client';

interface ConfigStore {
  config: ServerConfig | null;
  loading: boolean;
  error: string | null;
  fetchConfig: () => Promise<void>;
}

export const useConfigStore = create<ConfigStore>((set) => ({
  config: null,
  loading: false,
  error: null,

  fetchConfig: async () => {
    set({ loading: true, error: null });
    try {
      const config = await apiClient.getServerConfig();
      set({ config, loading: false });
    } catch (err) {
      console.error('Failed to fetch config:', err);
      set({ 
        error: err instanceof Error ? err.message : 'Failed to fetch config', 
        loading: false,
        // Fallback default config
        config: {
          port: 8080,
          host: 'localhost',
          debug: false,
          header_position: 'top'
        }
      });
    }
  },
}));
