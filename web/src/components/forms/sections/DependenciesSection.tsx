import React, { useState, useCallback, forwardRef, useRef } from 'react';
import { GitBranch, ArrowRight, KeyRound, Type } from 'lucide-react';
import { SectionNavigationCallbacks } from '@/types/formSection';
import { CommitInputHandle } from '@/components/common/CommitInput';
import { LabelSelectorAutocomplete } from '@/components/forms/LabelSelectorAutocomplete';
import { CardGridShell, TEAL_SCHEME } from '@/components/forms/CardPrimitives';
import { classNames } from '@/utils/helpers';

interface DependenciesSectionProps {
  dependencies: Array<Record<string, string>>;
  onChange: (dependencies: Array<Record<string, string>>) => void;
  isCollapsed: boolean;
  onToggleCollapse: () => void;
  navigationCallbacks: SectionNavigationCallbacks;
  hideHeader?: boolean;
}

interface EditState {
  /** null = not editing; deps.length = adding new */
  index: number | null;
  key: string;
  value: string;
}

export const DependenciesSection = forwardRef<HTMLButtonElement, DependenciesSectionProps>(({
  dependencies,
  onChange,
}, ref) => {
  const [edit, setEdit] = useState<EditState>({ index: null, key: '', value: '' });
  const autocompleteRef = useRef<CommitInputHandle>(null);

  const startAdd = useCallback(() =>
    setEdit({ index: dependencies.length, key: '', value: '' }), [dependencies.length]);

  const startEdit = useCallback((i: number) => {
    const [k, v] = Object.entries(dependencies[i])[0] ?? ['', ''];
    setEdit({ index: i, key: k, value: v });
  }, [dependencies]);

  const handleSave = useCallback(() => {
    const latest = (autocompleteRef.current as any)?.getValues?.() ?? { key: edit.key, value: edit.value };
    const k = (latest.key ?? '').trim();
    if (!k) { setEdit({ index: null, key: '', value: '' }); return; }
    const next = [...dependencies];
    const entry = { [k]: latest.value ?? '' };
    if (edit.index !== null && edit.index < dependencies.length) {
      next[edit.index] = entry;
    } else {
      next.push(entry);
    }
    onChange(next);
    setEdit({ index: null, key: '', value: '' });
  }, [edit, dependencies, onChange]);

  const handleCancel = useCallback(() =>
    setEdit({ index: null, key: '', value: '' }), []);

  const handleDelete = useCallback((i: number) => {
    onChange(dependencies.filter((_, idx) => idx !== i));
    if (edit.index === i) setEdit({ index: null, key: '', value: '' });
  }, [dependencies, onChange, edit.index]);

  return (
    <div ref={ref as any}>
      <CardGridShell
        scheme={TEAL_SCHEME}
        BgIcon={GitBranch}
        count={dependencies.length}
        editingIndex={edit.index}
        addLabel="Add dep"
        onAdd={startAdd}
        onClickItem={startEdit}
        onDeleteItem={handleDelete}
        onSave={handleSave}
        onCancel={handleCancel}
        renderItemHeaderLeft={(i) => {
          const [k] = Object.entries(dependencies[i])[0] ?? [''];
          return (
            <>
              <KeyRound className={classNames('w-2.5 h-2.5 flex-shrink-0', TEAL_SCHEME.iconColor)} />
              <span className="text-[11px] font-semibold text-gray-600 dark:text-gray-300 truncate leading-none font-mono">{k}</span>
            </>
          );
        }}
        renderItemBody={(i) => {
          const [, v] = Object.entries(dependencies[i])[0] ?? ['', ''];
          return (
            <div className="flex items-center gap-1">
              <Type className={classNames('w-2.5 h-2.5 flex-shrink-0 opacity-70', TEAL_SCHEME.iconColor)} />
              <span className="font-mono text-xs text-teal-700 dark:text-teal-300 truncate">
                {v || <span className="italic text-teal-400 dark:text-teal-600">any</span>}
              </span>
            </div>
          );
        }}
        renderFormHeader={(isNew) => (
          <>
            <GitBranch className="w-2.5 h-2.5 text-teal-400" />
            <span className="text-[10px] text-teal-500 dark:text-teal-400 font-medium">
              {isNew ? 'New dep' : 'Edit dep'}
            </span>
          </>
        )}
        renderFormContent={() => (
          <LabelSelectorAutocomplete
            ref={autocompleteRef}
            keyValue={edit.key}
            valuValue={edit.value}
            onKeyChange={k => setEdit(s => ({ ...s, key: k }))}
            onValueChange={v => setEdit(s => ({ ...s, value: v }))}
            onRemove={handleCancel}
            onLeftKey={handleCancel}
          />
        )}
        footerNote="Click a card to edit · hover to delete"
      />
    </div>
  );
});

DependenciesSection.displayName = 'DependenciesSection';
