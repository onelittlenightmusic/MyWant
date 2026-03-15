import React from 'react';
import { Trash2, Unlock, Lock } from 'lucide-react';
import { Achievement, LEVEL_COLORS, LEVEL_EMOJI, LEVEL_LABELS } from '@/types/achievement';
import { classNames } from '@/utils/helpers';

interface AchievementCardProps {
  achievement: Achievement;
  onDelete?: (id: string) => void;
  onUnlock?: (id: string) => void;
  onLock?: (id: string) => void;
}

export const AchievementCard: React.FC<AchievementCardProps> = ({ achievement, onDelete, onUnlock, onLock }) => {
  const level = achievement.level ?? 1;
  const colors = LEVEL_COLORS[level] ?? LEVEL_COLORS[1];
  const emoji = LEVEL_EMOJI[level] ?? '🏅';
  const levelLabel = LEVEL_LABELS[level] ?? 'Bronze';

  const earnedDate = achievement.earnedAt
    ? new Date(achievement.earnedAt).toLocaleDateString()
    : '';

  return (
    <div
      className={classNames(
        'relative rounded-lg border-2 p-4 shadow-sm transition-shadow hover:shadow-md',
        colors.border,
        colors.bg,
        !achievement.unlocked ? 'opacity-60' : ''
      )}
    >
      {/* Action buttons */}
      <div className="absolute top-2 right-2 flex gap-1">
        {onUnlock && !achievement.unlocked && (
          <button
            onClick={() => onUnlock(achievement.id)}
            className="p-1 rounded text-gray-400 hover:text-emerald-600 hover:bg-emerald-50 dark:hover:bg-emerald-900/20 transition-colors"
            title="Unlock achievement (activate capability)"
          >
            <Lock className="w-3.5 h-3.5" />
          </button>
        )}
        {onLock && achievement.unlocked && (
          <button
            onClick={() => onLock(achievement.id)}
            className="p-1 rounded text-emerald-500 hover:text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
            title="Lock achievement (deactivate capability)"
          >
            <Unlock className="w-3.5 h-3.5" />
          </button>
        )}
        {onDelete && (
          <button
            onClick={() => onDelete(achievement.id)}
            className="p-1 rounded text-gray-400 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 transition-colors"
            title="Delete achievement"
          >
            <Trash2 className="w-3.5 h-3.5" />
          </button>
        )}
      </div>

      {/* Header: emoji + title */}
      <div className="flex items-start gap-2 pr-6">
        <span className="text-2xl leading-none">{emoji}</span>
        <div className="flex-1 min-w-0">
          <h3 className={classNames('font-semibold text-sm leading-tight truncate', colors.text)}>
            {achievement.title}
          </h3>
          {achievement.description && (
            <p className="text-xs text-gray-500 dark:text-gray-400 mt-0.5 line-clamp-2">
              {achievement.description}
            </p>
          )}
        </div>
      </div>

      {/* Badges row */}
      <div className="flex flex-wrap gap-1.5 mt-3">
        <span className={classNames('text-xs px-2 py-0.5 rounded-full font-medium', colors.badge)}>
          {levelLabel}
        </span>
        {achievement.category && (
          <span className="text-xs px-2 py-0.5 rounded-full bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300 font-medium">
            {achievement.category}
          </span>
        )}
        {achievement.awardedBy && (
          <span className="text-xs px-2 py-0.5 rounded-full bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400">
            by {achievement.awardedBy}
          </span>
        )}
      </div>

      {/* Agent & Want */}
      <div className="mt-3 space-y-1">
        <div className="flex items-center gap-1.5">
          <span className="text-xs text-gray-400 w-10 flex-shrink-0">Agent</span>
          <span className="text-xs font-mono text-gray-700 dark:text-gray-300 truncate">
            {achievement.agentName}
          </span>
        </div>
        {achievement.wantName && (
          <div className="flex items-center gap-1.5">
            <span className="text-xs text-gray-400 w-10 flex-shrink-0">Want</span>
            <span className="text-xs text-gray-600 dark:text-gray-400 truncate">
              {achievement.wantName}
            </span>
          </div>
        )}
      </div>

      {/* Unlocks capability */}
      {achievement.unlocksCapability && (
        <div className={classNames(
          'mt-3 flex items-center gap-1.5 text-xs rounded px-2 py-1',
          achievement.unlocked
            ? 'text-emerald-700 dark:text-emerald-400 bg-emerald-50 dark:bg-emerald-900/20'
            : 'text-gray-500 dark:text-gray-400 bg-gray-100 dark:bg-gray-800'
        )}>
          {achievement.unlocked
            ? <Unlock className="w-3 h-3 flex-shrink-0" />
            : <Lock className="w-3 h-3 flex-shrink-0" />}
          <span className="font-mono truncate">{achievement.unlocksCapability}</span>
          {!achievement.unlocked && <span className="ml-auto text-gray-400">(locked)</span>}
        </div>
      )}

      {/* Earned date */}
      {earnedDate && (
        <div className="mt-3 text-right">
          <span className="text-xs text-gray-400">{earnedDate}</span>
        </div>
      )}
    </div>
  );
};
