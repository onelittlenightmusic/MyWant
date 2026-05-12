import React from 'react';

interface ProgressBarsProps {
  achievingPercentage: number;
}

export const ProgressBars: React.FC<ProgressBarsProps> = ({ achievingPercentage }) => {
  const isAchieved = achievingPercentage >= 100;
  return (
    <div 
      className="absolute top-0 left-0 right-0 h-1 z-[50] pointer-events-none bg-gray-200/30 dark:bg-gray-700/30 overflow-hidden"
    >
      <div 
        className={`h-full transition-all duration-500 ease-out ${isAchieved ? 'bg-green-500 shadow-[0_0_8px_rgba(34,197,94,0.6)]' : 'bg-blue-500 shadow-[0_0_8px_rgba(59,130,246,0.6)]'}`}
        style={{ 
          width: `${achievingPercentage}%`,
        }} 
      />
    </div>
  );
};
