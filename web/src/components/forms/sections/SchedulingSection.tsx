import React, { useState, useCallback, useMemo, forwardRef } from 'react';
import { Clock } from 'lucide-react';
import { CollapsibleFormSection } from '../CollapsibleFormSection';
import { ChipItem, SectionNavigationCallbacks } from '@/types/formSection';
import { CommitInput } from '@/components/common/CommitInput';

/**
 * Props for SchedulingSection
 */
interface SchedulingSectionProps {
  /** Current schedules as array of { at?, every } objects */
  schedules: Array<{ at?: string; every: string }>;
  /** Callback when schedules change */
  onChange: (schedules: Array<{ at?: string; every: string }>) => void;
  /** Whether the section is collapsed */
  isCollapsed: boolean;
  /** Callback to toggle collapsed state */
  onToggleCollapse: () => void;
  /** Navigation callbacks for moving between sections */
  navigationCallbacks: SectionNavigationCallbacks;
}

/**
 * Editing state for a schedule
 */
interface ScheduleDraft {
  at: string;
  everyValue: string;
  everyUnit: string;
}

/**
 * Parse "every" string like "5 minutes" into value and unit
 */
const parseEvery = (every: string): { value: string; unit: string } => {
  const parts = every.trim().split(/\s+/);
  if (parts.length === 2) {
    return { value: parts[0], unit: parts[1] };
  }
  return { value: every, unit: 'seconds' };
};

/**
 * Format value and unit into "every" string like "5 minutes"
 */
const formatEvery = (value: string, unit: string): string => {
  return `${value} ${unit}`.trim();
};

