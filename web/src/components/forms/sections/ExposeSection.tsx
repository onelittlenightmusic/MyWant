import React, { useState, useCallback } from 'react';
import { ArrowUpFromLine, ArrowRight, KeyRound } from 'lucide-react';
import { StateDef } from '@/types/wantType';
import { SelectInput } from '@/components/common/SelectInput';
import { classNames } from '@/utils/helpers';
import { CardGridShell, PURPLE_SCHEME } from '@/components/forms/CardPrimitives';

export interface ExposeEntry {
  currentState?: string;
  param?: string;
  as?: string;
}

interface ExposeSectionProps {
  exposes: ExposeEntry[];
  onExposesChange: (exposes: ExposeEntry[]) => void;
  stateDefs?: StateDef[];
}

const inputCls = classNames(
  'w-full text-xs px-2 py-1 rounded border bg-white dark:bg-gray-800 mb-1.5',
  'text-gray-700 dark:text-gray-200 placeholder-gray-300 dark:placeholder-gray-600',
  'border-purple-200 dark:border-purple-700',
  'focus:outline-none focus:ring-1 focus:ring-purple-400',
);

export const ExposeSection: React.FC<ExposeSectionProps> = ({
  exposes,
  onExposesChange,
  stateDefs,
}) => {
  const [adding, setAdding] = useState(false);
  const [newState, setNewState] = useState('');
  const [newAs, setNewAs] = useState('');

  const stateFields = stateDefs?.filter(s => s.name !== 'final_result') ?? [];
  const validExposes = exposes.filter(e => e.currentState && e.as);

  const handleSave = useCallback(() => {
    if (!newState || !newAs.trim()) return;
    const next = exposes.filter(e => e.currentState !== newState);
    next.push({ currentState: newState, as: newAs.trim() });
    onExposesChange(next);
    setNewState('');
    setNewAs('');
    setAdding(false);
  }, [exposes, onExposesChange, newState, newAs]);

  const handleCancel = useCallback(() => {
    setAdding(false);
    setNewState('');
    setNewAs('');
  }, []);

  const handleDelete = useCallback((i: number) => {
    const target = validExposes[i];
    onExposesChange(exposes.filter(e => e.currentState !== target.currentState));
  }, [validExposes, exposes, onExposesChange]);

  return (
    <CardGridShell
      scheme={PURPLE_SCHEME}
      BgIcon={ArrowUpFromLine}
      count={validExposes.length}
      editingIndex={adding ? validExposes.length : null}
      addLabel="Add expose"
      onAdd={() => setAdding(true)}
      onDeleteItem={handleDelete}
      onSave={handleSave}
      onCancel={handleCancel}
      saveDisabled={!newState || !newAs.trim()}
      renderItemHeaderLeft={(i) => {
        const e = validExposes[i];
        return (
          <>
            <KeyRound className={classNames('w-2.5 h-2.5 flex-shrink-0', PURPLE_SCHEME.iconColor)} />
            <span className="text-[11px] font-semibold text-gray-600 dark:text-gray-300 truncate leading-none font-mono">
              {e.currentState}
            </span>
          </>
        );
      }}
      renderItemBody={(i) => {
        const e = validExposes[i];
        return (
          <div className="flex items-center gap-1">
            <ArrowRight className="w-2.5 h-2.5 text-purple-400 dark:text-purple-500 flex-shrink-0 opacity-40" />
            <span className="font-mono text-xs text-purple-700 dark:text-purple-300 truncate">{e.as}</span>
          </div>
        );
      }}
      renderFormHeader={(isNew) => (
        <>
          <ArrowUpFromLine className="w-2.5 h-2.5 text-purple-400" />
          <span className="text-[10px] text-purple-500 dark:text-purple-400 font-medium">
            {isNew ? 'New expose' : 'Edit expose'}
          </span>
        </>
      )}
      renderFormContent={() => (
        <>
          <SelectInput
            value={newState}
            onChange={setNewState}
            options={[
              { value: '', label: 'state field' },
              ...stateFields
                .filter(s => !exposes.some(e => e.currentState === s.name))
                .map(s => ({ value: s.name })),
            ]}
            className="w-full mb-1.5"
          />
          <input
            type="text"
            value={newAs}
            onChange={e => setNewAs(e.target.value)}
            onKeyDown={e => {
              if (e.key === 'Enter') { e.preventDefault(); handleSave(); }
              if (e.key === 'Escape') { e.preventDefault(); handleCancel(); }
            }}
            placeholder="global key"
            className={inputCls}
          />
        </>
      )}
      footerNote="Exposed values are written to global parameters after each execution cycle."
    />
  );
};

ExposeSection.displayName = 'ExposeSection';
