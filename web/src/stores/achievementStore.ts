import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';
import { Achievement, AchievementRule, CreateAchievementRequest, CreateRuleRequest } from '@/types/achievement';
import { apiClient } from '@/api/client';

interface AchievementStore {
  achievements: Achievement[];
  rules: AchievementRule[];
  loading: boolean;
  error: string | null;

  fetchAchievements: () => Promise<void>;
  createAchievement: (request: CreateAchievementRequest) => Promise<Achievement>;
  deleteAchievement: (id: string) => Promise<void>;
  fetchRules: () => Promise<void>;
  createRule: (request: CreateRuleRequest) => Promise<AchievementRule>;
  deleteRule: (id: string) => Promise<void>;
  clearError: () => void;
}

export const useAchievementStore = create<AchievementStore>()(
  subscribeWithSelector((set) => ({
    achievements: [],
    rules: [],
    loading: false,
    error: null,

    fetchAchievements: async () => {
      set({ loading: true, error: null });
      try {
        const resp = await apiClient.listAchievements();
        set({ achievements: resp.achievements ?? [], loading: false });
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to fetch achievements',
          loading: false,
        });
      }
    },

    createAchievement: async (request: CreateAchievementRequest) => {
      set({ loading: true, error: null });
      try {
        const created = await apiClient.createAchievement(request);
        set((state) => ({ achievements: [created, ...state.achievements], loading: false }));
        return created;
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to create achievement',
          loading: false,
        });
        throw error;
      }
    },

    deleteAchievement: async (id: string) => {
      set({ loading: true, error: null });
      try {
        await apiClient.deleteAchievement(id);
        set((state) => ({
          achievements: state.achievements.filter((a) => a.id !== id),
          loading: false,
        }));
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to delete achievement',
          loading: false,
        });
        throw error;
      }
    },

    fetchRules: async () => {
      try {
        const resp = await apiClient.listAchievementRules();
        set({ rules: resp.rules ?? [] });
      } catch (error) {
        set({ error: error instanceof Error ? error.message : 'Failed to fetch rules' });
      }
    },

    createRule: async (request: CreateRuleRequest) => {
      set({ loading: true, error: null });
      try {
        const created = await apiClient.createAchievementRule(request);
        set((state) => ({ rules: [...state.rules, created], loading: false }));
        return created;
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to create rule',
          loading: false,
        });
        throw error;
      }
    },

    deleteRule: async (id: string) => {
      try {
        await apiClient.deleteAchievementRule(id);
        set((state) => ({ rules: state.rules.filter((r) => r.id !== id) }));
      } catch (error) {
        set({ error: error instanceof Error ? error.message : 'Failed to delete rule' });
        throw error;
      }
    },

    clearError: () => set({ error: null }),
  }))
);
