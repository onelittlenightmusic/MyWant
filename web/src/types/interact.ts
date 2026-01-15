import { Want } from './want';

export interface InteractSession {
  session_id: string;
  created_at: string;
  expires_at: string;
}

export interface InteractMessageRequest {
  message: string;
  context?: {
    preferRecipes?: boolean;
    categories?: string[];
    provider?: string;
  };
}

export interface Recommendation {
  id: string;
  title: string;
  approach: 'recipe' | 'custom' | 'hybrid';
  description: string;
  config: {
    wants: Want[];
  };
  metadata: {
    want_count: number;
    want_types_used: string[];
    recipes_used: string[];
    complexity: 'low' | 'medium' | 'high';
    pros_cons: {
      pros: string[];
      cons: string[];
    };
  };
}

export interface InteractMessageResponse {
  recommendations: Recommendation[];
  conversation_history: ConversationMessage[];
  timestamp: string;
}

export interface ConversationMessage {
  role: 'user' | 'assistant';
  content: string;
  timestamp: string;
}

export interface InteractDeployRequest {
  recommendation_id: string;
  modifications?: ConfigModifications;
}

export interface ConfigModifications {
  parameterOverrides?: Record<string, any>;
  disableWants?: string[];
}

export interface InteractDeployResponse {
  execution_id: string;
  want_ids: string[];
  status: string;
  message: string;
  timestamp: string;
}
