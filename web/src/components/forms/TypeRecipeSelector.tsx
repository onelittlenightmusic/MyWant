import React, { useState, useMemo } from 'react';
import { ChevronRight, Package, Zap, ChevronDown } from 'lucide-react';
import { WantType } from '@/types/wantType';
import { Recipe } from '@/types/recipe';
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
  wantTypes: WantType[];
  recipes: Recipe[];
  selectedId: string | null;
  showSearch: boolean;
  onSelect: (id: string, itemType: 'want-type' | 'recipe') => void;
  onGenerateName: (selectedId: string, itemType: 'want-type' | 'recipe', userInput?: string) => string;
}

export const TypeRecipeSelector: React.FC<TypeRecipeSelectorProps> = ({
  wantTypes,
  recipes,
  selectedId,
  showSearch,
  onSelect,
  onGenerateName
}) => {
  const [searchQuery, setSearchQuery] = useState('');
  const [userNameInput, setUserNameInput] = useState('');
  const [showSuffixOptions, setShowSuffixOptions] = useState(false);
  const [selectedCategory, setSelectedCategory] = useState<string | null>(null);

  // Convert want types and recipes to selector items
  const items = useMemo(() => {
    const wantTypeItems: TypeRecipeSelectorItem[] = wantTypes.map(wt => ({
      id: wt.name,
      type: 'want-type' as const,
      name: wt.name,
      title: wt.title || wt.name,
      description: wt.description || '',
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
        icon: <Package className="w-5 h-5" />
      }));

    return [...wantTypeItems, ...recipeItems];
  }, [wantTypes, recipes]);

  // Extract unique categories from want types
  const categories = useMemo(() => {
    const categorySet = new Set<string>();
    wantTypes.forEach(wt => {
      if (wt.category) {
        categorySet.add(wt.category);
      }
    });
    return Array.from(categorySet).sort();
  }, [wantTypes]);

  // Filter items based on search query and category
  const filteredItems = useMemo(() => {
    let filtered = items;

    // Apply category filter (only for want types, recipes always shown)
    if (selectedCategory) {
      filtered = filtered.filter(item =>
        item.type === 'recipe' || item.category === selectedCategory
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

  const selectedItem = items.find(item => item.id === selectedId);

  const handleSelect = (item: TypeRecipeSelectorItem) => {
    onSelect(item.id, item.type);
  };

  const generateNameForSelected = () => {
    if (!selectedId) return;

    const generatedName = onGenerateName(
      selectedId,
      selectedItem?.type || 'want-type',
      userNameInput
    );

    return generatedName;
  };

  return (
    <div className="space-y-3">
      {/* Search Input - Collapsible */}
      {showSearch && (
        <input
          type="text"
          placeholder="Search want types or recipes..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
          autoFocus
        />
      )}

      {/* Category Filter - Toggle Buttons */}
      {categories.length > 0 && (
        <div className="flex flex-wrap gap-2">
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
      <div className="space-y-4 max-h-96 overflow-y-auto border border-gray-200 rounded-lg p-4">
        {/* Want Types Section */}
        {groupedItems.wantTypes.length > 0 && (
          <div>
            <h3 className="text-sm font-semibold text-gray-700 mb-2 flex items-center gap-2">
              <Zap className="w-4 h-4" />
              Want Types ({groupedItems.wantTypes.length})
            </h3>
            <div className="space-y-2">
              {groupedItems.wantTypes.map(item => {
                const backgroundStyle = getBackgroundStyle(item.name);
                return (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => handleSelect(item)}
                    className={`w-full text-left p-3 rounded-lg border-2 transition-colors relative overflow-hidden ${
                      selectedId === item.id
                        ? 'border-blue-500 bg-blue-50'
                        : 'border-gray-200 hover:border-gray-300'
                    } ${backgroundStyle.className}`}
                    style={backgroundStyle.style}
                  >
                    {backgroundStyle.hasBackgroundImage && (
                      <div className={getBackgroundOverlayClass()}></div>
                    )}
                    <div className="flex items-start justify-between relative z-10">
                      <div className="flex-1">
                        <h4 className="font-medium text-gray-900">{item.title}</h4>
                        {item.category && (
                          <p className="text-xs text-gray-500">{item.category}</p>
                        )}
                        {item.description && (
                          <p className="text-sm text-gray-600 mt-1">{item.description}</p>
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
          <div className="pt-4 border-t border-gray-200">
            <h3 className="text-sm font-semibold text-gray-700 mb-2 flex items-center gap-2">
              <Package className="w-4 h-4" />
              Recipes ({groupedItems.recipes.length})
            </h3>
            <div className="space-y-2">
              {groupedItems.recipes.map(item => (
                <button
                  key={item.id}
                  type="button"
                  onClick={() => handleSelect(item)}
                  className={`w-full text-left p-3 rounded-lg border-2 transition-colors ${
                    selectedId === item.id
                      ? 'border-green-500 bg-green-50'
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
              ))}
            </div>
          </div>
        )}

        {filteredItems.length === 0 && (
          <div className="text-center py-8 text-gray-500">
            <p>No want types or recipes found matching "{searchQuery}"</p>
          </div>
        )}
      </div>

      {/* Selected Item Summary */}
      {selectedItem && (
        <div className="bg-gray-50 rounded-lg p-4 space-y-3">
          <div>
            <h4 className="font-medium text-gray-900">
              {selectedItem.type === 'want-type' ? 'Selected Want Type' : 'Selected Recipe'}
            </h4>
            <p className="text-sm text-gray-600 mt-1">{selectedItem.title}</p>
          </div>

          {/* Optional Suffix Button */}
          <button
            type="button"
            onClick={() => setShowSuffixOptions(!showSuffixOptions)}
            className="w-full flex items-center gap-2 px-3 py-2 text-sm text-gray-700 hover:bg-white rounded border border-gray-300 transition-colors"
          >
            <ChevronDown
              className={`w-4 h-4 transition-transform flex-shrink-0 ${showSuffixOptions ? 'rotate-180' : ''}`}
            />
            Add custom suffix (optional)
          </button>

          {/* Suffix Input - Collapsible */}
          {showSuffixOptions && (
            <input
              type="text"
              placeholder="e.g., 'demo', 'test'"
              value={userNameInput}
              onChange={(e) => setUserNameInput(e.target.value)}
              className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent text-sm"
            />
          )}
        </div>
      )}
    </div>
  );
};
