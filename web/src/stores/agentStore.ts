import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';
import { Agent, AgentResponse, CreateAgentRequest, Capability, CreateCapabilityRequest } from '@/types/agent';
import { apiClient } from '@/api/client';

interface AgentStore {
  // State
  agents: AgentResponse[];
  capabilities: Capability[];
  selectedAgent: AgentResponse | null;
  loading: boolean;
  error: string | null;

  // Actions
  fetchAgents: () => Promise<void>;
  fetchCapabilities: () => Promise<void>;
  createAgent: (request: CreateAgentRequest) => Promise<AgentResponse>;
  deleteAgent: (name: string) => Promise<void>;
  createCapability: (request: CreateCapabilityRequest) => Promise<Capability>;
  deleteCapability: (name: string) => Promise<void>;
  selectAgent: (agent: AgentResponse | null) => void;
  clearError: () => void;
  findAgentsByCapability: (capabilityName: string) => Promise<AgentResponse[]>;
}

export const useAgentStore = create<AgentStore>()(
  subscribeWithSelector((set, get) => ({
    // Initial state
    agents: [],
    capabilities: [],
    selectedAgent: null,
    loading: false,
    error: null,

    // Actions
    fetchAgents: async () => {
      set({ loading: true, error: null });
      try {
        const agents = await apiClient.listAgents();
        // Sort deterministically by name to ensure consistent ordering across fetches
        const sortedAgents = [...agents].sort((a, b) => {
          const nameA = a.name || '';
          const nameB = b.name || '';
          return nameA.localeCompare(nameB);
        });
        set({ agents: sortedAgents, loading: false });
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to fetch agents',
          loading: false
        });
      }
    },

    fetchCapabilities: async () => {
      set({ loading: true, error: null });
      try {
        const capabilities = await apiClient.listCapabilities();
        set({ capabilities, loading: false });
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to fetch capabilities',
          loading: false
        });
      }
    },

    createAgent: async (request: CreateAgentRequest) => {
      set({ loading: true, error: null });
      try {
        const agent = await apiClient.createAgent(request);
        set(state => ({
          agents: [...state.agents, agent],
          loading: false
        }));
        return agent;
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to create agent',
          loading: false
        });
        throw error;
      }
    },

    deleteAgent: async (name: string) => {
      set({ loading: true, error: null });
      try {
        await apiClient.deleteAgent(name);
        set(state => ({
          agents: state.agents.filter(a => a.name !== name),
          selectedAgent: state.selectedAgent?.name === name ? null : state.selectedAgent,
          loading: false
        }));
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to delete agent',
          loading: false
        });
        throw error;
      }
    },

    createCapability: async (request: CreateCapabilityRequest) => {
      set({ loading: true, error: null });
      try {
        const capability = await apiClient.createCapability(request);
        set(state => ({
          capabilities: [...state.capabilities, capability],
          loading: false
        }));
        return capability;
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to create capability',
          loading: false
        });
        throw error;
      }
    },

    deleteCapability: async (name: string) => {
      set({ loading: true, error: null });
      try {
        await apiClient.deleteCapability(name);
        set(state => ({
          capabilities: state.capabilities.filter(c => c.name !== name),
          loading: false
        }));
      } catch (error) {
        set({
          error: error instanceof Error ? error.message : 'Failed to delete capability',
          loading: false
        });
        throw error;
      }
    },

    selectAgent: (agent: AgentResponse | null) => {
      set({ selectedAgent: agent });
    },

    clearError: () => {
      set({ error: null });
    },

    findAgentsByCapability: async (capabilityName: string) => {
      try {
        const response = await apiClient.findAgentsByCapability(capabilityName);
        return response.agents;
      } catch (error) {
        console.error('Failed to find agents by capability:', error);
        return [];
      }
    },
  }))
);