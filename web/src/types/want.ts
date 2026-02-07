export interface Want {
  id?: string; // Want execution ID
  metadata: WantMetadata;
  spec: WantSpec;
  status: WantExecutionStatus;
  state?: Record<string, unknown>; // Runtime state including error details
  hidden_state?: Record<string, unknown>; // Internal framework fields
  stats?: WantStats;
  history?: WantHistory;
  results?: Record<string, unknown>;
  builder?: unknown; // ChainBuilder reference (not serialized)
  current_agent?: string; // Name of the agent currently executing for this want
  running_agents?: string[]; // Array of all currently running agent names
  hash?: string; // Hash for change detection (metadata, spec, all state fields, status)
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
  id?: string;
  controller: boolean;
  blockOwnerDeletion?: boolean;
}

export interface WantMetadata {
  id?: string;
  name: string;
  type: string;
  labels?: Record<string, string>;
  ownerReferences?: OwnerReference[];
  updatedAt?: number; // Server-managed timestamp for detecting metadata changes
  isSystemWant?: boolean; // true for system-managed wants (e.g., system-scheduler)
  orderKey?: string; // Fractional index for custom ordering (supports drag-and-drop reordering)
}

export interface WhenSpec {
  at?: string;    // Time expression like "7am", "17:30", "midnight"
  every: string;  // Frequency like "day", "5 minutes", "2 hours"
}

export interface WantSpec {
  params?: Record<string, unknown>;
  using?: Array<Record<string, string>>;
  recipe?: string;
  when?: WhenSpec[];
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

export type WantExecutionStatus = 'created' | 'initializing' | 'reaching' | 'suspended' | 'achieved' | 'failed' | 'stopped' | 'terminated' | 'deleting';

export type WantPhase = 'pending' | 'initializing' | 'reaching' | 'achieved' | 'failed' | 'stopped' | 'terminated';

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
  logHistory?: Array<{
    timestamp: number;
    logs: string;
  }>;
  agentHistory?: AgentExecution[]; // Complete history of agent executions for this want
}

export interface CreateWantRequest {
  metadata: WantMetadata;
  spec: WantSpec;
  status?: WantExecutionStatus;
  state?: Record<string, unknown>;
  history?: WantHistory;
}

export interface UpdateWantRequest {
  metadata: WantMetadata;
  spec: WantSpec;
  status?: WantExecutionStatus;
  state?: Record<string, unknown>;
  history?: WantHistory;
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
  status: 'reaching' | 'achieved' | 'failed' | 'terminated';
  error?: string;
}