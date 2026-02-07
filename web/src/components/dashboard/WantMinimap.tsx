import React from 'react';
import { Want } from '@/types/want';
import { DraftWant } from '@/types/draft';
import { classNames } from '@/utils/helpers';
import { getBackgroundStyle } from '@/utils/backgroundStyles';

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
  const backgroundStyle = getBackgroundStyle(want.metadata?.type, false);

  const minimapCardStyle = {
    ...backgroundStyle.style,
    height: '40px',
    minHeight: '40px'
  };

  const minimapCardClassName = classNames(
    'rounded border cursor-pointer transition-all duration-200',
    'hover:border-blue-500 hover:shadow-md',
    isSelected ? 'border-blue-500 border-2' : 'border-gray-300',
    backgroundStyle.className
  );

  return (
    <div
      className={minimapCardClassName}
      style={minimapCardStyle}
      onClick={onClick}
      title={want.metadata?.name || want.metadata?.id || want.id}
    />
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
