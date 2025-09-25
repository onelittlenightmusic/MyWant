import axios, { AxiosInstance, AxiosError } from 'axios';
import {
  Want,
  WantDetails,
  WantResults,
  CreateWantRequest,
  UpdateWantRequest,
  SuspendResumeResponse,
  WantStatusResponse
} from '@/types/want';
import { HealthCheck, ApiError, ErrorHistoryEntry, ErrorHistoryResponse } from '@/types/api';
import {
  Agent,
  AgentResponse,
  CreateAgentRequest,
  Capability,
  CapabilityResponse,
  CreateCapabilityRequest,
  AgentsListResponse,
  CapabilitiesListResponse,
  FindAgentsByCapabilityResponse
} from '@/types/agent';
import {
  GenericRecipe,
  RecipeListResponse,
  RecipeCreateResponse,
  RecipeUpdateResponse,
} from '@/types/recipe';

class MyWantApiClient {
  private client: AxiosInstance;

  constructor(baseURL: string = '') {
    this.client = axios.create({
      baseURL,
      timeout: 30000,
      headers: {
        'Content-Type': 'application/json',
      },
    });

    // Request interceptor
    this.client.interceptors.request.use(
      (config) => {
        console.log(`API Request: ${config.method?.toUpperCase()} ${config.url}`);
        return config;
      },
      (error) => Promise.reject(error)
    );

    // Response interceptor
    this.client.interceptors.response.use(
      (response) => {
        console.log(`API Response: ${response.status} ${response.config.url}`);
        return response;
      },
      (error: AxiosError) => {
        const apiError: ApiError = {
          message: error.message || 'An error occurred',
          status: error.response?.status || 500,
          code: error.code,
        };

        // Handle string error responses (from our Go server)
        if (error.response?.data && typeof error.response.data === 'string') {
          apiError.message = error.response.data;
        }
        // Handle object error responses
        else if (error.response?.data && typeof error.response.data === 'object') {
          const data = error.response.data as any;
          apiError.message = data.message || data.error || error.message;
        }

        // Specifically handle validation errors from want type validation
        if (error.response?.status === 400 && apiError.message.includes('Invalid want types:')) {
          // Extract the detailed error message for better formatting
          apiError.type = 'validation';
          apiError.details = apiError.message.replace('Invalid want types: ', '');
        }

        console.error('API Error:', apiError);
        return Promise.reject(apiError);
      }
    );
  }

  // Health check
  async healthCheck(): Promise<HealthCheck> {
    const response = await this.client.get<HealthCheck>('/health');
    return response.data;
  }

  // Want management
  async createWant(request: CreateWantRequest): Promise<Want> {
    const response = await this.client.post<Want>('/api/v1/wants', request);
    return response.data;
  }

  async createWantFromYaml(yaml: string, name?: string): Promise<Want> {
    const response = await this.client.post<Want>('/api/v1/wants', { yaml, name });
    return response.data;
  }

  async createWantFromYamlRaw(yaml: string): Promise<Want> {
    const response = await this.client.post<Want>('/api/v1/wants', yaml, {
      headers: {
        'Content-Type': 'application/yaml',
      },
    });
    return response.data;
  }

  async listWants(): Promise<Want[]> {
    const response = await this.client.get<{wants: Want[], execution_id: string, timestamp: string}>('/api/v1/wants');
    return response.data.wants;
  }

  async getWant(id: string): Promise<WantDetails> {
    const response = await this.client.get<WantDetails>(`/api/v1/wants/${id}`);
    return response.data;
  }

  async updateWant(id: string, request: UpdateWantRequest): Promise<Want> {
    const response = await this.client.put<Want>(`/api/v1/wants/${id}`, request);
    return response.data;
  }

  async updateWantFromYaml(id: string, yaml: string): Promise<Want> {
    const response = await this.client.put<Want>(`/api/v1/wants/${id}`, yaml, {
      headers: {
        'Content-Type': 'application/yaml',
      },
    });
    return response.data;
  }

  async deleteWant(id: string): Promise<void> {
    await this.client.delete(`/api/v1/wants/${id}`);
  }

  async getWantStatus(id: string): Promise<WantStatusResponse> {
    const response = await this.client.get<WantStatusResponse>(`/api/v1/wants/${id}/status`);
    return response.data;
  }

