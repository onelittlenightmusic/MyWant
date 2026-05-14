/**
 * Shared card UI primitives used by ParameterGridSection, LabelsSection,
 * DependenciesSection, ExposeSection, SchedulingSection.
 */
import React from 'react';
import { Plus, X, Check, LucideIcon } from 'lucide-react';
import { classNames } from '@/utils/helpers';

// ── Color scheme ──────────────────────────────────────────────────────────────

export interface CardScheme {
  cardBg: string;
  cardHover: string;
  bgIconColor: string;
  iconColor: string;
  formBorder: string;
  formBg: string;
  saveColor: string;
  addBorder: string;
  addIcon: string;
}

export const BLUE_SCHEME: CardScheme = {
  cardBg:      'bg-blue-50/60 dark:bg-blue-900/15',
  cardHover:   'hover:shadow hover:bg-blue-100/60 dark:hover:bg-blue-900/25',
  bgIconColor: 'text-blue-300 dark:text-blue-700',
  iconColor:   'text-blue-500 dark:text-blue-400',
  formBorder:  'border-blue-300 dark:border-blue-600',
  formBg:      'bg-blue-50/30 dark:bg-blue-900/10',
  saveColor:   'text-blue-600 dark:text-blue-400 hover:bg-blue-50 dark:hover:bg-blue-900/30',
  addBorder:   'border-blue-200 dark:border-blue-800/50 hover:border-blue-400 dark:hover:border-blue-600',
  addIcon:     'text-blue-300 dark:text-blue-700 group-hover:text-blue-500 dark:group-hover:text-blue-400',
};

export const GREEN_SCHEME: CardScheme = {
  cardBg:      'bg-green-50/70 dark:bg-green-900/15',
  cardHover:   'hover:shadow hover:bg-green-100/70 dark:hover:bg-green-900/25',
  bgIconColor: 'text-green-400 dark:text-green-600',
  iconColor:   'text-green-500 dark:text-green-400',
  formBorder:  'border-green-300 dark:border-green-600',
  formBg:      'bg-green-50/30 dark:bg-green-900/10',
  saveColor:   'text-green-600 dark:text-green-400 hover:bg-green-50 dark:hover:bg-green-900/30',
  addBorder:   'border-green-200 dark:border-green-800/50 hover:border-green-400 dark:hover:border-green-600',
  addIcon:     'text-green-300 dark:text-green-700 group-hover:text-green-500 dark:group-hover:text-green-400',
};

export const TEAL_SCHEME: CardScheme = {
  cardBg:      'bg-teal-50/70 dark:bg-teal-900/15',
  cardHover:   'hover:shadow hover:bg-teal-100/70 dark:hover:bg-teal-900/25',
  bgIconColor: 'text-teal-400 dark:text-teal-600',
  iconColor:   'text-teal-500 dark:text-teal-400',
  formBorder:  'border-teal-300 dark:border-teal-600',
  formBg:      'bg-teal-50/30 dark:bg-teal-900/10',
  saveColor:   'text-teal-600 dark:text-teal-400 hover:bg-teal-50 dark:hover:bg-teal-900/30',
  addBorder:   'border-teal-200 dark:border-teal-800/50 hover:border-teal-400 dark:hover:border-teal-600',
  addIcon:     'text-teal-300 dark:text-teal-700 group-hover:text-teal-500 dark:group-hover:text-teal-400',
};

export const PURPLE_SCHEME: CardScheme = {
  cardBg:      'bg-purple-50/70 dark:bg-purple-900/15',
  cardHover:   'hover:shadow hover:bg-purple-100/70 dark:hover:bg-purple-900/25',
  bgIconColor: 'text-purple-400 dark:text-purple-600',
  iconColor:   'text-purple-500 dark:text-purple-400',
  formBorder:  'border-purple-300 dark:border-purple-600',
  formBg:      'bg-purple-50/30 dark:bg-purple-900/10',
  saveColor:   'text-purple-600 dark:text-purple-400 hover:bg-purple-50 dark:hover:bg-purple-900/30',
  addBorder:   'border-purple-200 dark:border-purple-800/50 hover:border-purple-400 dark:hover:border-purple-600',
  addIcon:     'text-purple-300 dark:text-purple-700 group-hover:text-purple-500 dark:group-hover:text-purple-400',
};

