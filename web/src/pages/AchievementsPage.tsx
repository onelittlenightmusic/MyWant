import React, { useEffect, useState } from 'react';
import { Trophy, Zap, Trash2, Plus } from 'lucide-react';
import { useAchievementStore } from '@/stores/achievementStore';
import { Header } from '@/components/layout/Header';
import { AchievementGrid } from '@/components/dashboard/AchievementGrid';
import { ConfirmDeleteModal } from '@/components/modals/ConfirmDeleteModal';
import { classNames } from '@/utils/helpers';
import { AchievementRule, CreateRuleRequest } from '@/types/achievement';

type Tab = 'achievements' | 'rules';

export const AchievementsPage: React.FC = () => {
  const {
    achievements, rules, loading, error,
    fetchAchievements, fetchRules,
    createAchievement: _createAchievement,
    lockAchievement,
    unlockAchievement,
    deleteAchievement, createRule, deleteRule,
    clearError,
  } = useAchievementStore();

  const [tab, setTab] = useState<Tab>('achievements');
  const [pendingDeleteId, setPendingDeleteId] = useState<string | null>(null);
  const [pendingDeleteRuleId, setPendingDeleteRuleId] = useState<string | null>(null);
  const [showNewRule, setShowNewRule] = useState(false);
  const [newRule, setNewRule] = useState<Partial<CreateRuleRequest>>({
    active: true,
    condition: { completedCount: 1 },
    award: { title: '', level: 1, category: 'execution' },
  });

  useEffect(() => {
    fetchAchievements();
    fetchRules();
  }, [fetchAchievements, fetchRules]);

  useEffect(() => {
    if (error) {
      const t = setTimeout(clearError, 5000);
      return () => clearTimeout(t);
    }
  }, [error, clearError]);

  const handleLockAchievement = async (id: string) => {
    await lockAchievement(id);
  };

  const handleUnlockAchievement = async (id: string) => {
    await unlockAchievement(id);
  };

  const handleDeleteAchievement = async () => {
    if (!pendingDeleteId) return;
    await deleteAchievement(pendingDeleteId);
    setPendingDeleteId(null);
  };

  const handleDeleteRule = async () => {
    if (!pendingDeleteRuleId) return;
    await deleteRule(pendingDeleteRuleId);
    setPendingDeleteRuleId(null);
  };

  const handleCreateRule = async () => {
    if (!newRule.award?.title || !newRule.condition?.completedCount) return;
    await createRule(newRule as CreateRuleRequest);
    setShowNewRule(false);
    setNewRule({ active: true, condition: { completedCount: 1 }, award: { title: '', level: 1, category: 'execution' } });
  };

  // Stats
  const gold = achievements.filter((a) => a.level === 3).length;
  const silver = achievements.filter((a) => a.level === 2).length;
  const bronze = achievements.filter((a) => a.level === 1).length;
  const withUnlocks = achievements.filter((a) => a.unlocksCapability).length;

  return (
    <>
      <Header
        onCreateWant={() => {}}
        title="Achievements"
        hideCreateButton
        itemCount={achievements.length}
        itemLabel="achievement"
      />

      <main className="flex-1 overflow-y-auto bg-gray-50 dark:bg-gray-950">
        <div className="p-3 sm:p-6 pb-24 max-w-7xl mx-auto">
          {error && (
            <div className="mb-4 p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md flex items-center justify-between">
              <p className="text-sm text-red-700 dark:text-red-300">{error}</p>
              <button onClick={clearError} className="text-red-400 hover:text-red-600 ml-4 text-sm">✕</button>
            </div>
          )}

          {/* Stats */}
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-6">
            {[
              { label: '🥇 Gold', value: gold, color: 'text-yellow-600 dark:text-yellow-400' },
              { label: '🥈 Silver', value: silver, color: 'text-gray-500 dark:text-gray-400' },
              { label: '🥉 Bronze', value: bronze, color: 'text-amber-600 dark:text-amber-400' },
              { label: '🔓 Unlocks', value: withUnlocks, color: 'text-emerald-600 dark:text-emerald-400' },
            ].map((s) => (
              <div key={s.label} className="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-3 text-center">
                <div className={`text-2xl font-bold ${s.color}`}>{s.value}</div>
                <div className="text-xs text-gray-500 dark:text-gray-400 mt-0.5">{s.label}</div>
              </div>
            ))}
          </div>

          {/* Tabs */}
          <div className="flex gap-1 mb-4 border-b border-gray-200 dark:border-gray-700">
            {(['achievements', 'rules'] as Tab[]).map((t) => (
              <button
                key={t}
                onClick={() => setTab(t)}
                className={classNames(
                  'px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors',
                  tab === t
                    ? 'border-blue-500 text-blue-600 dark:text-blue-400'
                    : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
                )}
              >
                {t === 'achievements' ? (
                  <span className="flex items-center gap-1.5"><Trophy className="w-3.5 h-3.5" /> Achievements ({achievements.length})</span>
                ) : (
                  <span className="flex items-center gap-1.5"><Zap className="w-3.5 h-3.5" /> Rules ({rules.length})</span>
                )}
              </button>
            ))}
          </div>

          {/* Tab content */}
          {tab === 'achievements' && (
            achievements.length === 0 && !loading ? (
              <div className="text-center py-20 text-gray-400">
                <Trophy className="w-12 h-12 mx-auto mb-3 opacity-30" />
                <p className="text-lg font-medium mb-1">No achievements yet</p>
                <p className="text-sm">Achievements are earned when agents successfully complete wants.</p>
              </div>
            ) : (
              <AchievementGrid achievements={achievements} loading={loading} onDelete={(id) => setPendingDeleteId(id)} onUnlock={handleUnlockAchievement} onLock={handleLockAchievement} />
            )
          )}

          {tab === 'rules' && (
            <div className="space-y-3">
              <div className="flex justify-end">
                <button
                  onClick={() => setShowNewRule(true)}
                  className="flex items-center gap-1.5 text-sm px-3 py-1.5 bg-blue-600 hover:bg-blue-700 text-white rounded-md transition-colors"
                >
                  <Plus className="w-3.5 h-3.5" /> Add Rule
                </button>
              </div>

              {/* New rule form */}
              {showNewRule && (
                <div className="bg-white dark:bg-gray-800 border border-blue-200 dark:border-blue-700 rounded-lg p-4 space-y-3">
                  <h3 className="text-sm font-semibold text-gray-800 dark:text-gray-200">New Achievement Rule</h3>
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                    <div>
                      <label className="text-xs text-gray-500 mb-1 block">Capability (empty = any)</label>
                      <input
                        className="w-full text-sm px-2 py-1.5 border rounded dark:bg-gray-700 dark:border-gray-600"
                        placeholder="e.g. hotel_agency"
                        value={newRule.condition?.agentCapability ?? ''}
                        onChange={(e) => setNewRule((r) => ({ ...r, condition: { ...r.condition!, agentCapability: e.target.value } }))}
                      />
                    </div>
                    <div>
                      <label className="text-xs text-gray-500 mb-1 block">Min completions</label>
                      <input
                        type="number" min={1}
                        className="w-full text-sm px-2 py-1.5 border rounded dark:bg-gray-700 dark:border-gray-600"
                        value={newRule.condition?.completedCount ?? 1}
                        onChange={(e) => setNewRule((r) => ({ ...r, condition: { ...r.condition!, completedCount: Number(e.target.value) } }))}
                      />
                    </div>
                    <div>
                      <label className="text-xs text-gray-500 mb-1 block">Award title *</label>
                      <input
                        className="w-full text-sm px-2 py-1.5 border rounded dark:bg-gray-700 dark:border-gray-600"
                        placeholder="e.g. ホテル達人"
                        value={newRule.award?.title ?? ''}
                        onChange={(e) => setNewRule((r) => ({ ...r, award: { ...r.award!, title: e.target.value } }))}
                      />
                    </div>
                    <div>
                      <label className="text-xs text-gray-500 mb-1 block">Unlocks capability</label>
                      <input
                        className="w-full text-sm px-2 py-1.5 border rounded dark:bg-gray-700 dark:border-gray-600"
                        placeholder="e.g. hotel_expert"
                        value={newRule.award?.unlocksCapability ?? ''}
                        onChange={(e) => setNewRule((r) => ({ ...r, award: { ...r.award!, unlocksCapability: e.target.value } }))}
                      />
                    </div>
                    <div>
                      <label className="text-xs text-gray-500 mb-1 block">Level</label>
                      <select
                        className="w-full text-sm px-2 py-1.5 border rounded dark:bg-gray-700 dark:border-gray-600"
                        value={newRule.award?.level ?? 1}
                        onChange={(e) => setNewRule((r) => ({ ...r, award: { ...r.award!, level: Number(e.target.value) } }))}
                      >
                        <option value={1}>🥉 Bronze</option>
                        <option value={2}>🥈 Silver</option>
                        <option value={3}>🥇 Gold</option>
                      </select>
                    </div>
                    <div>
                      <label className="text-xs text-gray-500 mb-1 block">Category</label>
                      <select
                        className="w-full text-sm px-2 py-1.5 border rounded dark:bg-gray-700 dark:border-gray-600"
                        value={newRule.award?.category ?? 'execution'}
                        onChange={(e) => setNewRule((r) => ({ ...r, award: { ...r.award!, category: e.target.value } }))}
                      >
                        <option value="execution">execution</option>
                        <option value="quality">quality</option>
                        <option value="specialization">specialization</option>
                      </select>
                    </div>
                  </div>
                  <div className="flex gap-2 justify-end">
                    <button onClick={() => setShowNewRule(false)} className="text-sm px-3 py-1.5 border rounded text-gray-600 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-gray-700">Cancel</button>
                    <button onClick={handleCreateRule} className="text-sm px-3 py-1.5 bg-blue-600 hover:bg-blue-700 text-white rounded">Create</button>
                  </div>
                </div>
              )}

              {/* Rule list */}
              {rules.length === 0 ? (
                <div className="text-center py-12 text-gray-400">
                  <Zap className="w-10 h-10 mx-auto mb-2 opacity-30" />
                  <p>No rules yet. Add a rule to enable automatic achievement awarding.</p>
                </div>
              ) : (
                rules.map((rule) => <RuleCard key={rule.id} rule={rule} onDelete={() => setPendingDeleteRuleId(rule.id)} />)
              )}
            </div>
          )}
        </div>
      </main>

      <ConfirmDeleteModal isOpen={!!pendingDeleteId} onClose={() => setPendingDeleteId(null)} onConfirm={handleDeleteAchievement} want={null} loading={loading} title="Delete Achievement" message="Are you sure you want to delete this achievement?" />
      <ConfirmDeleteModal isOpen={!!pendingDeleteRuleId} onClose={() => setPendingDeleteRuleId(null)} onConfirm={handleDeleteRule} want={null} loading={loading} title="Delete Rule" message="Are you sure you want to delete this rule? No more achievements will be automatically awarded by it." />
    </>
  );
};

