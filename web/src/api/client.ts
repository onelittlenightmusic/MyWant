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
import { HealthCheck, ApiError } from '@/types/api';

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

        if (error.response?.data && typeof error.response.data === 'string') {
          apiError.message = error.response.data;
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
}

// Export singleton instance
export const apiClient = new MyWantApiClient();

export default MyWantApiClient;