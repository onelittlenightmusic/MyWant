import React, { useState, useRef, useEffect } from 'react';
import { Plus, BarChart3, ListChecks, Map, Bot } from 'lucide-react';
import { classNames } from '@/utils/helpers';
import { InteractBubble } from '@/components/interact/InteractBubble';
import { useConfigStore } from '@/stores/configStore';

interface HeaderProps {
  onCreateWant: () => void;
  title?: string;
  createButtonLabel?: string;
  itemCount?: number;
  itemLabel?: string;
  showSummary?: boolean;
  onSummaryToggle?: () => void;
  sidebarMinimized?: boolean;
  hideCreateButton?: boolean;
  showSelectMode?: boolean;
  onToggleSelectMode?: () => void;
  onInteractSubmit?: (message: string) => void;
  isInteractThinking?: boolean;
  gooseProvider?: string;
  onProviderChange?: (provider: string) => void;
  showMinimap?: boolean;
  onMinimapToggle?: () => void;
}

export const Header: React.FC<HeaderProps> = ({
  onCreateWant,
  title = 'MyWant',
  createButtonLabel = 'Add Want',
  itemCount,
  itemLabel,
  showSummary = false,
  onSummaryToggle,
  sidebarMinimized = false,
  hideCreateButton = false,
  showSelectMode = false,
  onToggleSelectMode,
  onInteractSubmit,
  isInteractThinking = false,
  gooseProvider = 'claude-code',
  onProviderChange,
  showMinimap = false,
  onMinimapToggle
}) => {
  const config = useConfigStore(state => state.config);
  const isBottom = config?.header_position === 'bottom';
  const [showProviderSelect, setShowProviderSelect] = useState(false);
  const [showBubbleOnMobile, setShowBubbleOnMobile] = useState(false);
  const selectRef = useRef<HTMLDivElement>(null);

  // Hide provider select and mobile bubble when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (selectRef.current && !selectRef.current.contains(event.target as Node)) {
        setShowProviderSelect(false);
        if (window.innerWidth < 1024) {
          setShowBubbleOnMobile(false);
        }
      }
    };

    if (showProviderSelect || showBubbleOnMobile) {
      document.addEventListener('mousedown', handleClickOutside);
      return () => document.removeEventListener('mousedown', handleClickOutside);
    }
  }, [showProviderSelect, showBubbleOnMobile]);

  const handleRobotClick = () => {
    if (window.innerWidth < 1024) {
      if (!showBubbleOnMobile) {
        setShowBubbleOnMobile(true);
        setShowProviderSelect(true); // Always show provider select when opening bubble on mobile
      } else {
        setShowBubbleOnMobile(false);
        setShowProviderSelect(false);
      }
    } else {
      setShowProviderSelect(prev => !prev);
    }
  };

  const handleInteractSubmitInternal = (message: string) => {
    onInteractSubmit?.(message);
    if (window.innerWidth < 1024) {
      setShowBubbleOnMobile(false);
    }
  };

  return (
    <header className={classNames(
      "bg-white px-3 sm:px-6 py-2 sm:py-4 fixed right-0 z-40 transition-all duration-300 ease-in-out left-0",
      isBottom ? "bottom-0 border-t border-gray-200" : "top-0 border-b border-gray-200",
      sidebarMinimized ? "lg:left-20" : "lg:left-44"
    )} style={isBottom ? { paddingBottom: 'calc(0.5rem + env(safe-area-inset-bottom))' } : {}}>
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center space-x-4 min-w-0">
          <h1 className="text-lg sm:text-2xl font-bold text-gray-900 whitespace-nowrap">{title}</h1>
          {itemLabel && (
            <div className="hidden sm:block text-sm text-gray-500 whitespace-nowrap">
              {itemCount} {itemLabel}{itemCount !== 1 ? 's' : ''}
            </div>
          )}
        </div>

        {/* InteractBubble - now shown on mobile via toggle */}
        {onInteractSubmit && (
          <div className={classNames(
            "flex-1 justify-center max-w-xl gap-2",
            // Desktop behavior: always flex
            "lg:flex lg:items-center",
            // Mobile behavior: absolute overlay or hidden
            showBubbleOnMobile 
              ? classNames(
                  "flex items-center absolute inset-x-0 bg-white p-4 border-gray-200 shadow-lg z-50 animate-slide-in",
                  isBottom ? "bottom-full mb-px border-t" : "top-full border-b"
                )
              : "hidden lg:flex"
          )} ref={selectRef}>
            <div className={classNames(
              "transition-all duration-300 ease-in-out overflow-hidden",
              showProviderSelect ? "opacity-100 max-w-xs" : "opacity-0 max-w-0"
            )}>
              <select
                value={gooseProvider}
                onChange={(e) => onProviderChange?.(e.target.value)}
                className="text-xs border border-gray-300 rounded-md py-1.5 pl-2 pr-8 focus:ring-primary-500 focus:border-primary-500 bg-white whitespace-nowrap"
              >
                <option value="claude-code">Claude</option>
                <option value="gemini-cli">Gemini</option>
              </select>
            </div>
            <InteractBubble
              onSubmit={handleInteractSubmitInternal}
              isThinking={isInteractThinking}
              onRobotClick={handleRobotClick}
              autoFocus={showBubbleOnMobile}
            />
          </div>
        )}

        <div className="flex items-center space-x-3 flex-shrink-0">
          {/* Robot toggle for mobile - only shown when onInteractSubmit is present */}
          {onInteractSubmit && (
            <button
              onClick={handleRobotClick}
              className={classNames(
                "lg:hidden p-2 rounded-full transition-colors bg-blue-600 shadow-md",
                showBubbleOnMobile ? "ring-2 ring-blue-400" : ""
              )}
              title="Speak to Agent"
            >
              <Bot className="h-5 w-5 text-white" />
            </button>
          )}

          {/* Minimap toggle button - only shown on mobile (lg:hidden) */}
          {onMinimapToggle && (
            <button
              onClick={onMinimapToggle}
              className={classNames(
                "lg:hidden p-2 rounded-md transition-colors",
                showMinimap ? "text-blue-600 bg-blue-50" : "text-gray-600 hover:bg-gray-100"
              )}
              title={showMinimap ? "Hide minimap" : "Show minimap"}
            >
              <Map className="h-5 w-5" />
            </button>
          )}

          {onToggleSelectMode && (
            <button
              onClick={onToggleSelectMode}
              className={`inline-flex items-center px-3 sm:px-4 py-2 font-medium rounded-full transition duration-150 ease-in-out focus:outline-none focus:ring-2 focus:ring-offset-2 whitespace-nowrap ${
                showSelectMode
                  ? 'bg-blue-100 text-blue-700 hover:bg-blue-200 focus:ring-blue-500'
                  : 'border border-gray-300 text-gray-700 bg-white hover:bg-gray-50 focus:ring-primary-500'
              }`}
              title={showSelectMode ? 'Exit Select Mode' : 'Enter Select Mode'}
            >
              <ListChecks className="h-4 w-4 sm:mr-2 flex-shrink-0" />
              <span className="hidden sm:inline">Select</span>
            </button>
          )}

          {onSummaryToggle && (
            <button
              onClick={onSummaryToggle}
              className={`inline-flex items-center px-3 sm:px-4 py-2 font-medium rounded-full transition duration-150 ease-in-out focus:outline-none focus:ring-2 focus:ring-offset-2 whitespace-nowrap ${
                showSummary
                  ? 'bg-blue-100 text-blue-700 hover:bg-blue-200 focus:ring-blue-500'
                  : 'border border-gray-300 text-gray-700 bg-white hover:bg-gray-50 focus:ring-primary-500'
              }`}
              title={showSummary ? 'Hide summary' : 'Show summary'}
            >
              <BarChart3 className="h-4 w-4 sm:mr-2 flex-shrink-0" />
              <span className="hidden sm:inline">Summary</span>
            </button>
          )}

          {!hideCreateButton && (
            <button
              onClick={onCreateWant}
              className="inline-flex items-center px-3 sm:px-4 py-2 bg-primary-600 hover:bg-primary-700 focus:ring-primary-500 focus:ring-offset-2 text-white font-medium rounded-full transition duration-150 ease-in-out focus:outline-none focus:ring-2 focus:ring-offset-2 whitespace-nowrap"
            >
              <Plus className="h-4 w-4 sm:mr-2 flex-shrink-0" />
              <span className="hidden sm:inline">{createButtonLabel}</span>
            </button>
          )}
        </div>
      </div>
    </header>
  );
};