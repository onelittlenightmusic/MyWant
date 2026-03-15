import React from 'react';
import { classNames } from '@/utils/helpers';

interface StackLayersProps {
  stackCount: number;
  isChild?: boolean;
}

export const StackLayers: React.FC<StackLayersProps> = ({ stackCount, isChild = false }) => {
  if (stackCount <= 0) return null;
  
  return (
    <>
      {Array.from({ length: stackCount }).map((_, i) => (
        <div
          key={i}
          className={classNames(
            "absolute inset-0 border border-gray-400 dark:border-gray-500 bg-gray-300 dark:bg-gray-600",
            isChild ? "rounded-md" : "rounded-lg"
          )}
          style={{
            transform: `translate(${(stackCount - i) * 6}px, ${(stackCount - i) * 6}px)`,
            zIndex: -(stackCount - i),
            opacity: 0.9 - i * 0.15,
          }}
        />
      ))}
    </>
  );
};
