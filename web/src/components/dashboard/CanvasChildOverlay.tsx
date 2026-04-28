import React, { useMemo } from 'react';
import { Want } from '@/types/want';
import { ChildMiniTile, ChildRole, CHILD_TILE_SIZE } from './ChildMiniTile';

const FLOAT_CARD_WIDTH = 280;
const FLOAT_CARD_HEIGHT = 220;
const CHILD_GAP = 8;
const CHILD_STEP = CHILD_TILE_SIZE + CHILD_GAP;
const ARM_GAP = 16;
const MAX_PER_SIDE = 4;

function getRole(want: Want): ChildRole {
  const r = want.metadata?.labels?.['child-role'];
  if (r === 'thinker') return 'thinker';
  if (r === 'monitor') return 'monitor';
  if (r === 'doer') return 'doer';
  return 'other';
}

interface CanvasChildOverlayProps {
  floatCard: React.ReactNode;
  childWants: Want[];
  /** Viewport coordinates of the parent tile's center */
  tileCenterX: number;
  tileCenterY: number;
  onClickChild: (want: Want) => void;
  onClose: () => void;
}

export const CanvasChildOverlay: React.FC<CanvasChildOverlayProps> = ({
  floatCard,
  childWants,
  tileCenterX,
  tileCenterY,
  onClickChild,
  onClose,
}) => {
  const groups = useMemo(() => ({
    thinkers: childWants.filter(w => getRole(w) === 'thinker').slice(0, MAX_PER_SIDE),
    monitors: childWants.filter(w => getRole(w) === 'monitor').slice(0, MAX_PER_SIDE),
    doers:    childWants.filter(w => getRole(w) === 'doer').slice(0, MAX_PER_SIDE),
    others:   childWants.filter(w => getRole(w) === 'other').slice(0, MAX_PER_SIDE),
  }), [childWants]);

  // Float card top-left, clamped to viewport
  const cardLeft = Math.max(8, Math.min(
    tileCenterX - FLOAT_CARD_WIDTH / 2,
    window.innerWidth - FLOAT_CARD_WIDTH - 8,
  ));
  const cardTop = Math.max(8, Math.min(
    tileCenterY - FLOAT_CARD_HEIGHT / 2,
    window.innerHeight - FLOAT_CARD_HEIGHT - 8,
  ));
  const cardCX = cardLeft + FLOAT_CARD_WIDTH / 2;
  const cardCY = cardTop + FLOAT_CARD_HEIGHT / 2;

  const renderGroup = (wants: Want[], role: ChildRole, direction: 'top' | 'left' | 'right' | 'bottom') => {
    if (wants.length === 0) return null;
    const n = wants.length;
    const totalLen = n * CHILD_TILE_SIZE + (n - 1) * CHILD_GAP;

    return wants.map((w, i) => {
      let left = 0;
      let top = 0;
      switch (direction) {
        case 'top':
          left = cardCX - totalLen / 2 + i * CHILD_STEP;
          top  = cardTop - ARM_GAP - CHILD_TILE_SIZE;
          break;
        case 'left':
          left = cardLeft - ARM_GAP - CHILD_TILE_SIZE;
          top  = cardCY - totalLen / 2 + i * CHILD_STEP;
          break;
        case 'right':
          left = cardLeft + FLOAT_CARD_WIDTH + ARM_GAP;
          top  = cardCY - totalLen / 2 + i * CHILD_STEP;
          break;
        case 'bottom':
          left = cardCX - totalLen / 2 + i * CHILD_STEP;
          top  = cardTop + FLOAT_CARD_HEIGHT + ARM_GAP;
          break;
      }

      return (
        <ChildMiniTile
          key={w.metadata?.id || w.id || i}
          want={w}
          role={role}
          left={left}
          top={top}
          animationDelay={i * 40}
          onClick={onClickChild}
        />
      );
    });
  };

  return (
    <div
      className="fixed inset-0"
      style={{ zIndex: 100 }}
      onClick={onClose}
    >
      {/* Backdrop blur */}
      <div className="absolute inset-0 bg-black/30 backdrop-blur-[1px]" />

      {/* Float card */}
      <div
        style={{
          position: 'fixed',
          left: cardLeft,
          top: cardTop,
          width: FLOAT_CARD_WIDTH,
          zIndex: 101,
        }}
        onClick={e => e.stopPropagation()}
      >
        {floatCard}
      </div>

      {/* Child tiles by role */}
      {renderGroup(groups.thinkers, 'thinker', 'top')}
      {renderGroup(groups.monitors, 'monitor', 'left')}
      {renderGroup(groups.doers,    'doer',    'right')}
      {renderGroup(groups.others,   'other',   'bottom')}
    </div>
  );
};
