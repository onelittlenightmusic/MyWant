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
      {childImages.map((img, index) => (
        <div
          key={index}
          className="relative flex-1"
          style={{
            backgroundImage: `url(${img})`,
            backgroundSize: 'cover',
            backgroundPosition: 'center',
            backgroundRepeat: 'no-repeat'
          }}
        >
          {/* Diagonal divider - only show if not last element */}
          {index < childImages.length - 1 && (
            <div
              className="absolute inset-y-0 right-0"
              style={{
                width: '5px',
                background: 'rgba(0, 0, 0, 0.7)',
                right: '-2px',
                transform: 'skewX(-22deg)',
                transformOrigin: 'top right',
                boxShadow: '2px 0 4px rgba(0,0,0,0.5)'
              }}
            />
          )}
        </div>
      ))}
    </div>
  );
};

export default React.memo(CompositeBackground, (prevProps, nextProps) => {
  return prevProps.children === nextProps.children;
});
