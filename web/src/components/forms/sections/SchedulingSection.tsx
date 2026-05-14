import React, { useState, useCallback, useEffect, forwardRef } from 'react';
import { Clock, Link2, X } from 'lucide-react';
import { WhenSpec } from '@/types/want';
import { SelectInput } from '@/components/common/SelectInput';
import { classNames } from '@/utils/helpers';
import { DisplayCard, AddCard, FormCard } from '@/components/forms/CardPrimitives';
import { TimerPickerContent, TimerMode, EVERY_PRESETS } from '@/components/forms/TimerPickerContent';
import { apiClient } from '@/api/client';
import { SectionNavigationCallbacks } from '@/types/formSection';

interface SchedulingSectionProps {
  schedules: WhenSpec[];
  onChange: (schedules: WhenSpec[]) => void;
  isCollapsed?: boolean;
  onToggleCollapse?: () => void;
  navigationCallbacks?: SectionNavigationCallbacks;
  hideHeader?: boolean;
}

// ── Color scheme ──────────────────────────────────────────────────────────────

const CARD_BG    = 'bg-amber-50/70 dark:bg-amber-900/15';
const CARD_HOVER = 'hover:shadow hover:bg-amber-100/70 dark:hover:bg-amber-900/25';
const BG_ICON_COLOR = 'text-amber-400 dark:text-amber-600';
const ICON_COLOR    = 'text-amber-500 dark:text-amber-400';
const FORM_BORDER   = 'border-amber-300 dark:border-amber-600';
const FORM_BG       = 'bg-amber-50/30 dark:bg-amber-900/10';
const SAVE_COLOR    = 'text-amber-600 dark:text-amber-400 hover:bg-amber-50 dark:hover:bg-amber-900/30';
const ADD_BORDER    = 'border-amber-200 dark:border-amber-800/50 hover:border-amber-400 dark:hover:border-amber-600';
const ADD_ICON      = 'text-amber-300 dark:text-amber-700 group-hover:text-amber-500 dark:group-hover:text-amber-400';

// ── Form state ────────────────────────────────────────────────────────────────

type InputMode = 'timer' | 'global_param';

interface FormState {
  inputMode: InputMode;
  timerMode: TimerMode;
  every: string;
  at: string;
  atRecurrence: string;
  fromGlobalParam: string;
}

const EMPTY_FORM: FormState = {
  inputMode: 'timer',
  timerMode: 'every',
  every: '',
  at: '',
  atRecurrence: '',
  fromGlobalParam: '',
};

// Convert long-form "5 minutes" to preset "5m", or return as-is if already short
const toPreset = (every: string): string => {
  if (!every) return '';
  if (EVERY_PRESETS.includes(every)) return every;
  const parts = every.trim().split(/\s+/);
  if (parts.length === 2) {
    const val = parseInt(parts[0]);
    const unit = parts[1].toLowerCase();
    if (unit.startsWith('sec')) return `${val}s`;
    if (unit.startsWith('min')) return `${val}m`;
    if (unit.startsWith('hour') || unit === 'h') return `${val}h`;
    if (unit.startsWith('day')) return `${val}d`;
  }
  return every;
};

const scheduleToForm = (s: WhenSpec): FormState => {
  if (s.fromGlobalParam) {
    return { ...EMPTY_FORM, inputMode: 'global_param', fromGlobalParam: s.fromGlobalParam };
  }
  if (s.at) {
    const rec = s.every === '1d' || s.every === 'day' ? 'day'
      : s.every === '1w' || s.every === 'week' ? 'week'
      : '';
    return { ...EMPTY_FORM, timerMode: 'at', at: s.at, atRecurrence: rec };
  }
  return { ...EMPTY_FORM, timerMode: 'every', every: toPreset(s.every || '') };
};

const formToSchedule = (f: FormState): WhenSpec | null => {
  if (f.inputMode === 'global_param') {
    return f.fromGlobalParam.trim() ? { fromGlobalParam: f.fromGlobalParam.trim() } : null;
  }
  if (f.timerMode === 'every') {
    return f.every ? { every: f.every } : null;
  }
  if (!f.at) return null;
  const recEvery = f.atRecurrence === 'day' ? '1d' : f.atRecurrence === 'week' ? '1w' : undefined;
  return { at: f.at, ...(recEvery && { every: recEvery }) };
};

