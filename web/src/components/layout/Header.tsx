import React, { useState, useRef, useEffect } from 'react';
import { Plus, Heart, BarChart3, ListChecks, Map, Bot, Radar, StickyNote, Menu, X, Zap, BookOpen, Activity, Settings, Trophy } from 'lucide-react';
import { Link, useLocation } from 'react-router-dom';
import { classNames } from '@/utils/helpers';
import { InteractBubble } from '@/components/interact/InteractBubble';
import { useConfigStore } from '@/stores/configStore';
import { SettingsModal } from '@/components/modals/SettingsModal';
import { Tooltip } from '@/components/ui/Tooltip';

interface HeaderProps {
  onCreateWant: () => void;
  onCreateTargetWant?: () => void;
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
  showRadarMode?: boolean;
  onRadarModeToggle?: () => void;
  showGlobalState?: boolean;
  onGlobalStateToggle?: () => void;
}

const menuItems = [
  { id: 'wants', label: 'Wants', icon: Heart, href: '/dashboard' },
  { id: 'agents', label: 'Agents', icon: Bot, href: '/agents' },
];

const advancedItems = [
  { id: 'wantTypes', label: 'Want Types', icon: Zap, href: '/want-types' },
  { id: 'recipes', label: 'Recipes', icon: BookOpen, href: '/recipes' },
  { id: 'achievements', label: 'Achievements', icon: Trophy, href: '/achievements' },
  { id: 'logs', label: 'Logs', icon: Activity, href: '/logs' },
];

