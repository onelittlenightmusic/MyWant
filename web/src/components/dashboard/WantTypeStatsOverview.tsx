import React from 'react';
import { WantTypeListItem } from '@/types/wantType';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';

interface WantTypeStatsOverviewProps {
  wantTypes: WantTypeListItem[];
  loading: boolean;
}

interface StatCardProps {
  title: string;
  value: number;
  color: string;
  icon: string;
}

const StatCard: React.FC<StatCardProps> = ({ title, value, color, icon }) => (
  <div className="bg-white rounded-lg border border-gray-200 p-6">
    <div className="flex items-center">
      <div className={`flex-shrink-0 p-3 rounded-full ${color}`}>
        <span className="text-2xl">{icon}</span>
      </div>
      <div className="ml-4">
        <p className="text-sm font-medium text-gray-600">{title}</p>
        <p className="text-3xl font-bold text-gray-900">{value}</p>
      </div>
    </div>
  </div>
);

export const WantTypeStatsOverview: React.FC<WantTypeStatsOverviewProps> = ({ wantTypes, loading }) => {
  if (loading && wantTypes.length === 0) {
    return (
      <div className="flex flex-col gap-3">
        {[...Array(1)].map((_, i) => (
          <div key={i} className="bg-white rounded-lg border border-gray-200 p-6">
            <div className="flex items-center justify-center h-16">
              <LoadingSpinner size="md" />
            </div>
          </div>
        ))}
      </div>
    );
  }

  const stats = {
    total: wantTypes.length
  };

  const statCards = [
    {
      title: 'Total Types',
      value: stats.total,
      color: 'bg-purple-100',
      icon: '⚡'
    }
  ];

  return (
    <div className="flex flex-col gap-3">
      {statCards.map((stat, index) => (
        <StatCard
          key={index}
          title={stat.title}
          value={stat.value}
          color={stat.color}
          icon={stat.icon}
        />
      ))}
    </div>
  );
};
