import React, { useState, useMemo, useRef, useImperativeHandle, forwardRef, useEffect, useCallback } from 'react';
import { ChevronRight, Package, Zap, ChevronDown, Search, X, Plane, Calculator, Layers, CheckCircle, Monitor, Tag } from 'lucide-react';
import { WantTypeListItem } from '@/types/wantType';
import { GenericRecipe } from '@/types/recipe';
import { getBackgroundStyle, getBackgroundOverlayClass } from '@/utils/backgroundStyles';
import { useWantStore } from '@/stores/wantStore';
import { suppressDragImage, classNames } from '@/utils/helpers';

export interface TypeRecipeSelectorItem {
  id: string;
  type: 'want-type' | 'recipe';
  name: string;
  title: string;
  description: string;
  category?: string;
  icon?: React.ReactNode;
}

interface TypeRecipeSelectorProps {
  wantTypes: WantTypeListItem[];
  recipes: GenericRecipe[];
  selectedId: string | null;
  showSearch: boolean;
  onSelect: (id: string, itemType: 'want-type' | 'recipe') => void;
  onClear?: () => void;
  onGenerateName: (selectedId: string, itemType: 'want-type' | 'recipe', userInput?: string) => string;
  onArrowDown?: () => void;
}

export interface TypeRecipeSelectorRef {
  focusSearch: () => void;
  focus: () => void;
  /** Move focus to next item in the filtered list (wraps around) */
  navigateNext: () => void;
  /** Move focus to previous item in the filtered list (wraps around) */
  navigatePrev: () => void;
  /** Select the currently highlighted item (no-op if none highlighted) */
  confirmFocused: () => void;
}