export const Header: React.FC<HeaderProps> = ({
  onCreateWant,
  onCreateTargetWant,
  title = 'MyWant',
  createButtonLabel = 'Add Want',
  itemCount,
  itemLabel,
  showSummary = false,
  onSummaryToggle,
  sidebarMinimized: _controlledMinimized,
  hideCreateButton = false,
  showSelectMode = false,
  onToggleSelectMode,
  onInteractSubmit,
  isInteractThinking = false,
  gooseProvider = 'claude-code',
  onProviderChange,
  showMinimap = false,
  onMinimapToggle,
  showRadarMode = false,
  onRadarModeToggle,
  showGlobalState = false,
  onGlobalStateToggle
}) => {
  const config = useConfigStore(state => state.config);
  const location = useLocation();

  const isBottom = config?.header_position === 'bottom';
  const [menuOpen, setMenuOpen] = useState(false);
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  const [showProviderSelect, setShowProviderSelect] = useState(false);
  const [showBubbleOnMobile, setShowBubbleOnMobile] = useState(false);
  const selectRef = useRef<HTMLDivElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);

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

  // Close hamburger menu when mouse leaves the menu area
  const handleMenuMouseLeave = () => setMenuOpen(false);

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

  // Dismiss keyboard on iOS when closing the menu
  const closeMenu = () => {
    (document.activeElement as HTMLElement)?.blur();
    setMenuOpen(false);
  };

  return (
    <>
    <header
      className={classNames(
        "bg-white dark:bg-gray-900 px-3 sm:px-6 py-2 sm:py-4 fixed left-0 right-0 z-40",
        isBottom ? "bottom-0 border-t border-gray-200 dark:border-gray-700" : "border-b border-gray-200 dark:border-gray-700",
      )}
      style={isBottom ? {} : { top: 'env(safe-area-inset-top, 0px)' }}
    >
      <div className="flex items-center justify-between gap-1 sm:gap-4">
        <div className="flex items-center space-x-2 min-w-0" ref={menuRef}>
          {/* Hamburger menu button — opens on hover */}
          <div className="relative" onMouseEnter={() => setMenuOpen(true)} onMouseLeave={handleMenuMouseLeave}>
            <button
              className="p-2 rounded-md text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors"
              aria-label="Toggle menu"
            >
              {menuOpen ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
            </button>

            {/* Dropdown menu */}
            {menuOpen && (
              <div className={classNames(
                "absolute left-0 w-48 z-50",
                isBottom ? "bottom-full pb-2" : "top-full pt-2"
              )}>
                {/* Transparent bridge to prevent mouseleave when moving to menu */}
                <div className="absolute inset-x-0 h-2 bg-transparent" style={isBottom ? { bottom: 0 } : { top: 0 }} />
                
                <div className="bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-800 rounded-lg shadow-lg overflow-hidden">
                  <nav className="p-2 space-y-1">
                  {menuItems.map(item => {
                    const Icon = item.icon;
                    const isActive = location.pathname === item.href;
                    return (
                      <Link
                        key={item.id}
                        to={item.href}
                        onClick={closeMenu}
                        onMouseDown={(e) => e.preventDefault()}
                        className={classNames(
                          'flex items-center px-3 py-2 rounded-md text-sm font-medium transition-colors',
                          isActive
                            ? 'bg-primary-100 text-primary-900 dark:bg-primary-900/30 dark:text-primary-300'
                            : 'text-gray-600 hover:bg-gray-100 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-gray-800 dark:hover:text-gray-200'
                        )}
                      >
                        <Icon className="h-4 w-4 mr-3" />
                        {item.label}
                      </Link>
                    );
                  })}
                </nav>
                <div className="border-t border-gray-200 dark:border-gray-800 p-2 space-y-1">
                  <p className="px-3 py-1 text-xs font-semibold text-gray-500 dark:text-gray-500 uppercase tracking-wider">Advanced</p>
                  {advancedItems.map(item => {
                    const Icon = item.icon;
                    const isActive = location.pathname === item.href;
                    return (
                      <Link
                        key={item.id}
                        to={item.href}
                        onClick={closeMenu}
                        onMouseDown={(e) => e.preventDefault()}
                        className={classNames(
                          'flex items-center px-3 py-2 rounded-md text-sm font-medium transition-colors',
                          isActive
                            ? 'bg-primary-100 text-primary-900 dark:bg-primary-900/30 dark:text-primary-300'
                            : 'text-gray-600 hover:bg-gray-100 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-gray-800 dark:hover:text-gray-200'
                        )}
                      >
                        <Icon className="h-4 w-4 mr-3" />
                        {item.label}
                      </Link>
                    );
                  })}
                  <button
                    onClick={() => { setIsSettingsOpen(true); closeMenu(); }}
                    className="w-full flex items-center px-3 py-2 rounded-md text-sm font-medium transition-colors text-gray-600 hover:bg-gray-100 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-gray-800 dark:hover:text-gray-200"
                  >
                    <Settings className="h-4 w-4 mr-3" />
                    Settings
                  </button>
                </div>
              </div>
            </div>
            )}
          </div>

          <h1 className="text-lg sm:text-2xl font-bold text-gray-900 dark:text-white whitespace-nowrap">{title}</h1>
          {itemLabel && (
            <div className="hidden sm:block text-sm text-gray-500 dark:text-gray-400 whitespace-nowrap">
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
                  "flex items-center absolute inset-x-0 bg-white dark:bg-gray-900 p-4 border-gray-200 dark:border-gray-700 shadow-lg z-50 animate-slide-in",
                  isBottom ? "bottom-full mb-px border-t" : "top-full border-b"
                )
              : "hidden lg:flex"
          )} ref={selectRef}
          onMouseLeave={() => { if (window.innerWidth >= 1024) setShowProviderSelect(false); }}
          >
            <div className={classNames(
              "transition-all duration-300 ease-in-out overflow-hidden",
              showProviderSelect ? "opacity-100 max-w-xs" : "opacity-0 max-w-0"
            )}>
              <select
                value={gooseProvider}
                onChange={(e) => onProviderChange?.(e.target.value)}
                className="text-xs border border-gray-300 dark:border-gray-600 rounded-md py-1.5 pl-2 pr-8 focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-gray-800 dark:text-gray-200 whitespace-nowrap"
              >
                <option value="claude-code">Claude</option>
                <option value="gemini-cli">Gemini</option>
              </select>
            </div>
            <InteractBubble
              onSubmit={handleInteractSubmitInternal}
              isThinking={isInteractThinking}
              onRobotClick={handleRobotClick}
              onRobotMouseEnter={() => { if (window.innerWidth >= 1024) setShowProviderSelect(true); }}
              autoFocus={showBubbleOnMobile}
            />
          </div>
        )}

        <div className="flex items-center space-x-1 sm:space-x-2 flex-shrink-0">
          {/* Robot toggle for mobile - only shown when onInteractSubmit is present */}
          {onInteractSubmit && (
            <Tooltip label="Speak to Agent">
              <button
                onClick={handleRobotClick}
                className={classNames(
                  "lg:hidden p-1.5 sm:p-2 rounded-full transition-colors bg-blue-600 shadow-md",
                  showBubbleOnMobile ? "ring-2 ring-blue-400" : ""
                )}
              >
                <Bot className="h-5 w-5 text-white" />
              </button>
            </Tooltip>
          )}

          {/* Minimap toggle button - only shown on mobile (lg:hidden) */}
          {onMinimapToggle && (
            <Tooltip label={showMinimap ? 'Hide Minimap' : 'Minimap'}>
              <button
                onClick={onMinimapToggle}
                className={classNames(
                  "lg:hidden p-1.5 sm:p-2 rounded-md transition-colors",
                  showMinimap ? "text-blue-600 bg-blue-50 dark:bg-blue-900/30" : "text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800"
                )}
              >
                <Map className="h-5 w-5" />
              </button>
            </Tooltip>
          )}

          {/* Global State toggle button - left of Radar */}
          {onGlobalStateToggle && (
            <Tooltip label={showGlobalState ? 'Memo ON' : 'Memo'} shortcut="g">
              <button
                onClick={onGlobalStateToggle}
                className={classNames(
                  "p-1.5 sm:p-2 rounded-md transition-colors",
                  showGlobalState
                    ? "text-green-600 bg-green-50 dark:bg-green-900/30 ring-2 ring-green-400 dark:ring-green-500"
                    : "text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800"
                )}
              >
                <StickyNote className="h-5 w-5" />
              </button>
            </Tooltip>
          )}

          {/* Correlation Radar toggle button */}
          {onRadarModeToggle && (
            <Tooltip label={showRadarMode ? 'Radar ON' : 'Correlation Radar'} shortcut="x">
              <button
                onClick={onRadarModeToggle}
                className={classNames(
                  "p-1.5 sm:p-2 rounded-md transition-colors",
                  showRadarMode
                    ? "text-orange-600 bg-orange-50 dark:bg-orange-900/30 ring-2 ring-orange-400 dark:ring-orange-500"
                    : "text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800"
                )}
              >
                <Radar className="h-5 w-5" />
              </button>
            </Tooltip>
          )}

          {onToggleSelectMode && (
            <Tooltip label={showSelectMode ? 'Exit Select' : 'Select'} shortcut="⇧S">
              <button
                onClick={onToggleSelectMode}
                className={`inline-flex items-center px-1.5 py-1.5 sm:px-3 sm:py-2 font-medium rounded-full transition duration-150 ease-in-out focus:outline-none focus:ring-2 focus:ring-offset-2 ${
                  showSelectMode
                    ? 'bg-blue-100 text-blue-700 hover:bg-blue-200 focus:ring-blue-500 dark:bg-blue-900/30 dark:text-blue-300'
                    : 'border border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-300 bg-white dark:bg-gray-800 hover:bg-gray-50 dark:hover:bg-gray-700 focus:ring-primary-500'
                }`}
              >
                <ListChecks className="h-4 w-4 flex-shrink-0" />
              </button>
            </Tooltip>
          )}

          {onSummaryToggle && (
            <Tooltip label={showSummary ? 'Hide Summary' : 'Summary'} shortcut="s">
              <button
                onClick={onSummaryToggle}
                className={`inline-flex items-center px-1.5 py-1.5 sm:px-3 sm:py-2 font-medium rounded-full transition duration-150 ease-in-out focus:outline-none focus:ring-2 focus:ring-offset-2 ${
                  showSummary
                    ? 'bg-blue-100 text-blue-700 hover:bg-blue-200 focus:ring-blue-500 dark:bg-blue-900/30 dark:text-blue-300'
                    : 'border border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-300 bg-white dark:bg-gray-800 hover:bg-gray-50 dark:hover:bg-gray-700 focus:ring-primary-500'
                }`}
              >
                <BarChart3 className="h-4 w-4 flex-shrink-0" />
              </button>
            </Tooltip>
          )}

          {!hideCreateButton && (
            <Tooltip label={createButtonLabel ?? 'Add Want'} shortcut="a">
              <button
                onClick={onCreateWant}
                className="inline-flex items-center px-1.5 py-1.5 sm:px-3 sm:py-2 bg-primary-600 hover:bg-primary-700 focus:ring-primary-500 focus:ring-offset-2 text-white font-medium rounded-full transition duration-150 ease-in-out focus:outline-none focus:ring-2 focus:ring-offset-2"
              >
                <span className="relative inline-flex flex-shrink-0">
                  <Heart className="h-4 w-4" />
                  <Plus className="h-2.5 w-2.5 absolute -top-1.5 -right-1.5" style={{ strokeWidth: 3 }} />
                </span>
              </button>
            </Tooltip>
          )}

          {!hideCreateButton && onCreateTargetWant && (
            <button
              onClick={onCreateTargetWant}
              className="inline-flex items-center px-1.5 py-1.5 sm:px-3 sm:py-2 border border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-300 bg-white dark:bg-gray-800 hover:bg-gray-50 dark:hover:bg-gray-700 focus:ring-primary-500 focus:ring-offset-2 font-medium rounded-full transition duration-150 ease-in-out focus:outline-none focus:ring-2 focus:ring-offset-2"
            >
              <span className="relative inline-flex flex-shrink-0">
                <span className="text-sm leading-none">🫙</span>
                <Plus className="h-2.5 w-2.5 absolute -top-1.5 -right-1.5 text-gray-500 dark:text-gray-400" style={{ strokeWidth: 3 }} />
              </span>
            </button>
          )}
        </div>
      </div>
      {isBottom && <div style={{ height: 'env(safe-area-inset-bottom, 0px)' }} />}
    </header>
    <SettingsModal isOpen={isSettingsOpen} onClose={() => setIsSettingsOpen(false)} />
    </>
  );
};