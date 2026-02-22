export interface Agent {
  name: string;
  type: 'do' | 'monitor' | 'think';
  capabilities: string[];
  uses: string[];
}

export interface AgentResponse {
  name: string;
  type: 'do' | 'monitor' | 'think';
  capabilities: string[];
  uses: string[];
}

export interface CreateAgentRequest {
  name: string;
  type: 'do' | 'monitor' | 'think';
  capabilities: string[];
  uses: string[];
}

export interface Capability {
  name: string;
  gives: string[];
  requires?: string[];
  description?: string;
}

export interface CapabilityResponse {
  name: string;
  gives: string[];
  requires?: string[];
  description?: string;
}

export interface CreateCapabilityRequest {
  name: string;
  gives: string[];
  requires?: string[];
  description?: string;
}

export interface AgentsListResponse {
  message?: string;
  agents: AgentResponse[];
}

export interface CapabilitiesListResponse {
  message?: string;
  capabilities: CapabilityResponse[];
}

export interface FindAgentsByCapabilityResponse {
  capability: string;
  agents: AgentResponse[];
}