const scheduleLabel = (s: WhenSpec): string => {
  if (s.fromGlobalParam) return s.fromGlobalParam;
  if (s.at && s.every) return `${s.at} / ${s.every}`;
  if (s.at) return `at ${s.at}`;
  return `every ${s.every || '--'}`;
};

// ── Component ─────────────────────────────────────────────────────────────────

export const SchedulingSection = forwardRef<HTMLButtonElement, SchedulingSectionProps>(({
  schedules,
  onChange,
}, ref) => {
  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const [form, setForm] = useState<FormState>(EMPTY_FORM);
  const [timerParams, setTimerParams] = useState<Record<string, { at?: string; every?: string }>>({});

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
      .catch(() => {});
  }, []);

  const startAdd = useCallback(() => {
    setEditingIndex(schedules.length);
    setForm(EMPTY_FORM);
  }, [schedules.length]);

  const startEdit = useCallback((idx: number) => {
    setEditingIndex(idx);
    setForm(scheduleToForm(schedules[idx]));
  }, [schedules]);

  const handleSave = useCallback(() => {
    const newSchedule = formToSchedule(form);
    if (!newSchedule) return;
    const next = [...schedules];
    if (editingIndex !== null && editingIndex < schedules.length) {
      next[editingIndex] = newSchedule;
    } else {
      next.push(newSchedule);
    }
    onChange(next);
    setEditingIndex(null);
    setForm(EMPTY_FORM);
  }, [form, schedules, editingIndex, onChange]);

  const handleCancel = useCallback(() => {
    setEditingIndex(null);
    setForm(EMPTY_FORM);
  }, []);

  const handleRemove = useCallback((idx: number) => {
    onChange(schedules.filter((_, i) => i !== idx));
    if (editingIndex === idx) {
      setEditingIndex(null);
      setForm(EMPTY_FORM);
    }
  }, [schedules, editingIndex, onChange]);

  const isSaveEnabled = formToSchedule(form) !== null;

  const isAdding = editingIndex === schedules.length;

  return (
    <div className="space-y-3 pt-1">
      <div className="grid grid-cols-2 gap-2">
        {schedules.map((s, idx) => {
          if (editingIndex === idx) {
            return (
              <FormCard
                key={idx}
                borderClass={FORM_BORDER}
                bgClass={FORM_BG}
                saveColorClass={SAVE_COLOR}
                colSpan2
                header={
                  <>
                    <Clock className="w-2.5 h-2.5 text-amber-400" />
                    <span className="text-[10px] text-amber-500 dark:text-amber-400 font-medium">Edit schedule</span>
                  </>
                }
                onSave={handleSave}
                onCancel={handleCancel}
                saveDisabled={!isSaveEnabled}
              >
                <ScheduleFormBody
                  form={form}
                  setForm={setForm}
                  timerParams={timerParams}
                />
              </FormCard>
            );
          }
          return (
            <DisplayCard
              key={idx}
              className={classNames(
                'relative rounded-xl p-2.5 shadow-sm transition-all duration-150 group cursor-pointer',
                CARD_BG, CARD_HOVER,
              )}
              BgIcon={s.fromGlobalParam ? Link2 : Clock}
              bgIconColor={BG_ICON_COLOR}
              onClick={() => startEdit(idx)}
              headerLeft={
                <>
                  {s.fromGlobalParam
                    ? <Link2 className={classNames('w-2.5 h-2.5 flex-shrink-0', ICON_COLOR)} />
                    : <Clock className={classNames('w-2.5 h-2.5 flex-shrink-0', ICON_COLOR)} />}
                  <span className="text-[11px] font-semibold text-gray-600 dark:text-gray-300 truncate leading-none font-mono">
                    {scheduleLabel(s)}
                  </span>
                </>
              }
              headerRight={
                <button
                  type="button"
                  onClick={(e) => { e.stopPropagation(); handleRemove(idx); }}
                  className="opacity-0 group-hover:opacity-100 w-4 h-4 flex items-center justify-center text-gray-300 dark:text-gray-600 hover:text-red-400 dark:hover:text-red-500 transition-all"
                  title="Remove"
                >
                  <X className="w-2.5 h-2.5" />
                </button>
              }
            >
              {s.fromGlobalParam && (s.at || s.every) && (
                <p className="text-[10px] text-amber-600 dark:text-amber-400 font-mono truncate">
                  {s.at ? `${s.at}${s.every ? ` / ${s.every}` : ''}` : `every ${s.every}`}
                </p>
              )}
            </DisplayCard>
          );
        })}

        {/* Add card or form */}
        {isAdding ? (
          <FormCard
            borderClass={FORM_BORDER}
            bgClass={FORM_BG}
            saveColorClass={SAVE_COLOR}
            colSpan2
            header={
              <>
                <Clock className="w-2.5 h-2.5 text-amber-400" />
                <span className="text-[10px] text-amber-500 dark:text-amber-400 font-medium">New schedule</span>
              </>
            }
            onSave={handleSave}
            onCancel={handleCancel}
            saveDisabled={!isSaveEnabled}
          >
            <ScheduleFormBody
              form={form}
              setForm={setForm}
              timerParams={timerParams}
            />
          </FormCard>
        ) : editingIndex === null ? (
          <AddCard
            borderClass={ADD_BORDER}
            iconClass={ADD_ICON}
            label="Add schedule"
            onClick={startAdd}
          />
        ) : null}
      </div>

      <p className="text-[10px] text-gray-400 dark:text-gray-500 pt-1">
        Schedules define when this want is triggered automatically.
      </p>
    </div>
  );
});

