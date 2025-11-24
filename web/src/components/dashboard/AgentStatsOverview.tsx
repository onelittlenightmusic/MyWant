import React from 'react';
import { Bot, Zap, Eye, Target } from 'lucide-react';
import { AgentResponse } from '@/types/agent';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';

interface AgentStatsOverviewProps {
  agents: AgentResponse[];
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

export const AgentStatsOverview: React.FC<AgentStatsOverviewProps> = ({ agents, loading, layout = 'vertical' }) => {
  const gridClass = layout === 'vertical'
    ? 'flex flex-col gap-3'
    : 'grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6';

  if (loading && agents.length === 0) {
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
    total: agents.length,
    doAgents: agents.filter(a => a.type === 'do').length,
    monitorAgents: agents.filter(a => a.type === 'monitor').length,
    totalCapabilities: agents.reduce((acc, agent) => acc + agent.capabilities.length, 0)
  };

  const statCards = [
    {
      title: 'Total Agents',
      value: stats.total,
      color: 'bg-blue-100',
      icon: <Bot className="h-6 w-6 text-blue-600" />
    },
    {
      title: 'Do Agents',
      value: stats.doAgents,
      color: 'bg-green-100',
      icon: <Zap className="h-6 w-6 text-green-600" />
    },
    {
      title: 'Monitor Agents',
      value: stats.monitorAgents,
      color: 'bg-purple-100',
      icon: <Eye className="h-6 w-6 text-purple-600" />
    },
    {
      title: 'Total Capabilities',
      value: stats.totalCapabilities,
      color: 'bg-orange-100',
      icon: <Target className="h-6 w-6 text-orange-600" />
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