const RuleCard: React.FC<{ rule: AchievementRule; onDelete: () => void }> = ({ rule, onDelete }) => (
  <div className="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4 flex items-start justify-between gap-4">
    <div className="flex-1 min-w-0">
      <div className="flex items-center gap-2 mb-1">
        <span className={classNames('w-2 h-2 rounded-full flex-shrink-0', rule.active ? 'bg-green-500' : 'bg-gray-400')} />
        <span className="font-medium text-sm text-gray-800 dark:text-gray-200">{rule.award.title}</span>
        <span className="text-xs px-1.5 py-0.5 rounded bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-400">
          {rule.award.level === 3 ? '🥇' : rule.award.level === 2 ? '🥈' : '🥉'} {rule.award.category}
        </span>
      </div>
      <p className="text-xs text-gray-500 dark:text-gray-400">
        Condition: {rule.condition.agentCapability ? `${rule.condition.agentCapability} ` : 'any agent '}
        completed ≥ {rule.condition.completedCount} want{rule.condition.completedCount !== 1 ? 's' : ''}
        {rule.condition.wantType ? ` of type "${rule.condition.wantType}"` : ''}
      </p>
      {rule.award.unlocksCapability && (
        <p className="text-xs text-emerald-600 dark:text-emerald-400 mt-1">🔓 Unlocks: {rule.award.unlocksCapability}</p>
      )}
      <p className="text-xs text-gray-400 mt-1 font-mono">{rule.id}</p>
    </div>
    <button onClick={onDelete} className="p-1.5 text-gray-400 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 rounded transition-colors flex-shrink-0">
      <Trash2 className="w-3.5 h-3.5" />
    </button>
  </div>
);