export const TypeRecipeSelector = forwardRef<TypeRecipeSelectorRef, TypeRecipeSelectorProps>(({
  wantTypes,
  recipes,
  selectedId,
  showSearch,
  onSelect,
  onClear,
  onGenerateName,
  onArrowDown
}, ref) => {
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedCategory, setSelectedCategory] = useState<string | null>(null);
  const [isExpanded, setIsExpanded] = useState(!selectedId); // Auto-expand if nothing selected
  const [focusedIndex, setFocusedIndex] = useState<number>(-1);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const collapsedButtonRef = useRef<HTMLButtonElement>(null);
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);
  // Stable refs for use inside useImperativeHandle (avoids stale closures)
  const filteredItemsRef = useRef<TypeRecipeSelectorItem[]>([]);
  const focusedIndexRef = useRef<number>(-1);
  const handleSelectRef = useRef<(item: TypeRecipeSelectorItem) => void>(() => {});

  // Long press for touch drag
  const touchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const touchStartPosRef = useRef<{ x: number, y: number } | null>(null);

  const handleTouchStart = (item: TypeRecipeSelectorItem, e: React.TouchEvent) => {
    const touch = e.touches[0];
    touchStartPosRef.current = { x: touch.clientX, y: touch.clientY };

    touchTimerRef.current = setTimeout(() => {
      // Long press triggered
      if (window.navigator.vibrate) window.navigator.vibrate(40);

      useWantStore.getState().setDraggingTemplate({
        id: item.id,
        type: item.type,
        name: item.title
      });
      useWantStore.getState().setTouchPos({ x: touch.clientX, y: touch.clientY });

      touchTimerRef.current = null;
    }, 200); // 200ms for long press
  };

  const handleTouchMove = (e: React.TouchEvent) => {
    const { draggingTemplate, setTouchPos } = useWantStore.getState();

    if (touchTimerRef.current && touchStartPosRef.current) {
      const touch = e.touches[0];
      const dist = Math.sqrt(
        Math.pow(touch.clientX - touchStartPosRef.current.x, 2) +
        Math.pow(touch.clientY - touchStartPosRef.current.y, 2)
      );
      if (dist > 15) {
        clearTimeout(touchTimerRef.current);
        touchTimerRef.current = null;
      }
    }

    if (draggingTemplate) {
      // If we are already dragging, prevent scrolling and update position
      if (e.cancelable) e.preventDefault();
      const touch = e.touches[0];
      setTouchPos({ x: touch.clientX, y: touch.clientY });
    }
  };

  const handleTouchEnd = () => {
    if (touchTimerRef.current) {
      clearTimeout(touchTimerRef.current);
      touchTimerRef.current = null;
    }
  };

  // Sync expansion state with selectedId changes
  useEffect(() => {
    if (!selectedId) {
      setIsExpanded(true);
    } else {
      setIsExpanded(false);
    }
  }, [selectedId]);

  // Expose focus methods to parent
  useImperativeHandle(ref, () => ({
    focusSearch: () => {
      searchInputRef.current?.focus();
    },
    focus: () => {
      collapsedButtonRef.current?.focus();
    },
    navigateNext: () => {
      const total = filteredItemsRef.current.length;
      if (total === 0) return;
      setFocusedIndex(i => {
        const next = i < total - 1 ? i + 1 : 0;
        itemRefs.current[next]?.focus();
        return next;
      });
    },
    navigatePrev: () => {
      const total = filteredItemsRef.current.length;
      if (total === 0) return;
      setFocusedIndex(i => {
        const prev = i > 0 ? i - 1 : total - 1;
        itemRefs.current[prev]?.focus();
        return prev;
      });
    },
    confirmFocused: () => {
      const idx = focusedIndexRef.current;
      const item = filteredItemsRef.current[idx];
      if (item) handleSelectRef.current(item);
    },
  }));

  const categoryIcons: Record<string, React.ReactNode> = {
    travel: <Plane className="h-3 w-3" />,
    mathematics: <Calculator className="h-3 w-3" />,
    math: <Calculator className="h-3 w-3" />,
    queue: <Layers className="h-3 w-3" />,
    approval: <CheckCircle className="h-3 w-3" />,
    system: <Monitor className="h-3 w-3" />,
  };

  // Convert want types and recipes to selector items
  const items = useMemo(() => {
    const wantTypeItems: TypeRecipeSelectorItem[] = wantTypes.map(wt => ({
      id: wt.name,
      type: 'want-type' as const,
      name: wt.name,
      title: wt.title || wt.name,
      description: wt.title || '',
      category: wt.category,
      icon: <Zap className="w-5 h-5" />
    }));

    const recipeItems: TypeRecipeSelectorItem[] = recipes
      .filter(r => r.recipe?.metadata?.custom_type)
      .map(r => ({
        id: r.recipe.metadata.custom_type || '',
        type: 'recipe' as const,
        name: r.recipe.metadata.custom_type || '',
        title: r.recipe.metadata.name || r.recipe.metadata.custom_type || '',
        description: r.recipe.metadata.description || '',
        category: r.recipe.metadata.category,
        icon: <Package className="w-5 h-5" />
      }));

    return [...wantTypeItems, ...recipeItems];
  }, [wantTypes, recipes]);

  // Extract unique categories from want types and recipes
  const categories = useMemo(() => {
    const categorySet = new Set<string>();
    wantTypes.forEach(wt => {
      if (wt.category) {
        categorySet.add(wt.category);
      }
    });
    recipes.forEach(recipe => {
      if (recipe.recipe?.metadata?.category) {
        categorySet.add(recipe.recipe.metadata.category);
      }
    });
    return Array.from(categorySet).sort();
  }, [wantTypes, recipes]);

  // Filter items based on search query and category
  const filteredItems = useMemo(() => {
    let filtered = items;

    // Apply category filter (both want types and recipes with categories)
    if (selectedCategory) {
      filtered = filtered.filter(item =>
        item.category === selectedCategory
      );
    }

    // Apply search query filter
    if (searchQuery.trim()) {
      const query = searchQuery.toLowerCase();
      filtered = filtered.filter(item =>
        item.title.toLowerCase().includes(query) ||
        item.description.toLowerCase().includes(query) ||
        item.name.toLowerCase().includes(query)
      );
    }

    return filtered;
  }, [items, searchQuery, selectedCategory]);

  // Keep stable refs in sync (for useImperativeHandle)
  filteredItemsRef.current = filteredItems;
  focusedIndexRef.current = focusedIndex;

  // Group items by type
  const groupedItems = useMemo(() => {
    return {
      wantTypes: filteredItems.filter(item => item.type === 'want-type'),
      recipes: filteredItems.filter(item => item.type === 'recipe')
    };
  }, [filteredItems]);

  // Get selected item
  const selectedItem = useMemo(() => {
    return items.find(item => item.id === selectedId);
  }, [items, selectedId]);

  const handleSelect = useCallback((item: TypeRecipeSelectorItem) => {
    onSelect(item.id, item.type);
    setIsExpanded(false);
    setFocusedIndex(-1);
  }, [onSelect]);

  // Keep handleSelect ref in sync (after definition, before useImperativeHandle uses it)
  handleSelectRef.current = handleSelect;

  const handleToggleExpand = useCallback(() => {
    setIsExpanded(prev => {
      // Clear selection when expanding from collapsed state
      if (!prev && onClear) {
        onClear();
      }
      return !prev;
    });
    setFocusedIndex(-1);
  }, [onClear]);

  // Handle keyboard navigation
  const handleKeyNavigation = useCallback((e: React.KeyboardEvent) => {
    const totalItems = filteredItems.length;
    if (totalItems === 0) return;

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      const newIndex = focusedIndex < totalItems - 1 ? focusedIndex + 1 : 0;
      setFocusedIndex(newIndex);
      itemRefs.current[newIndex]?.focus();
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      const newIndex = focusedIndex > 0 ? focusedIndex - 1 : totalItems - 1;
      setFocusedIndex(newIndex);
      itemRefs.current[newIndex]?.focus();
    } else if (e.key === 'Enter' && focusedIndex >= 0) {
      e.preventDefault();
      const item = filteredItems[focusedIndex];
      if (item) {
        handleSelect(item);
      }
    }
  }, [filteredItems, focusedIndex, handleSelect]);

  // Reset focused index when filtered items change
  useEffect(() => {
    setFocusedIndex(-1);
  }, [searchQuery, selectedCategory]);

  // Keyboard shortcut for Delete key in collapsed view
  useEffect(() => {
    if (!isExpanded && selectedItem) {
      const handleDeleteKey = (e: KeyboardEvent) => {
        if (e.key === 'Delete' || e.key === 'Backspace') {
          // Don't trigger if user is typing in an input
          const target = e.target as HTMLElement;
          const isInputElement =
            target.tagName === 'INPUT' ||
            target.tagName === 'TEXTAREA' ||
            target.isContentEditable;

          if (!isInputElement) {
            e.preventDefault();
            handleToggleExpand();
          }
        }
      };

      window.addEventListener('keydown', handleDeleteKey);
      return () => window.removeEventListener('keydown', handleDeleteKey);
    }
  }, [isExpanded, selectedItem, handleToggleExpand]);

  // Collapsed view - show only selected item
  if (!isExpanded && selectedItem) {
    const backgroundStyle = selectedItem.type === 'want-type'
      ? getBackgroundStyle(selectedItem.name)
      : { className: '', style: {}, hasBackgroundImage: false };

    return (
      <div className="space-y-2">
        <button
          ref={collapsedButtonRef}
          type="button"
          onClick={handleToggleExpand}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault();
              handleToggleExpand();
            } else if (e.key === 'ArrowDown' && onArrowDown) {
              e.preventDefault();
              onArrowDown();
            }
          }}
          className={`focusable-section-header w-full border rounded-lg p-3 sm:p-4 relative overflow-hidden focus:outline-none ${
            selectedItem.type === 'want-type'
              ? 'border-gray-200 dark:border-gray-700 bg-blue-50 dark:bg-blue-900/20'
              : 'border-gray-200 dark:border-gray-700 bg-green-50 dark:bg-green-900/20'
          } ${backgroundStyle.className}`}
          style={backgroundStyle.style}
        >
          {backgroundStyle.hasBackgroundImage && (
            <div className={getBackgroundOverlayClass()}></div>
          )}
          <div className="flex items-center justify-between relative z-10">
            <div className="flex items-center gap-2 sm:gap-3">
              {selectedItem.type === 'want-type' ? (
                <Zap className="w-4 h-4 sm:w-5 sm:h-5 text-blue-500" />
              ) : (
                <Package className="w-4 h-4 sm:w-5 sm:h-5 text-green-500" />
              )}
              <div className="text-left">
                <h4 className="text-sm sm:text-base font-medium text-gray-900 dark:text-white">{selectedItem.title}</h4>
                {selectedItem.category && (
                  <p className="text-[10px] sm:text-xs text-gray-600 dark:text-gray-300 mt-0.5 sm:mt-1">{selectedItem.category}</p>
                )}
              </div>
            </div>
            <span
              className={`px-3 sm:px-4 py-1.5 sm:py-2 text-xs sm:text-sm font-medium rounded-lg transition-colors ${
                selectedItem.type === 'want-type'
                  ? 'bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400'
                  : 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400'
              }`}
            >
              Change
            </span>
          </div>
        </button>
      </div>
    );
  }

  // Expanded view - show all options
  return (
    <div
      className="space-y-2 flex-1 flex flex-col min-h-0 overflow-hidden"
      style={{ WebkitTouchCallout: 'none', WebkitUserSelect: 'none', userSelect: 'none' } as React.CSSProperties}
    >
      {/* Search Input with Icon */}
      {showSearch && (
        <div className="relative flex-shrink-0">
          <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-gray-400 dark:text-gray-500 pointer-events-none" />
          <input
            ref={searchInputRef}
            type="text"
            placeholder='Search by keyword (press "/")'
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Escape') {
                e.preventDefault();
                if (searchQuery) {
                  setSearchQuery('');
                } else {
                  searchInputRef.current?.blur();
                }
              } else if (e.key === 'Tab' && !e.shiftKey && filteredItems.length > 0) {
                e.preventDefault();
                setFocusedIndex(0);
                itemRefs.current[0]?.focus();
              } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
                // Allow arrow keys from search input
                handleKeyNavigation(e);
              }
            }}
            className="focusable-section-header w-full pl-10 pr-10 py-2 border border-gray-300 dark:border-gray-600 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent dark:bg-gray-800 dark:text-white dark:placeholder-gray-500"
            autoFocus={window.innerWidth >= 1024}
          />
          {searchQuery && (
            <button
              type="button"
              onClick={() => {
                setSearchQuery('');
                searchInputRef.current?.focus();
              }}
              className="absolute right-3 top-1/2 transform -translate-y-1/2 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
              title="Clear search"
            >
              <X className="w-4 h-4" />
            </button>
          )}
        </div>
      )}

      {/* Category Filter - Toggle Buttons */}
      {showSearch && categories.length > 0 && (
        <div className="flex flex-wrap gap-2 flex-shrink-0">
          <button
            type="button"
            onClick={() => setSelectedCategory(null)}
            className={`px-1.5 sm:px-2 py-0.5 sm:py-1 text-[0.65rem] sm:text-xs rounded-full font-medium transition-colors ${
              selectedCategory === null
                ? 'bg-blue-500 text-white'
                : 'bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200 hover:bg-gray-200 dark:hover:bg-gray-600'
            }`}
          >
            All
          </button>
          {categories.map(category => (
            <button
              key={category}
              type="button"
              onClick={() => setSelectedCategory(category)}
              className={`inline-flex items-center gap-1 px-1.5 sm:px-2 py-0.5 sm:py-1 text-[0.65rem] sm:text-xs rounded-full font-medium transition-colors capitalize ${
                selectedCategory === category
                  ? 'bg-blue-500 text-white'
                  : 'bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200 hover:bg-gray-200 dark:hover:bg-gray-600'
              }`}
            >
              {categoryIcons[category] || <Tag className="h-3 w-3" />}
              {category}
            </button>
          ))}
        </div>
      )}

      {/* Scrollable Card List */}
      <div
        className="space-y-2 flex-1 overflow-y-auto custom-scrollbar border border-gray-200 dark:border-gray-700 rounded-lg p-2 bg-white dark:bg-gray-950 min-h-0"
        style={{ WebkitTouchCallout: 'none', WebkitUserSelect: 'none' } as React.CSSProperties}
      >
        {/* Want Types Section */}
        {groupedItems.wantTypes.length > 0 && (
          <div>
            <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-200 mb-2 flex items-center gap-2">
              <Zap className="w-4 h-4" />
              Want Types ({groupedItems.wantTypes.length})
            </h3>
            <div className="grid grid-cols-2 gap-1">
              {groupedItems.wantTypes.map((item, index) => {
                const backgroundStyle = getBackgroundStyle(item.name);
                const globalIndex = filteredItems.findIndex(i => i.id === item.id);
                const isFocused = focusedIndex === globalIndex;
                return (
                  <button
                    key={item.id}
                    ref={(el) => {
                      if (el) itemRefs.current[globalIndex] = el;
                    }}
                    type="button"
                    draggable
                    onClick={() => handleSelect(item)}
                    onContextMenu={(e) => e.preventDefault()}
                    onDragStart={(e) => {
                      suppressDragImage(e);
                      e.dataTransfer.effectAllowed = 'copy';
                      const data = JSON.stringify({
                        id: item.id,
                        type: 'want-type',
                        name: item.title
                      });
                      e.dataTransfer.setData('application/mywant-template', data);
                      
                      useWantStore.getState().setDraggingTemplate({
                        id: item.id,
                        type: 'want-type',
                        name: item.title
                      });
                    }}
                    onDragEnd={() => {
                      useWantStore.getState().setDraggingTemplate(null);
                    }}
                    onTouchStart={(e) => handleTouchStart(item, e)}
                    onTouchMove={handleTouchMove}
                    onTouchEnd={handleTouchEnd}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        handleSelect(item);
                      } else {
                        handleKeyNavigation(e);
                      }
                    }}
                    className={`w-full text-left px-2 py-1 rounded-lg border transition-colors relative overflow-hidden h-[36px] flex items-center select-none ${
                      selectedId === item.id
                        ? 'border-gray-200 dark:border-gray-700 bg-blue-100 dark:bg-blue-900/30'
                        : isFocused
                        ? 'border-gray-200 dark:border-gray-700 bg-blue-50 dark:bg-blue-900/20'
                        : 'border-gray-200 dark:border-gray-700 hover:border-gray-300 dark:hover:border-gray-600 hover:cursor-move'
                    } ${backgroundStyle.className}`}
                    style={{
                      ...backgroundStyle.style,
                      WebkitTouchCallout: 'none',
                      WebkitUserSelect: 'none',
                      touchAction: 'pan-y'
                    }}
                  >
                    {backgroundStyle.hasBackgroundImage && (
                      <div className={getBackgroundOverlayClass()}></div>
                    )}
                    <div className="flex items-start justify-between relative z-10 w-full select-none">
                      <div className="flex-1 flex items-start gap-1.5 min-w-0 select-none">
                        {item.category && (
                          <span className="inline-flex items-center text-gray-500 dark:text-gray-400 flex-shrink-0 mt-0.5 select-none" title={item.category}>
                            {categoryIcons[item.category] || <Tag className="h-3 w-3" />}
                          </span>
                        )}
                        <h4 className="text-sm sm:font-medium text-gray-900 dark:text-white line-clamp-2 break-words select-none">{item.title}</h4>
                      </div>
                      {selectedId === item.id && (
                        <ChevronRight className="w-5 h-5 text-blue-500 flex-shrink-0 ml-2 mt-0.5" />
                      )}
                    </div>
                  </button>
                );
              })}
            </div>
          </div>
        )}

        {/* Recipes Section */}
        {groupedItems.recipes.length > 0 && (
          <div className="pt-2 border-t border-gray-200 dark:border-gray-700">
            <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-200 mb-2 flex items-center gap-2">
              <Package className="w-4 h-4" />
              Recipes ({groupedItems.recipes.length})
            </h3>
            <div className="grid grid-cols-2 gap-1">
              {groupedItems.recipes.map((item, index) => {
                const backgroundStyle = getBackgroundStyle(item.name);
                const globalIndex = filteredItems.findIndex(i => i.id === item.id);
                const isFocused = focusedIndex === globalIndex;
                return (
                  <button
                    key={item.id}
                    ref={(el) => {
                      if (el) itemRefs.current[globalIndex] = el;
                    }}
                    type="button"
                    draggable
                    onClick={() => handleSelect(item)}
                    onContextMenu={(e) => e.preventDefault()}
                    onDragStart={(e) => {
                      suppressDragImage(e);
                      e.dataTransfer.effectAllowed = 'copy';
                      const data = JSON.stringify({
                        id: item.id,
                        type: 'recipe',
                        name: item.title
                      });
                      e.dataTransfer.setData('application/mywant-template', data);

                      useWantStore.getState().setDraggingTemplate({
                        id: item.id,
                        type: 'recipe',
                        name: item.title
                      });
                    }}
                    onDragEnd={() => {
                      useWantStore.getState().setDraggingTemplate(null);
                    }}
                    onTouchStart={(e) => handleTouchStart(item, e)}
                    onTouchMove={handleTouchMove}
                    onTouchEnd={handleTouchEnd}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        handleSelect(item);
                      } else {
                        handleKeyNavigation(e);
                      }
                    }}
                    className={`w-full text-left px-2 py-1 rounded-lg border transition-colors relative overflow-hidden h-[36px] flex items-center select-none ${
                      selectedId === item.id
                        ? 'border-gray-200 dark:border-gray-700 bg-green-100 dark:bg-green-900/30'
                        : isFocused
                        ? 'border-gray-200 dark:border-gray-700 bg-green-50 dark:bg-green-900/20'
                        : 'border-gray-200 dark:border-gray-700 hover:border-gray-300 dark:hover:border-gray-600 hover:cursor-move'
                    } ${backgroundStyle.className}`}
                    style={{
                      ...backgroundStyle.style,
                      WebkitTouchCallout: 'none',
                      WebkitUserSelect: 'none',
                      touchAction: 'pan-y'
                    }}
                  >
                    {backgroundStyle.hasBackgroundImage && (
                      <div className={getBackgroundOverlayClass()}></div>
                    )}
                    <div className="flex items-start justify-between relative z-10 w-full select-none">
                      <div className="flex-1 flex items-start gap-1.5 min-w-0 select-none">
                        {item.category && (
                          <span className="inline-flex items-center text-gray-500 dark:text-gray-400 flex-shrink-0 mt-0.5 select-none" title={item.category}>
                            {categoryIcons[item.category] || <Tag className="h-3 w-3" />}
                          </span>
                        )}
                        <h4 className="text-sm sm:font-medium text-gray-900 dark:text-white line-clamp-2 break-words select-none">{item.title}</h4>
                      </div>
                      {selectedId === item.id && (
                        <ChevronRight className="w-5 h-5 text-green-500 flex-shrink-0 ml-2 mt-0.5" />
                      )}
                    </div>
                  </button>
                );
              })}
            </div>
          </div>
        )}

        {filteredItems.length === 0 && (
          <div className="text-center py-8 text-gray-500 dark:text-gray-400">
            <p>No want types or recipes found matching "{searchQuery}"</p>
          </div>
        )}
      </div>

    </div>
  );
});

TypeRecipeSelector.displayName = 'TypeRecipeSelector';
