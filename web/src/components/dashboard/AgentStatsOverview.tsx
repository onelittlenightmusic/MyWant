import React from 'react';
import { Bot, Zap, Eye, Target, Brain } from 'lucide-react';
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
  <div className="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-6">
    <div className="flex items-center">
      <div className={`flex-shrink-0 p-3 rounded-full ${color}`}>
        <div className="text-xl">
          {icon}
        </div>
      </div>
      <div className="ml-4">
        <p className="text-sm font-medium text-gray-600 dark:text-gray-400">{title}</p>
        <p className="text-3xl font-bold text-gray-900 dark:text-gray-100">{value}</p>
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
        {[...Array(5)].map((_, i) => (
          <div key={i} className="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-6">
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
    thinkAgents: agents.filter(a => a.type === 'think').length,
    totalCapabilities: agents.reduce((acc, agent) => acc + agent.capabilities.length, 0)
  };

  const statCards = [
    {
      title: 'Total Agents',
      value: stats.total,
      color: 'bg-blue-100 dark:bg-blue-900/30',
      icon: <Bot className="h-6 w-6 text-blue-600 dark:text-blue-400" />
    },
    {
      title: 'Do Agents',
      value: stats.doAgents,
      color: 'bg-green-100 dark:bg-green-900/30',
      icon: <Zap className="h-6 w-6 text-green-600 dark:text-green-400" />
    },
    {
      title: 'Monitor Agents',
      value: stats.monitorAgents,
      color: 'bg-teal-100 dark:bg-teal-900/30',
      icon: <Eye className="h-6 w-6 text-teal-600 dark:text-teal-400" />
    },
    {
      title: 'Think Agents',
      value: stats.thinkAgents,
      color: 'bg-purple-100 dark:bg-purple-900/30',
      icon: <Brain className="h-6 w-6 text-purple-600 dark:text-purple-400" />
    },
    {
      title: 'Total Capabilities',
      value: stats.totalCapabilities,
      color: 'bg-orange-100 dark:bg-orange-900/30',
      icon: <Target className="h-6 w-6 text-orange-600 dark:text-orange-400" />
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
