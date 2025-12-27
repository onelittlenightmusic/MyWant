import React, { useId } from 'react';
import { Want } from '@/types/want';
import { getBackgroundImage } from '@/utils/backgroundStyles';
import { classNames } from '@/utils/helpers';

interface CompositeBackgroundProps {
  children: Want[];
  className?: string;
}

export const CompositeBackground: React.FC<CompositeBackgroundProps> = ({
  children,
  className
}) => {
  // Extract child background images, filtering out undefined values
  const childImages = children
    .map(child => getBackgroundImage(child.metadata?.type))
    .filter((img): img is string => img !== undefined);

  // Return null if no valid images found
  if (childImages.length === 0) {
    return null;
  }

  // Generate unique clip path IDs using React's useId hook
  const baseId = useId();
  const clipIds = childImages.map((_, index) => `${baseId}-clip-${index}`);

  return (
    <>
      {/* Hidden SVG definitions for clip paths */}
      <svg width="0" height="0" style={{ position: 'absolute' }}>
        <defs>
          {clipIds.map((clipId, index) => {
            const isFirst = index === 0;
            const isLast = index === childImages.length - 1;
            const isSingle = childImages.length === 1;

            // Calculate polygon points in objectBoundingBox units (0-1 range)
            let points = '';
            if (isSingle) {
              // No clipping needed for single image
              points = '0,0 1,0 1,1 0,1';
            } else if (isFirst) {
              // First segment: right edge angled (top-right to bottom-left)
              // Top-left, Top-right, Bottom-right (95% right), Bottom-left
              points = '0,0 1,0 0.95,1 0,1';
            } else if (isLast) {
              // Last segment: left edge angled
              // Top-left (5% from left), Top-right, Bottom-right, Bottom-left
              points = '0.05,0 1,0 1,1 0,1';
            } else {
              // Middle segments: both edges angled
              // Top-left (5%), Top-right, Bottom-right (95%), Bottom-left
              points = '0.05,0 1,0 0.95,1 0,1';
            }

            return (
              <clipPath
                key={clipId}
                id={clipId}
                clipPathUnits="objectBoundingBox"
              >
                <polygon points={points} />
              </clipPath>
            );
          })}
        </defs>
      </svg>

      {/* Background segments with SVG clipPath applied */}
      <div
        className={classNames(
          'absolute inset-0 flex',
          className
        )}
        style={{ zIndex: 0 }}
      >
        {childImages.map((img, index) => {
          const clipPathUrl = `url(#${clipIds[index]})`;
          return (
            <div
              key={index}
              className="relative flex-1"
              style={{
                clipPath: clipPathUrl,
                WebkitClipPath: clipPathUrl,
                overflow: 'hidden'
              }}
            >
              <img
                src={img}
                alt=""
                style={{
                  width: '100%',
                  height: '100%',
                  objectFit: 'cover',
                  objectPosition: 'center',
                  display: 'block'
                }}
              />
            </div>
          );
        })}
      </div>
    </>
  );
};

export default React.memo(CompositeBackground, (prevProps, nextProps) => {
  return prevProps.children === nextProps.children;
});
