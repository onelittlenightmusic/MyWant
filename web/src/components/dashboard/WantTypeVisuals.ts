/**
 * Shared visual constants and helpers for want-type cards.
 * Used by both WantTypeCard (sidebar) and WantCanvas (canvas tiles).
 */
import {
  Zap, Settings, Database, Share2,
  Plane, Calculator, Layers, CheckCircle, Monitor, Tag,
  LucideIcon,
} from 'lucide-react';
import { getBackgroundImage } from '@/utils/backgroundStyles';

// ── Dark-theme (canvas) gradient backgrounds per category ───────────────────
export const CATEGORY_BG_DARK: Record<string, string> = {
  travel:      'linear-gradient(160deg, #0284c7 0%, #0c4a6e 100%)',
  mathematics: 'linear-gradient(160deg, #7c3aed 0%, #4c1d95 100%)',
  math:        'linear-gradient(160deg, #7c3aed 0%, #4c1d95 100%)',
  queue:       'linear-gradient(160deg, #059669 0%, #064e3b 100%)',
  approval:    'linear-gradient(160deg, #d97706 0%, #92400e 100%)',
  system:      'linear-gradient(160deg, #475569 0%, #1e293b 100%)',
};
export const DEFAULT_BG_DARK = 'linear-gradient(160deg, #334155 0%, #1e293b 100%)';

// ── Pattern accent hex colors (shared) ───────────────────────────────────────
export const PATTERN_COLOR: Record<string, string> = {
  generator:   '#3b82f6',
  processor:   '#a855f7',
  sink:        '#ef4444',
  coordinator: '#22c55e',
  independent: '#f59e0b',
};
export const DEFAULT_PATTERN_COLOR = '#94a3b8';

// ── Tailwind badge classes for light theme (WantTypeCard) ────────────────────
export const PATTERN_BADGE_CLASS: Record<string, string> = {
  generator:   'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300',
  processor:   'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300',
  sink:        'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300',
  coordinator: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300',
  independent: 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300',
};
export const DEFAULT_PATTERN_BADGE_CLASS = 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-300';

export const CATEGORY_BADGE_CLASS: Record<string, string> = {
  travel:      'bg-blue-50 text-blue-700 dark:bg-blue-900/20 dark:text-blue-300',
  mathematics: 'bg-purple-50 text-purple-700 dark:bg-purple-900/20 dark:text-purple-300',
  math:        'bg-purple-50 text-purple-700 dark:bg-purple-900/20 dark:text-purple-300',
  queue:       'bg-green-50 text-green-700 dark:bg-green-900/20 dark:text-green-300',
  approval:    'bg-orange-50 text-orange-700 dark:bg-orange-900/20 dark:text-orange-300',
  system:      'bg-gray-50 text-gray-700 dark:bg-gray-800 dark:text-gray-300',
};
export const DEFAULT_CATEGORY_BADGE_CLASS = 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-300';

// ── Icon maps ─────────────────────────────────────────────────────────────────
export const CATEGORY_ICON: Record<string, LucideIcon> = {
  travel:      Plane,
  mathematics: Calculator,
  math:        Calculator,
  queue:       Layers,
  approval:    CheckCircle,
  system:      Monitor,
};

export const PATTERN_ICON: Record<string, LucideIcon> = {
  generator:   Zap,
  processor:   Settings,
  sink:        Database,
  coordinator: Share2,
  independent: Zap,
};

// ── Helper functions ──────────────────────────────────────────────────────────
export const getCategoryBgDark = (category: string): string =>
  CATEGORY_BG_DARK[category.toLowerCase()] ?? DEFAULT_BG_DARK;

export const getCategoryIcon = (category: string): LucideIcon =>
  CATEGORY_ICON[category.toLowerCase()] ?? Tag;

export const getPatternIcon = (pattern: string): LucideIcon =>
  PATTERN_ICON[pattern.toLowerCase()] ?? Zap;

export const getPatternColor = (pattern: string): string =>
  PATTERN_COLOR[pattern.toLowerCase()] ?? DEFAULT_PATTERN_COLOR;

export const getPatternBadgeClass = (pattern: string): string =>
  PATTERN_BADGE_CLASS[pattern.toLowerCase()] ?? DEFAULT_PATTERN_BADGE_CLASS;

export const getCategoryBadgeClass = (category: string): string =>
  CATEGORY_BADGE_CLASS[category.toLowerCase()] ?? DEFAULT_CATEGORY_BADGE_CLASS;

/**
 * Returns inline style props for the card's outer container background.
 * theme="dark"  → category gradient (or photo with dark overlay hint)
 * theme="light" → white base + optional photo
 */
export const getCardBackgroundStyle = (
  typeName: string,
  category: string,
  theme: 'dark' | 'light',
): React.CSSProperties => {
  const bgImage = getBackgroundImage(typeName);
  if (bgImage) {
    return {
      backgroundImage: `url(${bgImage})`,
      backgroundSize: 'cover',
      backgroundPosition: 'center center',
      backgroundRepeat: 'no-repeat',
      backgroundColor: theme === 'dark' ? 'rgba(15,23,42,0.85)' : undefined,
    };
  }
  if (theme === 'dark') {
    return { background: getCategoryBgDark(category) };
  }
  // light: plain white — handled by Tailwind className on the wrapper
  return {};
};

/**
 * Overlay rgba for the card (sits between background and content).
 */
export const getCardOverlayBg = (typeName: string, theme: 'dark' | 'light'): string => {
  const hasImage = !!getBackgroundImage(typeName);
  if (theme === 'dark') return hasImage ? 'rgba(15,23,42,0.45)' : 'rgba(0,0,0,0.12)';
  return hasImage ? 'rgba(255,255,255,0.40)' : 'rgba(255,255,255,0.20)';
};
