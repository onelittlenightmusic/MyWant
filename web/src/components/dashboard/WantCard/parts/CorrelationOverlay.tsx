import React from 'react';
import styles from '../../WantCard.module.css';

interface CorrelationOverlayProps {
  rate?: number;
}

function buildCorrelationOverlayVars(rate: number): React.CSSProperties {
  // Static orange overlay with opacity based on rate
  const alpha = Math.min(0.12 + rate * 0.1, 0.45);
  return { backgroundColor: `rgba(249, 115, 22, ${alpha})` } as React.CSSProperties;
}

export const CorrelationOverlay: React.FC<CorrelationOverlayProps> = ({ rate }) => {
  if (!rate || rate <= 0) return null;
  return <div className={styles.correlationOverlay} style={buildCorrelationOverlayVars(rate)} />;
};
