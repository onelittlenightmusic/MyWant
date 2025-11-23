import React, { useMemo, useEffect } from 'react';
import { AgentResponse } from '@/types/agent';
import { AgentCard } from './AgentCard';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';

interface AgentGridProps {
  agents: AgentResponse[];
  loading: boolean;
  searchQuery: string;
  typeFilters: ('do' | 'monitor')[];
  selectedAgent?: AgentResponse | null;
  onViewAgent: (agent: AgentResponse) => void;
  onEditAgent: (agent: AgentResponse) => void;
  onDeleteAgent: (agent: AgentResponse) => void;
  onGetFilteredAgents?: (agents: AgentResponse[]) => void;
}

export const AgentGrid: React.FC<AgentGridProps> = ({
  agents,
  loading,
  searchQuery,
  typeFilters,
  selectedAgent,
  onViewAgent,
  onEditAgent,
  onDeleteAgent,
  onGetFilteredAgents
}) => {
  const filteredAgents = useMemo(() => {
    return agents.filter(agent => {
      // Search filter
      if (searchQuery) {
        const query = searchQuery.toLowerCase();
        const agentName = agent.name || '';
        const agentType = agent.type || '';
        const capabilities = agent.capabilities || [];
        const uses = agent.uses || [];

        const matchesSearch =
          agentName.toLowerCase().includes(query) ||
          agentType.toLowerCase().includes(query) ||
          capabilities.some(cap => cap.toLowerCase().includes(query)) ||
          uses.some(use => use.toLowerCase().includes(query));

        if (!matchesSearch) return false;
      }

      // Type filter
      if (typeFilters.length > 0) {
        if (!typeFilters.includes(agent.type)) return false;
      }

      return true;
    }).sort((a, b) => {
      // Sort by name to ensure consistent ordering
      const nameA = a.name || '';
      const nameB = b.name || '';
      return nameA.localeCompare(nameB);
    });
  }, [agents, searchQuery, typeFilters]);

  // Notify parent of filtered agents for keyboard navigation
  useEffect(() => {
    onGetFilteredAgents?.(filteredAgents);
  }, [filteredAgents, onGetFilteredAgents]);

  if (loading && agents.length === 0) {
    return (
      <div className="flex items-center justify-center py-16">
        <LoadingSpinner size="lg" />
        <span className="ml-3 text-gray-600">Loading agents...</span>
      </div>
    );
  }

  if (agents.length === 0) {
    return (
      <div className="text-center py-16">
        <div className="mx-auto w-24 h-24 bg-gray-100 rounded-full flex items-center justify-center mb-4">
          <svg
            className="w-12 h-12 text-gray-400"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={1.5}
              d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"
            />
          </svg>
        </div>
        <h3 className="text-lg font-medium text-gray-900 mb-2">No agents yet</h3>
        <p className="text-gray-600 mb-4">
          Get started by creating your first agent.
        </p>
      </div>
    );
  }

  if (filteredAgents.length === 0) {
    return (
      <div className="text-center py-16">
        <div className="mx-auto w-24 h-24 bg-gray-100 rounded-full flex items-center justify-center mb-4">
          <svg
            className="w-12 h-12 text-gray-400"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={1.5}
              d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
            />
          </svg>
        </div>
        <h3 className="text-lg font-medium text-gray-900 mb-2">No matches found</h3>
        <p className="text-gray-600">
          No agents match your current search and filter criteria.
        </p>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6 items-start">
      {filteredAgents.map((agent, index) => (
        <div
          key={agent.name || `agent-${index}`}
          data-keyboard-nav-selected={selectedAgent?.name === agent.name}
        >
          <AgentCard
            agent={agent}
            selected={selectedAgent?.name === agent.name}
            onView={onViewAgent}
            onEdit={onEditAgent}
            onDelete={onDeleteAgent}
          />
        </div>
      ))}
    </div>
  );
};