export const AMBER_SCHEME: CardScheme = {
  cardBg:      'bg-amber-50/70 dark:bg-amber-900/15',
  cardHover:   'hover:shadow hover:bg-amber-100/70 dark:hover:bg-amber-900/25',
  bgIconColor: 'text-amber-400 dark:text-amber-600',
  iconColor:   'text-amber-500 dark:text-amber-400',
  formBorder:  'border-amber-300 dark:border-amber-600',
  formBg:      'bg-amber-50/30 dark:bg-amber-900/10',
  saveColor:   'text-amber-600 dark:text-amber-400 hover:bg-amber-50 dark:hover:bg-amber-900/30',
  addBorder:   'border-amber-200 dark:border-amber-800/50 hover:border-amber-400 dark:hover:border-amber-600',
  addIcon:     'text-amber-300 dark:text-amber-700 group-hover:text-amber-500 dark:group-hover:text-amber-400',
};

// ── Confirm / Cancel icon buttons ────────────────────────────────────────

export const CardFormButtons: React.FC<{
  onSave: () => void;
  onCancel: () => void;
  saveDisabled?: boolean;
  saveColorClass?: string;
}> = ({ onSave, onCancel, saveDisabled, saveColorClass = BLUE_SCHEME.saveColor }) => (
  <div className="flex gap-1 mt-1.5">
    <button
      type="button"
      onClick={onSave}
      disabled={saveDisabled}
      title="Save"
      className={classNames(
        'w-6 h-6 flex items-center justify-center rounded transition-colors',
        'disabled:opacity-40 disabled:cursor-not-allowed',
        saveColorClass,
      )}
    >
      <Check className="w-3.5 h-3.5" />
    </button>
    <button
      type="button"
      onClick={onCancel}
      title="Cancel"
      className="w-6 h-6 flex items-center justify-center rounded text-gray-400 dark:text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-700/50 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
    >
      <X className="w-3.5 h-3.5" />
    </button>
  </div>
);

// ── Display Card ──────────────────────────────────────────────────────────────

interface DisplayCardProps {
  /** Full outer className — caller computes state-based classes (focused, modified, etc.) */
  className: string;
  BgIcon?: LucideIcon;
  bgIconColor?: string;
  /** Set false to suppress the bg icon (e.g. when focused) */
  showBgIcon?: boolean;
  /** Renders a thin glow bar at the bottom */
  showFocusBar?: boolean;
  onClick?: () => void;
  /** Left slot: type icon + name */
  headerLeft: React.ReactNode;
  /** Right slot: badges, toggles, copy/delete buttons */
  headerRight?: React.ReactNode;
  children?: React.ReactNode;
}

export const DisplayCard: React.FC<DisplayCardProps> = ({
  className,
  BgIcon,
  bgIconColor = '',
  showBgIcon = true,
  showFocusBar,
  onClick,
  headerLeft,
  headerRight,
  children,
}) => (
  <div onClick={onClick} className={className}>
    {BgIcon && showBgIcon && (
      <div className="absolute inset-0 overflow-hidden rounded-xl pointer-events-none">
        <BgIcon className={classNames('absolute bottom-1 right-1.5 w-10 h-10 opacity-[0.12]', bgIconColor)} />
      </div>
    )}
    <div className="flex items-center justify-between mb-1.5">
      <div className="flex items-center gap-1 min-w-0">{headerLeft}</div>
      {headerRight && (
        <div className="flex items-center gap-0.5 ml-1 flex-shrink-0">{headerRight}</div>
      )}
    </div>
    {children}
    {showFocusBar && (
      <div className="absolute bottom-0 left-1/4 right-1/4 h-0.5 rounded-full bg-blue-400 dark:bg-blue-500" />
    )}
  </div>
);

// ── Add Card (dashed placeholder button) ─────────────────────────────────────

export const AddCard: React.FC<{
  borderClass: string;
  iconClass: string;
  label: string;
  onClick: () => void;
}> = ({ borderClass, iconClass, label, onClick }) => (
  <button
    type="button"
    onClick={onClick}
    className={classNames(
      'flex flex-col items-center justify-center gap-1 rounded-xl border-2 border-dashed',
      'transition-colors group min-h-[5rem] bg-transparent',
      borderClass,
    )}
  >
    <Plus className={classNames('w-4 h-4 transition-colors', iconClass)} />
    <span className={classNames('text-[9px] transition-colors', iconClass)}>{label}</span>
  </button>
);

// ── Form Card (dashed, inline add / edit form) ────────────────────────────────

export const FormCard: React.FC<{
  borderClass: string;
  bgClass: string;
  header: React.ReactNode;
  onSave: () => void;
  onCancel: () => void;
  saveDisabled?: boolean;
  saveColorClass?: string;
  colSpan2?: boolean;
  children: React.ReactNode;
}> = ({ borderClass, bgClass, header, onSave, onCancel, saveDisabled, saveColorClass, colSpan2, children }) => (
  <div className={classNames('rounded-xl border-2 border-dashed p-2.5', borderClass, bgClass, colSpan2 ? 'col-span-2' : '')}>
    <div className="flex items-center gap-1 mb-1.5">{header}</div>
    {children}
    <CardFormButtons
      onSave={onSave}
      onCancel={onCancel}
      saveDisabled={saveDisabled}
      saveColorClass={saveColorClass}
    />
  </div>
);

