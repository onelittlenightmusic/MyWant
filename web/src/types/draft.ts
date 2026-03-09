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
  isThinking?: boolean;
  error?: string;
}

// Label used to identify draft wants
export const DRAFT_WANT_LABEL = '__draft';

// Convert a backend Want to a frontend DraftWant
export function wantToDraft(want: Want): DraftWant | null {
  // Check if this is a draft want
  if (want.metadata?.labels?.[DRAFT_WANT_LABEL] !== 'true') {
    return null;
  }

  const current = want.state?.current || {};

  return {
    id: want.metadata.id || want.id || '',
    sessionId: current.sessionId as string || '',
    message: current.message as string || '',
    recommendations: (current.recommendations as Recommendation[]) || [],
    selectedRecommendation: null,
    isThinking: current.isThinking as boolean || false,
    createdAt: current.createdAt as string || '',
    error: current.error as string | undefined
  };
}

// Check if a Want is a draft want
export function isDraftWant(want: Want): boolean {
  return want.metadata?.labels?.[DRAFT_WANT_LABEL] === 'true';
}