  async getWantResults(id: string): Promise<WantResults> {
    const response = await this.client.get<WantResults>(`/api/v1/wants/${id}/results`);
    return response.data;
  }

  // Suspend/Resume operations
  async suspendWant(id: string): Promise<SuspendResumeResponse> {
    const response = await this.client.post<SuspendResumeResponse>(`/api/v1/wants/${id}/suspend`);
    return response.data;
  }

  async resumeWant(id: string): Promise<SuspendResumeResponse> {
    const response = await this.client.post<SuspendResumeResponse>(`/api/v1/wants/${id}/resume`);
    return response.data;
  }

  // Error history operations
  async listErrorHistory(): Promise<ErrorHistoryResponse> {
    const response = await this.client.get<ErrorHistoryResponse>('/api/v1/errors');
    return response.data;
  }

  async getErrorHistoryEntry(id: string): Promise<ErrorHistoryEntry> {
    const response = await this.client.get<ErrorHistoryEntry>(`/api/v1/errors/${id}`);
    return response.data;
  }

  async updateErrorHistoryEntry(id: string, updates: { resolved?: boolean; notes?: string }): Promise<ErrorHistoryEntry> {
    const response = await this.client.put<ErrorHistoryEntry>(`/api/v1/errors/${id}`, updates);
    return response.data;
  }

  async deleteErrorHistoryEntry(id: string): Promise<void> {
    await this.client.delete(`/api/v1/errors/${id}`);
  }

  // Agent management
  async createAgent(request: CreateAgentRequest): Promise<AgentResponse> {
    const response = await this.client.post<AgentResponse>('/api/v1/agents', request);
    return response.data;
  }

  async listAgents(): Promise<AgentResponse[]> {
    const response = await this.client.get<AgentsListResponse>('/api/v1/agents');
    return response.data.agents;
  }

  async getAgent(name: string): Promise<AgentResponse> {
    const response = await this.client.get<AgentResponse>(`/api/v1/agents/${name}`);
    return response.data;
  }

  async deleteAgent(name: string): Promise<void> {
    await this.client.delete(`/api/v1/agents/${name}`);
  }

  // Capability management
  async createCapability(request: CreateCapabilityRequest): Promise<CapabilityResponse> {
    const response = await this.client.post<CapabilityResponse>('/api/v1/capabilities', request);
    return response.data;
  }

  async listCapabilities(): Promise<CapabilityResponse[]> {
    const response = await this.client.get<CapabilitiesListResponse>('/api/v1/capabilities');
    return response.data.capabilities;
  }

  async getCapability(name: string): Promise<CapabilityResponse> {
    const response = await this.client.get<CapabilityResponse>(`/api/v1/capabilities/${name}`);
    return response.data;
  }

  async deleteCapability(name: string): Promise<void> {
    await this.client.delete(`/api/v1/capabilities/${name}`);
  }

  async findAgentsByCapability(capabilityName: string): Promise<FindAgentsByCapabilityResponse> {
    const response = await this.client.get<FindAgentsByCapabilityResponse>(`/api/v1/capabilities/${capabilityName}/agents`);
    return response.data;
  }

  // Recipe management
  async createRecipe(recipe: GenericRecipe): Promise<RecipeCreateResponse> {
    const response = await this.client.post<RecipeCreateResponse>('/api/v1/recipes', recipe);
    return response.data;
  }

  async listRecipes(): Promise<RecipeListResponse> {
    const response = await this.client.get<RecipeListResponse>('/api/v1/recipes');
    return response.data;
  }

  async getRecipe(id: string): Promise<GenericRecipe> {
    const response = await this.client.get<GenericRecipe>(`/api/v1/recipes/${id}`);
    return response.data;
  }

  async updateRecipe(id: string, recipe: GenericRecipe): Promise<RecipeUpdateResponse> {
    const response = await this.client.put<RecipeUpdateResponse>(`/api/v1/recipes/${id}`, recipe);
    return response.data;
  }

  async deleteRecipe(id: string): Promise<void> {
    await this.client.delete(`/api/v1/recipes/${id}`);
  }
}

// Export singleton instance
export const apiClient = new MyWantApiClient('http://localhost:8080');

export default MyWantApiClient;