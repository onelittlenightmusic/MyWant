/**
 * Shared visual face for want-type cards.
 * Renders: background + overlay + centered category icon + display name.
 *
 * Used by:
 *  - WantTypeCard  (theme="light", adds bottom bar via children)
 *  - WantCanvas    (theme="dark",  adds status bar / drag chrome via children)
 */
import React from 'react';
import { classNames } from '@/utils/helpers';
import {
  getCategoryIcon,
  getCardBackgroundStyle,
  getCardOverlayBg,
} from './WantTypeVisuals';

export interface WantCardFaceProps {
  typeName: string;
  displayName: string;
  category: string;
  /** 'dark' = canvas tiles (dark mode); 'light' = light mode or sidebar */
  theme: 'dark' | 'light';
  /** 'canvas' enables pastel gradients in light mode; 'sidebar' keeps plain white */
  context?: 'canvas' | 'sidebar';
  iconSize?: number;
  // Container passthrough
  className?: string;
  style?: React.CSSProperties;
  tabIndex?: number;
  divRef?: React.Ref<HTMLDivElement>;
  onClick?: React.MouseEventHandler<HTMLDivElement>;
  onContextMenu?: React.MouseEventHandler<HTMLDivElement>;
  draggable?: boolean;
  onDragStart?: React.DragEventHandler<HTMLDivElement>;
  onDragEnd?: React.DragEventHandler<HTMLDivElement>;
  /** Extra overlay content (status bar, bottom bar, selected glow, etc.) */
  children?: React.ReactNode;
  /** data-* attributes forwarded to the outer div */
  dataAttributes?: Record<string, string | boolean>;
}

export const WantCardFace: React.FC<WantCardFaceProps> = ({
  typeName,
  displayName,
  category,
  theme,
  context = 'sidebar',
  iconSize = 26,
  className,
  style,
  tabIndex,
  divRef,
  onClick,
  onContextMenu,
  draggable,
  onDragStart,
  onDragEnd,
  children,
  dataAttributes,
}) => {
  const CatIcon = getCategoryIcon(category);
  const bgStyle = getCardBackgroundStyle(typeName, category, theme, context);
  const overlayBg = getCardOverlayBg(typeName, theme, context);

  const isLight = theme === 'light';

  return (
    <div
      ref={divRef}
      className={classNames(
        'relative overflow-hidden',
        // Light theme base (matches original WantTypeCard)
        isLight && 'bg-white bg-opacity-70 dark:bg-gray-800 dark:bg-opacity-80',
        className,
      )}
      style={{ ...bgStyle, ...style }}
      tabIndex={tabIndex}
      onClick={onClick}
      onContextMenu={onContextMenu}
      draggable={draggable}
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      {...dataAttributes}
    >
      {/* Overlay between background and content */}
      <div
        className="absolute inset-0 pointer-events-none z-0"
        style={{ background: overlayBg }}
      />

      {/* Centered content: icon + name */}
      <div className="absolute inset-0 z-10 flex flex-col items-center justify-center gap-1.5 px-2">
        <CatIcon
          size={iconSize}
          style={{
            color: isLight ? undefined : 'rgba(255,255,255,0.9)',
            filter: 'drop-shadow(0 2px 4px rgba(0,0,0,0.5))',
            flexShrink: 0,
          }}
          className={isLight ? 'text-gray-600 dark:text-gray-300' : undefined}
        />
        <p
          className={classNames(
            'font-semibold text-center leading-tight',
            isLight
              ? 'text-gray-800 dark:text-gray-100'
              : 'text-white',
          )}
          style={{
            fontSize: 9,
            overflow: 'hidden',
            display: '-webkit-box',
            WebkitLineClamp: 2,
            WebkitBoxOrient: 'vertical' as const,
            textShadow: isLight ? undefined : '0 1px 3px rgba(0,0,0,0.7)',
            maxWidth: '100%',
          }}
        >
          {displayName}
        </p>
      </div>

      {/* Caller-supplied overlays (status bar, bottom bar, selected glow, etc.) */}
      {children}
    </div>
  );
};
