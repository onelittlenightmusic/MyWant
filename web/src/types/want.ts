export interface Want {
  id?: string; // Want execution ID
  metadata: WantMetadata;
  spec: WantSpec;
  status: WantExecutionStatus;
  state?: Record<string, unknown>; // Runtime state including error details
  stats?: WantStats;
  history?: WantHistory;
  results?: Record<string, unknown>;
  builder?: unknown; // ChainBuilder reference (not serialized)
  suspended?: boolean; // Suspension state
  current_agent?: string; // Name of the agent currently executing for this want
  running_agents?: string[]; // Array of all currently running agent names
  agent_history?: AgentExecution[]; // Complete history of agent executions for this want
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

export interface OwnerReference {
  apiVersion: string;
  kind: string;
  name: string;
  controller: boolean;
  blockOwnerDeletion?: boolean;
}

export interface WantMetadata {
  id?: string;
  name: string;
  type: string;
  labels?: Record<string, string>;
  ownerReferences?: OwnerReference[];
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

export interface WantDetails extends Want {
  execution_status?: WantExecutionStatus;
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

export interface SuspendResumeResponse {
  message: string;
  wantId: string;
  suspended: boolean;
  timestamp: string;
}

export interface WantStatusResponse {
  id: string;
  status: WantExecutionStatus;
  suspended?: boolean;
}

export interface AgentExecution {
  agent_name: string;
  agent_type: 'do' | 'monitor';
  start_time: string;
  end_time?: string;
  status: 'running' | 'completed' | 'failed' | 'terminated';
  error?: string;
}