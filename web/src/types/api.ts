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
  timestamp?: string;
  endpoint?: string;
  method?: string;
  requestData?: any;
  userAgent?: string;
}

export interface ErrorHistoryEntry extends ApiError {
  id: string;
  timestamp: string;
  endpoint: string;
  method: string;
  resolved?: boolean;
  notes?: string;
}

export interface ErrorHistoryResponse {
  errors: ErrorHistoryEntry[];
  total: number;
}

export interface HealthCheck {
  status: 'ok' | 'error';
  timestamp: string;
  version?: string;
  uptime?: string;
}

export interface LogEntry {
  timestamp: string;
  method: string;
  endpoint: string;
  resource: string;
  status: string;
  statusCode: number;
  errorMsg?: string;
  details?: string;
}

export interface LogsResponse {
  timestamp: string;
  count: number;
  logs: LogEntry[];
}