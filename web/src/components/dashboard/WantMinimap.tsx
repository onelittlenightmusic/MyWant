import React, { useState, useCallback } from 'react';
import { Want, WantExecutionStatus } from '@/types/want';
import { DraftWant } from '@/types/draft';
import { classNames } from '@/utils/helpers';
import { getBackgroundStyle } from '@/utils/backgroundStyles';
import styles from './WantCard.module.css';

/**
 * Get solid color for a want status dot (matches WantCard.getStatusColor)
 */
const getStatusDotColor = (status: WantExecutionStatus): string => {
  switch (status) {
    case 'achieved':
      return '#10b981'; // Green
    case 'reaching':
    case 'terminated':
      return '#9333ea'; // Purple
    case 'failed':
    case 'module_error':
      return '#ef4444'; // Red
    case 'config_error':
    case 'stopped':
    case 'waiting_user_action':
      return '#f59e0b'; // Amber/Yellow
    default:
      return '#d1d5db'; // Gray
  }
};

/**
 * Get a light overlay color for a want status (used in minimap)
 */
const getStatusOverlayColor = (status: WantExecutionStatus): string => {
  switch (status) {
    case 'achieved':
      return 'rgba(16, 185, 129, 0.25)';   // Green
    case 'reaching':
      return 'rgba(147, 51, 234, 0.25)';   // Purple
    case 'failed':
    case 'module_error':
      return 'rgba(239, 68, 68, 0.25)';    // Red
    case 'config_error':
    case 'stopped':
    case 'waiting_user_action':
      return 'rgba(245, 158, 11, 0.25)';   // Amber/Yellow
    case 'terminated':
      return 'rgba(107, 114, 128, 0.25)';  // Gray
    default:
      return 'transparent';
  }
};

interface WantMinimapProps {
  wants: Want[]; // Parent cards (filteredWants)
  drafts: DraftWant[]; // Draft cards
  selectedWantId?: string;
  activeDraftId?: string | null;
  onWantClick: (wantId: string) => void;
  onDraftClick: (draftId: string) => void;
  isOpen: boolean; // Mobile toggle control
}

interface MinimapCardProps {
  want: Want;
  isSelected: boolean;
  onClick: () => void;
}

interface MinimapDraftCardProps {
  draft: DraftWant;
  isSelected: boolean;
  onClick: () => void;
}

/**
 * Miniature version of a regular Want card
 */
const MinimapCard: React.FC<MinimapCardProps> = ({ want, isSelected, onClick }) => {
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
    'hover:border-blue-500 hover:shadow-md',
    isSelected ? 'border-blue-500 border-2' : 'border-gray-300',
    isBlinking && styles.minimapBlink,
    backgroundStyle.className
  );

  const dotColor = getStatusDotColor(want.status);
  const isPulsing = want.status === 'reaching' || want.status === 'waiting_user_action';

  return (
    <div
      className={minimapCardClassName}
      style={minimapCardStyle}
      onClick={handleClick}
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
const MinimapDraftCard: React.FC<MinimapDraftCardProps> = ({ draft, isSelected, onClick }) => {
  const minimapDraftCardClassName = classNames(
    'rounded border-2 border-dashed cursor-pointer transition-all duration-200',
    'bg-gray-100 hover:bg-gray-200',
    'hover:border-blue-400',
    isSelected ? 'border-blue-500 bg-blue-50' : 'border-gray-400',
    draft.isThinking && 'animate-pulse' // Thinking state animation
  );

  const minimapDraftCardStyle = {
    height: '40px',
    minHeight: '40px'
  };

  return (
    <div
      className={minimapDraftCardClassName}
      style={minimapDraftCardStyle}
      onClick={onClick}
      title={draft.message || 'Draft Want'}
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
  activeDraftId,
  onWantClick,
  onDraftClick,
  isOpen
}) => {
  return (
    <div
      className={classNames(
        "fixed top-16 right-0 w-[480px] h-[calc(100vh-4rem)] bg-gray-50 border-l border-gray-200 p-4 overflow-hidden transition-transform duration-300",
        "lg:translate-x-0", // Desktop: always visible
        isOpen ? "translate-x-0" : "translate-x-full lg:translate-x-0", // Mobile: toggle
        "z-30" // Below RightSidebar (z-40)
      )}
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
            />
          );
        })}

        {/* Draft cards */}
        {drafts.map(draft => (
          <MinimapDraftCard
            key={draft.id}
            draft={draft}
            isSelected={activeDraftId === draft.id}
            onClick={() => onDraftClick(draft.id)}
          />
        ))}

        {/* Add Want button placeholder (for layout consistency with WantGrid) */}
        <div
          className="rounded border border-dashed border-gray-300 bg-gray-100 opacity-50"
          style={{ height: '40px' }}
          title="Add Want (placeholder)"
        />
      </div>
    </div>
  );
};
