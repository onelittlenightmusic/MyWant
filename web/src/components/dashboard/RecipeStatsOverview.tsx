import React from 'react';
import { BookOpen, Settings } from 'lucide-react';
import { GenericRecipe } from '@/types/recipe';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';

interface RecipeStatsOverviewProps {
  recipes: GenericRecipe[];
  loading: boolean;
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

export const RecipeStatsOverview: React.FC<RecipeStatsOverviewProps> = ({ recipes, loading }) => {
  if (loading && recipes.length === 0) {
    return (
      <div className="flex flex-col gap-3">
        {[...Array(2)].map((_, i) => (
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
    total: recipes.length,
    parameters: recipes.reduce((acc, recipe) => acc + Object.keys(recipe.recipe.parameters || {}).length, 0)
  };

  const statCards = [
    {
      title: 'Total Recipes',
      value: stats.total,
      color: 'bg-blue-100',
      icon: <BookOpen className="h-6 w-6 text-blue-600" />
    },
    {
      title: 'Total Parameters',
      value: stats.parameters,
      color: 'bg-purple-100',
      icon: <Settings className="h-6 w-6 text-purple-600" />
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
