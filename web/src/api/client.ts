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
  StateDef,
  WantRecipeAnalysis,
} from '@/types/recipe';
import {
  WantTypeListResponse,
  WantTypeDetailResponse,
  WantTypeExamplesResponse,
  LabelsResponse,
} from '@/types/wantType';
import {
  InteractSession,
  InteractMessageRequest,
  InteractMessageResponse,
  InteractDeployRequest,
  InteractDeployResponse,
} from '@/types/interact';
import {
  CreateDraftWantData,
  UpdateDraftWantData,
  DRAFT_WANT_LABEL,
} from '@/types/draft';
import { ServerConfig } from '@/types/config';
import {
  Achievement, AchievementListResponse, CreateAchievementRequest,
  AchievementRule, AchievementRuleListResponse, CreateRuleRequest,
} from '@/types/achievement';

export interface WantHashEntry {
  id: string;
  hash: string;
  updated_at: number;
}

export interface WantHashesResponse {
  collection_hash: string;
  wants: WantHashEntry[];
}

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

    const isPollingUrl = (url?: string) =>
      url?.includes('/api/v1/state') || url?.includes('/api/v1/hash');

    // Request interceptor
    this.client.interceptors.request.use(
      (config) => {
        if (!isPollingUrl(config.url))
          console.log(`API Request: ${config.method?.toUpperCase()} ${config.url}`);
        return config;
      },
      (error) => Promise.reject(error)
    );

    // Response interceptor
    this.client.interceptors.response.use(
      (response) => {
        if (!isPollingUrl(response.config.url))
          console.log(`API Response: ${response.status} ${response.config.url}`);
        return response;
      },
      (error: AxiosError) => {
        const apiError: ApiError = {
          message: error.message || 'An error occurred',
          status: error.response?.status || 500,
          code: error.code,
        };

        // Handle specific Axios error codes
        if (error.code === 'ECONNABORTED') {
          apiError.message = 'Request timed out or was aborted. The server might be slow or restarting.';
        } else if (error.code === 'ERR_NETWORK') {
          apiError.message = 'Network error. Please check if the server is running.';
        }

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

  // Config management
  async getServerConfig(): Promise<ServerConfig> {
    return this.deduplicatedGet<ServerConfig>('/api/v1/config');
  }

  async updateServerConfig(config: Partial<ServerConfig>): Promise<ServerConfig> {
    const response = await this.client.put<ServerConfig>('/api/v1/config', config);
    return response.data;
  }

  async stopServer(): Promise<{ message: string }> {
    const response = await this.client.post<{ message: string }>('/api/v1/system/stop');
    return response.data;
  }

  async restartServer(): Promise<{ message: string }> {
    const response = await this.client.post<{ message: string }>('/api/v1/system/restart');
    return response.data;
  }

  // Want management
  async createWant(request: CreateWantRequest): Promise<Want> {
    const response = await this.client.post<Want>('/api/v1/wants', request);
    return response.data;
  }

  async listWants(options?: { includeCancelled?: boolean }): Promise<Want[]> {
    const url = options?.includeCancelled ? '/api/v1/wants?includeCancelled=true' : '/api/v1/wants';
    const data = await this.deduplicatedGet<{wants: Want[], execution_id: string, timestamp: string}>(url);
    return data.wants;
  }

  /**
   * Fetch only ID+hash for each want. Returns null when If-None-Match matches (304 Not Modified).
   */
  async listWantHashes(ifNoneMatch?: string): Promise<WantHashesResponse | null> {
    const headers: Record<string, string> = {};
    if (ifNoneMatch) headers['If-None-Match'] = `"${ifNoneMatch}"`;

    const response = await this.client.get<WantHashesResponse>('/api/v1/wants/hashes', {
      headers,
      validateStatus: (s) => s === 200 || s === 304,
    });

    if (response.status === 304) return null;
    return response.data;
  }

  /**
   * Fetch a single want with ETag-based conditional GET. Returns null data on 304.
   */
  async getWantConditional(id: string, ifNoneMatch?: string): Promise<{ data: Want | null; etag: string | undefined }> {
    const headers: Record<string, string> = {};
    if (ifNoneMatch) headers['If-None-Match'] = `"${ifNoneMatch}"`;

    const response = await this.client.get<Want>(`/api/v1/wants/${id}`, {
      headers,
      validateStatus: (s) => s === 200 || s === 304,
    });

    const etag = (response.headers['etag'] as string | undefined)?.replace(/^"|"$/g, '');
    if (response.status === 304) return { data: null, etag };
    return { data: response.data, etag };
  }

  async getWant(id: string): Promise<WantDetails> {
    return this.deduplicatedGet<WantDetails>(`/api/v1/wants/${id}`);
  }

  async updateWant(id: string, request: UpdateWantRequest): Promise<Want> {
    const response = await this.client.put<Want>(`/api/v1/wants/${id}`, request);
    return response.data;
  }

  async updateWantOrder(
    id: string,
    request: { previousWantId?: string; nextWantId?: string }
  ): Promise<{ success: boolean; orderKey: string; wantId: string }> {
    const response = await this.client.put(`/api/v1/wants/${id}/order`, request);
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

  async analyzeWantForRecipe(wantId: string): Promise<WantRecipeAnalysis> {
    return this.deduplicatedGet<WantRecipeAnalysis>(`/api/v1/wants/${wantId}/recipe-analysis`);
  }

  async saveRecipeFromWant(wantId: string, metadata: RecipeMetadata, state?: StateDef[]): Promise<{id: string, message: string, file: string, wants: number}> {
    const response = await this.client.post('/api/v1/recipes/from-want', { wantId, metadata, state });
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

  // Interactive want creation
  async createInteractSession(): Promise<InteractSession> {
    const response = await this.client.post('/api/v1/interact');
    return response.data;
  }

  async sendInteractMessage(
    sessionId: string,
    request: InteractMessageRequest
  ): Promise<InteractMessageResponse> {
    // Set timeout to 300s for Goose processing
    const response = await this.client.post(
      `/api/v1/interact/${sessionId}`,
      request,
      { timeout: 300000 }  // 5 minutes
    );
    return response.data;
  }

  async deployRecommendation(
    sessionId: string,
    request: InteractDeployRequest
  ): Promise<InteractDeployResponse> {
    const response = await this.client.post(
      `/api/v1/interact/${sessionId}/deploy`,
      request
    );
    return response.data;
  }

  async deleteInteractSession(sessionId: string): Promise<void> {
    await this.client.delete(`/api/v1/interact/${sessionId}`);
  }

  // Draft want management
  // Draft wants are regular wants with special labels, stored in backend for persistence

  async createDraftWant(data: CreateDraftWantData): Promise<{ id: string; execution_id: string }> {
    const draftId = `draft-${Date.now()}`;
    const want = {
      metadata: {
        id: draftId,
        name: `Draft: ${data.message.substring(0, 30)}${data.message.length > 30 ? '...' : ''}`,
        type: 'draft',
        labels: {
          [DRAFT_WANT_LABEL]: 'true',
        },
      },
      spec: {
        params: {},
      },
      state: {
        sessionId: data.sessionId,
        message: data.message,
        recommendations: data.recommendations || [],
        isThinking: data.isThinking ?? true,
        error: data.error,
        createdAt: new Date().toISOString(),
      },
    };

    const response = await this.client.post('/api/v1/wants', want);
    // Return the actual want ID we created, and the backend's execution ID
    return {
      id: draftId,
      execution_id: response.data.id
    };
  }

  async updateDraftWant(id: string, updates: UpdateDraftWantData): Promise<Want> {
    // First get the current want
    const current = await this.getWant(id);

    // Merge updates into state.current
    const updatedWant: UpdateWantRequest = {
      metadata: current.metadata,
      spec: current.spec,
      status: current.status,
      state: {
        ...current.state,
        current: {
          ...(current.state?.current || {}),
          ...updates,
        },
      },
      history: current.history,
    };

    const response = await this.client.put<Want>(`/api/v1/wants/${id}`, updatedWant);
    return response.data;
  }

  async deleteDraftWant(id: string): Promise<void> {
    await this.client.delete(`/api/v1/wants/${id}`);
  }

  // Memo (Global State)
  async getGlobalState(): Promise<{ state: Record<string, unknown>; timestamp: string }> {
    return this.deduplicatedGet<{ state: Record<string, unknown>; timestamp: string }>('/api/v1/global-state');
  }

  async getGlobalStateConditional(ifNoneMatch?: string): Promise<{ data: { state: Record<string, unknown>; timestamp: string } | null; etag: string | undefined }> {
    const headers: Record<string, string> = {};
    if (ifNoneMatch) headers['If-None-Match'] = `"${ifNoneMatch}"`;
    const response = await this.client.get<{ state: Record<string, unknown>; timestamp: string }>('/api/v1/global-state', {
      headers,
      validateStatus: (s) => s === 200 || s === 304,
    });
    const etag = (response.headers['etag'] as string | undefined)?.replace(/^"|"$/g, '');
    if (response.status === 304) return { data: null, etag };
    return { data: response.data, etag };
  }

  async deleteGlobalState(): Promise<void> {
    await this.client.delete('/api/v1/global-state');
  }

  // Want State (cross-want state access)
  async clearWantState(id: string): Promise<void> {
    await this.client.delete(`/api/v1/states/${id}`);
  }

  // Webhook messaging
  async sendWebhookMessage(wantName: string, text: string, sender?: string): Promise<{ status: string }> {
    const response = await this.client.post<{ status: string }>(
      `/api/v1/webhooks/${wantName}`,
      { text, sender: sender ?? 'user' }
    );
    return response.data;
  }

  // Global Parameters
  async getGlobalParameters(): Promise<{ parameters: Record<string, unknown>; count: number; types: Record<string, string[]> }> {
    return this.deduplicatedGet<{ parameters: Record<string, unknown>; count: number; types: Record<string, string[]> }>('/api/v1/global-parameters');
  }

  async getGlobalParametersConditional(ifNoneMatch?: string): Promise<{ data: { parameters: Record<string, unknown>; count: number; types: Record<string, string[]> } | null; etag: string | undefined }> {
    const headers: Record<string, string> = {};
    if (ifNoneMatch) headers['If-None-Match'] = `"${ifNoneMatch}"`;
    const response = await this.client.get<{ parameters: Record<string, unknown>; count: number; types: Record<string, string[]> }>('/api/v1/global-parameters', {
      headers,
      validateStatus: (s) => s === 200 || s === 304,
    });
    const etag = (response.headers['etag'] as string | undefined)?.replace(/^"|"$/g, '');
    if (response.status === 304) return { data: null, etag };
    return { data: response.data, etag };
  }

  async updateGlobalParameters(parameters: Record<string, unknown>): Promise<{ parameters: Record<string, unknown>; count: number; types: Record<string, string[]> }> {
    const response = await this.client.put<{ parameters: Record<string, unknown>; count: number; types: Record<string, string[]> }>('/api/v1/global-parameters', { parameters });
    return response.data;
  }

  // Achievements
  async listAchievements(): Promise<AchievementListResponse> {
    return this.deduplicatedGet<AchievementListResponse>('/api/v1/achievements');
  }

  async getAchievement(id: string): Promise<Achievement> {
    const response = await this.client.get<Achievement>(`/api/v1/achievements/${id}`);
    return response.data;
  }

  async createAchievement(request: CreateAchievementRequest): Promise<Achievement> {
    const response = await this.client.post<Achievement>('/api/v1/achievements', request);
    return response.data;
  }

  async updateAchievement(id: string, request: Partial<Achievement>): Promise<Achievement> {
    const response = await this.client.put<Achievement>(`/api/v1/achievements/${id}`, request);
    return response.data;
  }

  async lockAchievement(id: string): Promise<Achievement> {
    const response = await this.client.patch<Achievement>(`/api/v1/achievements/${id}/lock`);
    return response.data;
  }

  async unlockAchievement(id: string): Promise<Achievement> {
    const response = await this.client.patch<Achievement>(`/api/v1/achievements/${id}/unlock`);
    return response.data;
  }

  async deleteAchievement(id: string): Promise<void> {
    await this.client.delete(`/api/v1/achievements/${id}`);
  }

  // Achievement Rules
  async listAchievementRules(): Promise<AchievementRuleListResponse> {
    return this.deduplicatedGet<AchievementRuleListResponse>('/api/v1/achievements/rules');
  }

  async createAchievementRule(request: CreateRuleRequest): Promise<AchievementRule> {
    const response = await this.client.post<AchievementRule>('/api/v1/achievements/rules', request);
    return response.data;
  }

  async deleteAchievementRule(id: string): Promise<void> {
    await this.client.delete(`/api/v1/achievements/rules/${id}`);
  }

  // GUI state — backed by the gui_state want, surfaced via /api/v1/gui/state
  async getGUIState(): Promise<{ seq: number; state: Record<string, unknown> }> {
    const response = await this.client.get<{ seq: number; state: Record<string, unknown> }>('/api/v1/gui/state');
    return response.data;
  }

  async updateGUIState(updates: Record<string, unknown>): Promise<{ seq: number; state: Record<string, unknown> }> {
    const response = await this.client.put<{ seq: number; state: Record<string, unknown> }>('/api/v1/gui/state', updates);
    return response.data;
  }

  // @deprecated — use updateGUIState instead
  async updateGUIWantState(wantId: string, updates: Record<string, unknown>): Promise<void> {
    await this.client.put(`/api/v1/states/${wantId}`, updates);
  }
}

// Export singleton instance
export const apiClient = new MyWantApiClient('');

export default MyWantApiClient;