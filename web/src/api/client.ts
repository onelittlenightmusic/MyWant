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
import { HealthCheck, ApiError, ErrorHistoryEntry, ErrorHistoryResponse, LogsResponse } from '@/types/api';
import {
  AgentResponse,
  CreateAgentRequest,
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
  RecipeMetadata,
} from '@/types/recipe';
import {
  WantTypeListResponse,
  WantTypeDetailResponse,
  WantTypeExamplesResponse,
  LabelsResponse,
} from '@/types/wantType';

class MyWantApiClient {
  private client: AxiosInstance;
  private pendingRequests: Map<string, Promise<any>> = new Map();

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

  // Helper method for deduplicating GET requests
  private async deduplicatedGet<T>(url: string): Promise<T> {
    const key = `GET:${url}`;

    // If request is already pending, return the existing promise
    if (this.pendingRequests.has(key)) {
      return this.pendingRequests.get(key) as Promise<T>;
    }

    // Create new request and store it
    const promise = (async () => {
      try {
        const response = await this.client.get<T>(url);
        return response.data;
      } finally {
        // Clean up pending request after completion (success or error)
        this.pendingRequests.delete(key);
      }
    })();

    this.pendingRequests.set(key, promise);
    return promise;
  }

  // Health check
  async healthCheck(): Promise<HealthCheck> {
    return this.deduplicatedGet<HealthCheck>('/health');
  }

  // Want management
  async createWant(request: CreateWantRequest): Promise<Want> {
    const response = await this.client.post<Want>('/api/v1/wants', request);
    return response.data;
  }

  async listWants(): Promise<Want[]> {
    const data = await this.deduplicatedGet<{wants: Want[], execution_id: string, timestamp: string}>('/api/v1/wants');
    return data.wants;
  }

  async getWant(id: string): Promise<WantDetails> {
    return this.deduplicatedGet<WantDetails>(`/api/v1/wants/${id}`);
  }

  async updateWant(id: string, request: UpdateWantRequest): Promise<Want> {
    const response = await this.client.put<Want>(`/api/v1/wants/${id}`, request);
    return response.data;
  }

  async deleteWant(id: string): Promise<void> {
    await this.client.delete(`/api/v1/wants/${id}`);
  }

  async deleteWants(ids: string[]): Promise<void> {
    await this.client.delete('/api/v1/wants', { data: { ids } });
  }

  async getWantStatus(id: string): Promise<WantStatusResponse> {
    return this.deduplicatedGet<WantStatusResponse>(`/api/v1/wants/${id}/status`);
  }

  async getWantResults(id: string): Promise<WantResults> {
    return this.deduplicatedGet<WantResults>(`/api/v1/wants/${id}/results`);
  }

  // Suspend/Resume/Stop/Start operations
  async suspendWant(id: string): Promise<SuspendResumeResponse> {
    const response = await this.client.post<SuspendResumeResponse>(`/api/v1/wants/${id}/suspend`);
    return response.data;
  }

  async resumeWant(id: string): Promise<SuspendResumeResponse> {
    const response = await this.client.post<SuspendResumeResponse>(`/api/v1/wants/${id}/resume`);
    return response.data;
  }

  async stopWant(id: string): Promise<SuspendResumeResponse> {
    const response = await this.client.post<SuspendResumeResponse>(`/api/v1/wants/${id}/stop`);
    return response.data;
  }

  async startWant(id: string): Promise<SuspendResumeResponse> {
    const response = await this.client.post<SuspendResumeResponse>(`/api/v1/wants/${id}/start`);
    return response.data;
  }

  async suspendWants(ids: string[]): Promise<void> {
    await this.client.post('/api/v1/wants/suspend', { ids });
  }

  async resumeWants(ids: string[]): Promise<void> {
    await this.client.post('/api/v1/wants/resume', { ids });
  }

  async stopWants(ids: string[]): Promise<void> {
    await this.client.post('/api/v1/wants/stop', { ids });
  }

  async startWants(ids: string[]): Promise<void> {
    await this.client.post('/api/v1/wants/start', { ids });
  }

  // Error history operations
  async listErrorHistory(): Promise<ErrorHistoryResponse> {
    return this.deduplicatedGet<ErrorHistoryResponse>('/api/v1/errors');
  }

