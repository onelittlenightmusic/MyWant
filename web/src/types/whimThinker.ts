import { Recommendation, ConversationMessage } from './interact';
import { Want } from './want';

export const WHIM_THINKER_LABEL = '__whim_thinker';

export interface WhimThinkerWant {
  id: string;
  name: string;
  isThinking: boolean;
  recommendations: Recommendation[];
  conversationHistory: ConversationMessage[];
  error?: string;
}

/** Convert a backend Want to a WhimThinkerWant if it has the __whim_thinker label */
export function wantToWhimThinker(want: Want): WhimThinkerWant | null {
  if (want.metadata?.labels?.[WHIM_THINKER_LABEL] !== 'true') return null;

  const current = want.state?.current || {};
  return {
    id: want.metadata?.id || want.id || '',
    name: want.metadata?.name || '',
    isThinking: (current.isThinking as boolean) || false,
    recommendations: (current.recommendations as Recommendation[]) || [],
    conversationHistory: (current.conversationHistory as ConversationMessage[]) || [],
    error: (current.error as string) || undefined,
  };
}

export function isWhimThinkerWant(want: Want): boolean {
  return want.metadata?.labels?.[WHIM_THINKER_LABEL] === 'true';
}
