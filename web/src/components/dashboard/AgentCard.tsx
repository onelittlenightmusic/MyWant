import React from 'react';
import { Eye, Edit, Trash2, MoreHorizontal, Bot, Monitor, Zap, Settings, Brain } from 'lucide-react';
import { Agent } from '@/types/agent';
import { truncateText, classNames } from '@/utils/helpers';
import { getBackgroundStyle, getBackgroundOverlayClass } from '@/utils/backgroundStyles';

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
  const cardRef = React.useRef<HTMLDivElement>(null);

  // Focus the card when it's targeted by keyboard navigation
  React.useEffect(() => {
    if (selected && document.activeElement !== cardRef.current) {
      cardRef.current?.focus();
    }
  }, [selected]);

  const handleCardClick = () => {
    onView(agent);

    // Smooth scroll the card into view after selection
    requestAnimationFrame(() => {
      setTimeout(() => {
        const selectedElement = document.querySelector('[data-keyboard-nav-selected="true"]');
        if (selectedElement && selectedElement instanceof HTMLElement) {
          selectedElement.scrollIntoView({ behavior: 'smooth', block: 'center' });
        }
      }, 0);
    });
  };

  const getTypeIcon = () => {
    switch (agentType) {
      case 'do':
        return <Zap className="h-4 w-4" />;
      case 'monitor':
        return <Monitor className="h-4 w-4" />;
      case 'think':
        return <Brain className="h-4 w-4" />;
      default:
        return <Bot className="h-4 w-4" />;
    }
  };

  const getTypeColor = () => {
    switch (agentType) {
      case 'do':
        return 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300';
      case 'monitor':
        return 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300';
      case 'think':
        return 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300';
      default:
        return 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300';
    }
  };

  // Get background style for agent card
  const backgroundStyle = getBackgroundStyle(agentType, true);

  return (
    <div
      ref={cardRef}
      onClick={handleCardClick}
      tabIndex={0}
      data-keyboard-nav-selected={selected}
      data-keyboard-nav-id={agentName}
      className={classNames(
        'card hover:shadow-md dark:hover:shadow-blue-900/20 transition-all duration-300 cursor-pointer group relative overflow-hidden focus:outline-none focus:ring-2 focus:ring-blue-400 dark:focus:ring-blue-500 focus:ring-inset h-full flex flex-col min-h-[6rem] sm:min-h-[10rem]',
        selected ? 'border-blue-500 border-2 shadow-lg scale-[1.02] z-10' : 'border-gray-200 dark:border-gray-700',
        backgroundStyle.className,
        className
      )}
      style={backgroundStyle.style}
    >
      {/* Background overlay */}
      <div className={getBackgroundOverlayClass()}></div>

      {/* Content Area */}
      <div className="relative z-10 px-3 sm:px-6 pb-3 pt-3 order-1 flex-1">
        <p className="text-[10px] sm:text-sm text-gray-500 dark:text-gray-400 mt-1">
          {capabilities.length} capabilities · {uses.length} deps
        </p>
        
        {/* Capability tags if any */}
        <div className="flex flex-wrap gap-1 mt-2">
          {capabilities.slice(0, 3).map((cap, i) => (
            <span key={i} className="px-1.5 py-0.5 rounded-md bg-gray-100 dark:bg-gray-700 text-[8px] sm:text-[10px] text-gray-600 dark:text-gray-300">
              {cap}
            </span>
          ))}
          {capabilities.length > 3 && (
            <span className="px-1.5 py-0.5 rounded-md bg-gray-100 dark:bg-gray-700 text-[8px] sm:text-[10px] text-gray-600 dark:text-gray-300">
              +{capabilities.length - 3}
            </span>
          )}
        </div>
      </div>

      {/* Header (Title Area) - Moved to bottom to match WantCard */}
      <div className="relative z-20 order-2 mt-auto">
        <div className={classNames(
          "backdrop-blur-[2px] transition-colors duration-200 px-3 sm:px-6 py-1.5 flex items-center justify-between",
          selected ? "bg-blue-100/90 dark:bg-blue-900/70" : "bg-white/60 dark:bg-gray-900/70"
        )}>
          <div className="flex-1 min-w-0">
            <h3
              className="text-[9px] sm:text-[13px] font-semibold text-gray-900 dark:text-gray-100 truncate group-hover:text-primary-600 dark:group-hover:text-primary-400 transition-colors flex items-center gap-1.5"
              onClick={(e) => { e.stopPropagation(); onView(agent); }}
            >
              <Bot className="h-2 w-2 sm:h-3.5 sm:w-3.5 flex-shrink-0 text-blue-500" />
              {truncateText(agentName, 30)}
            </h3>
          </div>

          <div className="flex items-center space-x-1 sm:space-x-2 ml-1 sm:ml-2">
            {/* Type badge */}
            <div className={classNames(
              'inline-flex items-center px-1.5 sm:px-2 py-0.5 sm:py-1 rounded-full text-[8px] sm:text-[10px] font-medium',
              getTypeColor()
            )}>
              {getTypeIcon()}
              <span className="ml-1 capitalize hidden sm:inline">{agentType}</span>
            </div>

            {/* Status indicator */}
            <div className="flex items-center">
              <div className="w-1.5 h-1.5 sm:w-2 sm:h-2 rounded-full bg-green-500" title="Active" />
            </div>

            {/* Actions dropdown */}
            <div className="relative group/menu">
              <button 
                className="p-1 rounded-md text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
                onClick={(e) => e.stopPropagation()}
              >
                <MoreHorizontal className="h-3.5 w-3.5" />
              </button>

              <div className="absolute right-0 bottom-full mb-2 w-48 bg-white dark:bg-gray-800 rounded-md shadow-lg border border-gray-200 dark:border-gray-700 z-10 opacity-0 invisible group-hover/menu:opacity-100 group-hover/menu:visible transition-all duration-200">
                <div className="py-1">
                  <button
                    onClick={(e) => { e.stopPropagation(); onView(agent); }}
                    className="flex items-center w-full px-4 py-2 text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
                  >
                    <Eye className="h-4 w-4 mr-2" />
                    View Details
                  </button>

                  <button
                    onClick={(e) => { e.stopPropagation(); onEdit(agent); }}
                    className="flex items-center w-full px-4 py-2 text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
                  >
                    <Edit className="h-4 w-4 mr-2" />
                    Edit
                  </button>

                  <button
                    onClick={(e) => { e.stopPropagation(); /* TODO: Implement configure */ }}
                    className="flex items-center w-full px-4 py-2 text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
                  >
                    <Settings className="h-4 w-4 mr-2" />
                    Configure
                  </button>

                  <hr className="my-1 border-gray-200 dark:border-gray-700" />

                  <button
                    onClick={(e) => { e.stopPropagation(); onDelete(agent); }}
                    className="flex items-center w-full px-4 py-2 text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30"
                  >
                    <Trash2 className="h-4 w-4 mr-2" />
                    Delete
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};