/**
 * SchedulingSection - Adapter for scheduling using CollapsibleFormSection
 *
 * Manages scheduling as Array<{ at?, every }> internally with custom
 * edit form for time and frequency configuration.
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
  const [editingDraft, setEditingDraft] = useState<ScheduleDraft>({
    at: '',
    everyValue: '',
    everyUnit: 'seconds'
  });

  // Refs for focus management
  const headerRef = React.useRef<HTMLButtonElement>(null);
  const firstInputRef = React.useRef<HTMLInputElement>(null);

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
    if (editingIndex !== null && firstInputRef.current) {
      setTimeout(() => {
        firstInputRef.current?.focus();
      }, 100);
    }
  }, [editingIndex]);

  /**
   * Convert schedules to chip items for display
   */
  const chipItems = useMemo((): ChipItem[] => {
    return schedules
      .map((schedule, index) => {
        // Skip if this schedule is being edited
        if (index === editingIndex) return null;

        const parsed = parseEvery(schedule.every);
        const display = schedule.at
          ? `${schedule.at}, every ${parsed.value} ${parsed.unit}`
          : `every ${parsed.value} ${parsed.unit}`;

        return {
          key: `${index}`,
          display,
          icon: <Clock className="w-3 h-3" />,
        };
      })
      .filter((item) => item !== null) as ChipItem[];
  }, [schedules, editingIndex]);

  /**
   * Start editing an existing schedule
   */
  const handleEditChip = useCallback((chipIndex: number) => {
    // Map chip index to schedule index (accounting for hidden editing chip)
    let actualIndex = chipIndex;
    if (editingIndex !== null && chipIndex >= editingIndex) {
      actualIndex = chipIndex + 1;
    }

    const schedule = schedules[actualIndex];
    if (schedule) {
      const parsed = parseEvery(schedule.every);
      setEditingIndex(actualIndex);
      setEditingDraft({
        at: schedule.at || '',
        everyValue: parsed.value,
        everyUnit: parsed.unit
      });
    }
  }, [schedules, editingIndex]);

  /**
   * Remove a schedule
   */
  const handleRemoveChip = useCallback((chipIndex: number) => {
    // Map chip index to schedule index
    let actualIndex = chipIndex;
    if (editingIndex !== null && chipIndex >= editingIndex) {
      actualIndex = chipIndex + 1;
    }

    const newSchedules = schedules.filter((_, index) => index !== actualIndex);
    onChange(newSchedules);
  }, [schedules, editingIndex, onChange]);

  /**
   * Start adding a new schedule
   */
  const handleAddItem = useCallback(() => {
    setEditingIndex(schedules.length);
    setEditingDraft({ at: '', everyValue: '', everyUnit: 'seconds' });
  }, [schedules.length]);

  /**
   * Save the current schedule edit
   */
  const handleSave = useCallback(() => {
    // Only update if draft has an every value
    if (editingDraft.everyValue.trim()) {
      const newSchedules = [...schedules];
      const every = formatEvery(editingDraft.everyValue, editingDraft.everyUnit);

      // Create new schedule object
      const newSchedule: { at?: string; every: string } = {
        every,
        ...(editingDraft.at && { at: editingDraft.at })
      };

      if (editingIndex !== null && editingIndex < schedules.length) {
        // Replace existing schedule
        newSchedules[editingIndex] = newSchedule;
      } else {
        // Add new schedule
        newSchedules.push(newSchedule);
      }

      onChange(newSchedules);
    }

    // Reset editing state
    setEditingIndex(null);
    setEditingDraft({ at: '', everyValue: '', everyUnit: 'seconds' });
  }, [editingIndex, editingDraft, schedules, onChange]);

  /**
   * Cancel the current schedule edit
   */
  const handleCancel = useCallback(() => {
    setEditingIndex(null);
    setEditingDraft({ at: '', everyValue: '', everyUnit: 'seconds' });
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

  /**
   * Render the edit form
   */
  const renderEditForm = useCallback(() => (
    <div className="space-y-3">
      {/* At (time) - Optional */}
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1">
          At (optional - e.g., "7am", "17:30", "midnight")
        </label>
        <CommitInput
          ref={firstInputRef}
          type="text"
          value={editingDraft.at}
          onChange={(val) => setEditingDraft(prev => ({ ...prev, at: val }))}
          onKeyDown={(e) => {
            if (e.key === 'Escape') {
              e.preventDefault();
              handleEscape();
            }
          }}
          className="w-full"
          placeholder="Optional - e.g., 7am, 17:30"
        />
      </div>

      {/* Every (frequency) */}
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1">
          Every (required)
        </label>
        <div className="flex gap-2 items-end">
          <div className="flex-1">
            <CommitInput
              type="number"
              value={editingDraft.everyValue}
              onChange={(val) => setEditingDraft(prev => ({ ...prev, everyValue: val }))}
              onKeyDown={(e) => {
                if (e.key === 'Escape') {
                  e.preventDefault();
                  handleEscape();
                }
              }}
              className="w-full"
              placeholder="30"
              min="1"
            />
          </div>
          <div className="flex-1">
            <select
              value={editingDraft.everyUnit}
              onChange={(e) => setEditingDraft(prev => ({ ...prev, everyUnit: e.target.value }))}
              className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-amber-500 focus:border-transparent text-sm"
            >
              <option value="seconds">seconds</option>
              <option value="minutes">minutes</option>
              <option value="hours">hours</option>
              <option value="days">days</option>
              <option value="weeks">weeks</option>
            </select>
          </div>
        </div>
      </div>

      <div className="flex gap-2">
        <button
          type="button"
          onClick={handleSave}
          className="px-3 py-1.5 bg-amber-600 text-white text-sm rounded-md hover:bg-amber-700 focus:outline-none focus:ring-2 focus:ring-amber-500"
        >
          Save
        </button>
        <button
          type="button"
          onClick={handleCancel}
          className="px-3 py-1.5 border border-gray-300 text-gray-700 text-sm rounded-md hover:bg-gray-100 focus:outline-none focus:ring-2 focus:ring-gray-300"
        >
          Cancel
        </button>
      </div>
    </div>
  ), [editingDraft, handleSave, handleCancel, handleEscape]);

  /**
   * Render collapsed summary
   */
  const renderCollapsedSummary = useCallback(() => {
    if (schedules.length === 0) return null;

    return (
      <div className="flex flex-wrap gap-2">
        {schedules.map((schedule, index) => {
          const parsed = parseEvery(schedule.every);
          const display = schedule.at
            ? `${schedule.at}, every ${parsed.value} ${parsed.unit}`
            : `every ${parsed.value} ${parsed.unit}`;

          return (
            <span key={index} className="text-xs bg-amber-100 text-amber-700 px-2 py-1 rounded-full flex items-center gap-1">
              <Clock className="w-3 h-3" />
              {display}
            </span>
          );
        })}
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
