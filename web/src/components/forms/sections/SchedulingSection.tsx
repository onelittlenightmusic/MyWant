import React, { useState, useCallback, useMemo, useEffect, forwardRef, useRef } from 'react';
import { Clock, Link } from 'lucide-react';
import { CollapsibleFormSection } from '../CollapsibleFormSection';
import { ChipItem, SectionNavigationCallbacks } from '@/types/formSection';
import { CommitInput, CommitInputHandle } from '@/components/common/CommitInput';
import { SelectInput } from '@/components/common/SelectInput';
import { WhenSpec } from '@/types/want';
import { apiClient } from '@/api/client';

/**
 * Props for SchedulingSection
 */
interface SchedulingSectionProps {
  /** Current schedules */
  schedules: WhenSpec[];
  /** Callback when schedules change */
  onChange: (schedules: WhenSpec[]) => void;
  /** Whether the section is collapsed */
  isCollapsed: boolean;
  /** Callback to toggle collapsed state */
  onToggleCollapse: () => void;
  /** Navigation callbacks for moving between sections */
  navigationCallbacks: SectionNavigationCallbacks;
}

type ScheduleMode = 'manual' | 'global_param';

/**
 * Editing state for a schedule
 */
interface ScheduleDraft {
  mode: ScheduleMode;
  at: string;
  everyValue: string;
  everyUnit: string;
  fromGlobalParam: string;
}

const EMPTY_DRAFT: ScheduleDraft = {
  mode: 'manual',
  at: '',
  everyValue: '',
  everyUnit: 'seconds',
  fromGlobalParam: '',
};

/**
 * Parse "every" string like "5 minutes" or "day" into value and unit
 */
const parseEvery = (every: string): { value: string; unit: string } => {
  if (!every) return { value: '', unit: 'seconds' };

  const parts = every.trim().split(/\s+/);
  if (parts.length === 2) {
    return { value: parts[0], unit: parts[1] };
  }

  // Handle single words like "day", "week"
  const val = parts[0].toLowerCase();
  if (['day', 'week', 'month', 'year'].includes(val)) {
    return { value: '1', unit: val };
  }

  // If it's just a number, assume seconds
  if (!isNaN(Number(val))) {
    return { value: val, unit: 'seconds' };
  }

  return { value: '', unit: val || 'seconds' };
};

/**
 * Format value and unit into "every" string like "5 minutes"
 */
const formatEvery = (value: string, unit: string): string => {
  return `${value} ${unit}`.trim();
};

/**
 * Build a display label for a schedule chip
 */
const scheduleDisplay = (schedule: WhenSpec): string => {
  if (schedule.fromGlobalParam) {
    const suffix = schedule.every
      ? ` (${schedule.at ? `${schedule.at}, ` : ''}every ${schedule.every})`
      : '';
    return `${schedule.fromGlobalParam}${suffix}`;
  }
  const parsed = parseEvery(schedule.every ?? '');
  return schedule.at
    ? `${schedule.at}, every ${parsed.value} ${parsed.unit}`
    : `every ${parsed.value} ${parsed.unit}`;
};

/**
 * SchedulingSection - Adapter for scheduling using CollapsibleFormSection
 *
 * Manages scheduling as WhenSpec[] internally with custom edit form for
 * time/frequency configuration or global parameter reference.
 */
