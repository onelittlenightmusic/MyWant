import React from 'react';
import { Eye, MoreHorizontal, Zap, Settings, Database, Share2, Plane, Calculator, Layers, CheckCircle, Monitor, Tag } from 'lucide-react';
import { WantTypeListItem } from '@/types/wantType';
import { truncateText, classNames } from '@/utils/helpers';
import { getBackgroundStyle, getBackgroundOverlayClass } from '@/utils/backgroundStyles';

interface WantTypeCardProps {
  wantType: WantTypeListItem;
  selected?: boolean;
  onView: (wantType: WantTypeListItem) => void;
  className?: string;
}

const patternIcons: Record<string, React.ReactNode> = {
  generator: <Zap className="h-4 w-4" />,
  processor: <Settings className="h-4 w-4" />,
  sink: <Database className="h-4 w-4" />,
  coordinator: <Share2 className="h-4 w-4" />,
  independent: <Zap className="h-4 w-4" />,
};

const patternColors: Record<string, string> = {
  generator: 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300',
  processor: 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300',
  sink: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300',
  coordinator: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300',
  independent: 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300',
};

const categoryColors: Record<string, string> = {
  travel: 'bg-blue-50 text-blue-700 dark:bg-blue-900/20 dark:text-blue-300',
  mathematics: 'bg-purple-50 text-purple-700 dark:bg-purple-900/20 dark:text-purple-300',
  queue: 'bg-green-50 text-green-700 dark:bg-green-900/20 dark:text-green-300',
  approval: 'bg-orange-50 text-orange-700 dark:bg-orange-900/20 dark:text-orange-300',
  system: 'bg-gray-50 text-gray-700 dark:bg-gray-800 dark:text-gray-300',
  math: 'bg-purple-50 text-purple-700 dark:bg-purple-900/20 dark:text-purple-300',
};

const categoryIcons: Record<string, React.ReactNode> = {
  travel: <Plane className="h-3 w-3 sm:h-3.5 sm:w-3.5" />,
  mathematics: <Calculator className="h-3 w-3 sm:h-3.5 sm:w-3.5" />,
  math: <Calculator className="h-3 w-3 sm:h-3.5 sm:w-3.5" />,
  queue: <Layers className="h-3 w-3 sm:h-3.5 sm:w-3.5" />,
  approval: <CheckCircle className="h-3 w-3 sm:h-3.5 sm:w-3.5" />,
  system: <Monitor className="h-3 w-3 sm:h-3.5 sm:w-3.5" />,
};

export const WantTypeCard: React.FC<WantTypeCardProps> = ({
  wantType,
  selected = false,
  onView,
  className
}) => {
  const cardRef = React.useRef<HTMLDivElement>(null);

  // Focus the card when it's targeted by keyboard navigation
  React.useEffect(() => {
    if (selected && document.activeElement !== cardRef.current) {
      cardRef.current?.focus();
    }
  }, [selected]);

  const handleCardClick = (e: React.MouseEvent) => {
    const target = e.target as HTMLElement;

    // Don't trigger if clicking on interactive elements
    if (target.closest('button') || target.closest('[role="button"]')) {
      return;
    }

    onView(wantType);

    // Smooth scroll the card into view after selection
    requestAnimationFrame(() => {
      setTimeout(() => {
        const selectedElement = document.querySelector('[data-keyboard-nav-selected="true"]');
        if (selectedElement && selectedElement instanceof HTMLElement) {
          selectedElement.scrollIntoView({ behavior: 'smooth', block: 'center' });
        }
      }, 0);
    });
  };

  // Get background style for want type card (using wantType.name for image lookup)
  const backgroundStyle = getBackgroundStyle(wantType.name, true);

  return (
    <div
      ref={cardRef}
      onClick={handleCardClick}
      tabIndex={0}
      data-keyboard-nav-selected={selected}
      data-keyboard-nav-id={wantType.name}
      className={classNames(
        'card hover:shadow-md dark:hover:shadow-blue-900/20 transition-all duration-300 cursor-pointer group relative overflow-hidden focus:outline-none focus:ring-2 focus:ring-blue-400 dark:focus:ring-blue-500 focus:ring-inset h-full flex flex-col min-h-[6rem] sm:min-h-[10rem]',
        selected ? 'border-blue-500 border-2 shadow-lg scale-[1.02] z-10' : 'border-gray-200 dark:border-gray-700',
        className || ''
      )}
      style={backgroundStyle.style}
    >
      {/* Overlay - semi-transparent background */}
      <div className={getBackgroundOverlayClass()}></div>

      {/* Content Area */}
      <div className="relative z-10 px-3 sm:px-6 pb-3 pt-3 order-1 flex-1">
        <p className="text-[10px] sm:text-xs text-gray-500 dark:text-gray-400 mt-1 truncate">
          {wantType.name}
        </p>
      </div>

      {/* Header (Title Area) - Moved to bottom to match WantCard */}
      <div className="relative z-20 order-2 mt-auto">
        <div className={classNames(
          "backdrop-blur-[2px] transition-colors duration-200 px-3 sm:px-6 py-1.5 flex items-center justify-between",
          selected ? "bg-blue-100/90 dark:bg-blue-900/70" : "bg-white/60 dark:bg-gray-900/70"
        )}>
          <div className="flex-1 min-w-0">
            <h3
              className="text-[9px] sm:text-[13px] font-semibold text-gray-900 dark:text-gray-100 truncate group-hover:text-primary-600 dark:group-hover:text-primary-400 transition-colors cursor-pointer flex items-center gap-1.5"
              onClick={(e) => { e.stopPropagation(); onView(wantType); }}
            >
              <Zap className="h-2 w-2 sm:h-3.5 sm:w-3.5 flex-shrink-0 text-yellow-500" />
              {truncateText(wantType.title, 30)}
            </h3>
          </div>

          <div className="flex items-center space-x-1 sm:space-x-2 ml-1 sm:ml-2 flex-shrink-0">
            {/* Pattern badge */}
            <span className={classNames('inline-flex items-center gap-1 px-1.5 sm:px-2 py-0.5 sm:py-1 rounded-full text-[8px] sm:text-[10px] font-medium', patternColors[wantType.pattern] || 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-300')}>
              {patternIcons[wantType.pattern]}
              <span className="capitalize hidden sm:inline">{wantType.pattern}</span>
            </span>

            {/* Category badge */}
            <span
              className={classNames('inline-flex items-center px-1.5 sm:px-2 py-0.5 sm:py-1 rounded-full', categoryColors[wantType.category] || 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-300')}
              title={wantType.category}
            >
              {categoryIcons[wantType.category] || <Tag className="h-3 w-3 sm:h-3.5 sm:w-3.5" />}
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
    </div>
  );
};
