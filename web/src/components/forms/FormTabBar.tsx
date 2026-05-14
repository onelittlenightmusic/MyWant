import React from 'react';
import { SlidersHorizontal, Clock, Tag, Link2, BadgeInfo, ArrowUpFromLine } from 'lucide-react';
import { classNames } from '@/utils/helpers';

export type FormTab = 'name' | 'params' | 'labels' | 'schedule' | 'deps' | 'expose';
export const FORM_TABS: FormTab[] = ['name', 'params', 'labels', 'schedule', 'deps', 'expose'];

const TagLinkIcon: React.FC<{ className?: string }> = ({ className }) => (
  <span className="relative inline-flex items-center justify-center" style={{ width: '1em', height: '1em' }}>
    <Tag className={className} />
    <Link2 className="absolute -bottom-1 -right-1 w-2 h-2" />
  </span>
);

export const FORM_TAB_META: Record<FormTab, { label: string; icon: React.ElementType }> = {
  name:     { label: 'Name',     icon: BadgeInfo },
  params:   { label: 'Params',   icon: SlidersHorizontal },
  labels:   { label: 'Labels',   icon: Tag      },
  schedule: { label: 'Schedule', icon: Clock    },
  deps:     { label: 'Deps',     icon: TagLinkIcon },
  expose:   { label: 'Expose',   icon: ArrowUpFromLine },
};

export function FormTabBar({
  activeTab,
  onTabChange,
  badges,
  isBottom,
}: {
  activeTab: FormTab | 'add';
  onTabChange: (tab: FormTab) => void;
  badges: Record<FormTab, string | number | null>;
  isBottom: boolean;
}) {
  return (
    <div className={classNames(
      'flex flex-shrink-0 border-gray-200 dark:border-gray-700 bg-gray-50/50 dark:bg-gray-900/50',
      isBottom ? 'border-t' : 'border-b'
    )}>
      {FORM_TABS.map(tab => {
        const { label, icon: Icon } = FORM_TAB_META[tab];
        const badge = badges[tab];
        const isActive = activeTab !== 'add' && activeTab === tab;
        const isRequired = typeof badge === 'string' && badge.startsWith('★');
        return (
          <button
            key={tab}
            type="button"
            onClick={() => onTabChange(tab)}
            className={classNames(
              'flex-1 flex flex-col items-center justify-center py-1 sm:py-2 px-0.5 transition-all relative min-w-0',
              isActive
                ? 'text-blue-600 dark:text-blue-400 bg-white dark:bg-gray-800 shadow-sm'
                : 'text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-white/50 dark:hover:bg-gray-800/30'
            )}
          >
            <div className="relative">
              <Icon className="h-3.5 w-3.5 flex-shrink-0" />
              {badge !== null && (
                <span className={classNames(
                  'absolute -top-1.5 -right-2 min-w-[14px] h-[14px] flex items-center justify-center rounded-full text-[8px] leading-none font-bold px-0.5',
                  isRequired ? 'bg-red-500 text-white' : 'bg-blue-500 text-white'
                )}>
                  {badge}
                </span>
              )}
            </div>
            <span className="text-[9px] sm:text-[10px] font-bold uppercase tracking-tighter truncate w-full text-center mt-0.5">
              {label}
            </span>
            {isActive && (
              <div className={classNames(
                'absolute h-0.5 bg-blue-600 dark:bg-blue-400 left-0 right-0',
                isBottom ? 'top-0' : 'bottom-0'
              )} />
            )}
          </button>
        );
      })}
    </div>
  );
}
