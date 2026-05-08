import React from 'react';

interface ProgressBarsProps {
  achievingPercentage: number;
}

export const ProgressBars: React.FC<ProgressBarsProps> = ({ achievingPercentage }) => (
  <>
    {/* Solid base fill — ensures the card is never transparent regardless of
        what percentage value arrives or when the background image is loading. */}
    <div style={{
      position: 'absolute', top: 0, left: 0, height: '100%', width: '100%',
      background: 'var(--progress-base)',
      zIndex: 0, pointerEvents: 'none',
    }} />
    <div style={{
      position: 'absolute', top: 0, left: 0, height: '100%', width: `${achievingPercentage}%`,
      background: 'var(--progress-bg-light)', transition: 'width 0.3s ease-out',
      zIndex: 0, pointerEvents: 'none',
    }} />
    <div style={{
      position: 'absolute', top: 0, left: `${achievingPercentage}%`, height: '100%', width: `${100 - achievingPercentage}%`,
      background: 'var(--progress-bg-dark)', transition: 'width 0.3s ease-out, left 0.3s ease-out',
      zIndex: 0, pointerEvents: 'none',
    }} />
  </>
);
