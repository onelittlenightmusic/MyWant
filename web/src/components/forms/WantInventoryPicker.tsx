import React, { useState, useMemo, useRef, useEffect, useCallback, forwardRef, useImperativeHandle } from 'react';
import { Search, Zap, Package, Plane, Calculator, Layers, CheckCircle, Monitor } from 'lucide-react';
import { WantTypeListItem } from '@/types/wantType';
import { GenericRecipe } from '@/types/recipe';
import { getBackgroundStyle } from '@/utils/backgroundStyles';
import { suppressDragImage } from '@/utils/helpers';
import { useWantStore } from '@/stores/wantStore';

// ─── Shared constants ─────────────────────────────────────────────────────────

export const CATEGORY_COLORS: Record<string, string> = {
  system:      'bg-slate-600',
  travel:      'bg-sky-600',
  queue:       'bg-violet-600',
  mathematics: 'bg-emerald-600',
  math:        'bg-emerald-600',
  approval:    'bg-amber-600',
};

/** Returns a category icon sized to `px` pixels */
export const categoryIcon = (category: string | undefined, px: number): React.ReactNode => {
  const s = { width: px, height: px };
  const cls = 'text-white/90 drop-shadow flex-shrink-0';
  switch (category?.toLowerCase()) {
    case 'travel':      return <Plane       style={s} className={cls} />;
    case 'mathematics':
    case 'math':        return <Calculator  style={s} className={cls} />;
    case 'queue':       return <Layers      style={s} className={cls} />;
    case 'approval':    return <CheckCircle style={s} className={cls} />;
    case 'system':      return <Monitor     style={s} className={cls} />;
    default:            return null;
  }
};

// ─── Reusable single slot ────────────────────────────────────────────────────

export interface WantSlotProps {
  /** want type name or recipe custom_type */
  id: string;
  itemType: 'want-type' | 'recipe';
  category?: string;
  /** Slot side length in px (default 56) */
  size?: number;
  className?: string;
}

/**
 * Single Minecraft-style inventory slot.
 * Shared between WantInventoryPicker (grid) and WantForm (selected-type header).
 */
export const WantSlot: React.FC<WantSlotProps> = ({
  id,
  itemType,
  category,
  size = 56,
  className = '',
}) => {
  const bg = getBackgroundStyle(id);
  const colorClass = CATEGORY_COLORS[category?.toLowerCase() || ''] ?? 'bg-gray-500';
  const iconSize = Math.round(size * 0.38);
  const icon = itemType === 'recipe'
    ? <Package style={{ width: iconSize, height: iconSize }} className="text-white/90 drop-shadow" />
    : (categoryIcon(category, iconSize) ?? <Zap style={{ width: iconSize, height: iconSize }} className="text-white/90 drop-shadow" />);

  return (
    <div
      className={[
        'relative rounded-sm overflow-hidden flex-shrink-0',
        'border border-black/50 dark:border-black/70',
        'shadow-[inset_2px_2px_0px_rgba(255,255,255,0.22),inset_-2px_-2px_0px_rgba(0,0,0,0.35)]',
        className,
      ].join(' ')}
      style={{ width: size, height: size }}
    >
      {bg.hasBackgroundImage ? (
        <>
          <div
            className="absolute inset-0"
            style={{ ...bg.style, backgroundSize: 'cover', backgroundPosition: 'center' }}
          />
          <div className="absolute inset-0 bg-black/25" />
        </>
      ) : (
        <div className={`absolute inset-0 ${colorClass}`} />
      )}
      <div className="relative z-10 flex items-center justify-center w-full h-full">
        {icon}
      </div>
    </div>
  );
};

// ─── Inventory picker ─────────────────────────────────────────────────────────

interface SlotItem {
  id: string;
  type: 'want-type' | 'recipe';
  name: string;
  title: string;
  description: string;
  category?: string;
}

type SortMode = 'name' | 'category';

interface WantInventoryPickerProps {
  wantTypes: WantTypeListItem[];
  recipes: GenericRecipe[];
  onSelect: (id: string, itemType: 'want-type' | 'recipe') => void;
}

export interface WantInventoryPickerRef {
  navigate: (dir: 'up' | 'down' | 'left' | 'right') => void;
  confirmFocused: () => void;
  focusSearch: () => void;
}

