import React, { useState, useMemo, useRef, useImperativeHandle, forwardRef, useEffect, useCallback } from 'react';
import { ChevronRight, Package, Zap, ChevronDown, Search, X } from 'lucide-react';
import { WantTypeListItem } from '@/types/wantType';
import { GenericRecipe } from '@/types/recipe';
import { getBackgroundStyle, getBackgroundOverlayClass } from '@/utils/backgroundStyles';

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

  // Auto-expand when selectedId becomes null
  useEffect(() => {
    if (!selectedId) {
      setIsExpanded(true);
    }
  }, [selectedId]);

  // Expose focus methods to parent
  useImperativeHandle(ref, () => ({
    focusSearch: () => {
      searchInputRef.current?.focus();
    },
    focus: () => {
      collapsedButtonRef.current?.focus();
    }
  }));

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
          className={`focusable-section-header w-full border-2 rounded-lg p-4 relative overflow-hidden focus:outline-none focus:ring-2 ${
            selectedItem.type === 'want-type'
              ? 'border-blue-500 bg-blue-50 focus:ring-blue-500'
              : 'border-green-500 bg-green-50 focus:ring-green-500'
          } ${backgroundStyle.className}`}
          style={backgroundStyle.style}
        >
          {backgroundStyle.hasBackgroundImage && (
            <div className={getBackgroundOverlayClass()}></div>
          )}
          <div className="flex items-center justify-between relative z-10">
            <div className="flex items-center gap-3">
              {selectedItem.type === 'want-type' ? (
                <Zap className="w-5 h-5 text-blue-500" />
              ) : (
                <Package className="w-5 h-5 text-green-500" />
              )}
              <div>
                <h4 className="font-medium text-gray-900">{selectedItem.title}</h4>
                {selectedItem.category && (
                  <p className="text-xs text-gray-600 mt-1">{selectedItem.category}</p>
                )}
              </div>
            </div>
            <span
              className={`px-4 py-2 text-sm font-medium rounded-lg transition-colors ${
                selectedItem.type === 'want-type'
                  ? 'bg-blue-100 text-blue-700'
                  : 'bg-green-100 text-green-700'
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
    <div className="space-y-2 flex-1 flex flex-col min-h-0 overflow-hidden">
      {/* Search Input with Icon */}
      {showSearch && (
        <div className="relative flex-shrink-0">
          <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-gray-400 pointer-events-none" />
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
            className="focusable-section-header w-full pl-10 pr-10 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
            autoFocus
          />
          {searchQuery && (
            <button
              type="button"
              onClick={() => {
                setSearchQuery('');
                searchInputRef.current?.focus();
              }}
              className="absolute right-3 top-1/2 transform -translate-y-1/2 text-gray-400 hover:text-gray-600 transition-colors"
              title="Clear search"
            >
              <X className="w-4 h-4" />
            </button>
          )}
        </div>
      )}

      {/* Category Filter - Toggle Buttons */}
      {categories.length > 0 && (
        <div className="flex flex-wrap gap-2 flex-shrink-0">
          <button
            type="button"
            onClick={() => setSelectedCategory(null)}
            className={`px-3 py-1.5 text-sm rounded-full font-medium transition-colors ${
              selectedCategory === null
                ? 'bg-blue-500 text-white'
                : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
            }`}
          >
            All
          </button>
          {categories.map(category => (
            <button
              key={category}
              type="button"
              onClick={() => setSelectedCategory(category)}
              className={`px-3 py-1.5 text-sm rounded-full font-medium transition-colors capitalize ${
                selectedCategory === category
                  ? 'bg-blue-500 text-white'
                  : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
              }`}
            >
              {category}
            </button>
          ))}
        </div>
      )}

      {/* Scrollable Card List */}
      <div className="space-y-2 flex-1 overflow-y-auto border border-gray-200 rounded-lg p-4 bg-white min-h-0">
        {/* Want Types Section */}
        {groupedItems.wantTypes.length > 0 && (
          <div>
            <h3 className="text-sm font-semibold text-gray-700 mb-2 flex items-center gap-2">
              <Zap className="w-4 h-4" />
              Want Types ({groupedItems.wantTypes.length})
            </h3>
            <div className="space-y-1">
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
                    onClick={() => handleSelect(item)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        handleSelect(item);
                      } else {
                        handleKeyNavigation(e);
                      }
                    }}
                    className={`w-full text-left p-3 rounded-lg border-2 transition-colors relative overflow-hidden ${
                      selectedId === item.id
                        ? 'border-blue-500 bg-blue-50'
                        : isFocused
                        ? 'border-blue-400 bg-blue-50 ring-2 ring-blue-300'
                        : 'border-gray-200 hover:border-gray-300'
                    } ${backgroundStyle.className}`}
                    style={backgroundStyle.style}
                  >
                    {backgroundStyle.hasBackgroundImage && (
                      <div className={getBackgroundOverlayClass()}></div>
                    )}
                    <div className="flex items-center justify-between relative z-10">
                      <div className="flex-1 flex items-center gap-2">
                        <h4 className="font-medium text-gray-900">{item.title}</h4>
                        {item.category && (
                          <span className="text-xs bg-gray-100 text-gray-600 px-2 py-1 rounded-full">{item.category}</span>
                        )}
                      </div>
                      {selectedId === item.id && (
                        <ChevronRight className="w-5 h-5 text-blue-500 flex-shrink-0 ml-2" />
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
          <div className="pt-2 border-t border-gray-200">
            <h3 className="text-sm font-semibold text-gray-700 mb-2 flex items-center gap-2">
              <Package className="w-4 h-4" />
              Recipes ({groupedItems.recipes.length})
            </h3>
            <div className="space-y-1">
              {groupedItems.recipes.map((item, index) => {
                const globalIndex = filteredItems.findIndex(i => i.id === item.id);
                const isFocused = focusedIndex === globalIndex;
                return (
                  <button
                    key={item.id}
                    ref={(el) => {
                      if (el) itemRefs.current[globalIndex] = el;
                    }}
                    type="button"
                    onClick={() => handleSelect(item)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        handleSelect(item);
                      } else {
                        handleKeyNavigation(e);
                      }
                    }}
                    className={`w-full text-left p-3 rounded-lg border-2 transition-colors ${
                      selectedId === item.id
                        ? 'border-green-500 bg-green-50'
                        : isFocused
                        ? 'border-green-400 bg-green-50 ring-2 ring-green-300'
                        : 'border-gray-200 bg-white hover:border-gray-300'
                    }`}
                  >
                  <div className="flex items-start justify-between">
                    <div className="flex-1">
                      <h4 className="font-medium text-gray-900">{item.title}</h4>
                      {item.description && (
                        <p className="text-sm text-gray-600 mt-1">{item.description}</p>
                      )}
                    </div>
                    {selectedId === item.id && (
                      <ChevronRight className="w-5 h-5 text-green-500 flex-shrink-0 ml-2" />
                    )}
                  </div>
                </button>
                );
              })}
            </div>
          </div>
        )}

        {filteredItems.length === 0 && (
          <div className="text-center py-8 text-gray-500">
            <p>No want types or recipes found matching "{searchQuery}"</p>
          </div>
        )}
      </div>

    </div>
  );
});

TypeRecipeSelector.displayName = 'TypeRecipeSelector';
