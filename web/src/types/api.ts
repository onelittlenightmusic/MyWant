export interface ApiResponse<T = unknown> {
  data: T;
  success: boolean;
  message?: string;
}

export interface ApiError {
  message: string;
  status: number;
  code?: string;
  type?: 'validation' | 'runtime' | 'network';
  details?: string;
}

export interface HealthCheck {
  status: 'ok' | 'error';
  timestamp: string;
  version?: string;
  uptime?: string;
}