import React from 'react';
import { Eye, Edit, Trash2, MoreHorizontal, Bot, Monitor, Zap, Settings } from 'lucide-react';
import { Agent } from '@/types/agent';
import { truncateText, classNames } from '@/utils/helpers';

interface AgentCardProps {
  agent: Agent;
  selected?: boolean;
  onView: (agent: Agent) => void;
  onEdit: (agent: Agent) => void;
  onDelete: (agent: Agent) => void;
  className?: string;
}

export const AgentCard: React.FC<AgentCardProps> = ({
  agent,
  selected = false,
  onView,
  onEdit,
  onDelete,
  className
}) => {
  const agentName = agent.name || 'Unnamed Agent';
  const agentType = agent.type || 'unknown';
  const capabilities = agent.capabilities || [];
  const uses = agent.uses || [];

  const getTypeIcon = () => {
    switch (agentType) {
      case 'do':
        return <Zap className="h-4 w-4" />;
      case 'monitor':
        return <Monitor className="h-4 w-4" />;
      default:
        return <Bot className="h-4 w-4" />;
    }
  };

  const getTypeColor = () => {
    switch (agentType) {
      case 'do':
        return 'bg-blue-100 text-blue-800';
      case 'monitor':
        return 'bg-green-100 text-green-800';
      default:
        return 'bg-gray-100 text-gray-800';
    }
  };

  return (
    <div className={classNames(
      'card hover:shadow-md transition-shadow duration-200 cursor-pointer group',
      selected ? 'border-blue-500 border-2' : 'border-gray-200',
      className
    )}>
      {/* Header */}
      <div className="flex items-start justify-between mb-4">
        <div className="flex-1 min-w-0">
          <h3
            className="text-lg font-semibold text-gray-900 truncate group-hover:text-primary-600 transition-colors"
            onClick={() => onView(agent)}
          >
            {truncateText(agentName, 30)}
          </h3>
          <div className="flex items-center mt-2">
            <div className={classNames(
              'inline-flex items-center px-2 py-1 rounded-full text-xs font-medium',
              getTypeColor()
            )}>
              {getTypeIcon()}
              <span className="ml-1 capitalize">{agentType}</span>
            </div>
          </div>
        </div>

        <div className="flex items-center space-x-2">
          {/* Actions dropdown */}
          <div className="relative group/menu">
            <button className="p-1 rounded-md text-gray-400 hover:text-gray-600 hover:bg-gray-100">
              <MoreHorizontal className="h-4 w-4" />
            </button>

            <div className="absolute right-0 top-8 w-48 bg-white rounded-md shadow-lg border border-gray-200 z-10 opacity-0 invisible group-hover/menu:opacity-100 group-hover/menu:visible transition-all duration-200">
              <div className="py-1">
                <button
                  onClick={() => onView(agent)}
                  className="flex items-center w-full px-4 py-2 text-sm text-gray-700 hover:bg-gray-100"
                >
                  <Eye className="h-4 w-4 mr-2" />
                  View Details
                </button>

                <button
                  onClick={() => onEdit(agent)}
                  className="flex items-center w-full px-4 py-2 text-sm text-gray-700 hover:bg-gray-100"
                >
                  <Edit className="h-4 w-4 mr-2" />
                  Edit
                </button>

                <button
                  onClick={() => {/* TODO: Implement configure */}}
                  className="flex items-center w-full px-4 py-2 text-sm text-gray-700 hover:bg-gray-100"
                >
                  <Settings className="h-4 w-4 mr-2" />
                  Configure
                </button>

                <hr className="my-1" />

                <button
                  onClick={() => onDelete(agent)}
                  className="flex items-center w-full px-4 py-2 text-sm text-red-600 hover:bg-red-50"
                >
                  <Trash2 className="h-4 w-4 mr-2" />
                  Delete
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Capabilities */}
      {capabilities.length > 0 && (
        <div className="mb-4">
          <h4 className="text-sm font-medium text-gray-700 mb-2">Capabilities</h4>
          <div className="flex flex-wrap gap-1">
            {capabilities.slice(0, 3).map((capability) => (
              <span
                key={capability}
                className="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium bg-purple-100 text-purple-800"
              >
                {capability}
              </span>
            ))}
            {capabilities.length > 3 && (
              <span className="text-xs text-gray-500">
                +{capabilities.length - 3} more
              </span>
            )}
          </div>
        </div>
      )}

      {/* Dependencies */}
      {uses.length > 0 && (
        <div className="mb-4">
          <h4 className="text-sm font-medium text-gray-700 mb-2">Uses</h4>
          <div className="flex flex-wrap gap-1">
            {uses.slice(0, 3).map((use) => (
              <span
                key={use}
                className="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium bg-orange-100 text-orange-800"
              >
                {use}
              </span>
            ))}
            {uses.length > 3 && (
              <span className="text-xs text-gray-500">
                +{uses.length - 3} more
              </span>
            )}
          </div>
        </div>
      )}

      {/* Agent Details */}
      <div className="space-y-2 text-sm text-gray-600">
        <div className="flex justify-between">
          <span>Capabilities:</span>
          <span className="font-medium">{capabilities.length}</span>
        </div>

        <div className="flex justify-between">
          <span>Dependencies:</span>
          <span className="font-medium">{uses.length}</span>
        </div>
      </div>

      {/* Status indicator */}
      <div className="mt-4 pt-4 border-t border-gray-200">
        <div className="flex items-center justify-between">
          <span className="text-sm text-gray-600">Status</span>
          <div className="flex items-center">
            <div className="w-2 h-2 rounded-full bg-green-500 mr-2" />
            <span className="text-sm text-green-600 font-medium">Active</span>
          </div>
        </div>
      </div>
    </div>
  );
};