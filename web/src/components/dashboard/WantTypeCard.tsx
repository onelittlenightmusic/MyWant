import React from 'react';
import { Eye, MoreHorizontal, Zap } from 'lucide-react';
import { WantTypeListItem } from '@/types/wantType';
import { truncateText, classNames } from '@/utils/helpers';
import {
  getPatternIcon,
  getPatternBadgeClass,
  getCategoryBadgeClass,
  getCategoryIcon,
} from './WantTypeVisuals';
import { WantCardFace } from './WantCardFace';

interface WantTypeCardProps {
  wantType: WantTypeListItem;
  selected?: boolean;
  onView: (wantType: WantTypeListItem) => void;
  className?: string;
}

export const WantTypeCard: React.FC<WantTypeCardProps> = ({
  wantType,
  selected = false,
  onView,
  className,
}) => {
  const cardRef = React.useRef<HTMLDivElement>(null);

  React.useEffect(() => {
    if (selected && document.activeElement !== cardRef.current) {
      cardRef.current?.focus();
    }
  }, [selected]);

  const handleCardClick = (e: React.MouseEvent) => {
    const target = e.target as HTMLElement;
    if (target.closest('button') || target.closest('[role="button"]')) return;
    onView(wantType);
    requestAnimationFrame(() => {
      setTimeout(() => {
        const el = document.querySelector('[data-keyboard-nav-selected="true"]');
        if (el instanceof HTMLElement) el.scrollIntoView({ behavior: 'smooth', block: 'center' });
      }, 0);
    });
  };

  const PatIcon = getPatternIcon(wantType.pattern);
  const CatIcon = getCategoryIcon(wantType.category);

  return (
    <WantCardFace
      divRef={cardRef}
      typeName={wantType.name}
      displayName={wantType.title}
      category={wantType.category}
      theme="light"
      iconSize={28}
      onClick={handleCardClick}
      tabIndex={0}
      style={{ WebkitUserSelect: 'none', WebkitTouchCallout: 'none' } as React.CSSProperties}
      className={classNames(
        'card hover:shadow-md dark:hover:shadow-blue-900/20 transition-all duration-300 cursor-pointer group h-full flex flex-col min-h-[6rem] sm:min-h-[10rem] focus:outline-none focus:ring-2 focus:ring-blue-400 dark:focus:ring-blue-500 focus:ring-inset',
        selected ? 'border-blue-500 border-2 shadow-lg scale-[1.02] z-10' : 'border-gray-200 dark:border-gray-700',
        className ?? '',
      )}
      dataAttributes={{
        'data-keyboard-nav-selected': selected,
        'data-keyboard-nav-id': wantType.name,
      }}
    >
      {/* Bottom bar (title + badges) — same as original WantTypeCard */}
      <div className="absolute bottom-0 left-0 right-0 z-20 mt-auto select-none">
        <div className={classNames(
          'backdrop-blur-[2px] transition-colors duration-200 px-3 sm:px-6 py-1.5 flex items-center justify-between',
          selected ? 'bg-blue-100/90 dark:bg-blue-900/70' : 'bg-white/60 dark:bg-gray-900/70',
        )}>
          <div className="flex-1 min-w-0">
            <h3
              className="text-[9px] sm:text-[13px] font-semibold text-gray-900 dark:text-gray-100 truncate group-hover:text-primary-600 dark:group-hover:text-primary-400 transition-colors cursor-pointer flex items-center gap-1.5 select-none"
              onClick={(e) => { e.stopPropagation(); onView(wantType); }}
            >
              <Zap className="h-2 w-2 sm:h-3.5 sm:w-3.5 flex-shrink-0 text-yellow-500" />
              {truncateText(wantType.title, 30)}
            </h3>
          </div>

          <div className="flex items-center space-x-1 sm:space-x-2 ml-1 sm:ml-2 flex-shrink-0">
            {/* Pattern badge */}
            <span className={classNames(
              'inline-flex items-center gap-1 px-1.5 sm:px-2 py-0.5 sm:py-1 rounded-full text-[8px] sm:text-[10px] font-medium',
              getPatternBadgeClass(wantType.pattern),
            )}>
              <PatIcon className="h-3 w-3 sm:h-3.5 sm:w-3.5" />
              <span className="capitalize hidden sm:inline">{wantType.pattern}</span>
            </span>

            {/* Category badge */}
            <span
              className={classNames(
                'inline-flex items-center px-1.5 sm:px-2 py-0.5 sm:py-1 rounded-full',
                getCategoryBadgeClass(wantType.category),
              )}
              title={wantType.category}
            >
              <CatIcon className="h-3 w-3 sm:h-3.5 sm:w-3.5" />
            </span>

            {/* Actions menu */}
            <div className="relative group/menu">
              <button
                className="p-1 rounded-md text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-800"
                onClick={(e) => e.stopPropagation()}
              >
                <MoreHorizontal className="h-3.5 w-3.5" />
              </button>

              <div className="absolute right-0 bottom-full mb-2 w-40 bg-white dark:bg-gray-800 rounded-md shadow-lg border border-gray-200 dark:border-gray-700 z-10 opacity-0 invisible group-hover/menu:opacity-100 group-hover/menu:visible transition-all duration-200">
                <div className="py-1">
                  <button
                    onClick={(e) => { e.stopPropagation(); onView(wantType); }}
                    className="flex items-center w-full px-4 py-2 text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
                  >
                    <Eye className="h-4 w-4 mr-2" />
                    View Details
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </WantCardFace>
  );
};
