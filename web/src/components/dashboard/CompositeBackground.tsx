import React from 'react';
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

  return (
    <div
      className={classNames(
        'absolute inset-0 flex',
        className
      )}
      style={{ zIndex: 0 }}
    >
      {childImages.map((img, index) => {
        // Calculate clip-path for diagonal cuts
        let clipPath = '';
        const isFirst = index === 0;
        const isLast = index === childImages.length - 1;

        if (isFirst && childImages.length === 1) {
          // Single image, no clip needed
          clipPath = 'none';
        } else if (isFirst) {
          // First segment: right edge angled (top-right to bottom-left diagonal)
          clipPath = 'polygon(0 0, 100% 0, 95% 100%, 0 100%)';
        } else if (isLast) {
          // Last segment: left edge angled
          clipPath = 'polygon(5% 0, 100% 0, 100% 100%, 0 100%)';
        } else {
          // Middle segments: both edges angled
          clipPath = 'polygon(5% 0, 100% 0, 95% 100%, 0 100%)';
        }

        return (
          <div
            key={index}
            className="relative flex-1"
            style={{
              backgroundImage: `url(${img})`,
              backgroundSize: 'cover',
              backgroundPosition: 'center',
              backgroundRepeat: 'no-repeat',
              clipPath: clipPath
            }}
          />
        );
      })}
    </div>
  );
};

export default React.memo(CompositeBackground, (prevProps, nextProps) => {
  return prevProps.children === nextProps.children;
});
