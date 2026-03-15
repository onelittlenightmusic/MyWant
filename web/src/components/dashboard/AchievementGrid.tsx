import React, { useMemo, useState } from 'react';
import { Achievement } from '@/types/achievement';
import { AchievementCard } from './AchievementCard';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';

interface AchievementGridProps {
  achievements: Achievement[];
  loading: boolean;
  onDelete?: (id: string) => void;
  onUnlock?: (id: string) => void;
  onLock?: (id: string) => void;
}

export const AchievementGrid: React.FC<AchievementGridProps> = ({
  achievements,
  loading,
  onDelete,
  onUnlock,
  onLock,
}) => {
  const [searchQuery, setSearchQuery] = useState('');
  const [levelFilter, setLevelFilter] = useState<number | null>(null);
  const [categoryFilter, setCategoryFilter] = useState('');
  const [unlocksOnly, setUnlocksOnly] = useState(false);

  const categories = useMemo(() => {
    const set = new Set(achievements.map((a) => a.category).filter(Boolean));
    return Array.from(set).sort();
  }, [achievements]);

  const filtered = useMemo(() => {
    return achievements.filter((a) => {
      if (searchQuery) {
        const q = searchQuery.toLowerCase();
        if (
          !a.title.toLowerCase().includes(q) &&
          !a.agentName.toLowerCase().includes(q) &&
          !(a.description ?? '').toLowerCase().includes(q)
        ) {
          return false;
        }
      }
      if (levelFilter !== null && a.level !== levelFilter) return false;
      if (categoryFilter && a.category !== categoryFilter) return false;
      if (unlocksOnly && !a.unlocksCapability) return false;
      return true;
    });
  }, [achievements, searchQuery, levelFilter, categoryFilter, unlocksOnly]);

  if (loading) {
    return (
      <div className="flex justify-center items-center py-16">
        <LoadingSpinner />
      </div>
    );
  }

  return (
    <div>
      {/* Filters */}
      <div className="flex flex-wrap gap-2 mb-4">
        <input
          type="text"
          placeholder="Search achievements..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          className="px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
        />
        <select
          value={levelFilter ?? ''}
          onChange={(e) => setLevelFilter(e.target.value ? Number(e.target.value) : null)}
          className="px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
        >
          <option value="">All levels</option>
          <option value="1">🥉 Bronze</option>
          <option value="2">🥈 Silver</option>
          <option value="3">🥇 Gold</option>
        </select>
        {categories.length > 0 && (
          <select
            value={categoryFilter}
            onChange={(e) => setCategoryFilter(e.target.value)}
            className="px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            <option value="">All categories</option>
            {categories.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
        )}
        <label className="flex items-center gap-1.5 text-sm text-gray-600 dark:text-gray-400 cursor-pointer">
          <input
            type="checkbox"
            checked={unlocksOnly}
            onChange={(e) => setUnlocksOnly(e.target.checked)}
            className="rounded border-gray-300"
          />
          Unlocks capability only
        </label>
      </div>

      {/* Grid */}
      {filtered.length === 0 ? (
        <div className="text-center py-16 text-gray-400 dark:text-gray-600">
          {achievements.length === 0 ? (
            <div>
              <p className="text-lg mb-2">No achievements yet</p>
              <p className="text-sm">Achievements are awarded when agents complete wants.</p>
            </div>
          ) : (
            <p>No achievements match the current filters.</p>
          )}
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
          {filtered.map((a) => (
            <AchievementCard key={a.id} achievement={a} onDelete={onDelete} onUnlock={onUnlock} onLock={onLock} />
          ))}
        </div>
      )}

      {filtered.length > 0 && (
        <p className="mt-4 text-xs text-gray-400 text-right">
          {filtered.length} of {achievements.length} achievements
        </p>
      )}
    </div>
  );
};
