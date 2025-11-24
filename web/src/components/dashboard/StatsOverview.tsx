import React from 'react';
import { ClipboardList, Play, CheckCircle, AlertCircle } from 'lucide-react';
import { Want } from '@/types/want';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';

interface StatsOverviewProps {
  wants: Want[];
  loading: boolean;
  layout?: 'grid' | 'vertical';
}

interface StatCardProps {
  title: string;
  value: number;
  color: string;
  icon: React.ReactNode;
}

const StatCard: React.FC<StatCardProps> = ({ title, value, color, icon }) => (
  <div className="bg-white rounded-lg border border-gray-200 p-6">
    <div className="flex items-center">
      <div className={`flex-shrink-0 p-3 rounded-full ${color}`}>
        <div className="text-xl">
          {icon}
        </div>
      </div>
      <div className="ml-4">
        <p className="text-sm font-medium text-gray-600">{title}</p>
        <p className="text-3xl font-bold text-gray-900">{value}</p>
      </div>
    </div>
  </div>
);

export const StatsOverview: React.FC<StatsOverviewProps> = ({ wants, loading, layout = 'grid' }) => {
  const gridClass = layout === 'vertical'
    ? 'flex flex-col gap-3'
    : 'grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6';

  if (loading && wants.length === 0) {
    return (
      <div className={gridClass}>
        {[...Array(4)].map((_, i) => (
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
    total: wants.length,
    running: wants.filter(w => w.status === 'reaching').length,
    completed: wants.filter(w => w.status === 'completed').length,
    failed: wants.filter(w => w.status === 'failed').length,
  };

  const statCards = [
    {
      title: 'Total Wants',
      value: stats.total,
      color: 'bg-blue-100',
      icon: <ClipboardList className="h-6 w-6 text-blue-600" />
    },
    {
      title: 'Running',
      value: stats.running,
      color: 'bg-green-100',
      icon: <Play className="h-6 w-6 text-green-600" />
    },
    {
      title: 'Completed',
      value: stats.completed,
      color: 'bg-green-100',
      icon: <CheckCircle className="h-6 w-6 text-green-600" />
    },
    {
      title: 'Failed',
      value: stats.failed,
      color: 'bg-red-100',
      icon: <AlertCircle className="h-6 w-6 text-red-600" />
    }
  ];

  return (
    <div className={gridClass}>
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