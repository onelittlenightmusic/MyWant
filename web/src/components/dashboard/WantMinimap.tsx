import React, { useState, useCallback } from 'react';
import { Want, WantExecutionStatus } from '@/types/want';
import { isDraftWant } from '@/types/draft';
import { classNames } from '@/utils/helpers';
import { getBackgroundStyle } from '@/utils/backgroundStyles';
import { useConfigStore } from '@/stores/configStore';
import styles from './WantCard.module.css';

import { getStatusHexColor } from './WantCard/parts/StatusColor';

/**
 * Get a light overlay color for a want status (used in minimap)
 */
const getStatusOverlayColor = (status: WantExecutionStatus): string => {
  const hex = getStatusHexColor(status);
  // Convert hex to rgba for overlay
  if (hex.startsWith('#')) {
    const r = parseInt(hex.slice(1, 3), 16);
    const g = parseInt(hex.slice(3, 5), 16);
    const b = parseInt(hex.slice(5, 7), 16);
    return `rgba(${r}, ${g}, ${b}, 0.25)`;
  }
  return 'transparent';
};

interface WantMinimapProps {
  wants: Want[]; // Parent cards (filteredWants)
  drafts: Want[]; // Draft cards
  selectedWantId?: string;
  onWantClick: (wantId: string) => void;
  onWantDoubleClick?: (wantId: string) => void;
  onDraftClick: (draftId: string) => void;
  isOpen: boolean; // Mobile toggle control
}

interface MinimapCardProps {
  want: Want;
  isSelected: boolean;
  onClick: () => void;
  onDoubleClick?: () => void;
}

interface MinimapDraftCardProps {
  want: Want;
  isSelected: boolean;
  onClick: () => void;
}

/**
 * Miniature version of a regular Want card
 */
const MinimapCard: React.FC<MinimapCardProps> = ({ want, isSelected, onClick, onDoubleClick }) => {
  const [isBlinking, setIsBlinking] = useState(false);
  const backgroundStyle = getBackgroundStyle(want.metadata?.type, false);
  const statusOverlay = getStatusOverlayColor(want.status);

  const handleClick = useCallback(() => {
    setIsBlinking(false);
    // Force re-trigger by toggling off then on in next frame
    requestAnimationFrame(() => {
      setIsBlinking(true);
    });
    onClick();
  }, [onClick]);

  const minimapCardStyle = {
    ...backgroundStyle.style,
    height: '40px',
    minHeight: '40px'
  };

  const minimapCardClassName = classNames(
    'relative rounded border cursor-pointer transition-all duration-200 overflow-hidden',
    'hover:border-blue-500 hover:shadow-md dark:hover:border-blue-400',
    isSelected ? 'border-blue-500 border-2 dark:border-blue-400' : 'border-gray-300 dark:border-gray-700',
    isBlinking && styles.minimapBlink,
    backgroundStyle.className
  );

  const dotColor = getStatusHexColor(want.status);
  const isPulsing = want.status === 'reaching' || want.status === 'reaching_with_warning' || want.status === 'waiting_user_action';

  return (
    <div
      className={minimapCardClassName}
      style={minimapCardStyle}
      onClick={handleClick}
      onDoubleClick={onDoubleClick}
      onAnimationEnd={() => setIsBlinking(false)}
      title={want.metadata?.name || want.metadata?.id || want.id}
    >
      <div
        className="absolute inset-0 pointer-events-none"
        style={{ backgroundColor: statusOverlay }}
      />
      <div
        className={classNames(
          'absolute top-1 right-1 w-2 h-2 rounded-full z-10',
          isPulsing && styles.pulseGlow
        )}
        style={{ backgroundColor: dotColor }}
        title={want.status}
      />
    </div>
  );
};

/**
 * Miniature version of a Draft Want card
 */
const MinimapDraftCard: React.FC<MinimapDraftCardProps> = ({ want, isSelected, onClick }) => {
  const current = want.state?.current || {};
  const phase = (current.phase as string) || '';
  const isThinking = (current.isThinking as boolean) ||
    phase === 'ideating' || phase === 'decomposing' || phase === 're_planning';
  const message = (current.goal_text as string) || (current.message as string) || want.metadata?.name || 'Draft Want';

  return (
    <div
      className={classNames(
        'rounded border-2 border-dashed cursor-pointer transition-all duration-200',
        'bg-gray-100 hover:bg-gray-200 dark:bg-gray-800 dark:hover:bg-gray-700',
        'hover:border-blue-400 dark:hover:border-blue-300',
        isSelected ? 'border-blue-500 bg-blue-50 dark:border-blue-400 dark:bg-blue-900/30' : 'border-gray-400 dark:border-gray-600',
        isThinking && 'animate-pulse',
      )}
      style={{ height: '40px', minHeight: '40px' }}
      onClick={onClick}
      title={message}
    />
  );
};

/**
 * WantMinimap Component
 * Displays miniature versions of Want cards and Draft cards
 * Fixed position on the right side, matches WantGrid layout (3 columns)
 */
export const WantMinimap: React.FC<WantMinimapProps> = ({
  wants,
  drafts,
  selectedWantId,
  onWantClick,
  onWantDoubleClick,
  onDraftClick,
  isOpen
}) => {
  const config = useConfigStore(state => state.config);
  const isHeaderBottom = config?.header_position === 'bottom';

  return (
    <div
      className={classNames(
        "fixed right-0 w-full sm:w-[480px] bg-gray-50 border-l border-gray-200 p-4 overflow-hidden transition-transform duration-300 dark:bg-gray-950 dark:border-gray-800",
        "lg:translate-x-0", // Desktop: always visible
        isOpen ? "translate-x-0" : "translate-x-full lg:translate-x-0", // Mobile: toggle
        "z-30" // Below RightSidebar (z-40)
      )}
      style={{
        top: isHeaderBottom ? 'env(safe-area-inset-top, 0px)' : 'calc(env(safe-area-inset-top, 0px) + 4rem)',
        bottom: 'env(safe-area-inset-bottom, 0px)',
        height: 'auto'
      }}
    >
      <div className="grid grid-cols-3 gap-2 auto-rows-min">
        {/* Parent cards (filteredWants) */}
        {wants.map(want => {
          const wantId = want.metadata?.id || want.id || '';
          return (
            <MinimapCard
              key={wantId}
              want={want}
              isSelected={selectedWantId === wantId}
              onClick={() => onWantClick(wantId)}
              onDoubleClick={onWantDoubleClick ? () => onWantDoubleClick(wantId) : undefined}
            />
          );
        })}

        {/* Draft cards */}
        {drafts.map(draft => {
          const draftId = draft.metadata?.id || draft.id || '';
          return (
            <MinimapDraftCard
              key={draftId}
              want={draft}
              isSelected={selectedWantId === draftId}
              onClick={() => onDraftClick(draftId)}
            />
          );
        })}

        {/* Add Want button placeholder (for layout consistency with WantGrid) */}
        <div
          className="rounded border border-dashed border-gray-300 bg-gray-100 dark:border-gray-700 dark:bg-gray-800 opacity-50"
          style={{ height: '40px' }}
          title="Add Want (placeholder)"
        />
      </div>
    </div>
  );
};
