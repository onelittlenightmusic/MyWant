import React from 'react';
import { Eye, MoreHorizontal, Zap, Settings, Database, Share2 } from 'lucide-react';
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
  generator: 'bg-blue-100 text-blue-800',
  processor: 'bg-purple-100 text-purple-800',
  sink: 'bg-red-100 text-red-800',
  coordinator: 'bg-green-100 text-green-800',
  independent: 'bg-amber-100 text-amber-800',
};

const categoryColors: Record<string, string> = {
  travel: 'bg-blue-50 text-blue-700',
  mathematics: 'bg-purple-50 text-purple-700',
  queue: 'bg-green-50 text-green-700',
  approval: 'bg-orange-50 text-orange-700',
  system: 'bg-gray-50 text-gray-700',
  math: 'bg-purple-50 text-purple-700',
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
        'card hover:shadow-md transition-shadow duration-200 cursor-pointer group relative overflow-hidden focus:outline-none focus:ring-2 focus:ring-blue-400 focus:ring-inset',
        selected ? 'border-blue-500 border-2' : 'border-gray-200',
        className || ''
      )}
      style={backgroundStyle.style}
    >
      {/* Overlay - semi-transparent background */}
      <div className={getBackgroundOverlayClass()}></div>

      {/* Card content */}
      <div className="relative z-10">
      {/* Header */}
      <div className="flex items-start justify-between mb-4">
        <div className="flex-1 min-w-0">
          <h3
            className="text-lg font-semibold text-gray-900 truncate group-hover:text-primary-600 transition-colors cursor-pointer"
            onClick={() => onView(wantType)}
          >
            {truncateText(wantType.title, 30)}
          </h3>
          <p className="text-sm text-gray-500 mt-1">
            {wantType.name}
          </p>
          {wantType.version && (
            <p className="text-xs text-gray-400 mt-1">
              Version: {wantType.version}
            </p>
          )}
        </div>

        <div className="flex items-center space-x-2 flex-shrink-0">
          {/* Actions menu */}
          <div className="relative group/menu">
            <button className="p-1 rounded-md text-gray-400 hover:text-gray-600 hover:bg-gray-100">
              <MoreHorizontal className="h-4 w-4" />
            </button>

            <div className="absolute right-0 top-8 w-40 bg-white rounded-md shadow-lg border border-gray-200 z-10 opacity-0 invisible group-hover/menu:opacity-100 group-hover/menu:visible transition-all duration-200">
              <div className="py-1">
                <button
                  onClick={() => onView(wantType)}
                  className="flex items-center w-full px-4 py-2 text-sm text-gray-700 hover:bg-gray-100"
                >
                  <Eye className="h-4 w-4 mr-2" />
                  View Details
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Pattern and Category Stats */}
      <div className="space-y-2 mb-4">
        <div className="flex justify-between text-sm">
          <span className="text-gray-500">Pattern:</span>
          <span className={classNames('inline-flex items-center gap-1 px-2.5 py-1 rounded text-xs font-medium', patternColors[wantType.pattern] || 'bg-gray-100 text-gray-800')}>
            {patternIcons[wantType.pattern]}
            <span className="capitalize">{wantType.pattern}</span>
          </span>
        </div>
        <div className="flex justify-between text-sm">
          <span className="text-gray-500">Category:</span>
          <span className={classNames('inline-block px-2.5 py-1 rounded text-xs font-medium', categoryColors[wantType.category] || 'bg-gray-100 text-gray-800')}>
            {wantType.category}
          </span>
        </div>
      </div>

      {/* Summary */}
      <div className="pt-4 border-t border-gray-200">
        <p className="text-xs text-gray-600">
          {wantType.pattern} pattern in {wantType.category} category
        </p>
      </div>
      </div>
    </div>
  );
};
