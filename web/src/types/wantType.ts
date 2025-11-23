// Want Type Definitions from Backend API

export interface WantTypeMetadata {
  name: string;
  title: string;
  description: string;
  version: string;
  category: string;
  pattern: 'generator' | 'processor' | 'sink' | 'coordinator' | 'independent';
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

export interface ExampleDef {
  name: string;
  description: string;
  params: Record<string, unknown>;
  expectedBehavior: string;
  connectedTo?: string[];
}

export interface WantTypeDefinition {
  metadata: WantTypeMetadata;
  parameters: ParameterDef[];
  state: StateDef[];
  connectivity: ConnectivityDef;
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