export const SchedulingSection = forwardRef<HTMLButtonElement, SchedulingSectionProps>(({
  schedules,
  onChange,
  isCollapsed,
  onToggleCollapse,
  navigationCallbacks,
}, ref) => {
  // Editing state
  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const [editingDraft, setEditingDraft] = useState<ScheduleDraft>(EMPTY_DRAFT);

  // Available timer params fetched from the backend: key → { at?, every }
  const [timerParams, setTimerParams] = useState<Record<string, { at?: string; every?: string }>>({});

  // Refs for focus management and force commit
  const headerRef = useRef<HTMLButtonElement>(null);
  const firstInputRef = useRef<CommitInputHandle>(null);
  const everyValueRef = useRef<CommitInputHandle>(null);


  // Fetch timer-typed global parameters on mount
  useEffect(() => {
    apiClient.getGlobalParameters()
      .then(({ parameters, types }) => {
        const timerKeys = types?.timer ?? [];
        const timers: Record<string, { at?: string; every?: string }> = {};
        for (const key of timerKeys) {
          const raw = parameters[key] as Record<string, string> | undefined;
          if (raw) timers[key] = { at: raw.at, every: raw.every };
        }
        setTimerParams(timers);
      })
      .catch(() => {/* non-critical */});
  }, []);

  // Merge forwarded ref with local headerRef
  React.useEffect(() => {
    if (typeof ref === 'function') {
      ref(headerRef.current);
    } else if (ref) {
      (ref as React.MutableRefObject<HTMLButtonElement | null>).current = headerRef.current;
    }
  }, [ref]);

  // Auto-focus first input when editing starts
  React.useEffect(() => {
    if (editingIndex !== null && editingDraft.mode === 'manual') {
      setTimeout(() => firstInputRef.current?.focus(), 100);
    }
  }, [editingIndex, editingDraft.mode]);

  /**
   * Convert schedules to chip items for display
   */
  const chipItems = useMemo((): ChipItem[] => {
    return schedules
      .map((schedule, index) => {
        if (index === editingIndex) return null;
        return {
          key: `${index}`,
          display: scheduleDisplay(schedule),
          icon: schedule.fromGlobalParam
            ? <Link className="w-3 h-3" />
            : <Clock className="w-3 h-3" />,
        };
      })
      .filter((item) => item !== null) as ChipItem[];
  }, [schedules, editingIndex]);

  /**
   * Start editing an existing schedule
   */
  const handleEditChip = useCallback((chipIndex: number) => {
    let actualIndex = chipIndex;
    if (editingIndex !== null && chipIndex >= editingIndex) {
      actualIndex = chipIndex + 1;
    }

    const schedule = schedules[actualIndex];
    if (schedule) {
      if (schedule.fromGlobalParam) {
        setEditingIndex(actualIndex);
        setEditingDraft({
          mode: 'global_param',
          at: '',
          everyValue: '',
          everyUnit: 'seconds',
          fromGlobalParam: schedule.fromGlobalParam,
        });
      } else {
        const parsed = parseEvery(schedule.every ?? '');
        setEditingIndex(actualIndex);
        setEditingDraft({
          mode: 'manual',
          at: schedule.at || '',
          everyValue: parsed.value,
          everyUnit: parsed.unit,
          fromGlobalParam: '',
        });
      }
    }
  }, [schedules, editingIndex]);

  /**
   * Remove a schedule
   */
  const handleRemoveChip = useCallback((chipIndex: number) => {
    let actualIndex = chipIndex;
    if (editingIndex !== null && chipIndex >= editingIndex) {
      actualIndex = chipIndex + 1;
    }
    onChange(schedules.filter((_, index) => index !== actualIndex));
  }, [schedules, editingIndex, onChange]);

  /**
   * Start adding a new schedule
   */
  const handleAddItem = useCallback(() => {
    setEditingIndex(schedules.length);
    setEditingDraft(EMPTY_DRAFT);
  }, [schedules.length]);

  /**
   * Save the current schedule edit
   */
  const handleSave = useCallback(() => {
    let newSchedule: WhenSpec | null = null;

    if (editingDraft.mode === 'global_param') {
      const paramName = editingDraft.fromGlobalParam.trim();
      if (paramName) {
        newSchedule = { fromGlobalParam: paramName };
      }
    } else {
      const currentAt = firstInputRef.current?.getValue() ?? editingDraft.at;
      const currentEveryValue = everyValueRef.current?.getValue() ?? editingDraft.everyValue;
      if (currentEveryValue.trim()) {
        newSchedule = {
          every: formatEvery(currentEveryValue, editingDraft.everyUnit),
          ...(currentAt && { at: currentAt }),
        };
      }
    }

    if (newSchedule) {
      const newSchedules = [...schedules];
      if (editingIndex !== null && editingIndex < schedules.length) {
        newSchedules[editingIndex] = newSchedule;
      } else {
        newSchedules.push(newSchedule);
      }
      onChange(newSchedules);
    }

    setEditingIndex(null);
    setEditingDraft(EMPTY_DRAFT);
  }, [editingIndex, editingDraft, schedules, onChange]);

  /**
   * Cancel the current schedule edit
   */
  const handleCancel = useCallback(() => {
    setEditingIndex(null);
    setEditingDraft(EMPTY_DRAFT);
  }, []);

  /**
   * Handle escape key - cancel and return focus to header
   */
  const handleEscape = useCallback(() => {
    handleCancel();
    setTimeout(() => {
      headerRef.current?.focus();
    }, 0);
  }, [handleCancel]);

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === 'Escape') {
      e.preventDefault();
      handleEscape();
    } else if (e.key === 'Enter') {
      setTimeout(handleSave, 0);
    }
  }, [handleEscape, handleSave]);

  /**
   * Render the edit form
   */
  const renderEditForm = useCallback(() => (
    <div className="space-y-3">
      {/* Mode toggle */}
      <div className="flex gap-1 p-1 bg-gray-100 dark:bg-gray-700 rounded-md w-fit">
        <button
          type="button"
          onClick={() => setEditingDraft(prev => ({ ...prev, mode: 'manual' }))}
          className={`px-3 py-1 text-xs rounded font-medium transition-colors ${
            editingDraft.mode === 'manual'
              ? 'bg-white dark:bg-gray-600 text-gray-900 dark:text-gray-100 shadow-sm'
              : 'text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200'
          }`}
        >
          <Clock className="w-3 h-3 inline mr-1" />
          Schedule
        </button>
        <button
          type="button"
          onClick={() => setEditingDraft(prev => ({ ...prev, mode: 'global_param' }))}
          className={`px-3 py-1 text-xs rounded font-medium transition-colors ${
            editingDraft.mode === 'global_param'
              ? 'bg-white dark:bg-gray-600 text-gray-900 dark:text-gray-100 shadow-sm'
              : 'text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200'
          }`}
        >
          <Link className="w-3 h-3 inline mr-1" />
          Global Param
        </button>
      </div>

      {editingDraft.mode === 'global_param' ? (
        /* Global parameter reference - select from backend-classified timer keys */
        <div>
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
            Timer parameter (defined in parameters.yaml)
          </label>
          {Object.keys(timerParams).length > 0 ? (
            <SelectInput
              value={editingDraft.fromGlobalParam}
              onChange={(val) => setEditingDraft(prev => ({ ...prev, fromGlobalParam: val }))}
              options={[
                { value: '', label: '— select a timer —' },
                ...Object.entries(timerParams).map(([key, spec]) => ({
                  value: key,
                  label: `${key} — ${spec.at ? `${spec.at}, every ${spec.every}` : `every ${spec.every}`}`,
                })),
              ]}
              className="w-full"
            />
          ) : (
            <p className="text-sm text-gray-500 dark:text-gray-400 italic">
              No timer parameters found in parameters.yaml
            </p>
          )}
        </div>
      ) : (
        /* Manual schedule */
        <>
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              At (optional - e.g., "7am", "17:30", "midnight")
            </label>
            <CommitInput
              ref={firstInputRef}
              type="text"
              value={editingDraft.at}
              onChange={(val) => setEditingDraft(prev => ({ ...prev, at: val }))}
              onKeyDown={handleKeyDown}
              className="w-full"
              placeholder="Optional - e.g., 7am, 17:30"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Every (required)
            </label>
            <div className="flex gap-2 items-end">
              <div className="flex-1">
                <CommitInput
                  ref={everyValueRef}
                  type="number"
                  value={editingDraft.everyValue}
                  onChange={(val) => setEditingDraft(prev => ({ ...prev, everyValue: val }))}
                  onKeyDown={handleKeyDown}
                  className="w-full"
                  placeholder="30"
                  min="1"
                />
              </div>
              <div className="flex-1">
                <SelectInput
                  value={editingDraft.everyUnit}
                  onChange={(val) => setEditingDraft(prev => ({ ...prev, everyUnit: val }))}
                  options={[
                    { value: 'seconds' },
                    { value: 'minutes' },
                    { value: 'hours' },
                    { value: 'days' },
                    { value: 'weeks' },
                  ]}
                  className="w-full"
                />
              </div>
            </div>
          </div>
        </>
      )}

      <div className="flex gap-2">
        <button
          type="button"
          onClick={handleSave}
          className="px-3 py-1.5 bg-amber-600 text-white text-sm rounded-md hover:bg-amber-700 focus:outline-none focus:ring-2 focus:ring-amber-500 dark:bg-amber-700 dark:hover:bg-amber-600"
        >
          Save
        </button>
        <button
          type="button"
          onClick={handleCancel}
          className="px-3 py-1.5 text-gray-500 dark:text-gray-400 text-sm rounded-md hover:bg-gray-100 dark:hover:bg-gray-700 focus:outline-none"
        >
          Cancel
        </button>
      </div>
    </div>
  ), [editingDraft, handleSave, handleCancel, handleKeyDown]);

  /**
   * Render collapsed summary
   */
  const renderCollapsedSummary = useCallback(() => {
    if (schedules.length === 0) return null;

    return (
      <div className="flex flex-wrap gap-2">
        {schedules.map((schedule, index) => (
          <span key={index} className="text-xs bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300 px-2 py-1 rounded-full flex items-center gap-1">
            {schedule.fromGlobalParam
              ? <Link className="w-3 h-3" />
              : <Clock className="w-3 h-3" />}
            {scheduleDisplay(schedule)}
          </span>
        ))}
      </div>
    );
  }, [schedules]);

  return (
    <CollapsibleFormSection
      ref={headerRef}
      sectionId="scheduling"
      title="When"
      icon={<Clock className="w-5 h-5" />}
      colorScheme="amber"
      isCollapsed={isCollapsed}
      onToggleCollapse={onToggleCollapse}
      navigationCallbacks={navigationCallbacks}
      items={chipItems}
      onAddItem={handleAddItem}
      renderEditForm={renderEditForm}
      renderCollapsedSummary={renderCollapsedSummary}
      isEditing={editingIndex !== null}
      editingIndex={editingIndex ?? -1}
      onEditChip={handleEditChip}
      onRemoveChip={handleRemoveChip}
    />
  );
});

SchedulingSection.displayName = 'SchedulingSection';
