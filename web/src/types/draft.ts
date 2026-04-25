import { Recommendation } from './interact';
import { Want } from './want';

// Frontend display type for draft wants
export interface DraftWant {
  id: string; // Unique draft ID (matches want.metadata.id)
  sessionId: string; // Interact session ID
  message: string; // User's original message
  recommendations: Recommendation[];
  selectedRecommendation: Recommendation | null;
  isThinking: boolean; // Whether AI is still processing
  createdAt: string;
  error?: string; // Error message if recommendation failed
}

// Data for creating a draft want in the backend
export interface CreateDraftWantData {
  sessionId: string;
  message: string;
  recommendations?: Recommendation[];
  isThinking?: boolean;
  error?: string;
}

// Data for updating a draft want in the backend
export interface UpdateDraftWantData {
  recommendations?: Recommendation[];
  selected_recommendation_id?: string;
  isThinking?: boolean;
  error?: string;
}

// Label used to identify draft wants
export const DRAFT_WANT_LABEL = '__draft';

// Convert a backend Want to a frontend DraftWant
export function wantToDraft(want: Want): DraftWant | null {
  const current = want.state?.current || {};
  const phase = current.phase as string || '';
  
  // Treat as draft if labeled as draft OR if in ideating phase
  const isDraft = want.metadata?.labels?.[DRAFT_WANT_LABEL] === 'true' || phase === 'ideating';
  
  if (!isDraft) {
    return null;
  }

  const isThinking = current.isThinking as boolean || phase === 'ideating' || phase === 'decomposing' || phase === 're_planning';

  return {
    id: want.metadata.id || want.id || '',
    sessionId: (current.sessionId as string) || '',
    message: (current.goal_text as string) || (current.message as string) || want.metadata.name,
    recommendations: (current.proposed_recommendations as Recommendation[]) || (current.recommendations as Recommendation[]) || [],
    selectedRecommendation: null,
    isThinking: isThinking,
    createdAt: current.createdAt as string || new Date().toISOString(),
    error: current.error as string | undefined
  };
}

// Check if a Want is a draft want
export function isDraftWant(want: Want): boolean {
  if (want.metadata?.labels?.[DRAFT_WANT_LABEL] === 'true') return true;
  const phase = want.state?.current?.phase as string | undefined;
  return phase === 'ideating';
}