// ── Card Grid Shell ───────────────────────────────────────────────────────────
// Shared outer shell for label / dep / expose / schedule card grids.
// Handles: 2-col grid, display cards with hover-delete, add card, inline form card.

export interface CardGridShellProps {
  scheme: CardScheme;
  BgIcon: LucideIcon;
  /** Total number of existing items */
  count: number;
  /**
   * Which item is being edited.
   * - null  → not editing
   * - 0..count-1 → editing existing item at that index
   * - count → adding a new item
   */
  editingIndex: number | null;
  addLabel: string;
  onAdd: () => void;
  /** Called when the user clicks an existing item card (optional — if absent, cards are not clickable) */
  onClickItem?: (i: number) => void;
  onDeleteItem: (i: number) => void;
  onSave: () => void;
  onCancel: () => void;
  saveDisabled?: boolean;
  /** Header left content for each display card (icon + name) */
  renderItemHeaderLeft: (i: number) => React.ReactNode;
  /** Body content for each display card */
  renderItemBody?: (i: number) => React.ReactNode;
  /** Icon + title text inside the FormCard header */
  renderFormHeader: (isNew: boolean) => React.ReactNode;
  /** The actual form inputs */
  renderFormContent: () => React.ReactNode;
  /** Optional extra element shown when list is empty and not editing */
  emptyState?: React.ReactNode;
  /** Footer note below the grid */
  footerNote?: string;
}

export const CardGridShell: React.FC<CardGridShellProps> = ({
  scheme,
  BgIcon,
  count,
  editingIndex,
  addLabel,
  onAdd,
  onClickItem,
  onDeleteItem,
  onSave,
  onCancel,
  saveDisabled,
  renderItemHeaderLeft,
  renderItemBody,
  renderFormHeader,
  renderFormContent,
  emptyState,
  footerNote,
}) => {
  const isAdding = editingIndex === count;

  return (
    <div className="space-y-3 pt-1">
      {emptyState && count === 0 && editingIndex === null && emptyState}

      <div className="grid grid-cols-2 gap-2">
        {Array.from({ length: count }, (_, i) => {
          if (editingIndex === i) {
            return (
              <FormCard
                key={i}
                borderClass={scheme.formBorder}
                bgClass={scheme.formBg}
                saveColorClass={scheme.saveColor}
                header={renderFormHeader(false)}
                onSave={onSave}
                onCancel={onCancel}
                saveDisabled={saveDisabled}
              >
                {renderFormContent()}
              </FormCard>
            );
          }
          return (
            <DisplayCard
              key={i}
              className={classNames(
                'relative rounded-xl p-2.5 shadow-sm transition-all duration-150 group',
                scheme.cardBg, scheme.cardHover,
                onClickItem ? 'cursor-pointer' : '',
              )}
              BgIcon={BgIcon}
              bgIconColor={scheme.bgIconColor}
              onClick={onClickItem ? () => onClickItem(i) : undefined}
              headerLeft={renderItemHeaderLeft(i)}
              headerRight={
                <button
                  type="button"
                  onClick={e => { e.stopPropagation(); onDeleteItem(i); }}
                  className="opacity-0 group-hover:opacity-100 w-4 h-4 flex items-center justify-center text-gray-300 dark:text-gray-600 hover:text-red-400 dark:hover:text-red-500 transition-all"
                  title="Delete"
                >
                  <X className="w-2.5 h-2.5" />
                </button>
              }
            >
              {renderItemBody?.(i)}
            </DisplayCard>
          );
        })}

        {isAdding ? (
          <FormCard
            borderClass={scheme.formBorder}
            bgClass={scheme.formBg}
            saveColorClass={scheme.saveColor}
            header={renderFormHeader(true)}
            onSave={onSave}
            onCancel={onCancel}
            saveDisabled={saveDisabled}
          >
            {renderFormContent()}
          </FormCard>
        ) : (
          <AddCard
            borderClass={scheme.addBorder}
            iconClass={scheme.addIcon}
            label={addLabel}
            onClick={onAdd}
          />
        )}
      </div>

      {footerNote && (
        <p className="text-[10px] text-gray-400 dark:text-gray-500 pt-1">{footerNote}</p>
      )}
    </div>
  );
};
