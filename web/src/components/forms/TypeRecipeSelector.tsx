import React, { useState, useMemo } from 'react';
import { ChevronRight, Package, Zap } from 'lucide-react';
import { WantType } from '@/types/wantType';
import { Recipe } from '@/types/recipe';

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
  onSelect: (id: string, itemType: 'want-type' | 'recipe') => void;
  onGenerateName: (selectedId: string, itemType: 'want-type' | 'recipe', userInput?: string) => string;
}

export const TypeRecipeSelector: React.FC<TypeRecipeSelectorProps> = ({
  wantTypes,
  recipes,
  selectedId,
  onSelect,
  onGenerateName
}) => {
  const [searchQuery, setSearchQuery] = useState('');
  const [userNameInput, setUserNameInput] = useState('');

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

  // Filter items based on search query
  const filteredItems = useMemo(() => {
    if (!searchQuery.trim()) {
      return items;
    }

    const query = searchQuery.toLowerCase();
    return items.filter(item =>
      item.title.toLowerCase().includes(query) ||
      item.description.toLowerCase().includes(query) ||
      item.name.toLowerCase().includes(query)
    );
  }, [items, searchQuery]);

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
    <div className="space-y-4">
      {/* Search */}
      <div>
        <input
          type="text"
          placeholder="Search want types or recipes..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
        />
      </div>

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
              {groupedItems.wantTypes.map(item => (
                <button
                  key={item.id}
                  type="button"
                  onClick={() => handleSelect(item)}
                  className={`w-full text-left p-3 rounded-lg border-2 transition-colors ${
                    selectedId === item.id
                      ? 'border-blue-500 bg-blue-50'
                      : 'border-gray-200 bg-white hover:border-gray-300'
                  }`}
                >
                  <div className="flex items-start justify-between">
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
              ))}
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

      {/* Selected Item Summary & Name Generation */}
      {selectedItem && (
        <div className="bg-gray-50 rounded-lg p-4">
          <h4 className="font-medium text-gray-900 mb-3">
            {selectedItem.type === 'want-type' ? 'Selected Want Type' : 'Selected Recipe'}
          </h4>
          <p className="text-sm text-gray-600 mb-4">{selectedItem.title}</p>

          {/* Auto Name Generation */}
          <div className="space-y-2">
            <label className="block text-sm font-medium text-gray-700">
              Add suffix to auto-generated name (optional)
            </label>
            <input
              type="text"
              placeholder="e.g., 'example', 'demo' or leave empty"
              value={userNameInput}
              onChange={(e) => setUserNameInput(e.target.value)}
              className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
            />
            <p className="text-xs text-gray-500 mt-2">
              Generated name: <span className="font-mono font-medium">{generateNameForSelected()}</span>
            </p>
          </div>
        </div>
      )}
    </div>
  );
};
