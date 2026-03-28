import React from 'react';

interface FocusTriangleProps {
  visible: boolean;
}

export const FocusTriangle: React.FC<FocusTriangleProps> = ({ visible }) => {
  if (!visible) return null;
  return (
    <div
      className="absolute left-3 z-20 bg-blue-500 dark:bg-blue-400"
      style={{
        top: 0,
        transform: 'translateY(-100%)',
        width: '16px',
        height: '10px',
        clipPath: 'polygon(50% 100%, 0% 0%, 100% 0%)',
      }}
    />
  );
};
