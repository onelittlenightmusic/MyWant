// Want Type Definitions from Backend API

export interface WantTypeMetadata {
  name: string;
  title: string;
  description: string;
  version: string;
  category: string;
  pattern: 'generator' | 'processor' | 'sink' | 'coordinator' | 'independent';
  system_type?: boolean; // If true, hide from user-facing want type selector
}

export interface ParameterDef {
  name: string;
  description: string;
  type: string;
  default?: unknown;
  required: boolean;
  validation?: {
    min?: number;
    max?: number;
    pattern?: string;
    enum?: unknown[];
  };
  example?: unknown;
}

export interface StateDef {
  name: string;
  description: string;
  type: string;
  persistent: boolean;
  example?: unknown;
}

export interface ChannelDef {
  name: string;
  type: string;
  description: string;
  required?: boolean;
  multiple?: boolean;
}

export interface ConnectivityDef {
  inputs: ChannelDef[];
  outputs: ChannelDef[];
}

export interface ConnectionSpec {
  name: string;
  type: string;
  description: string;
  required?: boolean;
  multiple?: boolean;
}

export interface RequireSpec {
  type: 'none' | 'providers' | 'users' | 'providers_and_users';
  providers?: ConnectionSpec[];
  users?: ConnectionSpec[];
}

export interface AgentDef {
  name: string;
  role: 'monitor' | 'action' | 'validator' | 'transformer';
  description: string;
  example?: string;
}

export interface ConstraintDef {
  description: string;
  validation: string;
}

export interface WantMetadata {
  name: string;
  type: string;
  labels: Record<string, string>;
}

export interface WantSpec {
  params: Record<string, unknown>;
  using?: Array<Record<string, string>>;
}

export interface WantConfiguration {
  metadata: WantMetadata;
  spec: WantSpec;
}

export interface ExampleDef {
  name: string;
  description: string;
  want: WantConfiguration;
  expectedBehavior: string;
}

export interface WantTypeDefinition {
  metadata: WantTypeMetadata;
  parameters: ParameterDef[];
  state: StateDef[];
  connectivity: ConnectivityDef;
  require?: RequireSpec;
  agents: AgentDef[];
  constraints: ConstraintDef[];
  examples: ExampleDef[];
  relatedTypes?: string[];
  seeAlso?: string[];
}

// API Response Types
export interface WantTypeListItem {
  name: string;
  title: string;
  category: string;
  pattern: string;
  version: string;
  system_type?: boolean; // If true, hide from user-facing want type selector
}

export interface WantTypeListResponse {
  count: number;
  wantTypes: WantTypeListItem[];
}

export interface WantTypeDetailResponse {
  metadata: WantTypeMetadata;
  parameters: ParameterDef[];
  state: StateDef[];
  connectivity: ConnectivityDef;
  require?: RequireSpec;
  agents: AgentDef[];
  constraints: ConstraintDef[];
  examples: ExampleDef[];
  relatedTypes?: string[];
  seeAlso?: string[];
}

export interface WantTypeExamplesResponse {
  name: string;
  examples: ExampleDef[];
}

// Labels API Response Type
export interface LabelsResponse {
  labelKeys: string[];
  labelValues: Record<string, string[]>; // Map of key -> array of values
  count: number;
}

// Store state type
export interface WantTypeState {
  wantTypes: WantTypeListItem[];
  selectedWantType: WantTypeDefinition | null;
  loading: boolean;
  error: string | null;
}

// Filter types
export interface WantTypeFilters {
  category?: string;
  pattern?: string;
  searchTerm?: string;
}
