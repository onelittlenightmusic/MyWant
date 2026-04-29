import React, { useState, useRef, useEffect } from 'react';
import { Plus, Heart, BarChart3, ListChecks, Map, Bot, Radar, StickyNote, Menu, X, Zap, BookOpen, Activity, Settings, Trophy, LayoutGrid, Grid2X2 } from 'lucide-react';
import { Link, useLocation } from 'react-router-dom';
import { classNames } from '@/utils/helpers';
import { InteractBubble } from '@/components/interact/InteractBubble';
import { useConfigStore } from '@/stores/configStore';
import { SettingsModal } from '@/components/modals/SettingsModal';
import { Tooltip } from '@/components/ui/Tooltip';

interface HeaderProps {
  onCreateWant: () => void;
  onCreateTargetWant?: () => void;
  isAddWantActive?: boolean;
  isWhimActive?: boolean;
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
  showCanvasMode?: boolean;
  onCanvasModeToggle?: () => void;
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
  isAddWantActive = false,
  isWhimActive = false,
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
  onGlobalStateToggle,
  showCanvasMode = false,
  onCanvasModeToggle,
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
  const headerRef = useRef<HTMLElement>(null);

  // Set CSS variable for header height so sidebars can offset accordingly
  useEffect(() => {
    const el = headerRef.current;
    if (!el) return;
    const observer = new ResizeObserver(() => {
      document.documentElement.style.setProperty('--header-height', `${el.offsetHeight}px`);
    });
    observer.observe(el);
    document.documentElement.style.setProperty('--header-height', `${el.offsetHeight}px`);
    return () => observer.disconnect();
  }, []);

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

  const menuJustClosedRef = useRef(false);