  async getErrorHistoryEntry(id: string): Promise<ErrorHistoryEntry> {
    return this.deduplicatedGet<ErrorHistoryEntry>(`/api/v1/errors/${id}`);
  }

  async updateErrorHistoryEntry(id: string, updates: { resolved?: boolean; notes?: string }): Promise<ErrorHistoryEntry> {
    const response = await this.client.put<ErrorHistoryEntry>(`/api/v1/errors/${id}`, updates);
    return response.data;
  }

  async deleteErrorHistoryEntry(id: string): Promise<void> {
    await this.client.delete(`/api/v1/errors/${id}`);
  }

  // API logs operations
  async listLogs(): Promise<LogsResponse> {
    return this.deduplicatedGet<LogsResponse>('/api/v1/logs');
  }

  async clearLogs(): Promise<void> {
    await this.client.delete('/api/v1/logs');
  }

  // Agent management
  async createAgent(request: CreateAgentRequest): Promise<AgentResponse> {
    const response = await this.client.post<AgentResponse>('/api/v1/agents', request);
    return response.data;
  }

  async listAgents(): Promise<AgentResponse[]> {
    const data = await this.deduplicatedGet<AgentsListResponse>('/api/v1/agents');
    return data.agents;
  }

  async getAgent(name: string): Promise<AgentResponse> {
    return this.deduplicatedGet<AgentResponse>(`/api/v1/agents/${name}`);
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
    const data = await this.deduplicatedGet<CapabilitiesListResponse>('/api/v1/capabilities');
    return data.capabilities;
  }

  async getCapability(name: string): Promise<CapabilityResponse> {
    return this.deduplicatedGet<CapabilityResponse>(`/api/v1/capabilities/${name}`);
  }

  async deleteCapability(name: string): Promise<void> {
    await this.client.delete(`/api/v1/capabilities/${name}`);
  }

  async findAgentsByCapability(capabilityName: string): Promise<FindAgentsByCapabilityResponse> {
    return this.deduplicatedGet<FindAgentsByCapabilityResponse>(`/api/v1/capabilities/${capabilityName}/agents`);
  }

  // Recipe management
  async createRecipe(recipe: GenericRecipe): Promise<RecipeCreateResponse> {
    const response = await this.client.post<RecipeCreateResponse>('/api/v1/recipes', recipe);
    return response.data;
  }

  async listRecipes(): Promise<RecipeListResponse> {
    return this.deduplicatedGet<RecipeListResponse>('/api/v1/recipes');
  }

  async getRecipe(id: string): Promise<GenericRecipe> {
    return this.deduplicatedGet<GenericRecipe>(`/api/v1/recipes/${id}`);
  }

  async updateRecipe(id: string, recipe: GenericRecipe): Promise<RecipeUpdateResponse> {
    const response = await this.client.put<RecipeUpdateResponse>(`/api/v1/recipes/${id}`, recipe);
    return response.data;
  }

  async deleteRecipe(id: string): Promise<void> {
    await this.client.delete(`/api/v1/recipes/${id}`);
  }

  async saveRecipeFromWant(wantId: string, metadata: RecipeMetadata): Promise<{id: string, message: string, file: string, wants: number}> {
    const response = await this.client.post('/api/v1/recipes/from-want', { wantId, metadata });
    return response.data;
  }

  // Want Type management
  async listWantTypes(category?: string, pattern?: string): Promise<WantTypeListResponse> {
    const params = new URLSearchParams();
    if (category) params.append('category', category);
    if (pattern) params.append('pattern', pattern);
    const url = `/api/v1/want-types${params.toString() ? `?${params.toString()}` : ''}`;
    return this.deduplicatedGet<WantTypeListResponse>(url);
  }

  async getWantType(name: string): Promise<WantTypeDetailResponse> {
    return this.deduplicatedGet<WantTypeDetailResponse>(`/api/v1/want-types/${name}`);
  }

  async getWantTypeExamples(name: string): Promise<WantTypeExamplesResponse> {
    return this.deduplicatedGet<WantTypeExamplesResponse>(`/api/v1/want-types/${name}/examples`);
  }

  async getLabels(): Promise<LabelsResponse> {
    return this.deduplicatedGet<LabelsResponse>('/api/v1/labels');
  }
}

// Export singleton instance
export const apiClient = new MyWantApiClient('http://localhost:8080');

export default MyWantApiClient;