const GRID_COLS = 6;

interface TooltipState {
  item: SlotItem;
  x: number;
  y: number;
  above: boolean;
}

export const WantInventoryPicker = forwardRef<WantInventoryPickerRef, WantInventoryPickerProps>(
function WantInventoryPicker({
  wantTypes,
  recipes,
  onSelect,
}, ref) {
  const [searchQuery, setSearchQuery] = useState('');
  const [sortMode, setSortMode] = useState<SortMode>('category');
  const [tooltip, setTooltip] = useState<TooltipState | null>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const slotButtonRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const renderedItemsRef = useRef<SlotItem[]>([]);
  const onSelectRef = useRef(onSelect);
  onSelectRef.current = onSelect;

  useEffect(() => {
    const t = setTimeout(() => {
      const el = searchRef.current;
      if (!el) return;
      // Only focus when the sidebar is intentionally open.
      // RightSidebar sets data-sidebar-open based on its isOpen prop (via
      // useLayoutEffect), so this reflects intended state, not CSS animation.
      const sidebar = el.closest('[data-sidebar="true"]');
      if (sidebar && !sidebar.hasAttribute('data-sidebar-open')) return;
      el.focus();
    }, 80);
    return () => clearTimeout(t);
  }, []);

  const items = useMemo((): SlotItem[] => {
    const wantTypeItems: SlotItem[] = wantTypes.map(wt => ({
      id: wt.name,
      type: 'want-type',
      name: wt.name,
      title: wt.title || wt.name,
      description: '',
      category: wt.category,
    }));
    const recipeItems: SlotItem[] = recipes
      .filter(r => r.recipe?.metadata?.custom_type)
      .map(r => ({
        id: r.recipe.metadata.custom_type!,
        type: 'recipe',
        name: r.recipe.metadata.custom_type!,
        title: r.recipe.metadata.name || r.recipe.metadata.custom_type!,
        description: r.recipe.metadata.description || '',
        category: r.recipe.metadata.category,
      }));
    return [...wantTypeItems, ...recipeItems];
  }, [wantTypes, recipes]);

  const filteredItems = useMemo(() => {
    if (!searchQuery.trim()) return items;
    const q = searchQuery.toLowerCase();
    return items.filter(item =>
      item.title.toLowerCase().includes(q) ||
      item.name.toLowerCase().includes(q) ||
      (item.category || '').toLowerCase().includes(q)
    );
  }, [items, searchQuery]);

  const groups = useMemo(() => {
    const sorted = [...filteredItems].sort((a, b) => {
      if (sortMode === 'name') return a.title.localeCompare(b.title);
      if (a.type !== b.type) return a.type === 'want-type' ? -1 : 1;
      const catCmp = (a.category || '').localeCompare(b.category || '');
      return catCmp !== 0 ? catCmp : a.title.localeCompare(b.title);
    });

    if (sortMode === 'name') {
      return [{ label: null as string | null, items: sorted }];
    }

    const map = new Map<string, SlotItem[]>();
    sorted.forEach(item => {
      const key = item.category || 'other';
      if (!map.has(key)) map.set(key, []);
      map.get(key)!.push(item);
    });
    return Array.from(map.entries()).map(([label, groupItems]) => ({ label, items: groupItems }));
  }, [filteredItems, sortMode]);

  // Flat list in rendered order (row-major across groups) — used for D-pad navigation
  const renderedItems = useMemo(() => groups.flatMap(g => g.items), [groups]);
  renderedItemsRef.current = renderedItems;
  slotButtonRefs.current.length = renderedItems.length;

  // Visual (row, col) for each flat index, accounting for group boundaries.
  // Each group starts on its own set of rows in its own GRID_COLS-wide grid.
  const gridPositions = useMemo(() => {
    const positions: Array<{ row: number; col: number }> = [];
    let currentRow = 0;
    for (const group of groups) {
      for (let k = 0; k < group.items.length; k++) {
        positions.push({ row: currentRow + Math.floor(k / GRID_COLS), col: k % GRID_COLS });
      }
      currentRow += Math.ceil(group.items.length / GRID_COLS);
    }
    return positions;
  }, [groups]);

  useImperativeHandle(ref, () => ({
    focusSearch: () => { searchRef.current?.focus(); },
    navigate: (dir) => {
      const total = renderedItemsRef.current.length;
      if (total === 0) return;
      const currentIdx = slotButtonRefs.current.findIndex(el => el === document.activeElement);
      let next: number;
      if (currentIdx < 0) {
        next = (dir === 'up' || dir === 'left') ? total - 1 : 0;
      } else if (dir === 'right') {
        next = (currentIdx + 1) % total;
      } else if (dir === 'left') {
        next = currentIdx === 0 ? total - 1 : currentIdx - 1;
      } else {
        // up / down: use visual (row, col) to navigate across group boundaries correctly
        const { row, col } = gridPositions[currentIdx];
        const targetRow = dir === 'down' ? row + 1 : row - 1;
        const candidates = gridPositions
          .map((p, i) => ({ ...p, i }))
          .filter(p => p.row === targetRow);
        if (candidates.length === 0) {
          next = currentIdx; // already at top/bottom edge
        } else {
          const exact = candidates.find(p => p.col === col);
          if (exact) {
            next = exact.i;
          } else {
            // nearest column in target row
            next = candidates.reduce((best, c) =>
              Math.abs(c.col - col) < Math.abs(best.col - col) ? c : best
            ).i;
          }
        }
      }
      slotButtonRefs.current[next]?.focus();
    },
    confirmFocused: () => {
      const idx = slotButtonRefs.current.findIndex(el => el === document.activeElement);
      if (idx >= 0) {
        const item = renderedItemsRef.current[idx];
        if (item) onSelectRef.current(item.id, item.type);
      }
    },
  }), [gridPositions]);

  const handleMouseEnter = useCallback((e: React.MouseEvent<HTMLButtonElement>, item: SlotItem) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const spaceBelow = window.innerHeight - rect.bottom;
    const above = spaceBelow < 90;
    setTooltip({
      item,
      x: Math.min(rect.left, window.innerWidth - 196),
      y: above ? rect.top - 8 : rect.bottom + 8,
      above,
    });
  }, []);

  const handleMouseLeave = useCallback(() => setTooltip(null), []);

  const renderSlot = (item: SlotItem, flatIndex: number) => (
    <div key={item.id} className="flex flex-col items-center gap-0.5">
      <button
        type="button"
        ref={el => { slotButtonRefs.current[flatIndex] = el; }}
        draggable
        onClick={() => onSelect(item.id, item.type)}
        onMouseEnter={e => handleMouseEnter(e, item)}
        onMouseLeave={handleMouseLeave}
        onDragStart={e => {
          suppressDragImage(e);
          e.dataTransfer.effectAllowed = 'copy';
          e.dataTransfer.setData('application/mywant-template', JSON.stringify({
            id: item.id,
            type: item.type,
            name: item.title,
          }));
          useWantStore.getState().setDraggingTemplate({
            id: item.id,
            type: item.type,
            name: item.title,
          });
        }}
        onDragEnd={() => {
          useWantStore.getState().setDraggingTemplate(null);
        }}
        className={[
          'relative w-full aspect-square rounded-sm overflow-hidden cursor-grab active:cursor-grabbing',
          'border border-black/50 dark:border-black/70',
          'shadow-[inset_2px_2px_0px_rgba(255,255,255,0.22),inset_-2px_-2px_0px_rgba(0,0,0,0.35)]',
          'hover:outline hover:outline-[3px] hover:outline-sky-400 hover:outline-offset-0 hover:z-10',
          'focus:outline focus:outline-[3px] focus:outline-sky-400 focus:outline-offset-0 focus:z-10',
        ].join(' ')}
      >
        {(() => {
          const bg = getBackgroundStyle(item.name);
          const colorClass = CATEGORY_COLORS[item.category?.toLowerCase() || ''] ?? 'bg-gray-500';
          const icon = item.type === 'recipe'
            ? <Package className="w-5 h-5 text-white/90 drop-shadow" />
            : (categoryIcon(item.category, 20) ?? <Zap className="w-5 h-5 text-white/90 drop-shadow" />);
          return (
            <>
              {bg.hasBackgroundImage ? (
                <>
                  <div className="absolute inset-0" style={{ ...bg.style, backgroundSize: 'cover', backgroundPosition: 'center' }} />
                  <div className="absolute inset-0 bg-black/25" />
                </>
              ) : (
                <div className={`absolute inset-0 ${colorClass}`} />
              )}
              <div className="relative z-10 flex items-center justify-center w-full h-full">
                {icon}
              </div>
            </>
          );
        })()}
      </button>
      <span
        className="text-[9px] leading-none text-center text-gray-600 dark:text-gray-400 w-full truncate"
        title={item.title}
      >
        {item.title}
      </span>
    </div>
  );

  return (
    <div className="flex flex-col gap-2 p-3 h-full min-h-0">
      {/* Search + Sort row */}
      <div className="flex items-center gap-2 flex-shrink-0">
        <div className="relative flex-1 min-w-0">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-gray-400 pointer-events-none" />
          <input
            ref={searchRef}
            type="text"
            value={searchQuery}
            onChange={e => setSearchQuery(e.target.value)}
            onKeyDown={e => { if (e.key === 'Escape') setSearchQuery(''); }}
            placeholder="Search..."
            className="w-full pl-7 pr-2 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-white placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-sky-400 focus:border-transparent"
          />
        </div>
        <div className="flex rounded-md border border-gray-300 dark:border-gray-600 overflow-hidden text-xs flex-shrink-0">
          <button
            type="button"
            onClick={() => setSortMode('name')}
            className={`px-2.5 py-1.5 transition-colors ${
              sortMode === 'name'
                ? 'bg-sky-500 text-white'
                : 'bg-white dark:bg-gray-800 text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700'
            }`}
          >
            Name
          </button>
          <button
            type="button"
            onClick={() => setSortMode('category')}
            className={`px-2.5 py-1.5 transition-colors border-l border-gray-300 dark:border-gray-600 ${
              sortMode === 'category'
                ? 'bg-sky-500 text-white'
                : 'bg-white dark:bg-gray-800 text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700'
            }`}
          >
            Category
          </button>
        </div>
      </div>

      {/* Inventory grid */}
      <div className="flex-1 overflow-y-auto custom-scrollbar min-h-0">
        {filteredItems.length === 0 ? (
          <p className="text-sm text-gray-500 dark:text-gray-400 text-center py-8">
            No results for &ldquo;{searchQuery}&rdquo;
          </p>
        ) : (
          <div className="space-y-3">
            {(() => {
              let slotCounter = 0;
              return groups.map(({ label, items: groupItems }, gi) => (
                <div key={label ?? gi}>
                  {label !== null && (
                    <div className="flex items-center gap-1.5 mb-1.5">
                      <span className="text-[10px] font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                        {label}
                      </span>
                      <div className="flex-1 h-px bg-gray-200 dark:bg-gray-700" />
                      <span className="text-[9px] text-gray-400">{groupItems.length}</span>
                    </div>
                  )}
                  <div className="grid grid-cols-6 gap-1.5">
                    {groupItems.map(item => renderSlot(item, slotCounter++))}
                  </div>
                </div>
              ));
            })()}
          </div>
        )}
      </div>

      {/* Minecraft-style tooltip */}
      {tooltip && (
        <div
          className="fixed z-[9999] pointer-events-none"
          style={{
            left: tooltip.x,
            top: tooltip.above ? undefined : tooltip.y,
            bottom: tooltip.above ? window.innerHeight - tooltip.y : undefined,
          }}
        >
          <div className="bg-gray-900 border border-gray-600 rounded px-2.5 py-2 text-white shadow-xl max-w-[188px]">
            <p className="text-xs font-semibold leading-tight">{tooltip.item.title}</p>
            {tooltip.item.category && (
              <p className="text-[10px] text-gray-400 mt-0.5 capitalize">{tooltip.item.category}</p>
            )}
            {tooltip.item.description && tooltip.item.description !== tooltip.item.title && (
              <p className="text-[10px] text-gray-300 mt-1 leading-tight">{tooltip.item.description}</p>
            )}
            <p className="text-[9px] text-gray-500 mt-1.5">
              {tooltip.item.type === 'recipe' ? '📦 Recipe' : '⚡ Want Type'}
            </p>
          </div>
        </div>
      )}
    </div>
  );
});
