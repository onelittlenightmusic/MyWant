import React from 'react';
import { Bot, CheckCircle2, AlertCircle } from 'lucide-react';
import { Recommendation } from '@/types/interact';
import { classNames } from '@/utils/helpers';

interface RecommendationSelectorProps {
  recommendations: Recommendation[];
  selectedId: string | null;
  onSelect: (recommendation: Recommendation) => void;
}

export const RecommendationSelector: React.FC<RecommendationSelectorProps> = ({
  recommendations,
  selectedId,
  onSelect
}) => {
  const getComplexityColor = (complexity: string) => {
    switch (complexity) {
      case 'low': return 'text-green-600';
      case 'medium': return 'text-yellow-600';
      case 'high': return 'text-red-600';
      default: return 'text-gray-600';
    }
  };

  const getApproachBadge = (approach: string) => {
    const colors = {
      recipe: 'bg-green-100 text-green-800 border-green-300',
      custom: 'bg-blue-100 text-blue-800 border-blue-300',
      hybrid: 'bg-purple-100 text-purple-800 border-purple-300'
    };
    return colors[approach as keyof typeof colors] || colors.custom;
  };

  // Safety check
  if (!recommendations || !Array.isArray(recommendations) || recommendations.length === 0) {
    return (
      <div className="space-y-3">
        <div className="flex items-center gap-2 pb-2 border-b border-gray-200">
          <div className="flex items-center justify-center h-8 w-8 rounded-full bg-blue-600">
            <Bot className="h-5 w-5 text-white" />
          </div>
          <h3 className="text-lg font-semibold text-gray-900">
            AI Recommendations
          </h3>
        </div>
        <div className="text-center py-8 text-gray-500">
          No recommendations available
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {/* Header with robot icon */}
      <div className="flex items-center gap-2 pb-2 border-b border-gray-200">
        <div className="flex items-center justify-center h-8 w-8 rounded-full bg-blue-600">
          <Bot className="h-5 w-5 text-white" />
        </div>
        <h3 className="text-lg font-semibold text-gray-900">
          AI Recommendations
        </h3>
      </div>

      {/* Recommendation cards */}
      <div className="space-y-2 max-h-96 overflow-y-auto">
        {recommendations.map((rec, index) => (
          <button
            key={rec.id}
            onClick={() => onSelect(rec)}
            className={classNames(
              'w-full text-left p-4 rounded-lg border-2 transition-all',
              'hover:shadow-md focus:outline-none focus:ring-2 focus:ring-blue-500',
              selectedId === rec.id
                ? 'border-blue-500 bg-blue-50'
                : 'border-gray-200 hover:border-blue-300'
            )}
          >
            {/* Title row */}
            <div className="flex items-start justify-between gap-2 mb-2">
              <div className="flex items-center gap-2">
                <span className="font-semibold text-gray-900">
                  [{index + 1}] {rec.title}
                </span>
                <span className={classNames(
                  'text-xs px-2 py-0.5 rounded-full border',
                  getApproachBadge(rec.approach)
                )}>
                  {rec.approach}
                </span>
              </div>
              {selectedId === rec.id && (
                <CheckCircle2 className="h-5 w-5 text-blue-600 flex-shrink-0" />
              )}
            </div>

            {/* Description */}
            <p className="text-sm text-gray-600 mb-3">
              {rec.description}
            </p>

            {/* Metadata row */}
            <div className="flex items-center gap-4 text-xs text-gray-500 mb-2">
              <span>Wants: {rec.metadata?.want_count ?? 0}</span>
              <span className={getComplexityColor(rec.metadata?.complexity ?? 'medium')}>
                Complexity: {rec.metadata?.complexity ?? 'medium'}
              </span>
              {rec.metadata?.recipes_used && rec.metadata.recipes_used.length > 0 && (
                <span>Recipes: {rec.metadata.recipes_used.join(', ')}</span>
              )}
            </div>

            {/* Pros/Cons */}
            {rec.metadata?.pros_cons && (rec.metadata.pros_cons.pros?.length > 0 || rec.metadata.pros_cons.cons?.length > 0) && (
              <div className="mt-3 space-y-1">
                {rec.metadata.pros_cons.pros && rec.metadata.pros_cons.pros.length > 0 && (
                  <div className="flex items-start gap-2 text-xs">
                    <CheckCircle2 className="h-3 w-3 text-green-600 mt-0.5 flex-shrink-0" />
                    <span className="text-gray-700">
                      {rec.metadata.pros_cons.pros.join(', ')}
                    </span>
                  </div>
                )}
                {rec.metadata.pros_cons.cons && rec.metadata.pros_cons.cons.length > 0 && (
                  <div className="flex items-start gap-2 text-xs">
                    <AlertCircle className="h-3 w-3 text-yellow-600 mt-0.5 flex-shrink-0" />
                    <span className="text-gray-700">
                      {rec.metadata.pros_cons.cons.join(', ')}
                    </span>
                  </div>
                )}
              </div>
            )}
          </button>
        ))}
      </div>
    </div>
  );
};
