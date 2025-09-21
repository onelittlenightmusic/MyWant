export interface Want {
  metadata: WantMetadata;
  spec: WantSpec;
  status: WantExecutionStatus;
  stats?: WantStats;
  history?: WantHistory;
  results?: Record<string, unknown>;
  builder?: unknown; // ChainBuilder reference (not serialized)
}

export interface WantConfig {
  wants: WantDefinition[];
  metadata?: ConfigMetadata;
}

export interface WantDefinition {
  metadata: WantMetadata;
  spec: WantSpec;
  stats?: WantStats;
  status?: WantStatus;
}

export interface WantMetadata {
  id?: string;
  name: string;
  type: string;
  labels?: Record<string, string>;
}

export interface WantSpec {
  params?: Record<string, unknown>;
  using?: Array<Record<string, string>>;
  recipe?: string;
}

export interface WantStats {
  created_at?: string;
  started_at?: string;
  completed_at?: string;
  execution_count?: number;
}

export interface WantStatus {
  phase: WantPhase;
  message?: string;
  error?: string;
}

export interface ConfigMetadata {
  name?: string;
  description?: string;
  version?: string;
  labels?: Record<string, string>;
}

export type WantExecutionStatus = 'created' | 'running' | 'completed' | 'failed' | 'stopped';

export type WantPhase = 'pending' | 'initializing' | 'running' | 'completed' | 'failed' | 'stopped';

export interface WantDetails {
  id: string;
  execution_status: WantExecutionStatus;
  wants: WantDefinition[];
  results?: Record<string, unknown>;
}

export interface WantResults {
  data?: Record<string, unknown>;
  metrics?: {
    duration_ms?: number;
    items_processed?: number;
    success_count?: number;
    error_count?: number;
  };
  logs?: string[];
}

export interface WantHistory {
  parameterHistory?: Array<{
    wantName: string;
    stateValue: Record<string, unknown>;
    timestamp: string;
  }>;
  stateHistory?: Array<unknown>;
}

export interface CreateWantRequest {
  yaml?: string;
  name?: string;
}

export interface UpdateWantRequest {
  yaml: string;
}