SchedulingSection.displayName = 'SchedulingSection';

// ── Form body sub-component ───────────────────────────────────────────────────

const ScheduleFormBody: React.FC<{
  form: FormState;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
  timerParams: Record<string, { at?: string; every?: string }>;
}> = ({ form, setForm, timerParams }) => (
  <div className="space-y-2">
    {/* Input mode toggle */}
    <div className="flex gap-0.5 p-0.5 bg-gray-100 dark:bg-gray-800 rounded-lg mb-2">
      {(['timer', 'global_param'] as InputMode[]).map((m) => (
        <button
          key={m}
          type="button"
          onClick={() => setForm(f => ({ ...f, inputMode: m }))}
          className={classNames(
            'flex-1 text-[10px] font-mono font-semibold py-0.5 rounded-md transition-all',
            form.inputMode === m
              ? 'bg-white dark:bg-gray-700 text-amber-600 dark:text-amber-400 shadow-sm'
              : 'text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300',
          )}
        >
          {m === 'timer' ? 'schedule' : 'global param'}
        </button>
      ))}
    </div>

    {form.inputMode === 'global_param' ? (
      Object.keys(timerParams).length > 0 ? (
        <SelectInput
          value={form.fromGlobalParam}
          onChange={(val) => setForm(f => ({ ...f, fromGlobalParam: val }))}
          options={[
            { value: '', label: '— select a timer —' },
            ...Object.entries(timerParams).map(([key, spec]) => ({
              value: key,
              label: `${key} — ${spec.at ? `${spec.at}${spec.every ? `, every ${spec.every}` : ''}` : `every ${spec.every}`}`,
            })),
          ]}
          className="w-full"
        />
      ) : (
        <p className="text-[10px] text-gray-400 dark:text-gray-500 italic">
          No timer parameters found in parameters.yaml
        </p>
      )
    ) : (
      <TimerPickerContent
        mode={form.timerMode}
        every={form.every}
        at={form.at}
        atRecurrence={form.atRecurrence}
        onModeChange={(m) => setForm(f => ({ ...f, timerMode: m }))}
        onEveryChange={(v) => setForm(f => ({ ...f, every: v }))}
        onAtChange={(v) => setForm(f => ({ ...f, at: v }))}
        onAtRecurrenceChange={(r) => setForm(f => ({ ...f, atRecurrence: r }))}
      />
    )}
  </div>
);
