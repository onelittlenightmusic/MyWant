export interface Achievement {
  id: string;
  title: string;
  description: string;
  agentName: string;
  wantID: string;
  wantName: string;
  category: string; // execution | quality | specialization
  level: number;    // 1=bronze 2=silver 3=gold
  earnedAt: string;
  awardedBy: string; // system | capability_manager | human
  unlocksCapability?: string;
  /** When false (default), the achievement exists but its capability is not active yet. */
  unlocked: boolean;
  metadata?: Record<string, unknown>;
}

export interface AchievementListResponse {
  achievements: Achievement[];
  count: number;
}

export interface CreateAchievementRequest {
  title: string;
  description?: string;
  agentName: string;
  wantID?: string;
  wantName?: string;
  category?: string;
  level?: number;
  unlocksCapability?: string;
  metadata?: Record<string, unknown>;
}

export const LEVEL_LABELS: Record<number, string> = {
  1: 'Bronze',
  2: 'Silver',
  3: 'Gold',
};

export const LEVEL_COLORS: Record<number, { border: string; bg: string; text: string; badge: string }> = {
  1: {
    border: 'border-amber-600',
    bg: 'bg-amber-50 dark:bg-amber-900/20',
    text: 'text-amber-700 dark:text-amber-300',
    badge: 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300',
  },
  2: {
    border: 'border-gray-400',
    bg: 'bg-gray-50 dark:bg-gray-800/40',
    text: 'text-gray-600 dark:text-gray-300',
    badge: 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300',
  },
  3: {
    border: 'border-yellow-400',
    bg: 'bg-yellow-50 dark:bg-yellow-900/20',
    text: 'text-yellow-700 dark:text-yellow-300',
    badge: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/40 dark:text-yellow-300',
  },
};

export const LEVEL_EMOJI: Record<number, string> = {
  1: '🥉',
  2: '🥈',
  3: '🥇',
};

// ── Rules ─────────────────────────────────────────────────────────────────────

export interface AchievementCondition {
  agentCapability?: string;
  wantType?: string;
  completedCount: number;
}

export interface AchievementAward {
  title: string;
  description?: string;
  level: number;
  category: string;
  unlocksCapability?: string;
}

export interface AchievementRule {
  id: string;
  active: boolean;
  condition: AchievementCondition;
  award: AchievementAward;
}

export interface AchievementRuleListResponse {
  rules: AchievementRule[];
  count: number;
}

export interface CreateRuleRequest {
  active: boolean;
  condition: AchievementCondition;
  award: AchievementAward;
}