  // Close hamburger menu when mouse leaves the menu area
  const handleMenuMouseLeave = () => setMenuOpen(false);
  const handleMenuMouseEnter = () => {
    if (!menuJustClosedRef.current) setMenuOpen(true);
  };

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
    menuJustClosedRef.current = true;
    setTimeout(() => { menuJustClosedRef.current = false; }, 400);
    (document.activeElement as HTMLElement)?.blur();
    setMenuOpen(false);
  };

  return (
    <>
    <header
      ref={headerRef}
      className={classNames(
        "bg-white dark:bg-gray-900 px-3 sm:px-6 py-2 sm:py-4 fixed left-0 right-0 z-40",
        isBottom ? "bottom-0 border-t border-gray-200 dark:border-gray-700" : "border-b border-gray-200 dark:border-gray-700",
      )}
      style={isBottom ? {} : { top: 'env(safe-area-inset-top, 0px)' }}
    >
      <div className="flex items-center justify-between gap-1 sm:gap-4">
        <div className="flex items-center space-x-2 min-w-0 self-stretch" ref={menuRef}>
          {/* Hamburger menu button — opens on hover */}
          <div className="relative self-stretch -my-2 sm:-my-4" onMouseEnter={handleMenuMouseEnter} onMouseLeave={handleMenuMouseLeave}>
            <button
              onClick={() => menuOpen ? closeMenu() : setMenuOpen(true)}
              className={classNames(
                "flex flex-col items-center justify-center gap-0.5 px-3 sm:px-4 h-full transition-all duration-150 focus:outline-none min-w-[56px] sm:min-w-[64px]",
                menuOpen
                  ? "bg-gray-100 dark:bg-gray-800 text-gray-900 dark:text-white"
                  : "text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800"
              )}
              aria-label="Toggle menu"
            >
              {menuOpen ? (
                <X className="h-4 w-4" />
              ) : (
                <Menu className="h-4 w-4" />
              )}
              <span className="text-[9px] font-bold leading-none uppercase tracking-tighter hidden sm:block">
                {menuOpen ? 'Close' : 'Menu'}
              </span>
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

        {/* Right: full-height flush button grid */}
        <div className="flex items-stretch self-stretch -my-2 sm:-my-4 flex-shrink-0 overflow-hidden">

          {/* Robot - mobile only */}
          {onInteractSubmit && (
            <Tooltip label="Speak to Agent">
              <button
                onClick={handleRobotClick}
                className={classNames(
                  'lg:hidden flex flex-col items-center justify-center gap-0.5 px-3 h-full transition-all duration-150 focus:outline-none',
                  showBubbleOnMobile
                    ? 'bg-blue-600/90 text-white hover:brightness-110'
                    : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800'
                )}
              >
                <Bot className="h-4 w-4" />
                <span className="text-[9px] font-bold leading-none uppercase tracking-tighter hidden sm:block">Ask</span>
              </button>
            </Tooltip>
          )}

          {/* Minimap - mobile only */}
          {onMinimapToggle && (
            <Tooltip label={showMinimap ? 'Hide Minimap' : 'Minimap'}>
              <button
                onClick={onMinimapToggle}
                className={classNames(
                  'lg:hidden flex flex-col items-center justify-center gap-0.5 px-3 h-full transition-all duration-150 focus:outline-none',
                  showMinimap
                    ? 'bg-blue-500/90 text-white hover:brightness-110'
                    : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800'
                )}
              >
                <Map className="h-4 w-4" />
                <span className="text-[9px] font-bold leading-none uppercase tracking-tighter hidden sm:block">Map</span>
              </button>
            </Tooltip>
          )}

          {/* Memo */}
          {onGlobalStateToggle && (
            <Tooltip label={showGlobalState ? 'Memo ON' : 'Memo'} shortcut="g">
              <button
                onClick={onGlobalStateToggle}
                className={classNames(
                  'flex flex-col items-center justify-center gap-0.5 px-3 h-full transition-all duration-150 focus:outline-none',
                  showGlobalState
                    ? 'bg-green-600/90 text-white hover:brightness-110'
                    : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800'
                )}
              >
                <StickyNote className="h-4 w-4" />
                <span className="text-[9px] font-bold leading-none uppercase tracking-tighter hidden sm:block">Memo</span>
              </button>
            </Tooltip>
          )}

          {/* Radar */}
          {onRadarModeToggle && (
            <Tooltip label={showRadarMode ? 'Radar ON' : 'Correlation Radar'} shortcut="x">
              <button
                onClick={onRadarModeToggle}
                className={classNames(
                  'flex flex-col items-center justify-center gap-0.5 px-3 h-full transition-all duration-150 focus:outline-none',
                  showRadarMode
                    ? 'bg-orange-500/90 text-white hover:brightness-110'
                    : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800'
                )}
              >
                <Radar className="h-4 w-4" />
                <span className="text-[9px] font-bold leading-none uppercase tracking-tighter hidden sm:block">Radar</span>
              </button>
            </Tooltip>
          )}

          {/* Select */}
          {onToggleSelectMode && (
            <Tooltip label={showSelectMode ? 'Exit Select' : 'Select'} shortcut="⇧S">
              <button
                onClick={onToggleSelectMode}
                className={classNames(
                  'flex flex-col items-center justify-center gap-0.5 px-3 h-full transition-all duration-150 focus:outline-none',
                  showSelectMode
                    ? 'bg-blue-600/90 text-white hover:brightness-110'
                    : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800'
                )}
              >
                <ListChecks className="h-4 w-4" />
                <span className="text-[9px] font-bold leading-none uppercase tracking-tighter hidden sm:block">Select</span>
              </button>
            </Tooltip>
          )}

          {/* Summary */}
          {onSummaryToggle && (
            <Tooltip label={showSummary ? 'Hide Summary' : 'Summary'} shortcut="s">
              <button
                onClick={onSummaryToggle}
                className={classNames(
                  'flex flex-col items-center justify-center gap-0.5 px-3 h-full transition-all duration-150 focus:outline-none',
                  showSummary
                    ? 'bg-slate-600/90 text-white hover:brightness-110'
                    : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800'
                )}
              >
                <BarChart3 className="h-4 w-4" />
                <span className="text-[9px] font-bold leading-none uppercase tracking-tighter hidden sm:block">Stats</span>
              </button>
            </Tooltip>
          )}

          {/* List / Canvas toggle switch */}
          {onCanvasModeToggle && (
            <>
              <div className="w-px bg-gray-200 dark:bg-gray-700 self-stretch" />
              <Tooltip label={showCanvasMode ? 'Switch to List' : 'Switch to Canvas'}>
                <button
                  onClick={onCanvasModeToggle}
                  className="flex flex-col items-center justify-center gap-0.5 px-3 h-full transition-all duration-150 focus:outline-none text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800"
                >
                  {/* Switch track */}
                  <div className={classNames(
                    'relative flex items-center rounded-full transition-colors duration-200',
                    'w-8 h-4',
                    showCanvasMode ? 'bg-blue-500' : 'bg-gray-300 dark:bg-gray-600'
                  )}>
                    {/* Thumb with icon */}
                    <div className={classNames(
                      'absolute flex items-center justify-center w-3 h-3 rounded-full bg-white shadow transition-transform duration-200',
                      showCanvasMode ? 'translate-x-[18px]' : 'translate-x-[2px]'
                    )}>
                      {showCanvasMode
                        ? <Grid2X2 className="w-2 h-2 text-blue-500" />
                        : <LayoutGrid className="w-2 h-2 text-gray-400" />
                      }
                    </div>
                  </div>
                  <span className="text-[9px] font-bold leading-none uppercase tracking-tighter hidden sm:block">
                    {showCanvasMode ? 'Canvas' : 'List'}
                  </span>
                </button>
              </Tooltip>
            </>
          )}

          {/* Add Want / Add Whim — separated by a thin line */}
          {!hideCreateButton && (
            <>
              <div className="w-px bg-gray-200 dark:bg-gray-700 self-stretch" />
              <button
                onClick={onCreateWant}
                className={classNames(
                  "flex flex-col items-center justify-center gap-0.5 px-3 sm:px-4 h-full transition-all duration-150 focus:outline-none",
                  isAddWantActive
                    ? "bg-primary-600 text-white hover:brightness-110 active:opacity-80"
                    : "text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800"
                )}
              >
                <span className="relative inline-flex flex-shrink-0">
                  <Heart className="h-4 w-4" />
                  <Plus className="h-2.5 w-2.5 absolute -top-1.5 -right-1.5" style={{ strokeWidth: 3 }} />
                </span>
                <span className="text-[9px] font-bold leading-none uppercase tracking-tighter hidden sm:block">Want</span>
              </button>

              {onCreateTargetWant && (
                <button
                  onClick={onCreateTargetWant}
                  className={classNames(
                    "flex flex-col items-center justify-center gap-0.5 px-3 sm:px-4 h-full transition-all duration-150 focus:outline-none",
                    isWhimActive
                      ? "bg-indigo-600 text-white hover:brightness-110 active:opacity-80"
                      : "text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800"
                  )}
                >
                  <span className="relative inline-flex flex-shrink-0">
                    <span className="text-sm leading-none">🫙</span>
                    <Plus className="h-2.5 w-2.5 absolute -top-1.5 -right-1.5 text-white" style={{ strokeWidth: 3 }} />
                  </span>
                  <span className="text-[9px] font-bold leading-none uppercase tracking-tighter hidden sm:block">Whim</span>
                </button>
              )}
            </>
          )}
        </div>
      </div>
      {isBottom && <div style={{ height: 'env(safe-area-inset-bottom, 0px)' }} />}
    </header>
    <SettingsModal isOpen={isSettingsOpen} onClose={() => setIsSettingsOpen(false)} />
    </>
  );
};
