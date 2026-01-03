import React, { useRef, useEffect, useCallback, forwardRef } from 'react';
import { ChevronDown, ChevronRight } from 'lucide-react';
import { FocusableChip } from './FocusableChip';
import { CollapsibleFormSectionProps, ColorScheme } from '@/types/formSection';

/**
 * CollapsibleFormSection - Generic collapsible section with keyboard navigation
 *
 * Header keyboard shortcuts:
 * - Right Arrow: Focus first chip (if exists), expand if collapsed
 * - Left Arrow: Collapse section
 * - 'a' key: Add new item
 * - Up/Down Arrows: Navigate to adjacent sections
 * - Enter/Space: Toggle collapse
 *
 * Visual indicators:
 * - Left-side blue indicator bar when header is focused
 * - Chevron icon shows collapse state
 */
export const CollapsibleFormSection = forwardRef<HTMLButtonElement, CollapsibleFormSectionProps>(({
  sectionId,
  title,
  icon,
  colorScheme,
  isCollapsed,
  onToggleCollapse,
  navigationCallbacks,
  items,
  onAddItem,
  renderEditForm,
  renderCollapsedSummary,
  isEditing,
  editingIndex,
  onEditChip,
  onRemoveChip,
}, ref) => {
  const headerRef = useRef<HTMLButtonElement>(null);
  const chipRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const editFormRef = useRef<HTMLDivElement>(null);

  // Merge forwarded ref with local ref
  const mergedRef = useCallback((node: HTMLButtonElement | null) => {
    headerRef.current = node;
    if (typeof ref === 'function') {
      ref(node);
    } else if (ref) {
      (ref as React.MutableRefObject<HTMLButtonElement | null>).current = node;
    }
  }, [ref]);

  /**
   * Get header color classes based on color scheme
   */
  const getHeaderColorClasses = (scheme: ColorScheme): string => {
    switch (scheme) {
      case 'blue':
        return 'bg-blue-50 border-blue-200 hover:bg-blue-100';
      case 'amber':
        return 'bg-amber-50 border-amber-200 hover:bg-amber-100';
      case 'green':
        return 'bg-green-50 border-green-200 hover:bg-green-100';
    }
  };

  /**
   * Get focus indicator color based on color scheme
   */
  const getFocusIndicatorColor = (scheme: ColorScheme): string => {
    switch (scheme) {
      case 'blue':
        return 'before:bg-blue-500';
      case 'amber':
        return 'before:bg-amber-500';
      case 'green':
        return 'before:bg-green-500';
    }
  };

  /**
   * Handle header keyboard navigation
   */
  const handleHeaderKeyDown = useCallback((e: React.KeyboardEvent<HTMLButtonElement>) => {
    // Right arrow - focus first chip or do nothing
    if (e.key === 'ArrowRight') {
      e.preventDefault();

      // Expand if collapsed
      if (isCollapsed) {
        onToggleCollapse();
      }

      // Focus first chip if exists (after expansion animation)
      setTimeout(() => {
        const firstChip = chipRefs.current[0];
        if (firstChip && !isEditing) {
          firstChip.focus();
        }
        // Do nothing if no chips exist - this satisfies requirement #2
      }, 100);
    }
    // Left arrow - collapse section
    else if (e.key === 'ArrowLeft') {
      e.preventDefault();
      if (!isCollapsed) {
        onToggleCollapse();
      }
    }
    // 'a' key - add new item
    else if (e.key === 'a' && !e.shiftKey && !e.ctrlKey && !e.metaKey && !e.altKey) {
      e.preventDefault();

      // Expand if collapsed
      if (isCollapsed) {
        onToggleCollapse();
      }

      // Trigger add item callback
      setTimeout(() => {
        onAddItem();
      }, 100);
    }
    // Up arrow - navigate to previous section
    else if (e.key === 'ArrowUp') {
      e.preventDefault();
      navigationCallbacks.onNavigateUp();
    }
    // Down arrow - navigate to next section
    else if (e.key === 'ArrowDown') {
      e.preventDefault();
      navigationCallbacks.onNavigateDown();
    }
    // Enter/Space - toggle collapse
    else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onToggleCollapse();
    }
    // Tab - custom navigation (e.g. to Add button)
    else if (e.key === 'Tab' && navigationCallbacks.onTab) {
      e.preventDefault();
      navigationCallbacks.onTab();
    }
  }, [isCollapsed, isEditing, onToggleCollapse, onAddItem, navigationCallbacks]);

  /**
   * Handle chip navigation
   */
  const handleChipNavigateNext = useCallback((currentIndex: number) => {
    const nextIndex = currentIndex + 1;
    if (nextIndex < chipRefs.current.length) {
      chipRefs.current[nextIndex]?.focus();
    }
  }, []);

  const handleChipNavigatePrev = useCallback((currentIndex: number) => {
    const prevIndex = currentIndex - 1;
    if (prevIndex >= 0) {
      chipRefs.current[prevIndex]?.focus();
    }
  }, []);

  /**
   * Handle escape from chip - return focus to header
   */
  const handleChipEscape = useCallback(() => {
    headerRef.current?.focus();
  }, []);

  /**
   * Reset chip refs when items change
   */
  useEffect(() => {
    chipRefs.current = chipRefs.current.slice(0, items.length);
  }, [items.length]);

  /**
   * Focus header when editing is cancelled (escape during creation)
   */
  useEffect(() => {
    if (!isEditing && editFormRef.current && document.activeElement === editFormRef.current) {
      headerRef.current?.focus();
    }
  }, [isEditing]);

  return (
    <div className="space-y-2">
      {/* Section Header */}
      <button
        ref={mergedRef}
        type="button"
        onClick={onToggleCollapse}
        onKeyDown={handleHeaderKeyDown}
        className={`
          w-full text-left px-4 py-3 rounded-lg border-2
          transition-all duration-200 focus:outline-none
          relative
          ${getHeaderColorClasses(colorScheme)}

          before:absolute before:left-0 before:top-0
          before:bottom-0 before:w-1 before:rounded-l-md
          before:opacity-0 before:transition-opacity
          focus:before:opacity-100
          ${getFocusIndicatorColor(colorScheme)}
        `}
        aria-expanded={!isCollapsed}
        aria-label={`${title} section - Press Right to ${isCollapsed ? 'expand and ' : ''}focus items, 'a' to add new, Up/Down to navigate sections`}
      >
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            {/* Collapse/Expand Icon */}
            {isCollapsed ? (
              <ChevronRight className="w-5 h-5 text-gray-500" />
            ) : (
              <ChevronDown className="w-5 h-5 text-gray-500" />
            )}

            {/* Section Icon */}
            <span className="text-gray-700">
              {icon}
            </span>

            {/* Section Title */}
            <h3 className="text-lg font-semibold text-gray-900">
              {title}
            </h3>

            {/* Item Count Badge */}
            {items.length > 0 && (
              <span className={`
                px-2 py-0.5 text-xs font-medium rounded-full
                ${colorScheme === 'blue' ? 'bg-blue-100 text-blue-700' :
                  colorScheme === 'amber' ? 'bg-amber-100 text-amber-700' :
                  'bg-green-100 text-green-700'}
              `}>
                {items.length}
              </span>
            )}
          </div>

          {/* Collapsed Summary */}
          {isCollapsed && items.length > 0 && (
            <div className="text-sm text-gray-600 ml-4">
              {renderCollapsedSummary()}
            </div>
          )}
        </div>
      </button>

      {/* Section Content (Expanded) */}
      {!isCollapsed && (
        <div className="pl-4 space-y-3">
          {/* Chips Display */}
          {items.length > 0 && !isEditing && (
            <div className="flex flex-wrap gap-2">
              {items.map((item, index) => (
                <FocusableChip
                  key={item.key}
                  ref={(el) => {
                    chipRefs.current[index] = el;
                  }}
                  item={item}
                  colorScheme={colorScheme}
                  onEdit={() => onEditChip(index)}
                  onRemove={() => onRemoveChip(index)}
                  onNavigateNext={() => handleChipNavigateNext(index)}
                  onNavigatePrev={() => handleChipNavigatePrev(index)}
                  onEscape={handleChipEscape}
                />
              ))}
            </div>
          )}

          {/* Edit Form */}
          {isEditing && (
            <div ref={editFormRef} className="mt-2">
              {renderEditForm()}
            </div>
          )}

          {/* Add New Item Button (only if not editing) */}
          {!isEditing && (
            <button
              type="button"
              onClick={onAddItem}
              className={`
                px-4 py-2 text-sm font-medium rounded-lg
                border-2 border-dashed transition-colors
                focus:outline-none focus:ring-2
                ${colorScheme === 'blue'
                  ? 'border-blue-300 text-blue-600 hover:bg-blue-50 focus:ring-blue-300'
                  : colorScheme === 'amber'
                  ? 'border-amber-300 text-amber-600 hover:bg-amber-50 focus:ring-amber-300'
                  : 'border-green-300 text-green-600 hover:bg-green-50 focus:ring-green-300'}
              `}
              aria-label={`Add new ${title.toLowerCase()} item (or press 'a' on header)`}
            >
              + Add {title}
            </button>
          )}

          {/* Empty State */}
          {items.length === 0 && !isEditing && (
            <p className="text-sm text-gray-500 italic">
              No {title.toLowerCase()} added yet. Press 'a' on the header or click the button below to add.
            </p>
          )}
        </div>
      )}
    </div>
  );
});

CollapsibleFormSection.displayName = 'CollapsibleFormSection';
